package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

type allItemsPages struct{ cursors []string }

func (p *allItemsPages) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition{{Name: "merchant_catalog_list_items", Parameters: map[string]any{
		"type": "object", "properties": map[string]any{
			"cursor": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}, "status": map[string]any{"type": "string"},
		}, "required": []string{},
	}}}
}
func (p *allItemsPages) Execute(_ context.Context, _ merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, name string, raw []byte) (merchantcatalogai.MerchantCatalogDiscoveryResult, error) {
	if name != "merchant_catalog_list_items" {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("wrong tool")
	}
	var args struct {
		Cursor *string `json:"cursor"`
		Status *string `json:"status"`
		Limit  int     `json:"limit"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, err
	}
	if args.Status != nil || args.Limit != 50 {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("unexpected filter")
	}
	cursor := ""
	if args.Cursor != nil {
		cursor = *args.Cursor
	}
	p.cursors = append(p.cursors, cursor)
	switch cursor {
	case "":
		return merchantcatalogai.MerchantCatalogDiscoveryResult{Operation: name, Data: []string{"هاتف", "ساعة"}, HasMore: true, NextCursor: "next-from-server"}, nil
	case "next-from-server":
		return merchantcatalogai.MerchantCatalogDiscoveryResult{Operation: name, Data: []string{"سماعة"}}, nil
	default:
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("invented cursor")
	}
}

func TestMerchantListAllProductsPaginatesWithoutMutation(t *testing.T) {
	pages := &allItemsPages{}
	round := 0
	deepseek := &nativeContractProvider{name: "deepseek"}
	deepseek.call = func(req native.Request) (native.Response, error) {
		round++
		var query struct {
			ToolChoice string `json:"tool_choice"`
			Messages   []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Tools          []map[string]any `json:"tools"`
			ResponseFormat map[string]any   `json:"response_format"`
		}
		if err := json.Unmarshal(req.Payload, &query); err != nil {
			return native.Response{}, err
		}
		var choice map[string]any
		switch round {
		case 1, 2:
			if query.ToolChoice != "auto" || len(query.Tools) != 1 {
				t.Error("wrong discovery tools")
			}
			if round == 2 && (len(query.Messages) != 4 || !strings.Contains(query.Messages[3].Content, `"has_more":true`) || !strings.Contains(query.Messages[3].Content, "next-from-server")) {
				t.Error("first page missing")
			}
			var cursor any
			if round == 2 {
				cursor = "next-from-server"
			}
			args, _ := json.Marshal(map[string]any{"cursor": cursor, "status": nil, "limit": 50})
			choice = map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": fmt.Sprintf("list-%d", round), "type": "function", "function": map[string]any{"name": "merchant_catalog_list_items", "arguments": string(args)},
			}}}}
		case 3:
			if len(query.Messages) != 6 || !strings.Contains(query.Messages[5].Content, `"has_more":false`) {
				t.Error("last page missing")
			}
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "done"}}
		case 4:
			if len(query.Tools) != 0 || query.ResponseFormat["type"] != "json_schema" {
				t.Error("not a strict final proposal")
			}
			proposal := `{"schema_version":3,"status":"resolved","operation":"ask_merchant","response_text":"منتجات الكتالوج: هاتف، ساعة، سماعة."}`
			payload, _ := json.Marshal(map[string]string{"canonical_proposal_json": proposal})
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": string(payload)}}
		default:
			return native.Response{}, errors.New("unexpected extra round")
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{choice}, "usage": map[string]any{"prompt_tokens": 40, "completion_tokens": 15}})
		return native.Response{Payload: body, Model: "deepseek-flash", InputTokens: 40, OutputTokens: 15}, nil
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
	got, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1", Message: "أريد كل المنتجات", DefaultCurrency: "YER", Capabilities: pages,
	})
	if err != nil {
		t.Fatal(err)
	}
	if round != 4 || len(pages.cursors) != 2 || pages.cursors[0] != "" || pages.cursors[1] != "next-from-server" || got.IsMutation() ||
		got.Status != merchantcatalogai.StatusResolved || got.Operation != merchantcatalogai.OperationAskMerchant ||
		!strings.Contains(got.ResponseText, "هاتف") || !strings.Contains(got.ResponseText, "ساعة") || !strings.Contains(got.ResponseText, "سماعة") {
		t.Fatalf("incomplete merchant listing: %+v cursors=%v rounds=%d", got, pages.cursors, round)
	}
}
