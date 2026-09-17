package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type fakeAIRuntime struct {
	proposal ports.AIDecisionProposal
}

func (f fakeAIRuntime) Decide(context.Context, ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
	return f.proposal, nil
}

type fakeDecisionRepository struct {
	record ports.AIDecisionRecord
}

func (f *fakeDecisionRepository) CreateProposed(_ context.Context, draft ports.AIDecisionDraft) (ports.AIDecisionRecord, error) {
	f.record = ports.AIDecisionRecord{
		ID: draft.ID, BusinessID: draft.BusinessID, ConversationID: draft.ConversationID,
		SourceMessageReference: draft.SourceMessageReference, IntentBase: draft.IntentBase,
		DomainContext: draft.DomainContext, Entities: draft.Entities, EvidenceReferences: draft.EvidenceReferences,
		RequestedAction: draft.RequestedAction, ConfidenceValue: draft.ConfidenceValue, ConfidenceBand: draft.ConfidenceBand,
		RequiresHuman: draft.RequiresHuman, MissingInformation: draft.MissingInformation, ReasonCodes: draft.ReasonCodes,
		PolicyVersion: draft.PolicyVersion, ModelReference: draft.ModelReference, SchemaVersion: draft.SchemaVersion,
		Lifecycle: draft.Lifecycle, PolicyDecision: draft.PolicyDecision, CorrelationID: draft.CorrelationID,
		CausationID: draft.CausationID, ExpiresAt: draft.ExpiresAt, CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt,
		ResourceVersion: 1,
	}
	return f.record, nil
}

func (f *fakeDecisionRepository) List(context.Context, ports.AIDecisionFilter) (ports.AIDecisionPage, error) {
	return ports.AIDecisionPage{}, nil
}
func (f *fakeDecisionRepository) Get(context.Context, string, string) (ports.AIDecisionRecord, error) {
	return ports.AIDecisionRecord{}, nil
}
func (f *fakeDecisionRepository) RequestHumanReview(context.Context, ports.HumanReviewPatch) (ports.AIDecisionRecord, error) {
	return ports.AIDecisionRecord{}, nil
}

type fakeReferenceRepository struct {
	record ports.ConversationReferenceRecord
}

func (f fakeReferenceRepository) GetByID(context.Context, string, string) (ports.ConversationReferenceRecord, error) {
	return f.record, nil
}

func (f fakeReferenceRepository) GetCurrentByConversation(context.Context, string, string, string) (ports.ConversationReferenceRecord, error) {
	return f.record, nil
}

type fakeOutboundRepository struct {
	record ports.OutboundMessageRecord
}

func (f *fakeOutboundRepository) CreatePending(_ context.Context, draft ports.OutboundMessageDraft) (ports.OutboundMessageRecord, error) {
	f.record = ports.OutboundMessageRecord{ID: draft.ID, BusinessID: draft.BusinessID, ConversationID: draft.ConversationID, ConversationReferenceID: draft.ConversationReferenceID, ConnectionID: draft.ConnectionID, ProviderRef: draft.ProviderRef, Channel: draft.Channel, Origin: draft.Origin, Direction: "outbound", Transport: draft.Transport, ContentReference: draft.ContentReference, ProviderIdempotencyKey: draft.ProviderIdempotencyKey, Status: "pending", CorrelationID: draft.CorrelationID, CausationID: draft.CausationID}
	return f.record, nil
}
func (f *fakeOutboundRepository) GetByID(context.Context, string, string) (ports.OutboundMessageRecord, error) {
	return f.record, nil
}

type fakeOutboxStore struct {
	record ports.OutboxEntryRecord
	calls  int
}

func (f *fakeOutboxStore) ListClaimable(context.Context, int) ([]ports.OutboxEntryRecord, error) {
	return nil, nil
}

