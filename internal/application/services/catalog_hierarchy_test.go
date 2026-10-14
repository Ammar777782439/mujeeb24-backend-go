package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type aiCatalogReplyFixture struct {
	*capabilityBatchModel
	parts     []CatalogReplyInput
	omit      bool
	external  bool
	tooLong   bool
	failPart  int
}

func (m *aiCatalogReplyFixture) CountCatalogReplyTokens(_ context.Context, in CatalogReplyInput) (int, error) {
	return 100 + 200*len(in.Projection.Items), nil
}

func (m *aiCatalogReplyFixture) ComposeCatalogReply(_ context.Context, in CatalogReplyInput) (CatalogReplyOutput, error) {
	m.parts = append(m.parts, in)
	if m.failPart > 0 && len(m.parts) == m.failPart {
		return CatalogReplyOutput{}, fmt.Errorf("Gemini failed during part %d", m.failPart)
	}
	out := CatalogReplyOutput{Text: "بحسب المنتجات المتاحة، الخيارات والأسعار مختلفة. "}
	for _, item := range in.Projection.Items {
		out.CoveredItemIDs = append(out.CoveredItemIDs, item.ID)
		out.Text += item.Name + "؛ "
	}
	if m.omit && len(out.CoveredItemIDs) > 0 {
		out.CoveredItemIDs = out.CoveredItemIDs[1:]
	}
	if m.external {
		out.CoveredItemIDs = append(out.CoveredItemIDs, "from-another-business")
	}
	if m.tooLong {
		out.Text = strings.Repeat("ب", catalogReplyMaxRunes+1)
	}
	return out, nil
}

func TestCatalogHierarchyUsesGeminiForEveryCustomerReply(t *testing.T) {
	catalog := &capabilityCatalogRepo{}
	model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}}
	controller := &CatalogBatchController{CatalogAI: catalog, Gemini: model, RunRepo: &capabilityRunRepo{}, TokenBudget: 550, Now: time.Now, NewID: func() string { return "id" }}
	result, err := controller.RunCatalogEvaluation(context.Background(), CatalogEvaluationInput{AIRunID: "run", BusinessID: "business-1", Presentation: "hierarchy", CustomerMessage: "قارن لي كل الخيارات"})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.reads != 2 || catalog.revisionReads != 2 || model.finalCalls != 0 || len(model.seen) != 0 {
		t.Fatalf("unexpected catalog read or legacy reduction: pages=%d revisions=%d", catalog.reads, catalog.revisionReads)
	}
	if len(result.Messages) != 2 || len(model.parts) != 2 {
		t.Fatalf("expected two Gemini-authored messages, got %d messages and %d model calls", len(result.Messages), len(model.parts))
	}
	if result.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction || result.Decision.ResponseText != result.Messages[0] {
		t.Fatal("Gemini reply not propagated through normal validated capability")
	}
	for i, message := range result.Messages {
		if !strings.Contains(message, "بحسب المنتجات") || message != "بحسب المنتجات المتاحة، الخيارات والأسعار مختلفة. "+model.parts[i].Projection.Items[0].Name+"؛ "+model.parts[i].Projection.Items[1].Name+"؛ " {
			t.Fatalf("customer-facing message was not taken verbatim from Gemini: %q", message)
		}
		if model.parts[i].CustomerMessage != "قارن لي كل الخيارات" || len(model.parts[i].Projection.Catalogs) != 1 || len(model.parts[i].Projection.AttributeSchemas) != 1 {
			t.Fatal("model missing tenant-scoped catalog, schema, or customer intent")
		}
		for _, item := range model.parts[i].Projection.Items {
			if len(item.Variants) != 1 || len(item.Offers) != 1 {
				t.Fatal("model missing a commercial offer or its variant")
			}
		}
	}
}

func TestCatalogHierarchyRejectsMissingOrForeignGeminiItemCoverage(t *testing.T) {
	for _, mode := range []string{"omitted", "foreign", "overlong", "provider_failure"} {
		t.Run(mode, func(t *testing.T) {
			model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}, omit: mode == "omitted", external: mode == "foreign", tooLong: mode == "overlong"}
			if mode == "provider_failure" {
				model.failPart = 2
			}
			catalog := &capabilityCatalogRepo{}
			controller := &CatalogBatchController{CatalogAI: catalog, Gemini: model, RunRepo: &capabilityRunRepo{}, TokenBudget: 550, Now: time.Now, NewID: func() string { return "id" }}
			_, err := controller.RunCatalogEvaluation(context.Background(), CatalogEvaluationInput{AIRunID: "run", BusinessID: "business-1", Presentation: "hierarchy"})
			if err == nil {
				t.Fatal("invalid Gemini output must fail closed without a partial reply")
			}
		})
	}
}

func TestCatalogHierarchyRejectsCatalogRevisionChanges(t *testing.T) {
	model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}}
	controller := &CatalogBatchController{CatalogAI: &capabilityCatalogRepo{changed: true}, Gemini: model, RunRepo: &capabilityRunRepo{}, TokenBudget: 550, Now: time.Now, NewID: func() string { return "id" }}
	if _, err := controller.RunCatalogEvaluation(context.Background(), CatalogEvaluationInput{AIRunID: "run", BusinessID: "business-1", Presentation: "hierarchy"}); err == nil {
		t.Fatal("stale commercial evidence accepted")
	}
}

func TestAutoReplyEnqueuesEveryGeminiAuthoredCatalogPart(t *testing.T) {
	service, runtime, outbox, runs := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog, Presentation: "hierarchy"}
	model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}}
	service.CatalogBatch = &CatalogBatchController{CatalogAI: &capabilityCatalogRepo{}, Gemini: model, RunRepo: runs, TokenBudget: 550, Now: service.now, NewID: service.NewID}
	service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || !result.Enqueued || outbox.calls != 2 || len(model.parts) != 2 {
		t.Fatalf("complete Gemini-authored reply not enqueued: result=%+v err=%v outbox=%d", result, err, outbox.calls)
	}
	if !strings.Contains(outbox.record.DedupeKey, ":part:") {
		t.Fatal("multipart Gemini answer lost per-message idempotency key")
	}
}
