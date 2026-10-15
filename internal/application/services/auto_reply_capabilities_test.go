package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type capabilityRunRepo struct {
	ports.AIRunRepository
	run         ports.AIRunRecord
	transitions []string
	failStatus  string
	batches     map[string]ports.AICatalogBatchRecord
}

func (r *capabilityRunRepo) CreateRun(_ context.Context, run ports.AIRunRecord) (ports.AIRunRecord, error) {
	r.run = run
	return run, nil
}
func (r *capabilityRunRepo) GetByIdempotencyKey(context.Context, string, string) (ports.AIRunRecord, error) {
	return ports.AIRunRecord{}, errors.New("not found")
}
func (r *capabilityRunRepo) GetRun(context.Context, string, string) (ports.AIRunRecord, error) {
	return r.run, nil
}
func (r *capabilityRunRepo) UpdateRunStatus(_ context.Context, _, _ string, patch ports.AIRunStatusPatch) (ports.AIRunRecord, error) {
	if patch.Status == r.failStatus {
		return ports.AIRunRecord{}, errors.New("injected lifecycle failure")
	}
	r.run.Status = patch.Status
	r.transitions = append(r.transitions, patch.Status)
	return r.run, nil
}
func (r *capabilityRunRepo) CreateCatalogBatch(_ context.Context, batch ports.AICatalogBatchRecord) (ports.AICatalogBatchRecord, error) {
	if r.batches == nil {
		r.batches = map[string]ports.AICatalogBatchRecord{}
	}
	r.batches[batch.ID] = batch
	return batch, nil
}
func (r *capabilityRunRepo) UpdateCatalogBatch(_ context.Context, id string, patch ports.AICatalogBatchPatch) (ports.AICatalogBatchRecord, error) {
	batch := r.batches[id]
	batch.Status = patch.Status
	r.batches[id] = batch
	return batch, nil
}
func (*capabilityRunRepo) RecordUsage(_ context.Context, usage ports.AIUsageTelemetryRecord) (ports.AIUsageTelemetryRecord, error) {
	return usage, nil
}

func (*capabilityRunRepo) ListToolCalls(context.Context, string) ([]ports.AIToolCallRecord, error) {
	return nil, nil
}

type capabilityCatalogRepo struct {
	reads         int
	revisionReads int
	changed       bool
}

func (*capabilityCatalogRepo) GetManifest(context.Context, string) (ports.CatalogAIManifest, error) {
	return ports.CatalogAIManifest{}, nil
}
func (r *capabilityCatalogRepo) GetRevision(context.Context, string) (string, error) {
	r.revisionReads++
	if r.changed && r.revisionReads > 1 {
		return "changed", nil
	}
	return "stable", nil
}
func (r *capabilityCatalogRepo) ListProjectionPage(_ context.Context, request ports.CatalogAIProjectionRequest) (ports.CatalogAIProjectionPage, error) {
	if request.BusinessID != "business-1" || request.CatalogID != "" {
		return ports.CatalogAIProjectionPage{}, errors.New("wrong tenant or catalog filter")
	}
	r.reads++
	start := 1
	if request.Cursor == "page2" {
		start = 3
	}
	schemaVersion := 1
	page := ports.CatalogAIProjectionPage{Catalogs: []ports.CatalogRecord{{ID: "catalog", BusinessID: "business-1", Name: "Main", Status: "active"}}, HasMore: start == 1, NextCursor: "page2"}
	for i := start; i < start+2; i++ {
		id := fmt.Sprintf("item-%d", i)
		page.Items = append(page.Items, ports.CatalogAIProjectionBundle{
			Item:            ports.CatalogItemRecord{ID: id, BusinessID: "business-1", CatalogID: "catalog", Name: id, Status: "active", ItemType: "service", AttributeSchemaID: stringPtr("schema"), AttributeSchemaVersion: &schemaVersion},
			AttributeSchema: &ports.AttributeSchemaRecord{ID: "schema", BusinessID: "business-1", Name: "Schema", Version: 1},
			Variants:        []ports.VariantRecord{{ID: "variant-" + id, BusinessID: "business-1", CatalogItemID: id, Status: "active"}},
			Offers:          []ports.OfferRecord{{ID: "offer-" + id, BusinessID: "business-1", CatalogItemID: id, VariantID: stringPtr("variant-" + id), Status: "active", PricingMode: "fixed"}},
		})
	}
	return page, nil
}

