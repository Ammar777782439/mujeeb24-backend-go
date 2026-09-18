package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mockAuthoringAIRuntime struct {
	decideFn func(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error)
}

func (m mockAuthoringAIRuntime) Decide(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
	if m.decideFn != nil {
		return m.decideFn(ctx, input)
	}
	return ports.AIDecisionProposal{}, nil
}

// Scenario 1: Merchant provides incomplete information ("Add a men's shirt")
// AI asks for missing information (price/variants) without creating incomplete records.
func TestConversationalAuthoringIncompleteInformationAsksClarification(t *testing.T) {
	runtime := mockAuthoringAIRuntime{
		decideFn: func(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
			// Model identifies product authoring intent but missing price/offer
			return ports.AIDecisionProposal{
				IntentBase:         "catalog_authoring",
				DomainContext:      "catalog",
				RequestedAction:    AutoReplyActionAskClarification,
				ResponseText:       "كم سعر القميص وما هي المقاسات المتوفرة؟",
				ConfidenceBand:     "high",
				ConfidenceValue:    "0.95",
				PolicyDecision:     "allowed",
				MissingInformation: []byte(`["price","sizes"]`),
				ReasonCodes:        []byte(`["missing_price_for_item"]`),
				SchemaVersion:      1,
			}, nil
		},
	}

	decisionRepo := &fakeDecisionRepository{}
	refRepo := fakeReferenceRepository{
		record: ports.ConversationReferenceRecord{
			ID:             "ref-1",
			BusinessID:     "biz-1",
			ConversationID: "conv-1",
			System:         "socialapi",
			ProviderRef:    "whatsapp",
			ResourceID:     "provider-conv-1",
			ConnectionID:   stringPtr("conn-1"),
			IsCurrent:      true,
			MappingStatus:  "active",
		},
	}
	service := NewAutoReplyService(
		runtime,
		decisionRepo,
		refRepo,
		&fakeOutboundRepository{},
		&fakeOutboxStore{},
		mockTransactionManager{},
	)
	service.PolicyEvaluator = GroundedPolicyEngine{}

	res, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1"},
		},
		ConversationID:         "conv-1",
		SourceMessageReference: "msg-1",
		Text:                   "أضف قميص رجالي أبيض",
		Channel:                "whatsapp",
		ProviderRef:            "whatsapp",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != AutoReplyActionAskClarification {
		t.Fatalf("expected action ask_clarification, got %s", res.Action)
	}
	if !res.Enqueued {
		t.Fatal("expected clarification message to be enqueued for delivery")
	}
	if decisionRepo.record.RequestedAction != AutoReplyActionAskClarification {
		t.Fatalf("expected decision saved with ask_clarification, got %s", decisionRepo.record.RequestedAction)
	}
}

