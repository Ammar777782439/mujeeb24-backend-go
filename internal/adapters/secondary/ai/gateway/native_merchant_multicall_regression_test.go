package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func TestNativeMerchantMultipleReadToolsInOneRound(t *testing.T) {
	caps := &nativeMerchantReadStub{}
	round := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		round++
		var body struct {
			Messages []struct {
				Role       string                   `json:"role"`
				ToolCallID string                   `json:"tool_call_id"`
				ToolCalls  []nativeMerchantChatCall `json:"tool_calls"`
			} `json:"messages"`
			Tools             []map[string]any `json:"tools"`
			ParallelToolCalls *bool            `json:"parallel_tool_calls"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			return native.Response{}, err
		}
		if body.ParallelToolCalls != nil {
			t.Error("merchant parallel tool selection disabled")
		}
		var choice map[string]any
		switch round {
		case 1:
			if len(body.Tools) != 1 {
				t.Error("missing read tools")
			}
			choice = map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
				map[string]any{"id": "read-1", "type": "function", "function": map[string]any{"name": "read_catalog", "arguments": `{"query":"coffee"}`}},
				map[string]any{"id": "read-2", "type": "function", "function": map[string]any{"name": "read_catalog", "arguments": `{"query":"tea"}`}},
			}}}
		case 2:
			if len(body.Messages) != 5 || len(body.Messages[2].ToolCalls) != 2 || body.Messages[3].ToolCallID != "read-1" || body.Messages[4].ToolCallID != "read-2" {
				t.Error("missing read responses or call IDs")
			}
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "discovery complete"}}
		case 3:
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"ما السعر؟"}`)}}
		default:
			return native.Response{}, errors.New("unexpected merchant round")
		}
		payload, _ := json.Marshal(map[string]any{"choices": []any{choice}, "usage": map[string]any{"prompt_tokens": 40, "completion_tokens": 20}})
		return native.Response{Payload: payload, Model: "deepseek", InputTokens: 40, OutputTokens: 20}, nil
	}
	client := nativeContractClient(t, provider)
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(client, "merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1", Message: "add phone", DefaultCurrency: "YER", Capabilities: caps})
	if err != nil {
		t.Fatal(err)
	}
	if round != 3 || caps.calls != 2 || proposal.Operation != merchantcatalogai.OperationAskMerchant {
		t.Fatalf("unexpected result: rounds=%d reads=%d proposal=%+v", round, caps.calls, proposal)
	}
}

func TestNativeMerchantRecoversInvalidCreateWithClarification(t *testing.T) {
	round := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		round++
		var content string
		if round == 1 {
			wire := emptyCatalogWireProposal()
			wire.Operation = merchantcatalogai.OperationCreate
			wire.Status = merchantcatalogai.StatusResolved
			wire.ResponseText = "جاهز"
			wire.Update.ItemID = ""
			wire.Create.Fields = []catalogWireField{{Name: "name", ValueJSON: `"جلكسي اس 8"`}, {Name: "item_type", ValueJSON: `"physical_good"`}}
			b, _ := json.Marshal(wire)
			content = string(b)
		} else {
			content = marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"كم السعر؟"}`)
		}
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": content}}}, "usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20}})
		return native.Response{Payload: b, Model: "deepseek", InputTokens: 50, OutputTokens: 20}, nil
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1", Message: "add phone", DefaultCurrency: "YER"})
	if err != nil {
		t.Fatal(err)
	}
	if round != 2 || result.Status != merchantcatalogai.StatusNeedsMoreData || result.Create != nil || result.Operation != merchantcatalogai.OperationAskMerchant {
		t.Fatalf("unsafe create accepted: rounds=%d result=%+v", round, result)
	}
}
