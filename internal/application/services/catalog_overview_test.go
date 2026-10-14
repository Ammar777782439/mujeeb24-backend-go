package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type largeOverviewCatalogRepo struct {
	*capabilityCatalogRepo
	pages int
}

func (r *largeOverviewCatalogRepo) ListProjectionPage(_ context.Context, request ports.CatalogAIProjectionRequest) (ports.CatalogAIProjectionPage, error) {
	if request.BusinessID != "business-1" || request.CatalogID != "" {
		return ports.CatalogAIProjectionPage{}, errors.New("cross-tenant projection request")
	}
	page := 0
	if request.Cursor != "" {
		_, err := fmt.Sscanf(request.Cursor, "page-%d", &page)
		if err != nil {
			return ports.CatalogAIProjectionPage{}, err
		}
	}
	r.pages++
	result := ports.CatalogAIProjectionPage{HasMore: page < 3}
	if result.HasMore {
		result.NextCursor = fmt.Sprintf("page-%d", page+1)
	}
	for i := page*25; i < (page+1)*25; i++ {
		catalogID := fmt.Sprintf("catalog-%03d", i)
		itemID := fmt.Sprintf("item-%03d", i)
		result.Catalogs = append(result.Catalogs, ports.CatalogRecord{
			ID: catalogID, BusinessID: request.BusinessID, Name: fmt.Sprintf("قسم %d", i), Status: "active",
		})
		result.Items = append(result.Items, ports.CatalogAIProjectionBundle{
			Item: ports.CatalogItemRecord{
				ID: itemID, BusinessID: request.BusinessID, CatalogID: catalogID,
				Name: fmt.Sprintf("خدمة رقم %d", i), Status: "active", ItemType: "service",
			},
		})
	}
	return result, nil
}

type overviewModelFixture struct {
	*capabilityBatchModel
	batchCalls int
	mergeCalls int
	finalCalls int
	items map[string]struct{}
	missingCoverage bool
	oversized bool
}

func (*overviewModelFixture) CountOverviewTokens(_ context.Context, request CatalogOverviewRequest) (int, error) {
	if request.Batch != nil {
		return 150 + len(request.Batch.Items)*190, nil
	}
	return 150 + len(request.Summaries)*160, nil
}

func (m *overviewModelFixture) ComposeOverview(_ context.Context, request CatalogOverviewRequest) (CatalogOverviewResponse, error) {
	switch request.Stage {
	case "batch":
		m.batchCalls++
		if m.items == nil {
			m.items = map[string]struct{}{}
		}
		out := CatalogOverviewResponse{Text: "ملخص أقسام الخدمات المتاحة"}
		for _, item := range request.Batch.Items {
			m.items[item.ID] = struct{}{}
			out.CoveredItemIDs = append(out.CoveredItemIDs, item.ID)
		}
		if m.missingCoverage && len(out.CoveredItemIDs) > 0 {
			out.CoveredItemIDs = out.CoveredItemIDs[:len(out.CoveredItemIDs)-1]
		}
		return out, nil
	case "merge":
		m.mergeCalls++
		return CatalogOverviewResponse{Text: "ملخص شامل لأقسام السفر والاشتراكات والتقنية"}, nil
	case "final":
		m.finalCalls++
		result := CatalogOverviewResponse{Text: "أهلًا بك! لدينا أقسام مختلفة. أخبرني أي قسم تريد استكشافه وسأشرح خياراته وأسعاره."}
		if m.oversized {
			result.Text = strings.Repeat("أ", overviewFinalMaxRunes+1)
		}
		return result, nil
	default:
		return CatalogOverviewResponse{}, errors.New("unexpected overview stage")
	}
}