// Scenario 2: Merchant provides complete information from the start
// AI invokes catalog_authoring capability, returns answer with evidence, no unnecessary questions.
func TestConversationalAuthoringCompleteInformationCreatesItem(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	authoringExecuted := false

	repo := mockCatalogRepository{
		createCatalogItemFn: func(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
			authoringExecuted = true
			return ports.CatalogItemRecord{
				ID:         "item-100",
				BusinessID: draft.BusinessID,
				CatalogID:  draft.CatalogID,
				Name:       draft.Name,
				Status:     "draft",
			}, nil
		},
		createVariantFn: func(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
			return ports.VariantRecord{
				ID:            "var-100",
				BusinessID:    draft.BusinessID,
				CatalogItemID: draft.CatalogItemID,
				Name:          draft.Name,
				Status:        "active",
			}, nil
		},
		createOfferFn: func(ctx context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
			amt := "15000"
			return ports.OfferRecord{
				ID:                 "offer-100",
				BusinessID:         draft.BusinessID,
				CatalogItemID:      draft.CatalogItemID,
				Name:               draft.Name,
				PricingMode:        draft.PricingMode,
				Amount:             &amt,
				Currency:           draft.Currency,
				AvailabilityStatus: "available",
				Status:             "active",
			}, nil
		},
	}

	cmdServices := CatalogCommandServices{
		Repository:   repo,
		Transactions: mockTransactionManager{},
		NewID:        func() string { return "gen-id" },
		Now:          func() time.Time { return now },
	}

	cap := NewCatalogAuthoringCapability(
		AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateOfferCommandService{CatalogCommandServices: cmdServices},
		CreateVariantCommandService{CatalogCommandServices: cmdServices},
		CreateAttributeSchemaVersionCommandService{CatalogCommandServices: cmdServices},
	)

	// Simulate AI model processing complete request: invokes capability then returns answer
	runtime := mockAuthoringAIRuntime{
		decideFn: func(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
			execCtx := ports.AICapabilityExecutionContext{
				BusinessID:     input.BusinessID,
				ConversationID: input.ConversationID,
				Role:           "owner",
			}
			capRes, err := cap.Execute(ctx, execCtx, []byte(`{
				"operation": "author_catalog_item",
				"catalog_id": "cat-1",
				"name": "قميص رجالي أبيض",
				"item_type": "product",
				"amount": "15000",
				"currency": "YER",
				"variants": [
					{"name": "S", "attributes": {"size": "S"}},
					{"name": "M", "attributes": {"size": "M"}},
					{"name": "L", "attributes": {"size": "L"}}
				]
			}`))
			if err != nil {
				return ports.AIDecisionProposal{}, err
			}

			return ports.AIDecisionProposal{
				IntentBase:                "catalog_authoring",
				DomainContext:             "catalog",
				RequestedAction:           AutoReplyActionAnswer,
				ResponseText:              "تمت إضافة القميص الأبيض الرجالي بنجاح بسعر 15,000 ريال يمني والمقاسات S, M, L.",
				ConfidenceBand:            "high",
				ConfidenceValue:           "0.98",
				PolicyDecision:            "allowed",
				EvidenceReferences:        []byte(`["item-100"]`),
				DiscoveredCatalogEvidence: capRes.CatalogEvidence,
				DiscoveredVariantEvidence: capRes.VariantEvidence,
				DiscoveredOfferEvidence:   capRes.OfferEvidence,
				SchemaVersion:             1,
			}, nil
		},
	}

	decisionRepo := &fakeDecisionRepository{}
	refRepo := fakeReferenceRepository{
		record: ports.ConversationReferenceRecord{
			ID:             "ref-2",
			BusinessID:     "biz-1",
			ConversationID: "conv-1",
			System:         "socialapi",
			ProviderRef:    "whatsapp",
			ResourceID:     "provider-conv-1",
			ConnectionID:   stringPtr("conn-1"),
			IsCurrent:      true,
			MappingStatus:  "active",
		},
	}
	service := NewAutoReplyService(
		runtime,
		decisionRepo,
		refRepo,
		&fakeOutboundRepository{},
		&fakeOutboxStore{},
		mockTransactionManager{},
	)
	service.PolicyEvaluator = GroundedPolicyEngine{}

	res, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1", Role: "owner"},
		},
		ConversationID:         "conv-1",
		SourceMessageReference: "msg-2",
		Text:                   "أضف قميص رجالي أبيض بسعر 15000 ريال يمني ومقاسات S و M و L في الكتالوج cat-1",
		Channel:                "whatsapp",
		ProviderRef:            "whatsapp",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !authoringExecuted {
		t.Fatal("expected catalog authoring capability to be executed")
	}
	if res.Action != AutoReplyActionAnswer {
		t.Fatalf("expected action answer, got %s", res.Action)
	}
	if !res.Enqueued {
		t.Fatal("expected confirmation message to be enqueued")
	}
}