type capabilityBatchModel struct {
	seen            []string
	failBatch       int
	finalCalls      int
	finalCapability ports.CustomerSalesCapability
}

func (*capabilityBatchModel) CountBatchTokens(_ context.Context, input BatchEvaluationInput) (int, error) {
	return 100 + 100*len(input.Batch.Items), nil
}
func (*capabilityBatchModel) CountFinalTokens(context.Context, FinalEvaluationInput, string) (int, error) {
	return 200, nil
}
func (m *capabilityBatchModel) EvaluateBatch(_ context.Context, input BatchEvaluationInput) (ports.CatalogBatchResult, error) {
	if input.BatchNumber == m.failBatch {
		return ports.CatalogBatchResult{}, errors.New("injected Gemini failure")
	}
	candidates := []ports.CatalogBatchCandidate{}
	for _, item := range input.Batch.Items {
		if len(item.Variants) != 1 || len(item.Offers) != 1 || len(input.Batch.AttributeSchemas) != 1 || len(input.Batch.Catalogs) != 1 {
			return ports.CatalogBatchResult{}, errors.New("lost catalog relationships")
		}
		m.seen = append(m.seen, item.ID)
		if item.ID == "item-4" {
			candidates = append(candidates, ports.CatalogBatchCandidate{ItemID: item.ID, VariantIDs: []string{item.Variants[0].ID}, OfferIDs: []string{item.Offers[0].ID}})
		}
	}
	return ports.CatalogBatchResult{BatchNumber: input.BatchNumber, Candidates: candidates}, nil
}
func (m *capabilityBatchModel) FinalEvaluateWithDetails(_ context.Context, input FinalEvaluationInput, _ string) (ports.CapabilityDecision, ports.CustomerSalesUsageTelemetry, error) {
	m.finalCalls++
	if !reflect.DeepEqual(m.seen, []string{"item-1", "item-2", "item-3", "item-4"}) {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, errors.New("final ran before complete catalog coverage")
	}
	capability := m.finalCapability
	if capability == "" {
		capability = ports.CustomerSalesCapabilityRespondWithoutAction
	}
	return ports.CapabilityDecision{Capability: capability, ResponseText: "الخيار المناسب متوفر", Selected: []ports.SelectedReference{{ItemID: "item-4", VariantID: stringPtr("variant-item-4"), OfferID: stringPtr("offer-item-4")}}}, ports.CustomerSalesUsageTelemetry{}, nil
}

func capabilityService() (AutoReplyService, *FakeCustomerSalesDecisionPort, *fakeOutboxStore, *capabilityRunRepo) {
	runtime := NewFakeCustomerSalesDecisionPort("مرحبا")
	outbox := &fakeOutboxStore{}
	service := NewAutoReplyService(runtime, &fakeDecisionRepository{}, fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference", BusinessID: "business-1", ConversationID: "conversation-1", ProviderRef: "socialapi", ResourceID: "external", ConnectionID: stringPtr("connection"), IsCurrent: true, MappingStatus: "active"}}, &fakeOutboundRepository{}, outbox, fakeTransactionManager{})
	service.Validation = allowAllValidationPipeline()
	service.Validation.ReferenceValidator = capabilityReferenceValidator{}
	service.AIUsage = &stubAIUsageRepo{}
	service.Subscriptions = &stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub", BusinessID: "business-1", Status: "ACTIVE"}}}
	service.AIPricing = &stubPricingRepoAlwaysFail{}
	sequence := 0
	service.NewID = func() string { sequence++; return fmt.Sprintf("id-%d", sequence) }
	runRepo := &capabilityRunRepo{}
	service.RunRepository = runRepo
	return service, runtime, outbox, runRepo
}
func capabilityCommand() commands.AutoReplyCommand {
	return commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound", Text: "شيء مناسب لعائلة", Channel: "whatsapp", ProviderRef: "socialapi"}
}

func TestAutoReplyCapabilityFullCatalogCoversAllPagesBeforeFinal(t *testing.T) {
	service, runtime, outbox, runRepo := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog}
	catalog := &capabilityCatalogRepo{}
	model := &capabilityBatchModel{}
	service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Evaluator: model, RunRepo: runRepo, TokenBudget: 250, Now: service.now, NewID: service.NewID}
	service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Enqueued || outbox.calls != 1 || catalog.reads != 2 || model.finalCalls != 1 || len(runRepo.batches) != 4 {
		t.Fatalf("incomplete full catalog result=%+v pages=%d final=%d", result, catalog.reads, model.finalCalls)
	}
	want := []string{ports.AIRunStatusContextBuilt, ports.AIRunStatusRunning, ports.AIRunStatusValidating, ports.AIRunStatusAuthorized, ports.AIRunStatusExecuting, ports.AIRunStatusCompleted}
	if !reflect.DeepEqual(runRepo.transitions, want) {
		t.Fatalf("incorrect lifecycle %v", runRepo.transitions)
	}
}

