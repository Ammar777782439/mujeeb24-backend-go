package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func TestMerchantInvalidFinalCallEnvelopeRetriesOnceAndFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		recover bool
	}{
		{name: "valid_on_retry", recover: true},
		{name: "invalid_again_must_fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rounds := 0
			provider := &nativeContractProvider{name: "gemini"}
			provider.call = func(req native.Request) (native.Response, error) {
				rounds++
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
					t.Error("strict final transport must be preserved")
				}
				if rounds == 2 && (len(body.Messages) == 0 ||
					!strings.Contains(body.Messages[0].Content, "Final tool-call envelope recovery") ||
					!strings.Contains(body.Messages[0].Content, "submit_merchant_catalog_proposal")) {
					t.Error("bounded final retry missing trusted instructions")
				}
				if rounds == 1 || !tc.recover {
					return native.Response{}, &native.MerchantCatalogCallEnvelopeError{Reason: "invalid_arguments_json"}
				}
				final := marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"كم سعر المنتج؟"}`)
				encoded, _ := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message":       map[string]any{"role": "assistant", "content": final},
					}},
					"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
				})
				return native.Response{Payload: encoded, Model: "deepseek-v4-pro", InputTokens: 100, OutputTokens: 20}, nil
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف جلكسي اس 8", DefaultCurrency: "YER",
			})
			if rounds != 2 {
				t.Fatalf("expected one retry, got %d calls", rounds)
			}
			if !tc.recover {
				if err == nil {
					t.Fatalf("second malformed final call was accepted: %+v", proposal)
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
				t.Fatalf("unexpected successful proposal after retry: %+v", proposal)
			}
		})
	}
}

func TestMerchantInvalidDiscoveryCallEnvelopeNeverTriggersFinalRetry(t *testing.T) {
	calls := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(native.Request) (native.Response, error) {
		calls++
		return native.Response{}, &native.MerchantCatalogCallEnvelopeError{Reason: "missing_call_id"}
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
		Message: "أضف جلكسي اس 8", DefaultCurrency: "YER",
		Capabilities: &nativeMerchantReadStub{},
	})
	if err == nil || calls != 1 {
		t.Fatalf("read tool authorization stage unexpectedly retried calls=%d err=%v", calls, err)
	}
}