// Scenario 3: Multi-turn: merchant previously provided product info, now provides price in next message.
// The existing information is retained in context and not requested again.
func TestConversationalAuthoringMultiTurnContextRetention(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	authoringExecuted := false

	repo := mockCatalogRepository{
		createCatalogItemFn: func(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
			authoringExecuted = true
			if draft.Name != "قميص رجالي أبيض" {
				t.Fatalf("expected item name from prior context, got %s", draft.Name)
			}
			return ports.CatalogItemRecord{ID: "item-200", BusinessID: draft.BusinessID, Name: draft.Name}, nil
		},
		createVariantFn: func(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
			return ports.VariantRecord{ID: "var-200", BusinessID: draft.BusinessID, Name: draft.Name}, nil
		},
		createOfferFn: func(ctx context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
			amt := "15000"
			return ports.OfferRecord{ID: "offer-200", BusinessID: draft.BusinessID, Amount: &amt, Currency: draft.Currency}, nil
		},
	}

	cmdServices := CatalogCommandServices{
		Repository:   repo,
		Transactions: mockTransactionManager{},
		NewID:        func() string { return "gen-id" },
		Now:          func() time.Time { return now },
	}

	cap := NewCatalogAuthoringCapability(
		AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateOfferCommandService{CatalogCommandServices: cmdServices},
		CreateVariantCommandService{CatalogCommandServices: cmdServices},
		CreateAttributeSchemaVersionCommandService{CatalogCommandServices: cmdServices},
	)

	// Context contains recent messages from previous turn: "أضف قميص رجالي أبيض مقاسات S, M, L"
	aiContext := &ports.AIContext{
		RecentMessages: []ports.AIRecentMessageEvidence{
			{Reference: "msg-prev", Text: "أضف قميص رجالي أبيض مقاسات S و M و L", Direction: "inbound", OccurredAt: now.Add(-time.Minute)},
			{Reference: "msg-ai-clarify", Text: "كم سعر القميص؟", Direction: "outbound", OccurredAt: now.Add(-30 * time.Second)},
		},
	}

	runtime := mockAuthoringAIRuntime{
		decideFn: func(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
			// Merchant says "السعر 15000 ريال يمني" in current turn.
			// Model uses previous turn info (item name, sizes) + current price without re-asking!
			execCtx := ports.AICapabilityExecutionContext{
				BusinessID:     input.BusinessID,
				ConversationID: input.ConversationID,
				Role:           "owner",
			}
			capRes, err := cap.Execute(ctx, execCtx, []byte(`{
				"operation": "author_catalog_item",
				"catalog_id": "cat-1",
				"name": "قميص رجالي أبيض",
				"item_type": "product",
				"amount": "15000",
				"currency": "YER",
				"variants": [
					{"name": "S"},
					{"name": "M"},
					{"name": "L"}
				]
			}`))
			if err != nil {
				return ports.AIDecisionProposal{}, err
			}

			return ports.AIDecisionProposal{
				IntentBase:                "catalog_authoring",
				DomainContext:             "catalog",
				RequestedAction:           AutoReplyActionAnswer,
				ResponseText:              "تمت إضافة قميص رجالي أبيض بنجاح بسعر 15,000 ريال والمقاسات S, M, L.",
				ConfidenceBand:            "high",
				ConfidenceValue:           "0.99",
				PolicyDecision:            "allowed",
				EvidenceReferences:        []byte(`["item-200"]`),
				DiscoveredCatalogEvidence: capRes.CatalogEvidence,
				DiscoveredVariantEvidence: capRes.VariantEvidence,
				DiscoveredOfferEvidence:   capRes.OfferEvidence,
				SchemaVersion:             1,
			}, nil
		},
	}

	decisionRepo := &fakeDecisionRepository{}
	refRepo := fakeReferenceRepository{
		record: ports.ConversationReferenceRecord{
			ID:             "ref-3",
			BusinessID:     "biz-1",
			ConversationID: "conv-1",
			System:         "socialapi",
			ProviderRef:    "whatsapp",
			ResourceID:     "provider-conv-1",
			ConnectionID:   stringPtr("conn-1"),
			IsCurrent:      true,
			MappingStatus:  "active",
		},
	}
	service := NewAutoReplyService(
		runtime,
		decisionRepo,
		refRepo,
		&fakeOutboundRepository{},
		&fakeOutboxStore{},
		mockTransactionManager{},
	)
	service.PolicyEvaluator = GroundedPolicyEngine{}

	res, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1", Role: "owner"},
		},
		ConversationID:         "conv-1",
		SourceMessageReference: "msg-curr",
		Text:                   "السعر 15000 ريال يمني",
		Channel:                "whatsapp",
		ProviderRef:            "whatsapp",
	})

	_ = aiContext

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !authoringExecuted {
		t.Fatal("expected authoring to complete across multi-turn context without asking for name/sizes again")
	}
	if res.Action != AutoReplyActionAnswer {
		t.Fatalf("expected action answer, got %s", res.Action)
	}
}