func TestAutoReplyCatalogFailuresNeverFinalizeOrSend(t *testing.T) {
	for _, mode := range []string{"batch failure", "revision changed"} {
		t.Run(mode, func(t *testing.T) {
			service, runtime, outbox, runRepo := capabilityService()
			runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog}
			catalog := &capabilityCatalogRepo{changed: mode == "revision changed"}
			model := &capabilityBatchModel{}
			if mode == "batch failure" {
				model.failBatch = 3
			}
			service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Evaluator: model, RunRepo: runRepo, TokenBudget: 250, Now: service.now, NewID: service.NewID}
			service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
			result, err := service.Handle(context.Background(), capabilityCommand())
			if err == nil || result.Enqueued || outbox.calls != 0 || model.finalCalls != 0 {
				t.Fatalf("incomplete catalog must not produce final decision: %+v err=%v", result, err)
			}
		})
	}
}

func TestAutoReplyGreetingDoesNotLoadCatalog(t *testing.T) {
	service, _, outbox, runRepo := capabilityService()
	catalog := &capabilityCatalogRepo{}
	service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Evaluator: &capabilityBatchModel{}, RunRepo: runRepo, TokenBudget: 250, Now: service.now, NewID: service.NewID}
	command := capabilityCommand()
	command.Text = "السلام عليكم"
	result, err := service.Handle(context.Background(), command)
	if err != nil || !result.Enqueued || outbox.calls != 1 || catalog.reads != 0 || catalog.revisionReads != 0 {
		t.Fatalf("greeting unnecessarily loaded catalog: %+v err=%v", result, err)
	}
}

func TestAutoReplyFailedAuthorizationTransitionBlocksOutbound(t *testing.T) {
	for _, status := range []string{ports.AIRunStatusValidating, ports.AIRunStatusAuthorized, ports.AIRunStatusExecuting} {
		t.Run(status, func(t *testing.T) {
			service, _, outbox, runRepo := capabilityService()
			runRepo.failStatus = status
			result, err := service.Handle(context.Background(), capabilityCommand())
			if err == nil || result.Enqueued || outbox.calls != 0 {
				t.Fatalf("transition failure must prevent execution: %+v err=%v", result, err)
			}
		})
	}
}

func TestAutoReplyHandoffKeepsHumanStateAndAction(t *testing.T) {
	service, runtime, outbox, _ := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityHandoff}
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityHandoff, ResponseText: "سأحوّل طلبك للفريق"}
	conversations := &fakeConversationRuntimeRepository{}
	service.Conversations = conversations
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || !result.Enqueued || result.Action != "human_request" || !result.Decision.RequiresHuman || outbox.calls != 1 {
		t.Fatalf("handoff %+v err=%v", result, err)
	}
	if len(conversations.transitions) != 1 || *conversations.transitions[0].State != "waiting_human" {
		t.Fatalf("handoff lost waiting_human state: %+v", conversations.transitions)
	}
}

type capabilityReferenceValidator struct{ allowAllReferenceValidator }