func (f *fakeOutboxStore) Enqueue(_ context.Context, draft ports.OutboxEntryDraft) (ports.OutboxEntryRecord, error) {
	f.calls++
	f.record = ports.OutboxEntryRecord{ID: draft.ID, BusinessID: draft.BusinessID, OutboundMessageID: draft.OutboundMessageID, CommandType: draft.CommandType, DedupeKey: draft.DedupeKey, Status: "pending", AvailableAt: draft.AvailableAt, CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt}
	return f.record, nil
}
func (f *fakeOutboxStore) Get(context.Context, string, string) (ports.OutboxEntryRecord, error) {
	return f.record, nil
}
func (f *fakeOutboxStore) List(context.Context, ports.OutboxFilter) (ports.OutboxPage, error) {
	return ports.OutboxPage{}, nil
}
func (f *fakeOutboxStore) Claim(context.Context, string, ports.OutboxLease) (ports.OutboxClaimResult, error) {
	return ports.OutboxClaimResult{}, nil
}
func (f *fakeOutboxStore) MarkCompleted(context.Context, string, ports.OutboxCompletion) (ports.OutboxEntryRecord, error) {
	return f.record, nil
}
func (f *fakeOutboxStore) MarkRetryableFailure(context.Context, string, ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	return f.record, nil
}
func (f *fakeOutboxStore) MoveToDeadLetter(context.Context, string, ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	return f.record, nil
}
func (f *fakeOutboxStore) Requeue(context.Context, string, time.Time, time.Time) (ports.OutboxEntryRecord, error) {
	return f.record, nil
}

type fakeTransactionManager struct{}

func (fakeTransactionManager) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeConversationRuntimeRepository struct {
	transitions []ports.ConversationLifecycleTransition
}

func (f *fakeConversationRuntimeRepository) List(context.Context, string, string, string, string, *string, int, string) (ports.ConversationPage, error) {
	return ports.ConversationPage{}, nil
}

func (f *fakeConversationRuntimeRepository) Update(context.Context, ports.ConversationUpdate) (ports.ConversationRecord, error) {
	return ports.ConversationRecord{}, nil
}

func (f *fakeConversationRuntimeRepository) AdvanceVersion(context.Context, string, string, int64) (ports.ConversationRecord, error) {
	return ports.ConversationRecord{}, nil
}

func (f *fakeConversationRuntimeRepository) TransitionLifecycle(_ context.Context, transition ports.ConversationLifecycleTransition) (ports.ConversationRecord, error) {
	f.transitions = append(f.transitions, transition)
	return ports.ConversationRecord{
		BusinessID: transition.BusinessID,
		ID:         transition.ConversationID,
	}, nil
}

func TestSafeAutoReplyRuntimeProducesStructuredAnswer(t *testing.T) {
	proposal, err := (SafeAutoReplyRuntime{}).Decide(context.Background(), ports.AIDecisionInput{Text: "مرحبا", PolicyVersion: "auto-reply-v1"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if proposal.RequestedAction != AutoReplyActionAnswer || proposal.PolicyDecision != "allowed" || proposal.SchemaVersion != 1 || proposal.ResponseText == "" {
		t.Fatalf("unexpected structured proposal: %#v", proposal)
	}
}

func TestAutoReplyServicePersistsDecisionAndEnqueuesAnswerAtomically(t *testing.T) {
	decisions := &fakeDecisionRepository{}
	outbound := &fakeOutboundRepository{}
	outbox := &fakeOutboxStore{}
	messages := &fakeMessageRepository{}
	convRepo := &fakeConversationRuntimeRepository{}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "information_request", DomainContext: "safe_ack", Entities: []byte(`{}`), EvidenceReferences: []byte(`[]`), RequestedAction: "answer", ResponseText: "تم استلام رسالتك", ConfidenceBand: "medium", PolicyDecision: "allowed", PolicyVersion: "auto-reply-v1", ModelReference: "test-runtime", SchemaVersion: 1, MissingInformation: []byte(`[]`), ReasonCodes: []byte(`["safe"]`)}},
		decisions,
		fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "socialapi", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: stringPtr("connection-1"), IsCurrent: true, MappingStatus: "active"}},
		outbound,
		outbox,
		fakeTransactionManager{},
	)
	service.MessageRepository = messages
	service.Conversations = convRepo
	service.Now = func() time.Time { return now }
	ids := []string{"decision-1", "outbound-1", "msg-1", "outbox-1"}
	service.NewID = func() string {
		value := ids[0]
		ids = ids[1:]
		return value
	}

	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}, CorrelationID: "not-a-uuid"}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "مرحبا", Channel: "facebook", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Enqueued || result.Action != "answer" || result.Decision.ID != "decision-1" || result.OutboundMessageID != "outbound-1" || result.OutboxEntryID != "outbox-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	text, err := DecodeInlineTextContentReference(outbound.record.ContentReference)
	if err != nil || text != "تم استلام رسالتك" {
		t.Fatalf("content reference text=%q err=%v", text, err)
	}
	if outbound.record.Status != "pending" || outbound.record.Origin != "ai" || outbound.record.Transport != "provider" || outbox.record.CommandType != OutboundSendCommandType || outbox.record.DedupeKey != "auto-reply:inbound-1" || outbox.calls != 1 {
		t.Fatalf("unexpected persistence: outbound=%#v outbox=%#v calls=%d", outbound.record, outbox.record, outbox.calls)
	}
	if messages.recorded.ID == "" || messages.recorded.Origin != "ai" || messages.recorded.Direction != "outbound" {
		t.Fatalf("unexpected communication message: %#v", messages.recorded)
	}
	if messages.recorded.OutboundMessageID == nil || *messages.recorded.OutboundMessageID != "outbound-1" {
		t.Fatalf("expected communication message outbound_message_id outbound-1, got %v", messages.recorded.OutboundMessageID)
	}
	if len(convRepo.transitions) != 1 {
		t.Fatalf("expected 1 conversation transition, got %d", len(convRepo.transitions))
	}
	tr := convRepo.transitions[0]
	if tr.BusinessID != "business-1" || tr.ConversationID != "conversation-1" || tr.State == nil || *tr.State != "waiting_customer" || tr.Ownership == nil || *tr.Ownership != "ai" {
		t.Errorf("unexpected conversation transition: %#v", tr)
	}
}

