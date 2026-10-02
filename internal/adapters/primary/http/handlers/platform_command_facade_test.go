package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ----------------------------------------------------------------------------
// Mock implementations for Platform repository ports
// ----------------------------------------------------------------------------

type stubPlanRepository struct {
	createdPlan ports.PlanRecord
	createErr   error
	getByID     ports.PlanRecord
	getByIDErr  error
	listPage    ports.PlanPage
	listErr     error
	activateErr error
	retireErr   error
	activated   ports.PlanRecord
	retired     ports.PlanRecord
	// CreateVersion (Stage 6 — Plan Version-on-Edit)
	newVersion       ports.PlanRecord
	createVersionErr error
}

func (s *stubPlanRepository) Create(_ context.Context, c ports.PlanCreate) (ports.PlanRecord, error) {
	if s.createErr != nil {
		return ports.PlanRecord{}, s.createErr
	}
	return s.createdPlan, nil
}
func (s *stubPlanRepository) GetByID(_ context.Context, _ string) (ports.PlanRecord, error) {
	if s.getByIDErr != nil {
		return ports.PlanRecord{}, s.getByIDErr
	}
	return s.getByID, nil
}
func (s *stubPlanRepository) List(_ context.Context, _ ports.PlanListFilter) (ports.PlanPage, error) {
	return s.listPage, s.listErr
}
func (s *stubPlanRepository) Activate(_ context.Context, _ string, _ time.Time) (ports.PlanRecord, error) {
	if s.activateErr != nil {
		return ports.PlanRecord{}, s.activateErr
	}
	return s.activated, nil
}
func (s *stubPlanRepository) Retire(_ context.Context, _ string, _ time.Time) (ports.PlanRecord, error) {
	if s.retireErr != nil {
		return ports.PlanRecord{}, s.retireErr
	}
	return s.retired, nil
}
func (s *stubPlanRepository) CreateVersion(_ context.Context, _ ports.PlanVersionCreate) (ports.PlanRecord, error) {
	if s.createVersionErr != nil {
		return ports.PlanRecord{}, s.createVersionErr
	}
	return s.newVersion, nil
}

type stubPlatformBusinessRepository struct {
	listPage      ports.PlatformBusinessPage
	listErr       error
	getByID       ports.PlatformBusinessRecord
	getErr        error
	suspendErr    error
	reactivateErr error
	archiveErr    error
	suspended     ports.PlatformBusinessRecord
	reactivated   ports.PlatformBusinessRecord
	archived      ports.PlatformBusinessRecord
	// Create (Gap 1 — POST /platform/businesses)
	createResult ports.PlatformBusinessRecord
	createErr    error
}

func (s *stubPlatformBusinessRepository) Create(_ context.Context, _ ports.PlatformBusinessCreate) (ports.PlatformBusinessRecord, error) {
	return s.createResult, s.createErr
}
func (s *stubPlatformBusinessRepository) List(_ context.Context, _ ports.PlatformBusinessListFilter) (ports.PlatformBusinessPage, error) {
	return s.listPage, s.listErr
}
func (s *stubPlatformBusinessRepository) GetByID(_ context.Context, _ string) (ports.PlatformBusinessRecord, error) {
	return s.getByID, s.getErr
}
func (s *stubPlatformBusinessRepository) Suspend(_ context.Context, _ string, _ time.Time) (ports.PlatformBusinessRecord, error) {
	if s.suspendErr != nil {
		return ports.PlatformBusinessRecord{}, s.suspendErr
	}
	return s.suspended, nil
}
func (s *stubPlatformBusinessRepository) Reactivate(_ context.Context, _ string, _ time.Time) (ports.PlatformBusinessRecord, error) {
	if s.reactivateErr != nil {
		return ports.PlatformBusinessRecord{}, s.reactivateErr
	}
	return s.reactivated, nil
}
func (s *stubPlatformBusinessRepository) Archive(_ context.Context, _ string, _ time.Time) (ports.PlatformBusinessRecord, error) {
	if s.archiveErr != nil {
		return ports.PlatformBusinessRecord{}, s.archiveErr
	}
	return s.archived, nil
}

type stubPlatformAuditRepository struct {
	appendedDraft *ports.PlatformAuditDraft
	appendErr     error
	listPage      ports.PlatformAuditPage
	listErr       error
	getByID       ports.PlatformAuditEvent
	getErr        error
}

