package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type businessAIPolicyStub struct {
	ports.BusinessManagementRepository
	policies map[string]string
	err      error
	calls    int
}

func (s *businessAIPolicyStub) GetRuntimePolicy(_ context.Context, businessID string) (ports.BusinessRuntimePolicyRecord, error) {
	s.calls++
	if s.err != nil {
		return ports.BusinessRuntimePolicyRecord{}, s.err
	}
	return ports.BusinessRuntimePolicyRecord{BusinessID: businessID, AIMode: s.policies[businessID]}, nil
}

func TestBusinessAIGuardUsesCurrentTenantPolicyAndFailsClosed(t *testing.T) {
	repo := &businessAIPolicyStub{policies: map[string]string{"disabled-merchant": "disabled", "other-merchant": "restricted_auto"}}
	guard := &BusinessAIGuard{Policies: repo}
	if err := guard.Check(context.Background(), "disabled-merchant"); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("disabled business was allowed: %v", err)
	}
	for _, mode := range []string{"restricted_auto", "approval", "assist"} {
		repo.policies["other-merchant"] = mode
		if err := guard.Check(context.Background(), "other-merchant"); err != nil {
			t.Fatalf("legitimate generation mode %s incorrectly blocked: %v", mode, err)
		}
	}
	repo.policies["other-merchant"] = "disabled"
	if err := guard.Check(context.Background(), "other-merchant"); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("updated opt-out did not take effect on next check: %v", err)
	}
	repo.policies["other-merchant"] = "unknown"
	if err := guard.Check(context.Background(), "other-merchant"); !errors.Is(err, ports.ErrBusinessAIPolicyUnavailable) {
		t.Fatalf("unknown mode did not fail closed: %v", err)
	}
	if err := guard.Check(context.Background(), ""); !errors.Is(err, ports.ErrBusinessAIPolicyUnavailable) {
		t.Fatalf("missing tenant was allowed: %v", err)
	}
	repo.err = errors.New("neon unavailable")
	if err := guard.Check(context.Background(), "other-merchant"); !errors.Is(err, ports.ErrBusinessAIPolicyUnavailable) {
		t.Fatalf("database failure did not fail closed: %v", err)
	}
	if err := (&BusinessAIGuard{}).Check(context.Background(), "other-merchant"); !errors.Is(err, ports.ErrBusinessAIPolicyUnavailable) {
		t.Fatalf("missing repository was allowed: %v", err)
	}
}

func TestAutoReplyDisabledBeforeDecisionOrOutbound(t *testing.T) {
	decisions := &fakeDecisionRepository{}
	outbox := &fakeOutboxStore{}
	repo := &businessAIPolicyStub{policies: map[string]string{"business-1": "disabled"}}
	service := NewAutoReplyService(
		NewFakeCustomerSalesDecisionPort("should never generate"),
		decisions,
		fakeReferenceRepository{},
		&fakeOutboundRepository{},
		outbox,
		fakeTransactionManager{},
	)
	service.AIGuard = &BusinessAIGuard{Policies: repo}
	got, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta:           commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}},
		ConversationID: "conversation-1", SourceMessageReference: "incoming-1",
		Text:           "كم سعر المنتج؟", Channel: "whatsapp", ProviderRef: "socialapi",
	})
	if err != nil || got.Enqueued || got.Action != "no_action" || outbox.calls != 0 || decisions.record.ID != "" {
		t.Fatalf("disabled AI triggered execution: result=%+v err=%v", got, err)
	}
	if repo.calls != 1 {
		t.Fatalf("expected one early policy read, got %d", repo.calls)
	}
}

func TestWebhookExecutionGateBlocksDisabledBusinessBeforeAIQueue(t *testing.T) {
	repo := &businessAIPolicyStub{policies: map[string]string{"business-1": "disabled"}}
	svc := &AICostProtectionService{
		PlatformOperations: &stubPlatformOperationsRepo{state: ports.AIRuntimeState{AdminState: ports.ProviderAdminEnabled}},
		BusinessAIGuard:    &BusinessAIGuard{Policies: repo},
	}
	allowed, reason := svc.IsAIExecutionAllowed(context.Background(), "business-1")
	if allowed || reason != "business_ai_disabled" {
		t.Fatalf("webhook would enqueue AI: allowed=%t reason=%q", allowed, reason)
	}
	repo.err = errors.New("database unavailable")
	allowed, reason = svc.IsAIExecutionAllowed(context.Background(), "business-1")
	if allowed || reason != "business_ai_policy_unavailable" {
		t.Fatalf("webhook failed open on database error: allowed=%t reason=%q", allowed, reason)
	}
}

func TestSummaryGateBlocksDisabledBusinessBeforeGenerating(t *testing.T) {
	repo := &businessAIPolicyStub{policies: map[string]string{"business-1": "disabled"}}
	svc := &ConversationSummaryService{
		CostProtection: &AICostProtectionService{
			PlatformOperations: &stubPlatformOperationsRepo{state: ports.AIRuntimeState{AdminState: ports.ProviderAdminEnabled}},
			BusinessAIGuard:    &BusinessAIGuard{Policies: repo},
		},
	}
	got, err := svc.MaybeSummarize(context.Background(), "business-1", "conversation-1")
	if err != nil || got.Summarized || !strings.Contains(got.SkippedReason, "business_ai_disabled") {
		t.Fatalf("disabled business generated summary: %+v err=%v", got, err)
	}
}