func TestAutoReplyServiceDoesNotEnqueueNonAnswerDecision(t *testing.T) {
	outbox := &fakeOutboxStore{}
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "handoff_request", RequestedAction: "ask_clarification", ConfidenceBand: "high", PolicyDecision: "requires_approval", PolicyVersion: "auto-reply-v1", SchemaVersion: 1, Entities: []byte(`{}`), EvidenceReferences: []byte(`[]`), MissingInformation: []byte(`[]`), ReasonCodes: []byte(`[]`)}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{},
		&fakeOutboundRepository{},
		outbox,
		fakeTransactionManager{},
	)
	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "أحتاج مساعدة", Channel: "facebook", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Enqueued || outbox.calls != 0 || result.Action != "ask_clarification" {
		t.Fatalf("unexpected non-answer result=%#v outbox_calls=%d", result, outbox.calls)
	}
}

func TestAutoReplyServiceEnqueuesAllowedClarification(t *testing.T) {
	outbox := &fakeOutboxStore{}
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "information_request", RequestedAction: "ask_clarification", ResponseText: "أي باقة تقصد؟", ConfidenceBand: "medium", PolicyDecision: "allowed", PolicyVersion: "auto-reply-v1", SchemaVersion: 1, Entities: []byte(`{}`), EvidenceReferences: []byte(`[]`), MissingInformation: []byte(`[]`), ReasonCodes: []byte(`[]`)}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "socialapi", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: stringPtr("connection-1"), IsCurrent: true, MappingStatus: "active"}},
		&fakeOutboundRepository{},
		outbox,
		fakeTransactionManager{},
	)
	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "كم سعرها؟", Channel: "whatsapp", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Enqueued || outbox.calls != 1 || result.Action != "ask_clarification" || result.OutboundMessageID == "" {
		t.Fatalf("allowed clarification must be enqueued, got %#v outbox_calls=%d", result, outbox.calls)
	}
}

func TestAutoReplyServiceRejectsEmptyClarificationText(t *testing.T) {
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "information_request", RequestedAction: "ask_clarification", ConfidenceBand: "medium", PolicyDecision: "allowed", PolicyVersion: "auto-reply-v1", SchemaVersion: 1, Entities: []byte(`{}`), EvidenceReferences: []byte(`[]`), MissingInformation: []byte(`[]`), ReasonCodes: []byte(`[]`)}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "socialapi", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: stringPtr("connection-1"), IsCurrent: true, MappingStatus: "active"}},
		&fakeOutboundRepository{},
		&fakeOutboxStore{},
		fakeTransactionManager{},
	)
	if _, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "كم سعرها؟", Channel: "whatsapp", ProviderRef: "socialapi"}); err == nil {
		t.Fatal("empty clarification text must fail validation")
	}
}

