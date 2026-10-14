package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestHierarchicalCatalogIncludesAllPagesWithoutReduction(t *testing.T) {
	catalog := &capabilityCatalogRepo{}
	model := &capabilityBatchModel{}
	controller := &CatalogBatchController{CatalogAI: catalog, Gemini: model, RunRepo: &capabilityRunRepo{}, TokenBudget: 100, Now: time.Now, NewID: func() string { return "id" }}
	result, err := controller.RunCatalogEvaluation(context.Background(), CatalogEvaluationInput{AIRunID: "run", BusinessID: "business-1", Presentation: "hierarchy"})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.reads != 2 || catalog.revisionReads != 2 || model.finalCalls != 0 || len(model.seen) != 0 {
		t.Fatalf("hierarchy must read complete catalog without AI reduction: reads=%d revisions=%d final=%d evaluated=%d", catalog.reads, catalog.revisionReads, model.finalCalls, len(model.seen))
	}
	if result.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction || len(result.Messages) == 0 {
		t.Fatalf("invalid hierarchy terminal decision: %+v", result.Decision)
	}
	text := strings.Join(result.Messages, "\n")
	for _, id := range []string{"item-1", "item-2", "item-3", "item-4", "📂 Main", "• "} {
		if !strings.Contains(text, id) {
			t.Fatalf("hierarchy omitted %q", id)
		}
	}
	if result.Decision.ResponseText != result.Messages[0] {
		t.Fatal("first message differs from validated response")
	}
}

func TestCatalogHierarchyIsHumanReadableAndNeverSplitsProduct(t *testing.T) {
	original := strings.Repeat("تفاصيل الخدمة العربية الطويلة ", 100)
	price, currency, variantID := "12000.0000", "YER", "variant-1"
	lines := &catalogHierarchyLines{}
	item := CatalogAIItem{ID: "item-1", Name: "الباقة الأساسية", CatalogID: "catalog-1", LongDescription: &original,
		Variants: []CatalogAIVariant{{ID: variantID, CatalogItemID: "item-1", Name: "سنوي"}},
		Offers: []CatalogAIOffer{{ID: "offer-1", CatalogItemID: "item-1", VariantID: &variantID, Name: "سعر السنة", Amount: &price, Currency: &currency}}}
	if err := lines.appendItem("باقات مجيب", item, nil); err != nil {
		t.Fatal(err)
	}
	messages, err := lines.messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("one short product should be one customer reply, got %d messages", len(messages))
	}
	got := messages[0]
	for _, want := range []string{"أهلًا وسهلًا", "📂 باقات مجيب", "• الباقة الأساسية", "سنوي: 12,000 ريال يمني"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing customer-facing value %q: %q", want, got)
		}
	}
	if strings.Contains(got, "12000.0000") || strings.Contains(got, "catalog-1") || strings.Contains(got, "variant-1") || strings.Contains(got, original) {
		t.Fatal("customer-facing summary leaked raw/oversized commercial projection")
	}
	if item.LongDescription == nil || *item.LongDescription != original {
		t.Fatal("source product description was modified")
	}
}

func TestCatalogHierarchyGroupsCategoriesAndItems(t *testing.T) {
	lines := &catalogHierarchyLines{}
	price, currency := "250.0000", "SAR"
	for i := 0; i < 8; i++ {
		item := CatalogAIItem{ID: fmt.Sprintf("item-%d", i), Name: fmt.Sprintf("خيار رقم %d", i), CatalogID: "catalog-1",
			Offers: []CatalogAIOffer{{ID: fmt.Sprintf("offer-%d", i), CatalogItemID: fmt.Sprintf("item-%d", i), Amount: &price, Currency: &currency}}}
		if err := lines.appendItem("باقات السفر", item, nil); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := lines.messages()
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(messages, "\n")
	if strings.Count(all, "📂 باقات السفر") != 1 {
		t.Fatalf("section header repeated per product: %q", all)
	}
	for i := 0; i < 8; i++ {
		if strings.Count(all, fmt.Sprintf("• خيار رقم %d", i)) != 1 {
			t.Fatalf("item %d is missing or repeated", i)
		}
	}
}

func TestCatalogHierarchyRejectsDetachedOfferAndOversizedItem(t *testing.T) {
	wrong := "variant-from-another-item"
	lines := &catalogHierarchyLines{}
	err := lines.appendItem("C", CatalogAIItem{ID: "item-1", Name: "Item", Offers: []CatalogAIOffer{{ID: "offer", CatalogItemID: "item-1", VariantID: &wrong}}}, nil)
	if err == nil {
		t.Fatal("detached offer incorrectly accepted")
	}
	lines = &catalogHierarchyLines{}
	long := strings.Repeat("اسم طويل جدا ", 180)
	item := CatalogAIItem{ID: "item-2", Name: "Item", Offers: []CatalogAIOffer{{ID: "offer", CatalogItemID: "item-2", Name: long}}}
	if err := lines.appendItem("Catalog", item, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := lines.messages(); err == nil {
		t.Fatal("overlong product must not be split into incoherent fragments")
	}
}

func TestAutoReplyHierarchicalCatalogSkipsCandidateReduction(t *testing.T) {
	service, runtime, outbox, runs := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog, Presentation: "hierarchy"}
	model := &capabilityBatchModel{}
	service.CatalogBatch = &CatalogBatchController{CatalogAI: &capabilityCatalogRepo{}, Gemini: model, RunRepo: runs, TokenBudget: 100, Now: service.now, NewID: service.NewID}
	service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || !result.Enqueued || outbox.calls == 0 || model.finalCalls != 0 || len(model.seen) != 0 {
		t.Fatalf("hierarchy should enqueue grounded reply without reduction: %+v err=%v outbound=%d", result, err, outbox.calls)
	}
}

type verboseHierarchyCatalogRepo struct {
	*capabilityCatalogRepo
}

func (r *verboseHierarchyCatalogRepo) ListProjectionPage(ctx context.Context, request ports.CatalogAIProjectionRequest) (ports.CatalogAIProjectionPage, error) {
	page, err := r.capabilityCatalogRepo.ListProjectionPage(ctx, request)
	if err == nil && len(page.Items) > 0 && request.Cursor == "" {
		description := strings.Repeat("كامل وصف المنتج بدون إسقاط البيانات ", 140)
		page.Items[0].Item.LongDescription = &description
	}
	return page, err
}

func TestAutoReplyHierarchyEnqueuesAllPartsAtomically(t *testing.T) {
	service, runtime, outbox, runs := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog, Presentation: "hierarchy"}
	catalog := &verboseHierarchyCatalogRepo{capabilityCatalogRepo: &capabilityCatalogRepo{}}
	service.CatalogBatch = &CatalogBatchController{CatalogAI: catalog, Gemini: &capabilityBatchModel{}, RunRepo: runs, TokenBudget: 100, Now: service.now, NewID: service.NewID}
	service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || !result.Enqueued || outbox.calls < 2 {
		t.Fatalf("hierarchical multi-part output was not fully enqueued: parts=%d result=%+v err=%v", outbox.calls, result, err)
	}
	if !strings.Contains(outbox.record.DedupeKey, ":part:") {
		t.Fatalf("last part lost its independent delivery key: %q", outbox.record.DedupeKey)
	}
}
