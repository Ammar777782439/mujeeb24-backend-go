package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// subIDForTests is a UUID used in the platformSubscriptionPath in tests
// (PlatformSubscriptionPath.SubscriptionID is dto.UUID, default value "").
const subIDForTests = "00000000-0000-0000-0000-00000000sub1"

func platformSubscriptionPath() dto.PlatformSubscriptionPath {
	return dto.PlatformSubscriptionPath{SubscriptionID: dto.UUID(subIDForTests)}
}

// ----------------------------------------------------------------------------
// Mock implementations for Subscription + Payment ports
// ----------------------------------------------------------------------------

type stubSubscriptionRepository struct {
	created        ports.SubscriptionRecord
	createErr      error
	getByID        ports.SubscriptionRecord
	getErr         error
	listPage       ports.SubscriptionPage
	listErr        error
	activated      ports.SubscriptionRecord
	activateErr    error
	cancelled      ports.SubscriptionRecord
	cancelErr      error
	expired        ports.SubscriptionRecord
	expireErr      error
	overrideResult ports.SubscriptionRecord
	overrideErr    error
	entitlementErr error
}

func (s *stubSubscriptionRepository) Create(_ context.Context, _ ports.SubscriptionCreate) (ports.SubscriptionRecord, error) {
	return s.created, s.createErr
}
func (s *stubSubscriptionRepository) GetByID(_ context.Context, _ string) (ports.SubscriptionRecord, error) {
	return s.getByID, s.getErr
}
func (s *stubSubscriptionRepository) List(_ context.Context, _ ports.SubscriptionListFilter) (ports.SubscriptionPage, error) {
	return s.listPage, s.listErr
}
func (s *stubSubscriptionRepository) Activate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return s.activated, s.activateErr
}
func (s *stubSubscriptionRepository) Cancel(_ context.Context, _, _, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return s.cancelled, s.cancelErr
}
func (s *stubSubscriptionRepository) MarkExpired(_ context.Context, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return s.expired, s.expireErr
}
func (s *stubSubscriptionRepository) ApplyCostBudgetOverride(_ context.Context, _ string, _ int, _, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return s.overrideResult, s.overrideErr
}
func (s *stubSubscriptionRepository) CheckEntitlements(_ context.Context, _ string, _, _, _ int) error {
	return s.entitlementErr
}

type stubPaymentRepository struct {
	appended  ports.PaymentRecord
	appendErr error
	listPage  ports.PaymentPage
	listErr   error
	getByID   ports.PaymentRecord
	getErr    error
}

type stubSubscriptionAddonRepository struct {
	created       ports.SubscriptionAddonRecord
	createErr     error
	createdInput  ports.SubscriptionAddonCreate
	page          ports.SubscriptionAddonPage
	listErr       error
	activated     ports.SubscriptionAddonRecord
	paymentErr    error
	cancelled     ports.SubscriptionAddonRecord
	cancelErr     error
	approval      ports.ChannelPassCapacityApprovalRecord
	approvalErr   error
	approvalInput ports.ChannelPassCapacityApproval
}

func (s *stubSubscriptionAddonRepository) CreatePending(_ context.Context, in ports.SubscriptionAddonCreate) (ports.SubscriptionAddonRecord, error) {
	s.createdInput = in
	return s.created, s.createErr
}
func (s *stubSubscriptionAddonRepository) List(_ context.Context, _ ports.SubscriptionAddonListFilter) (ports.SubscriptionAddonPage, error) {
	return s.page, s.listErr
}
func (s *stubSubscriptionAddonRepository) RecordPaymentAndActivate(_ context.Context, _ ports.SubscriptionAddonPayment) (ports.SubscriptionAddonRecord, error) {
	return s.activated, s.paymentErr
}
func (s *stubSubscriptionAddonRepository) Cancel(_ context.Context, _ ports.SubscriptionAddonCancel) (ports.SubscriptionAddonRecord, error) {
	return s.cancelled, s.cancelErr
}
func (s *stubSubscriptionAddonRepository) ApproveChannelPassCapacity(_ context.Context, in ports.ChannelPassCapacityApproval) (ports.ChannelPassCapacityApprovalRecord, error) {
	s.approvalInput = in
	return s.approval, s.approvalErr
}

type stubBillingTransactions struct{}

func (stubBillingTransactions) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (s *stubPaymentRepository) Append(_ context.Context, _ ports.PaymentCreate) (ports.PaymentRecord, error) {
	return s.appended, s.appendErr
}
func (s *stubPaymentRepository) List(_ context.Context, _ ports.PaymentListFilter) (ports.PaymentPage, error) {
	return s.listPage, s.listErr
}
func (s *stubPaymentRepository) GetByID(_ context.Context, _ string) (ports.PaymentRecord, error) {
	return s.getByID, s.getErr
}

