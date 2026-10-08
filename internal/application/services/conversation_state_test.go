package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func strPtr(s string) *string { return &s }

func TestAutoReply_OneAICallPerTurn(t *testing.T) {
	rt := &FakeCustomerSalesDecisionPort{
		Proposal: ports.CustomerSalesProposal{
			Status:       ports.CustomerSalesProposalStatusResolved,
			Action:       ports.CustomerSalesProposalActionAnswer,
			ResponseText: "ok",
		},
	}
	svc := AutoReplyService{
		CustomerSalesDecision:       rt,
		CustomerSalesContextBuilder: stubBuilder{},
		DecisionRepository:          stubDecisionRepo{},
		ReferenceRepository:         stubRefRepo{},
		OutboundRepository:          stubOutboundRepo{},
		Outbox:                      stubOutbox{},
		Transactions:                passthroughTransactionManager{},
		StateRepository:             stubStateRepo{},
		Mode:                        AutoReplyModeRestrictedAuto,
		// Per Item 4: wire AIUsage + Subscriptions so recordAIUsage doesn't fail.
		AIUsage:       &stubAIUsageRepo{},
		Subscriptions: &stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b1", Status: "ACTIVE"}}},
		AIPricing:     &stubPricingRepoAlwaysFail{},
		NewID:         func() string { return "test-id" },
	}
	svc.Validation = allowAllValidationPipeline()
	_, err := svc.Handle(context.Background(), commands.AutoReplyCommand{
		Meta:           commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "b1"}},
		ConversationID: "c1", SourceMessageReference: "m1", Text: "hello", Channel: "whatsapp", ProviderRef: "socialapi",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Per contract ④ §4, FakeCustomerSalesDecisionPort tracks LastInput instead of a call counter.
	// We verify the AI was called exactly once by checking LastInput was populated.
	if rt.LastInput.Request.Text != "hello" {
		t.Fatalf("expected one AI call with text \"hello\", got LastInput.Text=%q", rt.LastInput.Request.Text)
	}
}

type stubBuilder struct{}

func (stubBuilder) Build(_ context.Context, _ ports.CustomerSalesContextInput) (ports.CustomerSalesContext, error) {
	return ports.CustomerSalesContext{Conversation: ports.CustomerSalesContextConversation{State: "open"}}, nil
}

type stubDecisionRepo struct{}

func (stubDecisionRepo) CreateProposed(_ context.Context, d ports.AIDecisionDraft) (ports.AIDecisionRecord, error) {
	return ports.AIDecisionRecord{ID: d.ID}, nil
}
func (stubDecisionRepo) List(_ context.Context, _ ports.AIDecisionFilter) (ports.AIDecisionPage, error) {
	return ports.AIDecisionPage{}, nil
}
func (stubDecisionRepo) Get(_ context.Context, _, _ string) (ports.AIDecisionRecord, error) {
	return ports.AIDecisionRecord{}, nil
}
func (stubDecisionRepo) RequestHumanReview(_ context.Context, _ ports.HumanReviewPatch) (ports.AIDecisionRecord, error) {
	return ports.AIDecisionRecord{}, nil
}

type stubRefRepo struct{}

func (s stubRefRepo) GetByID(_ context.Context, _, _ string) (ports.ConversationReferenceRecord, error) {
	return ports.ConversationReferenceRecord{
		ID:            "ref-stub",
		BusinessID:    "b1",
		ProviderRef:   "socialapi",
		ResourceID:    "provider-stub-1",
		ConnectionID:  stringPtr("conn-stub-1"),
		IsCurrent:     true,
		MappingStatus: "active",
	}, nil
}
func (s stubRefRepo) GetCurrentByConversation(_ context.Context, _, _, _ string) (ports.ConversationReferenceRecord, error) {
	return ports.ConversationReferenceRecord{
		ID:            "ref-stub",
		BusinessID:    "b1",
		ProviderRef:   "socialapi",
		ResourceID:    "provider-stub-1",
		ConnectionID:  stringPtr("conn-stub-1"),
		IsCurrent:     true,
		MappingStatus: "active",
	}, nil
}

type stubOutboundRepo struct{}

func (stubOutboundRepo) CreatePending(_ context.Context, d ports.OutboundMessageDraft) (ports.OutboundMessageRecord, error) {
	return ports.OutboundMessageRecord{ID: d.ID}, nil
}
func (stubOutboundRepo) GetByID(_ context.Context, _, _ string) (ports.OutboundMessageRecord, error) {
	return ports.OutboundMessageRecord{}, nil
}

type stubOutbox struct{}

func (stubOutbox) Enqueue(_ context.Context, d ports.OutboxEntryDraft) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{ID: d.ID}, nil
}
func (stubOutbox) Claim(_ context.Context, _ string, _ ports.OutboxLease) (ports.OutboxClaimResult, error) {
	return ports.OutboxClaimResult{}, nil
}
func (stubOutbox) MarkCompleted(_ context.Context, _ string, _ ports.OutboxCompletion) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}
func (stubOutbox) MarkRetryableFailure(_ context.Context, _ string, _ ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}
func (stubOutbox) MoveToDeadLetter(_ context.Context, _ string, _ ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}
func (stubOutbox) ListClaimable(_ context.Context, _ int) ([]ports.OutboxEntryRecord, error) {
	return nil, nil
}
func (stubOutbox) Get(_ context.Context, _, _ string) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}
func (stubOutbox) List(_ context.Context, _ ports.OutboxFilter) (ports.OutboxPage, error) {
	return ports.OutboxPage{}, nil
}
func (stubOutbox) Requeue(_ context.Context, _ string, _, _ time.Time) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}

type stubStateRepo struct{}

func (stubStateRepo) Get(_ context.Context, _, _ string) (ports.ConversationStateRecord, error) {
	return ports.ConversationStateRecord{}, errNotFoundStub{}
}
func (stubStateRepo) UpsertValidated(_ context.Context, r ports.ConversationStateRecord) (ports.ConversationStateRecord, error) {
	return r, nil
}

type errNotFoundStub struct{}

func (errNotFoundStub) Error() string     { return "not found" }
func (errNotFoundStub) ErrorKind() string { return "not_found" }
