package services

import "testing"

func TestNativeCatalogTokenBudgetReservesOutputAndMargin(t *testing.T) {
	budget, err := NativeCatalogTokenBudget(32768, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 25396 {
		t.Fatalf("budget=%d; want 25396", budget)
	}
	if budget+4096+3276 != 32768 {
		t.Fatalf("unsafe context allocation: %d", budget)
	}
}

func TestNativeCatalogTokenBudgetFailsClosedForUndersizedFallback(t *testing.T) {
	for _, tc := range []struct{ context, output int }{
		{8192, 8000},
		{4096, 2048},
		{0, 1024},
		{32768, 0},
	} {
		if _, err := NativeCatalogTokenBudget(tc.context, tc.output); err == nil {
			t.Fatalf("accepted context=%d output=%d without safety margin", tc.context, tc.output)
		}
	}
}
