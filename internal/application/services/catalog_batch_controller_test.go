package services

import (
	"context"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type exactTokenBatchGeminiStub struct {
	countFn func(BatchEvaluationInput) (int, error)
	seen    []BatchEvaluationInput
}

func (s *exactTokenBatchGeminiStub) CountBatchTokens(_ context.Context, input BatchEvaluationInput) (int, error) {
	s.seen = append(s.seen, input)
	if s.countFn != nil {
		return s.countFn(input)
	}
	return 0, nil
}

func (s *exactTokenBatchGeminiStub) EvaluateBatch(_ context.Context, input BatchEvaluationInput) (ports.CatalogBatchResult, error) {
	return ports.CatalogBatchResult{BatchNumber: input.BatchNumber}, nil
}

func (s *exactTokenBatchGeminiStub) FinalEvaluateWithDetails(_ context.Context, _ FinalEvaluationInput, _ string) (ports.CustomerSalesProposal, ports.CustomerSalesUsageTelemetry, error) {
	return ports.CustomerSalesProposal{}, ports.CustomerSalesUsageTelemetry{}, nil
}

func TestSplitIntoBatchesUsesExactRequestTokensAndSequentialNumbers(t *testing.T) {
	gemini := &exactTokenBatchGeminiStub{
		countFn: func(input BatchEvaluationInput) (int, error) {
			// Simulate fixed prompt/contract overhead + per-item payload.
			return 100 + 100*len(input.Batch.Items), nil
		},
	}
	controller := &CatalogBatchController{
		Gemini:      gemini,
		TokenBudget: 250,
	}
	schemaID := "schema-1"
	projection := CatalogAIProjection{
		Catalogs: []CatalogAICatalog{{ID: "catalog-1", Name: "Main", Status: "active"}},
		AttributeSchemas: []CatalogAIAttributeSchema{{
			ID: schemaID, Name: "Common", Version: 1,
		}},
		Items: []CatalogAIItem{
			{ID: "item-1", CatalogID: "catalog-1", AttributeSchemaID: &schemaID},
			{ID: "item-2", CatalogID: "catalog-1", AttributeSchemaID: &schemaID},
			{ID: "item-3", CatalogID: "catalog-1", AttributeSchemaID: &schemaID},
		},
	}
	input := CatalogEvaluationInput{
		AIRunID:         "run-1",
		BusinessID:      "business-1",
		ConversationID:  "conversation-1",
		CustomerMessage: "اريد منتج مناسب",
	}

	batches, err := controller.splitIntoBatches(context.Background(), projection, input, 7)
	if err != nil {
		t.Fatalf("splitIntoBatches: %v", err)
	}
	if len(batches) != 3 {
		t.Fatalf("expected 3 exact-token batches, got %d", len(batches))
	}
	for i, batch := range batches {
		wantNumber := 7 + i
		if batch.BatchNumber != wantNumber {
			t.Fatalf("batch[%d].BatchNumber=%d, want %d", i, batch.BatchNumber, wantNumber)
		}
		if len(batch.Items) != 1 {
			t.Fatalf("batch[%d] contains %d items; exact token split should keep one", i, len(batch.Items))
		}
		if len(batch.Catalogs) != 1 || batch.Catalogs[0].ID != "catalog-1" {
			t.Fatalf("batch[%d] lost catalog relationship: %+v", i, batch.Catalogs)
		}
		if len(batch.AttributeSchemas) != 1 || batch.AttributeSchemas[0].ID != schemaID {
			t.Fatalf("batch[%d] lost used schema: %+v", i, batch.AttributeSchemas)
		}
	}
	if len(gemini.seen) == 0 || gemini.seen[0].CustomerMessage != input.CustomerMessage {
		t.Fatal("exact token counter did not receive the real BatchEvaluationInput")
	}
}

func TestSplitIntoBatchesRejectsSingleOversizedItem(t *testing.T) {
	gemini := &exactTokenBatchGeminiStub{
		countFn: func(input BatchEvaluationInput) (int, error) {
			return 9999, nil
		},
	}
	controller := &CatalogBatchController{Gemini: gemini, TokenBudget: 500}
	projection := CatalogAIProjection{
		Items: []CatalogAIItem{{ID: "huge-item", CatalogID: "catalog-1"}},
	}
	_, err := controller.splitIntoBatches(context.Background(), projection, CatalogEvaluationInput{
		AIRunID: "run-1", BusinessID: "business-1", CustomerMessage: "test",
	}, 1)
	if err == nil {
		t.Fatal("expected oversized single item to fail closed")
	}
	if !strings.Contains(err.Error(), "huge-item") || !strings.Contains(err.Error(), "exceeds token budget") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBatchCandidatesEnforcesItemVariantOfferRelationships(t *testing.T) {
	batch := CatalogAIBatchPayload{
		Items: []CatalogAIItem{
			{
				ID: "item-1",
				Variants: []CatalogAIVariant{{ID: "variant-1", CatalogItemID: "item-1"}},
				Offers: []CatalogAIOffer{{ID: "offer-1", CatalogItemID: "item-1"}},
			},
			{
				ID: "item-2",
				Variants: []CatalogAIVariant{{ID: "variant-2", CatalogItemID: "item-2"}},
				Offers: []CatalogAIOffer{{ID: "offer-2", CatalogItemID: "item-2"}},
			},
		},
	}

	if err := validateBatchCandidates(batch, []ports.CatalogBatchCandidate{{
		ItemID: "item-1", VariantIDs: []string{"variant-1"}, OfferIDs: []string{"offer-1"},
	}}); err != nil {
		t.Fatalf("valid relational candidate rejected: %v", err)
	}

	if err := validateBatchCandidates(batch, []ports.CatalogBatchCandidate{{
		ItemID: "item-1", VariantIDs: []string{"variant-2"},
	}}); err == nil {
		t.Fatal("expected cross-item variant candidate to be rejected")
	}

	if err := validateBatchCandidates(batch, []ports.CatalogBatchCandidate{{
		ItemID: "item-1", OfferIDs: []string{"offer-2"},
	}}); err == nil {
		t.Fatal("expected cross-item offer candidate to be rejected")
	}
}
