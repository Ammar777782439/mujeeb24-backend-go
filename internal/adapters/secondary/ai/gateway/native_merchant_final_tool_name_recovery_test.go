package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func TestMerchantWrongFinalToolNameRetriesOnceWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		recover bool
	}{
		{name: "valid_final_after_retry", recover: true},
		{name: "second_wrong_name_fails_closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			provider := &nativeContractProvider{name: "gemini"}
			provider.call = func(req native.Request) (native.Response, error) {
				calls++
				var body struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
					Tools          []map[string]any `json:"tools"`
					ResponseFormat map[string]any   `json:"response_format"`
				}
				if err := json.Unmarshal(req.Payload, &body); err != nil {
					return native.Response{}, err
				}
				if len(body.Tools) != 0 || body.ResponseFormat["type"] != "json_schema" {
					t.Error("strict final proposal transport lost")
				}
				if calls == 2 && (len(body.Messages) == 0 ||
					!strings.Contains(body.Messages[0].Content, "Final tool name recovery") ||
					!strings.Contains(body.Messages[0].Content, "submit_merchant_catalog_proposal")) {
					t.Error("retry did not require the exact final tool name")
				}
				if calls == 1 || !tc.recover {
					return native.Response{}, &native.MerchantCatalogToolNameError{}
				}
				final := marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"كم سعر جلكسي اس 8؟"}`)
				raw, _ := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message":       map[string]any{"role": "assistant", "content": final},
					}},
					"usage": map[string]any{"prompt_tokens": 80, "completion_tokens": 25},
				})
				return native.Response{Payload: raw, Model: "deepseek-v4-pro", InputTokens: 80, OutputTokens: 25}, nil
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(
				nativeContractClient(t, provider), "merchant", 2048, 300000,
			)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف جلكسي اس 8", DefaultCurrency: "YER",
			})
			if calls != 2 {
				t.Fatalf("must attempt exactly twice, got %d", calls)
			}
			if !tc.recover {
				if err == nil {
					t.Fatalf("second unoffered function must fail closed: %+v", proposal)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if proposal.Operation != merchantcatalogai.OperationAskMerchant ||
				proposal.Status != merchantcatalogai.StatusNeedsMoreData ||
				proposal.Create != nil || proposal.Update != nil ||
				proposal.SchemaVersion != merchantcatalogai.ProposalSchemaVersion {
				t.Fatalf("final response must only request merchant clarification: %+v", proposal)
			}
		})
	}
}

func TestMerchantDiscoveryErrorDoesNotRetryAsFinalToolName(t *testing.T) {
	calls := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(native.Request) (native.Response, error) {
		calls++
		return native.Response{}, &native.MerchantCatalogToolNameError{}
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(
		nativeContractClient(t, provider), "merchant", 2048, 300000,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
		Message: "أضف جلكسي اس 8", DefaultCurrency: "YER", Capabilities: &nativeMerchantReadStub{},
	})
	if err == nil || calls != 1 {
		t.Fatalf("discovery must never use final repair retry, calls=%d err=%v", calls, err)
	}
}
