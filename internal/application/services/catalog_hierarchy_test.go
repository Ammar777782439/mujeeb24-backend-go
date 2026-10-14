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
	wrongLink bool
	emptyText bool
}

func (m *aiCatalogReplyFixture) CountCatalogReplyTokens(_ context.Context, in CatalogReplyInput) (int, error) {
	return 100 + 200*len(in.Projection.Items), nil
}

func (m *aiCatalogReplyFixture) ComposeCatalogReply(_ context.Context, in CatalogReplyInput) (CatalogReplyOutput, error) {
	m.parts = append(m.parts, in)
	if m.failPart > 0 && len(m.parts) == m.failPart {
		return CatalogReplyOutput{}, fmt.Errorf("Gemini failed during part %d", m.failPart)
	}
	out := CatalogReplyOutput{Introduction: "بحسب المنتجات المتاحة، الخيارات والأسعار مختلفة."}
	byCatalog := map[string]int{}
	for _, item := range in.Projection.Items {
		index, exists := byCatalog[item.CatalogID]
		if !exists {
			index = len(out.Sections)
			byCatalog[item.CatalogID] = index
			out.Sections = append(out.Sections, CatalogReplySection{CatalogID: item.CatalogID, Heading: "هذه الفئة تشمل الخيارات الآتية:"})
		}
		if m.omit && len(out.Sections[index].Items) == 0 {
			continue
		}
		if m.wrongLink {
			out.Sections[index].CatalogID = "wrong-catalog"
		}
		itemID := item.ID
		if m.external {
			itemID = "from-another-business"
		}
		text := "الخيار " + item.Name + " له تفاصيل وأسعار في الكتالوج."
		if m.emptyText {
			text = ""
		}
		out.Sections[index].Items = append(out.Sections[index].Items, CatalogReplyItem{ItemID: itemID, Text: text})
	}
	if m.tooLong {
		out.Introduction = strings.Repeat("ب", catalogReplyMaxRunes+1)
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
		t.Fatal("Gemini reply not propagated through validated capability")
	}
	for i, message := range result.Messages {
		if !strings.Contains(message, "بحسب المنتجات") || !strings.Contains(message, model.parts[i].Projection.Items[0].Name) {
			t.Fatalf("Gemini text not present in customer reply: %q", message)
		}
		if model.parts[i].CustomerMessage != "قارن لي كل الخيارات" || len(model.parts[i].Projection.Catalogs) != 1 || len(model.parts[i].Projection.AttributeSchemas) != 1 {
			t.Fatal("Gemini missing customer intent, tenant catalog or attribute schema")
		}
		for _, item := range model.parts[i].Projection.Items {
			if len(item.Variants) != 1 || len(item.Offers) != 1 {
				t.Fatal("Gemini missing commercial offers or variants")
			}
		}
	}
}

func TestCatalogHierarchyRejectsMissingOrForeignGeminiItemCoverage(t *testing.T) {
	for _, mode := range []string{"omitted", "foreign", "overlong", "provider_failure", "wrong_catalog", "empty_text"} {
		t.Run(mode, func(t *testing.T) {
			model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}, omit: mode == "omitted", external: mode == "foreign", tooLong: mode == "overlong", wrongLink: mode == "wrong_catalog", emptyText: mode == "empty_text"}
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

func TestTargetedCatalogDetailAndComparisonSkipReductionAndKeepAllOffers(t *testing.T) {
	for _, presentation := range []string{"detail", "compare"} {
		t.Run(presentation, func(t *testing.T) {
			service, runtime, outbox, runs := capabilityService()
			runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog, Presentation: presentation}
			catalog := &capabilityCatalogRepo{}
			model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}}
			service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Gemini: model, RunRepo: runs, TokenBudget: 550, Now: service.now, NewID: service.NewID}
			service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
			command := capabilityCommand()
			command.Text = "تفاصيل باقة سنوية"
			result, err := service.Handle(context.Background(), command)
			if err != nil || !result.Enqueued {
				t.Fatalf("targeted model-selected catalog answer not queued: %+v %v", result, err)
			}
			if outbox.calls != 1 || model.finalCalls != 0 || len(model.parts) != 1 || catalog.reads != 2 {
				t.Fatalf("targeted detail used legacy reduction or replied with all items: outbox=%d final=%d replies=%d pages=%d", outbox.calls, model.finalCalls, len(model.parts), catalog.reads)
			}
			if len(model.seen) != 4 || len(model.parts[0].Projection.Items) != 1 {
				t.Fatalf("Gemini did not evaluate all pages then select one item: scanned=%v selected=%d", model.seen, len(model.parts[0].Projection.Items))
			}
			item := model.parts[0].Projection.Items[0]
			if item.ID != "item-4" || len(item.Variants) != 1 || len(item.Offers) != 1 {
				t.Fatalf("selected item lost its commercial children: %+v", item)
			}
		})
	}
}

func TestCatalogReplyPartitionReservesCustomerFacingOutputSpace(t *testing.T) {
	model := &aiCatalogReplyFixture{capabilityBatchModel: &capabilityBatchModel{}}
	controller := &CatalogBatchController{Gemini: model, TokenBudget: 20000}
	projection := CatalogAIProjection{Catalogs: []CatalogAICatalog{{ID: "c1", Name: "المنتجات", Status: "active"}}}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("item-%d", i)
		projection.Items = append(projection.Items, CatalogAIItem{ID: id, CatalogID: "c1", Name: fmt.Sprintf("منتج رقم %d", i)})
	}
	parts, err := controller.partitionCatalogForReply(context.Background(), model, CatalogEvaluationInput{CustomerMessage: "أريد جميع المنتجات"}, projection)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) < 2 {
		t.Fatalf("large all-options response was not split to reserve Gemini output space: %d parts", len(parts))
	}
	covered := map[string]bool{}
	for _, part := range parts {
		if estimateCatalogReplyRunes(part.Items) > catalogReplyMaxRunes*3/4 {
			t.Fatal("part exceeded AI prose output budget")
		}
		for _, item := range part.Items {
			if covered[item.ID] {
				t.Fatalf("item appeared in multiple model parts: %s", item.ID)
			}
			covered[item.ID] = true
		}
	}
	if len(covered) != 12 {
		t.Fatalf("catalog splitting lost product information: %d of 12", len(covered))
	}
}

func TestStructuredCatalogReplyAllowsNaturalProductNames(t *testing.T) {
	projection := CatalogAIProjection{Items: []CatalogAIItem{{ID: "i-1", CatalogID: "c-1", Name: "باقة مجيب الأساسية"}}}
	got, err := assembleCatalogReply(projection, CatalogReplyOutput{
		Introduction: "أهلًا بك!",
		Sections: []CatalogReplySection{{CatalogID: "c-1", Heading: "خدمات الاشتراك",
			Items: []CatalogReplyItem{{ItemID: "i-1", Text: "مجيب الأساسي يقدم خيارات الاشتراك والميزات حسب الحاجة."}}}},
		Conclusion: "أي ميزة تهمك أكثر؟",
	})
	if err != nil || !strings.Contains(got, "مجيب الأساسي") || !strings.Contains(got, "أي ميزة") {
		t.Fatalf("valid natural-language product description rejected: %q, %v", got, err)
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
