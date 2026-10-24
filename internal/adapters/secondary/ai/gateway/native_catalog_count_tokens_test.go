package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
)

func TestNativeCatalogUsesConservativeBudgetWithoutProviderTokenizer(t *testing.T) {
	provider := &customerNativeProvider{name: "deepseek"}
	router, err := native.NewRouter(provider, nil, native.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCatalogAdapter(client, "mujeeb-catalog", 4096, 32768)
	if err != nil {
		t.Fatal(err)
	}
	request := chatCompletionRequest{
		Model: "mujeeb-catalog",
		Messages: []chatCompletionMessage{
			{Role: "system", Content: "Only verified catalog facts"},
			{Role: "user", Content: "Find all matching items"},
		},
		MaxTokens: 1024,
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := adapter.count(context.Background(), request)
	if err != nil || actual != len(encoded)+4096 {
		t.Fatalf("unsafe admission count %d: %v", actual, err)
	}
	measured, exact, standby, err := client.CountTokens(context.Background(), request)
	if err != nil || measured != 0 || exact || standby {
		t.Fatalf("unexpected count: %d exact=%t standby=%t err=%v", measured, exact, standby, err)
	}
}