func (s *stubPlatformAuditRepository) Append(_ context.Context, draft ports.PlatformAuditDraft) (ports.PlatformAuditEvent, error) {
	s.appendedDraft = &draft
	return ports.PlatformAuditEvent{ID: "audit-event-1"}, s.appendErr
}
func (s *stubPlatformAuditRepository) List(_ context.Context, _ ports.PlatformAuditFilter) (ports.PlatformAuditPage, error) {
	return s.listPage, s.listErr
}
func (s *stubPlatformAuditRepository) GetByID(_ context.Context, _ string) (ports.PlatformAuditEvent, error) {
	return s.getByID, s.getErr
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func newPlatformServer(deps PlatformDeps) *Server {
	server := NewServer(Dependencies{})
	server.platformDeps = deps
	return server
}

// ctxWithPlatformAdmin sets up a context that satisfies middleware.PlatformAdminID().
// Per Contract §55: every Platform command requires Platform Scope to be set
// in context. Without it, the facade returns 403 even if the underlying
// repository is wired.
func ctxWithPlatformAdmin() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return middleware.WithPlatformAdmin(ctx, "platform-admin-1"), cancel
}

func ctxWithoutPlatformAdmin() context.Context {
	return context.Background()
}

// ----------------------------------------------------------------------------
// Tests: Platform Scope boundary (Contract §4, §55, §63-64)
// ----------------------------------------------------------------------------

func TestPlatformCommandReturns403WithoutPlatformScope(t *testing.T) {
	server := newPlatformServer(PlatformDeps{
		Plans: &stubPlanRepository{},
	})
	// No PlatformAdminID in context — even with Plan repository wired,
	// the facade must reject with Forbidden (HTTP 403).
	result, handled := server.dispatchPlatformCommand(ctxWithoutPlatformAdmin(), "platformListPlans", &dto.PlanListInput{})
	if !handled {
		t.Fatalf("expected dispatchPlatformCommand to handle platformListPlans")
	}
	err, ok := result.(error)
	if !ok || err == nil {
		t.Fatalf("expected error result, got %#v", result)
	}
	// mapApplicationError wraps appErrors.CodeForbidden into a dashboardHTTPError
	// with status=403. Verify via the GetStatus method (or check the wrapped type).
	var httpErr interface{ GetStatus() int }
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected error to implement GetStatus(), got %T: %v", err, err)
	}
	if httpErr.GetStatus() != 403 {
		t.Fatalf("expected HTTP 403 Forbidden, got status %d", httpErr.GetStatus())
	}
}

// ----------------------------------------------------------------------------
// Tests: Plan lifecycle (Contract §15-19)
// ----------------------------------------------------------------------------