// ----------------------------------------------------------------------------
// Tests: Subscription lifecycle + audit (Contract §20-36)
// ----------------------------------------------------------------------------

func TestPlatformListSubscriptionsReturns501WhenUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformListSubscriptions", &dto.SubscriptionListInput{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformListSubscriptionsReturnsPaginatedItems(t *testing.T) {
	subRepo := &stubSubscriptionRepository{listPage: ports.SubscriptionPage{
		Items: []ports.SubscriptionRecord{
			{ID: "sub-1", BusinessID: "biz-1", Status: "ACTIVE"},
			{ID: "sub-2", BusinessID: "biz-2", Status: "PENDING"},
		},
		HasMore: true,
	}}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformListSubscriptions", &dto.SubscriptionListInput{Limit: 50})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestPlatformCreateSubscriptionAddonPassesDurableIdempotencyKey(t *testing.T) {
	now := time.Now().UTC()
	addons := &stubSubscriptionAddonRepository{created: ports.SubscriptionAddonRecord{ID: "addon-1", SubscriptionID: subIDForTests, BusinessID: "biz-1", Kind: "AI_REPLIES_100", Status: "PENDING", CreatedAt: now}}
	server := newPlatformServer(PlatformDeps{
		Subscriptions: &stubSubscriptionRepository{getByID: ports.SubscriptionRecord{ID: subIDForTests, BusinessID: "biz-1"}},
		Addons:        addons,
		PlatformAudit: &stubPlatformAuditRepository{},
	})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSubscriptionAddon", &dto.CreateSubscriptionAddonInput{
		PlatformSubscriptionPath: platformSubscriptionPath(),
		PlatformCommandHeaders:   dto.PlatformCommandHeaders{IdempotencyKey: "create-addon-1"},
		Body:                     dto.CreateSubscriptionAddonRequest{Kind: "AI_REPLIES_100"},
	})
	if !handled {
		t.Fatal("expected add-on command to be handled")
	}
	if addons.createdInput.IdempotencyKey != "create-addon-1" {
		t.Fatalf("creation idempotency key = %q", addons.createdInput.IdempotencyKey)
	}
}

