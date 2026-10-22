package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

type merchantListingPages struct {
	cursors []string
}

func (r *merchantListingPages) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition{{
		Name: "merchant_catalog_list_items",
		Description: "Read the selected catalog items by page",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cursor": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
				"status": map[string]any{"type": "string"},
			},
			"required": []string{},
		},
	}}
}

func (r *merchantListingPages) Execute(_ context.Context, _ merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, name string, raw []byte) (merchantcatalogai.MerchantCatalogDiscoveryResult, error) {
	if name != "merchant_catalog_list_items" {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("unexpected tool")
	}
	var params struct {
		Cursor *string `json:"cursor"`
		Status *string `json:"status"`
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, err
	}
	if params.Status != nil || params.Limit != 50 {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("unrequested listing filter")
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	r.cursors = append(r.cursors, cursor)
	switch cursor {
	case "":
		return merchantcatalogai.MerchantCatalogDiscoveryResult{
			Data: []map[string]string{{"name": "هاتف"}, {"name": "ساعة"}},
			HasMore: true, NextCursor: "server-next",
			Operation: "merchant_catalog_list_items",
		}, nil
	case "server-next":
		return merchantcatalogai.MerchantCatalogDiscoveryResult{
			Data: []map[string]string{{"name": "سماعة"}},
			Operation: "merchant_catalog_list_items",
		}, nil
	default:
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("invented listing cursor")
	}
}

func TestNativeMerchantAllProductsReadsAllPagesWithoutMutation(t *testing.T) {
	reads := &merchantListingPages{}
	round := 0
	deepseek := &nativeContractProvider{name: "deepseek"}
	deepseek.call = func(req native.Request) (native.Response, error) {
		round++
		var body struct {
			ToolChoice string `json:"tool_choice"`
			Messages []struct {
				Role string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			ResponseFormat map[string]any `json:"response_format"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			return native.Response{}, err
		}
		var choice map[string]any
		switch round {
		case 1, 2:
			if body.ToolChoice != "auto" || len(body.Tools) != 1 || body.Tools[0].Function.Name != "merchant_catalog_list_items" {
				t.Error("listing tool was not offered")
			}
			if round == 2 && (len(body.Messages) != 4 || !strings.Contains(body.Messages[3].Content, `"has_more":true`) || !strings.Contains(body.Messages[3].Content, "server-next")) {
				t.Error("first page and server cursor missing")
			}
			cursor := any(nil)
			if round == 2 {
				cursor = "server-next"
			}
			args, _ := json.Marshal(map[string]any{"cursor": cursor, "status": nil, "limit": 50})
			choice = map[string]any{"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{map[string]any{
					"id": "list-call-" + string(rune('0'+round)), "type": "function",
					"function": map[string]any{"name": "merchant_catalog_list_items", "arguments": string(args)},
				}},
			}}
		case 3:
			if body.ToolChoice != "auto" || len(body.Messages) != 6 ||
				!strings.Contains(body.Messages[5].Content, `"has_more":false`) {
				t.Error("terminal listing page was not delivered")
			}
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "Read complete"}}
		case 4:
			if len(body.Tools) != 0 || body.ResponseFormat["type"] != "json_schema" {
				t.Error("final proposal must be isolated from read tools")
			}
			proposal := `{"schema_version":3,"status":"resolved","operation":"ask_merchant","response_text":"منتجات الكتالوج: هاتف، ساعة، سماعة."}`
			envelope, _ := json.Marshal(map[string]string{"canonical_proposal_json": proposal})
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(envelope)}}
		default:
			return native.Response{}, errors.New("unexpected extra inference round")
		}
		encoded, _ := json.Marshal(map[string]any{"choices": []any{choice},
			"usage": map[string]any{"prompt_tokens": 30, "completion_tokens": 10}})
		return native.Response{Payload: encoded, Model: "deepseek-flash", InputTokens: 30, OutputTokens: 10}, nil
	}
	gemini := &nativeContractProvider{name: "gemini", call: func(native.Request) (native.Response, error) {
		return native.Response{}, &native.StatusError{Provider: "gemini", Code: 503}
	}}
	router, err := native.NewRouter(gemini, deepseek, native.Policy{ConsecutiveFailures: 3, Cooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(client, "mujeeb-merchant", 4096, 300000)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
		Message: "أريد كل المنتجات", DefaultCurrency: "YER",
		EntityContract: []byte(`{"required":"item"}`), Capabilities: reads,
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Operation != merchantcatalogai.OperationAskMerchant ||
		proposal.Status != merchantcatalogai.StatusResolved || proposal.IsMutation() ||
		round != 4 || len(reads.cursors) != 2 || reads.cursors[0] != "" || reads.cursors[1] != "server-next" ||
		!strings.Contains(proposal.ResponseText, "هاتف") ||
		!strings.Contains(proposal.ResponseText, "ساعة") ||
		!strings.Contains(proposal.ResponseText, "سماعة") {
		t.Fatalf("all-products result incomplete: proposal=%+v cursors=%v rounds=%d", proposal, reads.cursors, round)
	}
}