func TestHundredCatalogOverviewReviewsEveryItemAndSendsSingleReply(t *testing.T) {
	repo := &largeOverviewCatalogRepo{capabilityCatalogRepo: &capabilityCatalogRepo{}}
	model := &overviewModelFixture{capabilityBatchModel: &capabilityBatchModel{}}
	service, runtime, outbox, runs := capabilityService()
	runtime.Decision = ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog, Presentation: "overview"}
	service.CatalogBatch = &CatalogBatchController{
		CatalogAI: repo, Gemini: model, RunRepo: runs, TokenBudget: 1000,
		Now: time.Now, NewID: func() string { return fmt.Sprintf("id-%d", time.Now().UnixNano()) },
	}
	service.EntityContractPayload = mustMarshal(BuildCatalogEntityContractPayload())
	result, err := service.Handle(context.Background(), capabilityCommand())
	if err != nil || !result.Enqueued {
		t.Fatalf("full catalog overview failed: result=%+v err=%v", result, err)
	}
	if repo.pages != 4 || repo.revisionReads != 2 || len(model.items) != 100 {
		t.Fatalf("missing original catalog coverage: pages=%d revisions=%d items=%d", repo.pages, repo.revisionReads, len(model.items))
	}
	if model.batchCalls < 25 || model.mergeCalls < 1 || model.finalCalls != 1 || model.capabilityBatchModel.finalCalls != 0 {
		t.Fatalf("overview incorrectly reduced candidates or skipped Gemini synthesis: batches=%d merges=%d final=%d legacy=%d", model.batchCalls, model.mergeCalls, model.finalCalls, model.capabilityBatchModel.finalCalls)
	}
	if outbox.calls != 1 {
		t.Fatalf("100 catalogs produced %d customer messages instead of one", outbox.calls)
	}
}

func TestCatalogOverviewFailsClosedIfBatchClaimsIncompleteCoverage(t *testing.T) {
	repo := &largeOverviewCatalogRepo{capabilityCatalogRepo: &capabilityCatalogRepo{}}
	model := &overviewModelFixture{capabilityBatchModel: &capabilityBatchModel{}, missingCoverage: true}
	controller := &CatalogBatchController{
		CatalogAI: repo, Gemini: model, RunRepo: &capabilityRunRepo{}, TokenBudget: 1000,
		Now: time.Now, NewID: func() string { return "id" },
	}
	_, err := controller.RunCatalogEvaluation(context.Background(), CatalogEvaluationInput{
		AIRunID: "run-1", BusinessID: "business-1", Presentation: "overview",
	})
	if err == nil || !strings.Contains(err.Error(), "coverage") {
		t.Fatalf("incomplete Gemini evaluation incorrectly accepted: %v", err)
	}
	if model.finalCalls != 0 {
		t.Fatal("Gemini final overview called before complete coverage")
	}
}

func TestCatalogOverviewRejectsOversizedCustomerResponse(t *testing.T) {
	repo := &largeOverviewCatalogRepo{capabilityCatalogRepo: &capabilityCatalogRepo{}}
	model := &overviewModelFixture{capabilityBatchModel: &capabilityBatchModel{}, oversized: true}
	controller := &CatalogBatchController{
		CatalogAI: repo, Gemini: model, RunRepo: &capabilityRunRepo{}, TokenBudget: 1000,
		Now: time.Now, NewID: func() string { return "id" },
	}
	_, err := controller.RunCatalogEvaluation(context.Background(), CatalogEvaluationInput{
		AIRunID: "run-2", BusinessID: "business-1", Presentation: "overview",
	})
	if err == nil || !strings.Contains(err.Error(), "reply limit") {
		t.Fatalf("too-long Gemini customer answer accepted: %v", err)
	}
}

func TestCatalogOverviewRejectsRepeatedOrUnknownItemIDs(t *testing.T) {
	batch := CatalogAIBatchPayload{Items: []CatalogAIItem{{ID: "a"}, {ID: "b"}}}
	for _, ids := range [][]string{{"a"}, {"a", "a"}, {"a", "b", "foreign"}} {
		if err := validateOverviewItemCoverage(batch, ids); err == nil {
			t.Fatalf("invalid AI coverage accepted: %v", ids)
		}
	}
	if err := validateOverviewItemCoverage(batch, []string{"b", "a"}); err != nil {
		t.Fatalf("out-of-order but complete coverage incorrectly rejected: %v", err)
	}
}
