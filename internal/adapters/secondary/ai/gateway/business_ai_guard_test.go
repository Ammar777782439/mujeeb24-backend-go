package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

type gatewayAIGuard struct {
	calls    int
	disabled bool
}

func (g *gatewayAIGuard) Check(_ context.Context, _ string) error {
	g.calls++
	if g.disabled {
		return ports.ErrBusinessAIDisabled
	}
	return nil
}

func TestDisabledCatalogBlocksTokenCountBatchAndFinalInference(t *testing.T) {
	requests := 0
	p := &nativeContractProvider{name: "future-provider", call: func(native.Request) (native.Response, error) {
		requests++
		return native.Response{}, errors.New("provider must not be called")
	}}
	client := nativeContractClient(t, p)
	adapter, err := NewNativeCatalogAdapter(client, "mujeeb-catalog", 1024, 8192)
	if err != nil {
		t.Fatal(err)
	}
	g := &gatewayAIGuard{disabled: true}
	adapter.SetAIExecutionGuard(g)
	batch := services.BatchEvaluationInput{BusinessID: "b-1", BatchNumber: 1}
	final := services.FinalEvaluationInput{BusinessID: "b-1"}
	if _, err := adapter.CountBatchTokens(context.Background(), batch); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("countTokens was not blocked: %v", err)
	}
	if _, err := adapter.EvaluateBatch(context.Background(), batch); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("catalog batch was not blocked: %v", err)
	}
	if _, err := adapter.CountFinalTokens(context.Background(), final, ""); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("final countTokens was not blocked: %v", err)
	}
	if _, _, err := adapter.FinalEvaluateWithDetails(context.Background(), final, ""); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("catalog final model call was not blocked: %v", err)
	}
	if requests != 0 || g.calls != 4 {
		t.Fatalf("disabled model still called: provider=%d guard=%d", requests, g.calls)
	}
}

func TestMerchantModelRechecksPolicyBeforeEachToolRound(t *testing.T) {
	g := &gatewayAIGuard{}
	requests := 0
	read := &nativeMerchantReadStub{}
	p := &nativeContractProvider{name: "future-provider"}
	p.call = func(req native.Request) (native.Response, error) {
		requests++
		if requests != 1 {
			t.Fatal("model called after merchant disabled AI")
		}
		var sent struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal(req.Payload, &sent); err != nil || len(sent.Tools) == 0 {
			t.Fatalf("merchant discovery tools missing: %v", err)
		}
		g.disabled = true
		body := []byte(`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"read_catalog","arguments":"{\"query\":\"coffee\"}"}}]}}],"usage":{"prompt_tokens":50,"completion_tokens":15}}`)
		return native.Response{Payload: body, Model: "future-model", InputTokens: 50, OutputTokens: 15}, nil
	}
	client := nativeContractClient(t, p)
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(client, "mujeeb-merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	adapter.SetAIExecutionGuard(g)
	_, err = adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "b-1", PrincipalID: "owner-1", SessionID: "session-1",
		Message: "عدل المنتج", DefaultCurrency: "YER",
		EntityContract: []byte(`{"required":"item"}`), Capabilities: read,
	})
	if !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("second merchant model round was not blocked: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one provider request before opt-out, got %d", requests)
	}
	if g.calls < 2 {
		t.Fatalf("merchant did not recheck opt-out before next model round: %d", g.calls)
	}
}