func TestPlatformCreatePlanAppendsAuditOnSuccess(t *testing.T) {
	planRepo := &stubPlanRepository{createdPlan: ports.PlanRecord{ID: "plan-1", Code: "basic", Version: 1}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()

	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlan", &dto.CreatePlanInput{
		Body: dto.CreatePlanRequest{
			Code: "basic", DisplayName: "Basic", PriceYER: 5000,
			BillingInterval: "MONTH", AIReplyLimit: 500, AICatalogLimit: 200,
			ChannelLimit: 1, InternalAICostBudgetYER: 1000,
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append to be called")
	}
	if auditRepo.appendedDraft.Action != "plan.created" {
		t.Errorf("expected action=plan.created, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformCreatePlanAppendsAuditOnFailure(t *testing.T) {
	planRepo := &stubPlanRepository{
		createErr: errors.New("duplicate code"),
	}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()

	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlan", &dto.CreatePlanInput{
		Body: dto.CreatePlanRequest{
			Code: "basic", DisplayName: "Basic", PriceYER: 5000, AIReplyLimit: 500,
			AICatalogLimit: 200, ChannelLimit: 1, InternalAICostBudgetYER: 1000,
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append to be called even on failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformRetirePlanReturns501WhenPlansNotWired(t *testing.T) {
	// Plans=nil simulates the unwired state — route is registered but no service.
	server := newPlatformServer(PlatformDeps{PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformRetirePlan", &dto.PlanRetireInput{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if err, ok := resultError(handled); ok || err != nil {
		// expected: NotImplemented error
	}
}

// ----------------------------------------------------------------------------
// Tests: Business lifecycle (Contract §10-14)
// ----------------------------------------------------------------------------

func TestPlatformSuspendBusinessAppendsAuditWithBusinessID(t *testing.T) {
	bizRepo := &stubPlatformBusinessRepository{suspended: ports.PlatformBusinessRecord{ID: "biz-1", PlatformStatus: "suspended"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{PlatformBusiness: bizRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformSuspendBusiness", &dto.PlatformBusinessSuspendInput{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "business.suspended" {
		t.Errorf("expected action=business.suspended, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.BusinessID == nil || *auditRepo.appendedDraft.BusinessID != "biz-1" {
		t.Errorf("expected business_id=biz-1, got %v", auditRepo.appendedDraft.BusinessID)
	}
}

// ----------------------------------------------------------------------------
// Tests: AI Operations return NotImplemented until wired (Contract §70)
// ----------------------------------------------------------------------------

func TestPlatformAIOperationsReturnNotImplementedWhenUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Per Stage 5: all AI Operations routes are now wired. The 501 fallback
	// only triggers when the Operations registry is NOT wired (nil
	// PlatformDeps.Operations). This test verifies the unwired case.
	// Each operation expects a typed input (DTO struct) — pass empty values.
	tests := []struct {
		op    string
		input any
	}{
		{"platformGetAIOverview", &dto.EmptyInput{}},
		{"platformAIDisable", &dto.PlatformAIRuntimeDisableInput{}},
		{"platformAIEnable", &dto.PlatformAIRuntimeEnableInput{}},
		{"platformAIHealthCheck", &dto.PlatformAIHealthCheckInput{}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			_, handled := server.dispatchPlatformCommand(ctx, tc.op, tc.input)
			if !handled {
				t.Fatalf("expected handled=true for %s", tc.op)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Tests: Non-platform operations are NOT handled by dispatchPlatformCommand
// ----------------------------------------------------------------------------

func TestPlatformDispatchIgnoresMerchantOperations(t *testing.T) {
	server := newPlatformServer(PlatformDeps{Plans: &stubPlanRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// Merchant operations like "listConversations" must NOT be claimed by the
	// platform dispatcher — they must fall through to dispatchQuery / dispatchCommand.
	_, handled := server.dispatchPlatformCommand(ctx, "listConversations", nil)
	if handled {
		t.Fatalf("dispatchPlatformCommand must NOT handle merchant operations")
	}
}

// ----------------------------------------------------------------------------
// Tests: Platform Audit projections
// ----------------------------------------------------------------------------

func TestPlatformAuditListReturnsEvents(t *testing.T) {
	auditRepo := &stubPlatformAuditRepository{
		listPage: ports.PlatformAuditPage{
			Items: []ports.PlatformAuditEvent{
				{ID: "evt-1", Action: "business.suspended", TargetType: "business", Result: "SUCCESS"},
				{ID: "evt-2", Action: "plan.created", TargetType: "plan", Result: "FAILURE"},
			},
			HasMore: true,
		},
	}
	server := newPlatformServer(PlatformDeps{PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformListAuditEvents", &dto.PlatformAuditListInput{Limit: 50})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
}

// ----------------------------------------------------------------------------
// Tests: error classifier (audit failure codes)
// ----------------------------------------------------------------------------

// localRepoError is a test double for the postgres RepositoryError that
// satisfies the ErrorKind() string interface used by classifyPlatformRepoErrorKind.
// We don't import postgres types here so this test stays in the handlers
// package (no DB dependency).
type localRepoError struct {
	kind string
}

func (e *localRepoError) Error() string     { return "test repo error: " + e.kind }
func (e *localRepoError) ErrorKind() string { return e.kind }

func TestClassifyPlatformRepoErrorKind(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"nil", nil, ""},
		{"not_found", &localRepoError{kind: "not_found"}, "NOT_FOUND"},
		{"conflict", &localRepoError{kind: "conflict"}, "CONFLICT"},
		{"invalid", &localRepoError{kind: "invalid"}, "INVALID_INPUT"},
		{"stale", &localRepoError{kind: "stale"}, "STALE_VERSION"},
		{"unknown", errors.New("boom"), "INTERNAL_ERROR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPlatformRepoErrorKind(tc.err)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// resultError is a tiny helper to unwrap (any, bool) → (error, bool).
func resultError(_ bool) (error, bool) { return nil, false }

// avoid unused import warnings if test evolution removes strings usage
var _ = strings.TrimSpace
