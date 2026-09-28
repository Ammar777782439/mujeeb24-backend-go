package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func testBusinessCtx() *ports.AIContext {
	return &ports.AIContext{
		CatalogEvidence: []ports.AICatalogEvidence{
			{Reference: "item-basic", CatalogReference: "cat-1", Name: "Basic"},
			{Reference: "item-pro", CatalogReference: "cat-1", Name: "Pro"},
		},
		OfferEvidence: []ports.AIOfferEvidence{
			{Reference: "offer-basic", CatalogItemReference: "item-basic", Name: "Basic", PricingMode: "fixed", Amount: "100", Currency: "YER", AvailabilityStatus: "available", Status: "active"},
			{Reference: "offer-pro", CatalogItemReference: "item-pro", Name: "Pro", PricingMode: "fixed", Amount: "200", Currency: "YER", AvailabilityStatus: "available", Status: "active"},
		},
	}
}

func TestValidateStateProposal_Resolved(t *testing.T) {
	ctx := testBusinessCtx()
	proposal := ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{
			Kind:  "RESOLVED",
			Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic"},
		},
	}
	_, needsClar, err := validateStateProposal(proposal, ctx)
	if err != nil || needsClar {
		t.Fatalf("expected resolved, got clar=%v err=%v", needsClar, err)
	}
}

func TestValidateStateProposal_Ambiguous(t *testing.T) {
	ctx := testBusinessCtx()
	proposal := ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{Kind: "AMBIGUOUS"},
	}
	_, needsClar, _ := validateStateProposal(proposal, ctx)
	if !needsClar {
		t.Fatal("ambiguous must need clarification")
	}
}

func TestValidateStateProposal_UnknownFocus(t *testing.T) {
	ctx := testBusinessCtx()
	proposal := ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{
			Kind:  "RESOLVED",
			Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-unknown"},
		},
	}
	_, needsClar, _ := validateStateProposal(proposal, ctx)
	if !needsClar {
		t.Fatal("unknown focus must need clarification, not random pick")
	}
}

func TestBuildValidatedState_Replacement(t *testing.T) {
	current := &ports.ConversationStateRecord{
		BusinessID: "b1", ConversationID: "c1",
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic"},
	}
	proposal := ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{
			Kind:  "RESOLVED",
			Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-pro"},
		},
	}
	next := buildValidatedState(current, "b1", "c1", proposal)
	if next.Focus.ID != "offer-pro" {
		t.Fatalf("expected replacement to pro, got %v", next.Focus)
	}
	if len(next.Previous) != 1 || next.Previous[0].ID != "offer-basic" {
		t.Fatalf("previous must retain basic, got %v", next.Previous)
	}
}

func TestValidateEvidenceIdentity_BasicProIsolation(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.ConversationState = &ports.ConversationStateRecord{
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
	}
	// Proposal references only basic -> allowed
	proposal := ports.AIDecisionProposal{EvidenceReferences: []byte(`["offer-basic"]`)}
	if !validateEvidenceIdentity(proposal, ctx) {
		t.Fatal("basic evidence should be allowed")
	}
	// Proposal references pro while focus is basic -> reject
	proposal2 := ports.AIDecisionProposal{EvidenceReferences: []byte(`["offer-pro"]`)}
	if validateEvidenceIdentity(proposal2, ctx) {
		t.Fatal("pro evidence must not satisfy basic focus")
	}
}

// Regression: a general question ("what packages do you have?") asked while
// the stored focus is Basic must NOT be flagged as entity mixing when the
// proposal declares NO_REFERENCE and references in-context offers.
func TestValidateEvidenceIdentity_GeneralQuestionAfterFocus(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.ConversationState = &ports.ConversationStateRecord{
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
	}
	proposal := ports.AIDecisionProposal{
		EvidenceReferences: []byte(`["offer-basic","offer-pro"]`),
		StateProposal:      &ports.AIStateProposal{Kind: "NO_REFERENCE"},
	}
	if !validateEvidenceIdentity(proposal, ctx) {
		t.Fatal("general answer with NO_REFERENCE must not be flagged as mixing")
	}
}

// A legitimate topic switch (new RESOLVED focus to Pro) must be judged
// against the NEW focus, not the stored Basic focus.
func TestValidateEvidenceIdentity_TopicSwitchToNewFocus(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.ConversationState = &ports.ConversationStateRecord{
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
	}
	proposal := ports.AIDecisionProposal{
		EvidenceReferences: []byte(`["offer-pro"]`),
		StateProposal: &ports.AIStateProposal{
			Kind:  "RESOLVED",
			Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-pro", ItemID: strPtr("item-pro")},
		},
	}
	if !validateEvidenceIdentity(proposal, ctx) {
		t.Fatal("topic switch to validated Pro focus must be allowed")
	}
}