type stubAIContextBuilder struct {
	context ports.AIContext
}

func (s stubAIContextBuilder) Build(context.Context, ports.ContextBuildInput) (ports.AIContext, error) {
	return s.context, nil
}

type stubStateRepository struct {
	record ports.ConversationStateRecord
	saved  *ports.ConversationStateRecord
}

func (s *stubStateRepository) Get(context.Context, string, string) (ports.ConversationStateRecord, error) {
	return s.record, nil
}

func (s *stubStateRepository) UpsertValidated(_ context.Context, record ports.ConversationStateRecord) (ports.ConversationStateRecord, error) {
	s.saved = &record
	return record, nil
}

// End-to-end regression for the blocked greeting: stored focus on Basic +
// general answer citing both offers and the parent catalog must be enqueued,
// with the stored focus preserved (NO_REFERENCE changes nothing).
func TestAutoReplyServiceEnqueuesGeneralAnswerAfterFocus(t *testing.T) {
	states := &stubStateRepository{record: ports.ConversationStateRecord{
		BusinessID: "business-1", ConversationID: "conversation-1",
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: stringPtr("item-basic")},
	}}
	outbox := &fakeOutboxStore{}
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "information_request", RequestedAction: "answer", ResponseText: "الباقات: الأساسية والاحترافية", ConfidenceBand: "high", PolicyDecision: "allowed", PolicyVersion: "auto-reply-v1", SchemaVersion: 1, Entities: []byte(`{}`), EvidenceReferences: []byte(`["offer-basic","offer-pro","cat-1"]`), MissingInformation: []byte(`[]`), ReasonCodes: []byte(`[]`), StateProposal: &ports.AIStateProposal{Kind: "NO_REFERENCE"}}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "socialapi", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: stringPtr("connection-1"), IsCurrent: true, MappingStatus: "active"}},
		&fakeOutboundRepository{},
		outbox,
		fakeTransactionManager{},
	)
	service.ContextBuilder = stubAIContextBuilder{context: ports.AIContext{
		CatalogEvidence: []ports.AICatalogEvidence{
			{Reference: "item-basic", CatalogReference: "cat-1"},
			{Reference: "item-pro", CatalogReference: "cat-1"},
		},
		OfferEvidence: []ports.AIOfferEvidence{
			{Reference: "offer-basic", CatalogItemReference: "item-basic"},
			{Reference: "offer-pro", CatalogItemReference: "item-pro"},
		},
		ConversationState: &states.record,
	}}
	service.StateRepository = states
	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "السلام عليكم، ايش الباقات الي عندكم؟", Channel: "whatsapp", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Enqueued || outbox.calls != 1 || result.Action != "answer" {
		t.Fatalf("general answer must be enqueued, got %#v outbox_calls=%d", result, outbox.calls)
	}
}

func TestAutoReplyServiceHandlesHumanOwnership(t *testing.T) {
	builder := NewAutoReplyContextBuilder(
		contextBusinessRepository{record: ports.BusinessRecord{ID: "business-1"}},
		contextConversationRepository{record: ports.ConversationRecord{ID: "conversation-1", BusinessID: "business-1", CustomerID: "customer-1", Ownership: "human"}},
		contextCustomerRepository{record: ports.CustomerRecord{ID: "customer-1", BusinessID: "business-1"}},
		contextCatalogRepository{},
		contextMessageRepository{},
	)

	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "greeting", RequestedAction: AutoReplyActionAnswer, ConfidenceBand: "high", SchemaVersion: 1, PolicyDecision: "allowed", ResponseText: "Hello"}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{},
		&fakeOutboundRepository{},
		&fakeOutboxStore{},
		fakeTransactionManager{},
	)
	service.ContextBuilder = builder

	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta:                   commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}},
		ConversationID:         "conversation-1",
		SourceMessageReference: "msg-1",
		Text:                   "hello",
		Channel:                "whatsapp",
		ProviderRef:            "provider-1",
	})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if result.Action != "no_action" || result.Enqueued {
		t.Fatalf("expected no_action for human owned conversation, got %s (enqueued=%v)", result.Action, result.Enqueued)
	}
}

