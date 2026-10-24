package services

import (
	"context"
	"fmt"
	"testing"
)

func TestCatalogBatchBudgetAccommodatesObserved9625Estimate(t *testing.T) {
	budget, err := NativeCatalogTokenBudget(32768, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if budget < 9625 {
		t.Fatalf("regression: valid 9625-token estimate exceeds route budget %d", budget)
	}
	projection := CatalogAIProjection{}
	for i := 0; i < 12; i++ {
		projection.Items = append(projection.Items, CatalogAIItem{ID: fmt.Sprintf("item-%02d", i)})
	}
	adapter := &catalogTokenEstimatorStub{countFn: func(BatchEvaluationInput) (int, error) {
		return 9625, nil
	}}
	controller := &CatalogBatchController{Evaluator: adapter, TokenBudget: budget}
	batches, err := controller.splitIntoBatches(context.Background(), projection, CatalogEvaluationInput{AIRunID: "run", BusinessID: "business"}, 1)
	if err != nil {
		t.Fatalf("safe context budget rejected observed request: %v", err)
	}
	if len(batches) != 1 || len(batches[0].Items) != 12 {
		t.Fatalf("expected complete 12-item batch; got %+v", batches)
	}
}

func TestCatalogBatchingPreservesLargeItemCollection(t *testing.T) {
	const items = 1024
	projection := CatalogAIProjection{}
	for i := 0; i < items; i++ {
		projection.Items = append(projection.Items, CatalogAIItem{
			ID:         fmt.Sprintf("item-%05d", i),
			Name:       fmt.Sprintf("Catalog item %d", i),
			Attributes: map[string]any{"size": i},
		})
	}
	adapter := &catalogTokenEstimatorStub{countFn: func(input BatchEvaluationInput) (int, error) {
		return 350 + 75*len(input.Batch.Items), nil
	}}
	controller := &CatalogBatchController{Evaluator: adapter, TokenBudget: 900}
	batches, err := controller.splitIntoBatches(context.Background(), projection, CatalogEvaluationInput{AIRunID: "run", BusinessID: "business"}, 1)
	if err != nil {
		t.Fatalf("large catalog batching failed: %v", err)
	}
	if len(batches) < 2 {
		t.Fatal("large catalog was not split into bounded batches")
	}
	seen := make(map[string]int, items)
	for _, batch := range batches {
		for _, item := range batch.Items {
			seen[item.ID]++
		}
	}
	if len(seen) != items {
		t.Fatalf("lost commercial items during split: %d of %d preserved", len(seen), items)
	}
	for _, item := range projection.Items {
		if seen[item.ID] != 1 {
			t.Fatalf("item %s appeared %d times, expected one", item.ID, seen[item.ID])
		}
	}
}