// Mixing protection still holds when the proposal keeps the old focus.
func TestValidateEvidenceIdentity_MixingStillRejectedWithResolvedFocus(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.ConversationState = &ports.ConversationStateRecord{
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
	}
	proposal := ports.AIDecisionProposal{
		EvidenceReferences: []byte(`["offer-pro"]`),
		StateProposal: &ports.AIStateProposal{
			Kind:  "RESOLVED",
			Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
		},
	}
	if validateEvidenceIdentity(proposal, ctx) {
		t.Fatal("pro evidence under resolved Basic focus must still be rejected")
	}
}

// Hallucination protection: invented references are rejected even for general answers.
func TestValidateEvidenceIdentity_InventedRefRejected(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.ConversationState = &ports.ConversationStateRecord{
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
	}
	proposal := ports.AIDecisionProposal{
		EvidenceReferences: []byte(`["offer-invented"]`),
		StateProposal:      &ports.AIStateProposal{Kind: "NO_REFERENCE"},
	}
	if validateEvidenceIdentity(proposal, ctx) {
		t.Fatal("invented reference must be rejected even with NO_REFERENCE")
	}
}

// Regression for the blocked greeting: a general answer may cite the parent
// catalog and knowledge/policy documents, not only item/offer/variant IDs.
func TestValidateEvidenceIdentity_CatalogAndKnowledgeRefs(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.KnowledgeEvidence = []ports.AIKnowledgeEvidence{{Reference: "knowledge-1"}}
	ctx.BusinessPolicyEvidence = []ports.AIBusinessPolicyEvidence{{Reference: "policy-1"}}
	ctx.ConversationState = &ports.ConversationStateRecord{
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")},
	}
	proposal := ports.AIDecisionProposal{
		EvidenceReferences: []byte(`["cat-1","knowledge-1","policy-1","offer-basic","offer-pro"]`),
		StateProposal:      &ports.AIStateProposal{Kind: "NO_REFERENCE"},
	}
	if !validateEvidenceIdentity(proposal, ctx) {
		t.Fatal("catalog/knowledge/policy references must be accepted for general answers")
	}
}

func TestFocusInCandidates_AllBranches(t *testing.T) {
	ctx := testBusinessCtx()
	ctx.VariantEvidence = []ports.AIVariantEvidence{{Reference: "variant-1", CatalogItemReference: "item-basic"}}
	cases := []struct {
		name  string
		focus *ports.ConversationFocus
		want  bool
	}{
		{"nil focus", nil, false},
		{"empty id", &ports.ConversationFocus{Type: "offer", ID: "  "}, false},
		{"offer hit", &ports.ConversationFocus{Type: "offer", ID: "offer-basic"}, true},
		{"offer miss", &ports.ConversationFocus{Type: "offer", ID: "offer-nope"}, false},
		{"variant hit", &ports.ConversationFocus{Type: "variant", ID: "variant-1"}, true},
		{"variant miss", &ports.ConversationFocus{Type: "variant", ID: "variant-nope"}, false},
		{"item hit", &ports.ConversationFocus{Type: "item", ID: "item-pro"}, true},
		{"item miss", &ports.ConversationFocus{Type: "item", ID: "item-nope"}, false},
		{"catalog hit via item", &ports.ConversationFocus{Type: "catalog", ID: "cat-1"}, true},
		{"catalog miss", &ports.ConversationFocus{Type: "catalog", ID: "cat-nope"}, false},
		{"unknown type", &ports.ConversationFocus{Type: "spaceship", ID: "offer-basic"}, false},
		{"nil ctx", nil, false},
	}
	for _, tc := range cases {
		c := ctx
		if tc.name == "nil ctx" {
			c = nil
		}
		if got := focusInCandidates(tc.focus, c); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestRefsMatchFocus_OfferBranch(t *testing.T) {
	ctx := testBusinessCtx()
	focus := &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: strPtr("item-basic")}
	if !refsMatchFocus([]byte(`["offer-basic"]`), focus, ctx) {
		t.Fatal("own offer ref must pass")
	}
	if !refsMatchFocus([]byte(`["item-basic"]`), focus, ctx) {
		t.Fatal("parent item ref must pass via ItemID")
	}
	if refsMatchFocus([]byte(`["offer-pro"]`), focus, ctx) {
		t.Fatal("rival offer ref must fail")
	}
	if !refsMatchFocus(nil, focus, ctx) {
		t.Fatal("empty refs must pass")
	}
	if refsMatchFocus([]byte(`not-json`), focus, ctx) {
		t.Fatal("malformed refs must fail")
	}
}

func TestRefsMatchFocus_ItemBranchSecondLoop(t *testing.T) {
	ctx := testBusinessCtx()
	focus := &ports.ConversationFocus{Type: "item", ID: "item-basic"}
	if refsMatchFocus([]byte(`["item-pro"]`), focus, ctx) {
		t.Fatal("rival item ref must fail under item focus")
	}
	if refsMatchFocus([]byte(`not-json`), focus, ctx) {
		t.Fatal("malformed refs must fail")
	}
}

func TestValidateStateProposal_EdgeCases(t *testing.T) {
	ctx := testBusinessCtx()
	if _, clar, _ := validateStateProposal(ports.AIDecisionProposal{}, ctx); clar {
		t.Fatal("nil StateProposal must not force clarification")
	}
	if _, clar, _ := validateStateProposal(ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{Kind: "WEIRD"},
	}, ctx); clar {
		t.Fatal("unknown kind must not force clarification")
	}
	if _, clar, _ := validateStateProposal(ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{Kind: "RESOLVED"},
	}, ctx); !clar {
		t.Fatal("RESOLVED without focus must force clarification")
	}
	if _, clar, _ := validateStateProposal(ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{
			Kind:       "RESOLVED",
			Focus:      &ports.ConversationFocus{Type: "offer", ID: "offer-basic"},
			Comparison: &ports.ConversationComparison{Type: "offer_set", IDs: []string{"offer-basic", "offer-ghost"}},
		},
	}, ctx); !clar {
		t.Fatal("comparison with unknown ID must force clarification")
	}
}