func (capabilityReferenceValidator) ValidateSelection(_ context.Context, _ string, ref ports.SelectedReference, evidence ports.CatalogAIEvidenceSet) error {
	if !evidence.ContainsSelection(ref) {
		return errors.New("selection was not exposed to the model")
	}
	return nil
}
func TestInvalidSelectionDoesNotTriggerCatalogOrExpandEvidence(t *testing.T) {
	service, runtime, outbox, runRepo := capabilityService()
	runtime.Decision.Selected = []ports.SelectedReference{{ItemID: "invented"}}
	service.Validation.ReferenceValidator = capabilityReferenceValidator{}
	catalog := &capabilityCatalogRepo{}
	service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Evaluator: &capabilityBatchModel{}, RunRepo: runRepo, TokenBudget: 250, Now: service.now, NewID: service.NewID}
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || result.Enqueued || outbox.calls != 0 || catalog.reads != 0 {
		t.Fatalf("invalid references expanded trust boundary: %+v err=%v", result, err)
	}
}
func TestUnavailableCatalogCapabilityFailsWithoutSending(t *testing.T) {
	service, runtime, outbox, _ := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog}
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err == nil || result.Enqueued || outbox.calls != 0 {
		t.Fatalf("unavailable catalog capability executed: %+v err=%v", result, err)
	}
	for _, capability := range runtime.LastInput.Capabilities {
		if capability == ports.CustomerSalesCapabilityFullCatalog {
			t.Fatal("unavailable catalog tool was offered")
		}
	}
}

func TestHumanWaitingTurnStopsBeforeModelAndEndsRun(t *testing.T) {
	service, runtime, outbox, runRepo := capabilityService()
	service.CustomerSalesContextBuilder = stubCustomerSalesContextBuilder{context: ports.CustomerSalesContext{Conversation: ports.CustomerSalesContextConversation{State: "waiting_human"}}}
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || result.Enqueued || outbox.calls != 0 || runtime.LastInput.Request.Text != "" || runRepo.run.Status != ports.AIRunStatusCancelled {
		t.Fatalf("waiting-human turn must stop and end its trace: %+v status=%s err=%v", result, runRepo.run.Status, err)
	}
}

type recordingCapabilityPolicy struct {
	got     ports.CapabilityDecision
	outcome string
}

func (p *recordingCapabilityPolicy) Evaluate(_ context.Context, decision ports.CapabilityDecision, _ *ports.CustomerSalesContext) ports.CustomerSalesPolicyDecision {
	p.got = decision
	return ports.CustomerSalesPolicyDecision{Decision: p.outcome}
}

func TestFullCatalogPreservesCapabilityThroughPolicyExecutionAndAudit(t *testing.T) {
	for _, capability := range ports.CustomerSalesCapabilities(false) {
		t.Run(string(capability), func(t *testing.T) {
			service, runtime, outbox, runRepo := capabilityService()
			runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog}
			model := &capabilityBatchModel{finalCapability: capability}
			service.CatalogBatch = &CatalogBatchController{CatalogAI: &capabilityCatalogRepo{}, Evaluator: model, RunRepo: runRepo, TokenBudget: 250, Now: service.now, NewID: service.NewID}
			service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
			policy := &recordingCapabilityPolicy{outcome: "allowed"}
			service.Validation.CustomerSalesPolicy = policy
			conversations := &fakeConversationRuntimeRepository{}
			service.Conversations = conversations
			result, err := service.Handle(context.Background(), capabilityCommand())
			if err != nil || !result.Enqueued || outbox.calls != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if policy.got.Capability != capability || len(policy.got.Selected) != 1 || policy.got.Selected[0].ItemID != "item-4" {
				t.Fatalf("policy lost final decision: %+v", policy.got)
			}
			audit := service.DecisionRepository.(*fakeDecisionRepository).record
			if audit.RequestedCapability == nil || *audit.RequestedCapability != string(capability) || audit.EffectiveCapability == nil || *audit.EffectiveCapability != string(capability) || audit.IntentBase != "" || result.Capability != string(capability) {
				t.Fatalf("capability lost in audit or result: %+v %+v", audit, result)
			}
			definition, _ := ports.CustomerSalesCapabilityDefinitionFor(capability)
			if len(conversations.transitions) != 1 || conversations.transitions[0].State == nil || *conversations.transitions[0].State != definition.ConversationState || audit.RequiresHuman != definition.RequiresHuman {
				t.Fatalf("wrong executor effects: %+v", conversations.transitions)
			}
			if definition.ConversationOwnership == "" && conversations.transitions[0].Ownership != nil {
				t.Fatal("handoff forged human ownership")
			}
		})
	}
}

