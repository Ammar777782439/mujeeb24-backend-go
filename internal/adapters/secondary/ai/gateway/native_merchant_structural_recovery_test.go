package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func TestNativeMerchantRetriesMalformedDeepSeekFinalOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		recover bool
	}{
		{name: "valid_second_attempt", recover: true},
		{name: "repeated_invalid_must_fail_closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			provider := &nativeContractProvider{name: "gemini"}
			provider.call = func(req native.Request) (native.Response, error) {
				calls++
				var body struct {
					Messages []struct {
						Role    string `json:"role"`
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
				if calls == 2 && !strings.Contains(body.Messages[0].Content, "Strict wire recovery") {
					t.Error("repair request was not included in trusted instruction")
				}
				if calls == 1 || !tc.recover {
					return native.Response{}, &native.MerchantCatalogSchemaError{Cause: errors.New("create: model result has extra or missing object fields")}
				}
				response := marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"كم سعر المنتج؟"}`)
				raw, _ := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message": map[string]any{"role": "assistant", "content": response},
					}},
					"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 25},
				})
				return native.Response{Payload: raw, Model: "deepseek-v4-pro", InputTokens: 100, OutputTokens: 25}, nil
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف جلكسي اس 8", DefaultCurrency: "YER",
			})
			if calls != 2 {
				t.Fatalf("expected exactly two bounded attempts, got %d", calls)
			}
			if !tc.recover {
				if err == nil {
					t.Fatalf("repeated bad final was accepted: %+v", proposal)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if proposal.Operation != merchantcatalogai.OperationAskMerchant ||
				proposal.Status != merchantcatalogai.StatusNeedsMoreData ||
				proposal.Create != nil || proposal.SchemaVersion != merchantcatalogai.ProposalSchemaVersion {
				t.Fatalf("repaired proposal must only ask for missing facts: %+v", proposal)
			}
		})
	}
}