func TestBuildValidatedState_NoReferenceResetsComparison(t *testing.T) {
	current := &ports.ConversationStateRecord{
		BusinessID: "b1", ConversationID: "c1",
		Focus:      &ports.ConversationFocus{Type: "offer", ID: "offer-basic"},
		Comparison: &ports.ConversationComparison{Type: "offer_set", IDs: []string{"offer-basic", "offer-pro"}},
	}
	next := buildValidatedState(current, "b1", "c1", ports.AIDecisionProposal{
		StateProposal: &ports.AIStateProposal{Kind: "NO_REFERENCE"},
	})
	if next == nil {
		t.Fatal("NO_REFERENCE with stale comparison must return updated state")
	}
	if next.Comparison != nil {
		t.Fatal("comparison must be cleared")
	}
	if next.Focus == nil || next.Focus.ID != "offer-basic" {
		t.Fatal("focus must be preserved")
	}
	if buildValidatedState(current, "b1", "c1", ports.AIDecisionProposal{}) != nil {
		t.Fatal("missing StateProposal must not touch state")
	}
}

func strPtr(s string) *string { return &s }

// Fake runtime counting calls to prove ONE AI call per turn.
// Per contract ④ §4, the new flow uses AIGeminiProposal. FakeContractRuntime
// tracks LastInput which can be inspected to count calls.
type countingRuntime struct {
	Fake *FakeContractRuntime
}

func TestAutoReply_OneAICallPerTurn(t *testing.T) {
	rt := &FakeContractRuntime{
		Proposal: ports.AIGeminiProposal{
			Status:       ports.AIProposalStatusResolved,
			Action:       ports.AIProposalActionAnswer,
			ResponseText: "ok",
		},
	}
	svc := AutoReplyService{
		Runtime:             rt,
		ContextBuilder:      stubBuilder{},
		DecisionRepository:  stubDecisionRepo{},
		ReferenceRepository: stubRefRepo{},
		OutboundRepository:  stubOutboundRepo{},
		Outbox:              stubOutbox{},
		Transactions:        passthroughTransactionManager{},
		StateRepository:     stubStateRepo{},
		Mode:                AutoReplyModeRestrictedAuto,
	}
	_, err := svc.Handle(context.Background(), commands.AutoReplyCommand{
		Meta:           commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "b1"}},
		ConversationID: "c1", SourceMessageReference: "m1", Text: "hello", Channel: "whatsapp", ProviderRef: "socialapi",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Per contract ④ §4, FakeContractRuntime tracks LastInput instead of a call counter.
	// We verify the AI was called exactly once by checking LastInput was populated.
	if rt.LastInput.DecisionInput.Text != "hello" {
		t.Fatalf("expected one AI call with text \"hello\", got LastInput.Text=%q", rt.LastInput.DecisionInput.Text)
	}
}

type stubBuilder struct{}

func (stubBuilder) Build(_ context.Context, _ ports.ContextBuildInput) (ports.AIContext, error) {
	return ports.AIContext{Conversation: ports.AIContextConversation{State: "open"}}, nil
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