func TestCatalogCapabilityRequiresCompleteLocalWiring(t *testing.T) {
	cases := []struct {
		name   string
		remove func(*AutoReplyService)
	}{
		{"controller", func(s *AutoReplyService) { s.CatalogBatch = nil }},
		{"catalog repo", func(s *AutoReplyService) { s.CatalogBatch.CatalogAI = nil }},
		{"batch model", func(s *AutoReplyService) { s.CatalogBatch.Evaluator = nil }},
		{"batch trace", func(s *AutoReplyService) { s.CatalogBatch.RunRepo = nil }},
		{"runtime trace", func(s *AutoReplyService) { s.RunRepository = nil }},
		{"clock", func(s *AutoReplyService) { s.CatalogBatch.Now = nil }},
		{"IDs", func(s *AutoReplyService) { s.CatalogBatch.NewID = nil }},
		{"entity contract", func(s *AutoReplyService) { s.EntityContractPayload = nil }},
		{"invalid contract", func(s *AutoReplyService) { s.EntityContractPayload = []byte(`{"entity_contract":{}}`) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, runtime, _, runRepo := capabilityService()
			catalog := &capabilityCatalogRepo{}
			service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Evaluator: &capabilityBatchModel{}, RunRepo: runRepo, Now: service.now, NewID: service.NewID}
			service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
			tc.remove(&service)
			if _, err := service.Handle(context.Background(), capabilityCommand()); err != nil {
				t.Fatal(err)
			}
			for _, capability := range runtime.LastInput.Capabilities {
				if capability == ports.CustomerSalesCapabilityFullCatalog {
					t.Fatal("incomplete retrieval offered")
				}
			}
			if catalog.reads != 0 {
				t.Fatal("read catalog without an explicit ready request")
			}
		})
	}
	controller := &CatalogBatchController{CatalogAI: &capabilityCatalogRepo{}, Evaluator: &plainTokenBatchModel{}, RunRepo: &capabilityRunRepo{}, Now: func() time.Time { return time.Now() }, NewID: func() string { return "id" }}
	if controller.Ready() {
		t.Fatal("missing token counter considered ready")
	}
	controller.TokenCounter = constantTokenCounter{}
	if !controller.Ready() {
		t.Fatal("compatibility token counter not accepted")
	}
}

type plainTokenBatchModel struct{ CatalogEvaluationClient }
type constantTokenCounter struct{}

func (constantTokenCounter) CountTokens(context.Context, any) (int, error) { return 100, nil }

func TestFullCatalogCapabilityPolicyGatePreventsExecution(t *testing.T) {
	for _, outcome := range []string{"denied", "requires_approval"} {
		t.Run(outcome, func(t *testing.T) {
			service, runtime, outbox, runRepo := capabilityService()
			runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog}
			service.CatalogBatch = &CatalogBatchController{CatalogAI: &capabilityCatalogRepo{}, Evaluator: &capabilityBatchModel{finalCapability: ports.CustomerSalesCapabilityHandoff}, RunRepo: runRepo, TokenBudget: 250, Now: service.now, NewID: service.NewID}
			service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
			policy := &recordingCapabilityPolicy{outcome: outcome}
			service.Validation.CustomerSalesPolicy = policy
			result, err := service.Handle(context.Background(), capabilityCommand())
			if err != nil || result.Enqueued || outbox.calls != 0 {
				t.Fatalf("policy bypass: %+v err=%v", result, err)
			}
			audit := service.DecisionRepository.(*fakeDecisionRepository).record
			if policy.got.Capability != ports.CustomerSalesCapabilityHandoff || audit.RequestedCapability == nil || *audit.RequestedCapability != "handoff_to_human" || audit.PolicyDecision == nil || *audit.PolicyDecision != outcome {
				t.Fatalf("blocked capability lost in audit: %+v", audit)
			}
			for _, status := range runRepo.transitions {
				if status == ports.AIRunStatusAuthorized || status == ports.AIRunStatusExecuting {
					t.Fatal("blocked policy entered execution")
				}
			}
		})
	}
}
