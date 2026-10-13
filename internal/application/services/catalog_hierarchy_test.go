package services

import (
	"context"
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
	for _, id := range []string{"item-1", "item-2", "item-3", "item-4", "📁 Main", "├─"} {
		if !strings.Contains(text, id) {
			t.Fatalf("hierarchy omitted %q", id)
		}
	}
	if result.Decision.ResponseText != result.Messages[0] {
		t.Fatal("first message differs from validated response")
	}
}

func TestHierarchicalCatalogNeverTruncatesLongArabicDescriptions(t *testing.T) {
	original := strings.Repeat("تفاصيل الباقة العربية الكاملة ", 270)
	price, currency, variantID := "12000.0000", "YER", "variant-1"
	lines := &catalogHierarchyLines{}
	item := CatalogAIItem{ID: "item-1", Name: "الباقة الأساسية", CatalogID: "catalog-1", LongDescription: &original, Variants: []CatalogAIVariant{{ID: variantID, CatalogItemID: "item-1", Name: "سنوي"}}, Offers: []CatalogAIOffer{{ID: "offer-1", CatalogItemID: "item-1", VariantID: &variantID, Name: "سعر السنة", Amount: &price, Currency: &currency}}}
	if err := lines.appendItem("باقات مجيب", item, nil); err != nil {
		t.Fatal(err)
	}
	messages, err := lines.messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) < 2 {
		t.Fatal("long hierarchy should be broken into safe transport messages")
	}
	var parts []string
	for i, msg := range messages {
		if !strings.Contains(msg, "الجزء") {
			t.Fatalf("part %d is not labeled", i+1)
		}
		if len([]rune(msg)) > catalogHierarchyPartRunes+90 {
			t.Fatal("provider message exceeds safe length")
		}
		parts = append(parts, strings.SplitN(msg, "\n", 2)[1])
	}
	reconstructed := strings.Join(parts, "")
	if !strings.Contains(reconstructed, original) || !strings.Contains(reconstructed, "12000.0000 YER") || !strings.Contains(reconstructed, "سنوي") {
		t.Fatal("hierarchy text was truncated or commercial relations were lost")
	}
}

func TestHierarchicalCatalogRejectsDetachedOfferAndOverlongListing(t *testing.T) {
	wrong := "variant-from-another-item"
	lines := &catalogHierarchyLines{}
	err := lines.appendItem("C", CatalogAIItem{ID: "item-1", Offers: []CatalogAIOffer{{ID: "offer", CatalogItemID: "item-1", VariantID: &wrong}}}, nil)
	if err == nil {
		t.Fatal("detached offer incorrectly accepted")
	}
	veryLong := &catalogHierarchyLines{}
	veryLong.add(strings.Repeat("ف", catalogHierarchyPartRunes*catalogHierarchyMaxParts+1))
	if _, err := veryLong.messages(); err == nil {
		t.Fatal("unsafe number of messages accepted")
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