func TestAutoReplyServiceHandlesWaitingHuman(t *testing.T) {
	builder := NewAutoReplyContextBuilder(
		contextBusinessRepository{record: ports.BusinessRecord{ID: "business-1"}},
		contextConversationRepository{record: ports.ConversationRecord{ID: "conversation-1", BusinessID: "business-1", CustomerID: "customer-1", State: "waiting_human"}},
		contextCustomerRepository{record: ports.CustomerRecord{ID: "customer-1", BusinessID: "business-1"}},
		contextCatalogRepository{},
		contextMessageRepository{},
	)

	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "greeting", RequestedAction: AutoReplyActionAnswer, ConfidenceBand: "high", SchemaVersion: 1, PolicyDecision: "allowed", ResponseText: "Hello"}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{},
		&fakeOutboundRepository{},
		&fakeOutboxStore{},
		fakeTransactionManager{},
	)
	service.ContextBuilder = builder

	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta:                   commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}},
		ConversationID:         "conversation-1",
		SourceMessageReference: "msg-1",
		Text:                   "hello",
		Channel:                "whatsapp",
		ProviderRef:            "provider-1",
	})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if result.Action != "no_action" || result.Enqueued {
		t.Fatalf("expected no_action for waiting_human conversation, got %s (enqueued=%v)", result.Action, result.Enqueued)
	}
}

func TestAutoReplyServiceHandlesDraftOrder(t *testing.T) {
	outbox := &fakeOutboxStore{}
	convRepo := &fakeConversationRuntimeRepository{}
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "purchase", RequestedAction: "draft_order", ConfidenceBand: "high", PolicyDecision: "requires_approval", PolicyVersion: "auto-reply-v1", SchemaVersion: 1, Entities: []byte(`{"items":[{"id":"item-1"}]}`), EvidenceReferences: []byte(`[]`), MissingInformation: []byte(`[]`), ReasonCodes: []byte(`[]`)}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{},
		&fakeOutboundRepository{},
		outbox,
		fakeTransactionManager{},
	)
	service.Conversations = convRepo
	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "أريد شراء هذا", Channel: "whatsapp", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Enqueued || outbox.calls != 0 || result.Action != "draft_order" {
		t.Fatalf("unexpected result for draft_order=%#v outbox_calls=%d", result, outbox.calls)
	}
	if len(convRepo.transitions) != 1 {
		t.Fatalf("expected 1 conversation transition, got %d", len(convRepo.transitions))
	}
	if convRepo.transitions[0].State == nil || *convRepo.transitions[0].State != "waiting_human" {
		t.Errorf("expected state waiting_human for draft_order, got %#v", convRepo.transitions[0])
	}
}

func TestAutoReplyServiceHandlesDraftLead(t *testing.T) {
	outbox := &fakeOutboxStore{}
	convRepo := &fakeConversationRuntimeRepository{}
	service := NewAutoReplyService(
		fakeAIRuntime{proposal: ports.AIDecisionProposal{IntentBase: "inquiry", RequestedAction: "draft_lead", ConfidenceBand: "high", PolicyDecision: "requires_approval", PolicyVersion: "auto-reply-v1", SchemaVersion: 1, Entities: []byte(`{"name":"Ammar"}`), EvidenceReferences: []byte(`[]`), MissingInformation: []byte(`[]`), ReasonCodes: []byte(`[]`)}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{},
		&fakeOutboundRepository{},
		outbox,
		fakeTransactionManager{},
	)
	service.Conversations = convRepo
	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-1", Text: "مهتم بالخدمة", Channel: "whatsapp", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Enqueued || outbox.calls != 0 || result.Action != "draft_lead" {
		t.Fatalf("unexpected result for draft_lead=%#v outbox_calls=%d", result, outbox.calls)
	}
	if len(convRepo.transitions) != 1 {
		t.Fatalf("expected 1 conversation transition, got %d", len(convRepo.transitions))
	}
	if convRepo.transitions[0].State == nil || *convRepo.transitions[0].State != "waiting_human" {
		t.Errorf("expected state waiting_human for draft_lead, got %#v", convRepo.transitions[0])
	}
}