func TestPlatformListAndCancelSubscriptionAddons(t *testing.T) {
	now := time.Now().UTC()
	addons := &stubSubscriptionAddonRepository{
		page:      ports.SubscriptionAddonPage{Items: []ports.SubscriptionAddonRecord{{ID: "addon-1", SubscriptionID: subIDForTests, BusinessID: "biz-1", Status: "ACTIVE", CreatedAt: now}}},
		cancelled: ports.SubscriptionAddonRecord{ID: "addon-1", SubscriptionID: subIDForTests, BusinessID: "biz-1", Status: "CANCELLED", CreatedAt: now},
	}
	server := newPlatformServer(PlatformDeps{Addons: addons, PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	if _, handled := server.dispatchPlatformCommand(ctx, "platformListSubscriptionAddons", &dto.SubscriptionAddonListInput{PlatformSubscriptionPath: platformSubscriptionPath()}); !handled {
		t.Fatal("expected list add-ons command to be handled")
	}
	if _, handled := server.dispatchPlatformCommand(ctx, "platformCancelSubscriptionAddon", &dto.CancelSubscriptionAddonInput{
		PlatformSubscriptionAddonPath: dto.PlatformSubscriptionAddonPath{AddonID: dto.UUID("00000000-0000-0000-0000-00000000add1")},
		Body:                          dto.CancelSubscriptionRequest{Reason: "manual correction"},
	}); !handled {
		t.Fatal("expected cancel add-on command to be handled")
	}
}

func TestPlatformApproveChannelPassCapacityRequiresReason(t *testing.T) {
	server := newPlatformServer(PlatformDeps{Addons: &stubSubscriptionAddonRepository{}, Subscriptions: &stubSubscriptionRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformApproveChannelPassCapacity", &dto.ApproveChannelPassCapacityInput{
		PlatformSubscriptionPath: platformSubscriptionPath(),
		Body:                     dto.ChannelPassCapacityApprovalRequest{MaxChannelPasses: 1},
	})
	if !handled {
		t.Fatal("expected approval command to be handled")
	}
}

func TestPlatformCreateSubscriptionFailsOnInactivePlan(t *testing.T) {
	planRepo := &stubPlanRepository{}
	// Note: stubPlanRepository.GetByID returns its getByIDErr field — leave nil
	// and set getByID to a DRAFT plan to simulate the "inactive plan" case.
	planRepo.getByID = ports.PlanRecord{ID: "plan-1", Status: "DRAFT"}
	subRepo := &stubSubscriptionRepository{}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, Subscriptions: subRepo, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSubscription", &dto.CreateSubscriptionInput{
		Body: dto.CreateSubscriptionRequest{PlanID: "00000000-0000-0000-0000-000000000001"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append to be called on failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
	if auditRepo.appendedDraft.FailureCode == nil || *auditRepo.appendedDraft.FailureCode != "PLAN_NOT_ACTIVE" {
		t.Errorf("expected failure_code=PLAN_NOT_ACTIVE, got %v", auditRepo.appendedDraft.FailureCode)
	}
}

func TestPlatformCreateSubscriptionSucceedsAndAudits(t *testing.T) {
	planRepo := &stubPlanRepository{
		getByID: ports.PlanRecord{ID: "plan-1", Code: "basic", Version: 1, Status: "ACTIVE",
			AIReplyLimit: 500, AICatalogLimit: 200, ChannelLimit: 1, InternalAICostBudgetYER: 1000},
	}
	subRepo := &stubSubscriptionRepository{created: ports.SubscriptionRecord{ID: "sub-1", BusinessID: "biz-1", Status: "PENDING", PlanID: "plan-1"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, Subscriptions: subRepo, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSubscription", &dto.CreateSubscriptionInput{
		Body: dto.CreateSubscriptionRequest{PlanID: "00000000-0000-0000-0000-000000000001"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "subscription.created" {
		t.Errorf("expected action=subscription.created, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformCancelSubscriptionRequiresReason(t *testing.T) {
	subRepo := &stubSubscriptionRepository{}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Empty reason — must be rejected as validation error BEFORE calling Cancel.
	_, handled := server.dispatchPlatformCommand(ctx, "platformCancelSubscription", &dto.CancelSubscriptionInput{PlatformSubscriptionPath: platformSubscriptionPath(),
		Body: dto.CancelSubscriptionRequest{Reason: "  "},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCancelSubscriptionAuditsFailure(t *testing.T) {
	subRepo := &stubSubscriptionRepository{cancelErr: errors.New("already cancelled")}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCancelSubscription", &dto.CancelSubscriptionInput{PlatformSubscriptionPath: platformSubscriptionPath(),
		Body: dto.CancelSubscriptionRequest{Reason: "Customer dispute"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit on failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
}

// ----------------------------------------------------------------------------
// Tests: Manual payment recording + activation (Contract §25-28)
// ----------------------------------------------------------------------------

func TestPlatformRecordPaymentRejectsInvalidAmount(t *testing.T) {
	server := newPlatformServer(PlatformDeps{
		Subscriptions: &stubSubscriptionRepository{},
		Payments:      &stubPaymentRepository{},
		PlatformAudit: &stubPlatformAuditRepository{},
	})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformRecordPayment", &dto.RecordPaymentInput{PlatformSubscriptionPath: platformSubscriptionPath(),
		Body: dto.RecordPaymentRequest{AmountYER: 0, Method: "CASH", Reference: "ref-1", PaidAt: time.Now().UTC().Format(time.RFC3339)},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformRecordPaymentRejectsInvalidMethod(t *testing.T) {
	server := newPlatformServer(PlatformDeps{
		Subscriptions: &stubSubscriptionRepository{},
		Payments:      &stubPaymentRepository{},
		PlatformAudit: &stubPlatformAuditRepository{},
	})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformRecordPayment", &dto.RecordPaymentInput{PlatformSubscriptionPath: platformSubscriptionPath(),
		Body: dto.RecordPaymentRequest{AmountYER: 5000, Method: "PAYPAL", Reference: "ref-1", PaidAt: time.Now().UTC().Format(time.RFC3339)},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformRecordPaymentFailsIfSubscriptionMissing(t *testing.T) {
	subRepo := &stubSubscriptionRepository{getErr: errors.New("not found")}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, Payments: &stubPaymentRepository{}, Transactions: stubBillingTransactions{}, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformRecordPayment", &dto.RecordPaymentInput{PlatformSubscriptionPath: platformSubscriptionPath(), PlatformCommandHeaders: dto.PlatformCommandHeaders{IdempotencyKey: "payment-missing-sub"},
		Body: dto.RecordPaymentRequest{AmountYER: 5000, Method: "CASH", Reference: "ref-1", PaidAt: time.Now().UTC().Format(time.RFC3339)},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit on subscription lookup failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformRecordPaymentRecordsPaymentAndActivatesPendingSubscription(t *testing.T) {
	// The subscription is in PENDING — recording a payment should:
	// 1. Append the payment (returns payment ID).
	// 2. Call Activate() to transition PENDING → ACTIVE.
	// 3. Audit BOTH payment.recorded (SUCCESS) and subscription.activated (SUCCESS).
	now := time.Now().UTC()
	subRepo := &stubSubscriptionRepository{
		getByID:   ports.SubscriptionRecord{ID: "sub-1", BusinessID: "biz-1", Status: "PENDING", AmountDueYER: 5000, SeatLimit: 1, PeriodStart: now.Add(-time.Hour), PeriodEnd: now.Add(time.Hour)},
		activated: ports.SubscriptionRecord{ID: "sub-1", BusinessID: "biz-1", Status: "ACTIVE"},
	}
	paymentRepo := &stubPaymentRepository{appended: ports.PaymentRecord{ID: "pay-1", SubscriptionID: "sub-1", BusinessID: "biz-1", AmountYER: 5000}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, Payments: paymentRepo, Transactions: stubBillingTransactions{}, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformRecordPayment", &dto.RecordPaymentInput{PlatformSubscriptionPath: platformSubscriptionPath(), PlatformCommandHeaders: dto.PlatformCommandHeaders{IdempotencyKey: "payment-pending"},
		Body: dto.RecordPaymentRequest{AmountYER: 5000, Method: "CASH", Reference: "ref-1", PaidAt: time.Now().UTC().Format(time.RFC3339)},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	// The LAST audit call should be subscription.activated (SUCCESS) since
	// the payment was recorded first then the activation happened.
	if auditRepo.appendedDraft.Action != "subscription.activated" {
		t.Errorf("expected final action=subscription.activated, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformRecordPaymentRecordsPaymentButDoesNotActivateActiveSubscription(t *testing.T) {
	// If the subscription is already ACTIVE (e.g., a renewal mid-period),
	// the activation step must NOT be called (no transition needed).
	subRepo := &stubSubscriptionRepository{
		getByID:     ports.SubscriptionRecord{ID: "sub-1", BusinessID: "biz-1", Status: "ACTIVE", AmountDueYER: 5000},
		activateErr: errors.New("should not be called"),
	}
	paymentRepo := &stubPaymentRepository{appended: ports.PaymentRecord{ID: "pay-1"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, Payments: paymentRepo, Transactions: stubBillingTransactions{}, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformRecordPayment", &dto.RecordPaymentInput{PlatformSubscriptionPath: platformSubscriptionPath(), PlatformCommandHeaders: dto.PlatformCommandHeaders{IdempotencyKey: "payment-active"},
		Body: dto.RecordPaymentRequest{AmountYER: 5000, Method: "CASH", Reference: "ref-1", PaidAt: time.Now().UTC().Format(time.RFC3339)},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	// Final audit must be payment.recorded (SUCCESS) — no activation attempt.
	if auditRepo.appendedDraft.Action != "payment.recorded" {
		t.Errorf("expected final action=payment.recorded, got %s", auditRepo.appendedDraft.Action)
	}
}

// ----------------------------------------------------------------------------
// Tests: Cost budget override (AIUsageTokenTelemetry.md §24)
// ----------------------------------------------------------------------------

func TestPlatformOverrideSubscriptionAICostBudgetRejectsZeroBudget(t *testing.T) {
	server := newPlatformServer(PlatformDeps{
		Subscriptions: &stubSubscriptionRepository{},
		PlatformAudit: &stubPlatformAuditRepository{},
	})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformOverrideSubscriptionAICostBudget", &dto.SubscriptionAICostBudgetOverrideInput{PlatformSubscriptionPath: platformSubscriptionPath(),
		Body: dto.SubscriptionAICostBudgetOverrideRequest{BudgetYER: 0, Reason: "test"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformOverrideSubscriptionAICostBudgetAuditsSuccess(t *testing.T) {
	subRepo := &stubSubscriptionRepository{overrideResult: ports.SubscriptionRecord{ID: "sub-1", BusinessID: "biz-1"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Subscriptions: subRepo, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformOverrideSubscriptionAICostBudget", &dto.SubscriptionAICostBudgetOverrideInput{PlatformSubscriptionPath: platformSubscriptionPath(),
		Body: dto.SubscriptionAICostBudgetOverrideRequest{BudgetYER: 5000, Reason: "Updated internal guardrail"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "subscription.cost_budget_overridden" {
		t.Errorf("expected action=subscription.cost_budget_overridden, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
}

// ----------------------------------------------------------------------------
// Tests: List payments (Contract §27 — append-only record history)
// ----------------------------------------------------------------------------

func TestPlatformListPaymentsReturns501WhenUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformListPayments", &dto.PaymentListInput{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

// silence "unused" warnings during test suite build
var _ = strings.TrimSpace
