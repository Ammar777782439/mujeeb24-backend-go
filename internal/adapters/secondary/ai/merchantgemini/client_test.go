package merchantgemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mockCapabilityDispatcher struct {
	defs      []ports.AICapabilityDefinition
	executeFn func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error)
}

func (m mockCapabilityDispatcher) Definitions() []ports.AICapabilityDefinition {
	return m.defs
}

func (m mockCapabilityDispatcher) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, execCtx, name, rawParams)
	}
	return ports.AICapabilityResult{}, nil
}

func TestMerchantClientChatSimple(t *testing.T) {
	const apiKey = "test-merchant-gemini-key"
	var received struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), "key="+apiKey) {
			t.Errorf("expected key in URL: %s", r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"أهلاً بك يا تاجرنا العزيز! كيف أقدر أساعدك اليوم؟"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         apiKey,
		Model:          "gemini-2.5-flash",
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	output, err := client.Chat(context.Background(), ports.MerchantAIChatInput{
		BusinessID:     "biz-1",
		PrincipalID:    "user-1",
		SessionID:      "sess-1",
		CurrentMessage: "مرحبا",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !strings.Contains(output.ResponseText, "أهلاً بك يا تاجرنا") {
		t.Fatalf("unexpected response text: %s", output.ResponseText)
	}
	if output.Action != "ask_clarification" { // because it has "?"
		t.Fatalf("expected action=ask_clarification, got %s", output.Action)
	}
	if len(received.Contents) != 1 || received.Contents[0].Role != "user" || received.Contents[0].Parts[0].Text != "مرحبا" {
		t.Fatalf("unexpected received contents: %#v", received.Contents)
	}
	if !strings.Contains(received.SystemInstruction.Parts[0].Text, "مجيب كونسيرج") {
		t.Fatalf("expected merchant system prompt: %#v", received.SystemInstruction)
	}
}

func TestMerchantClientChatMultiTurnHistory(t *testing.T) {
	const apiKey = "test-merchant-gemini-key"
	var receivedContents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		receivedContents = req.Contents
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"تم تحديد الكتالوج الرئيسي."}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         apiKey,
		Model:          "gemini-2.5-flash",
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	now := time.Now().UTC()
	history := []ports.MerchantAIMessageRecord{
		{ID: "m-1", BusinessID: "biz-1", SessionID: "sess-1", SenderType: "merchant", Text: "أريد إضافة منتج", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "m-2", BusinessID: "biz-1", SessionID: "sess-1", SenderType: "assistant", Text: "في أي كتالوج تحب تضيفه؟", CreatedAt: now.Add(-1 * time.Minute)},
		{ID: "m-3", BusinessID: "biz-1", SessionID: "sess-1", SenderType: "merchant", Text: "الكتالوج الرئيسي", CreatedAt: now},
	}

	output, err := client.Chat(context.Background(), ports.MerchantAIChatInput{
		BusinessID:     "biz-1",
		PrincipalID:    "user-1",
		SessionID:      "sess-1",
		CurrentMessage: "الكتالوج الرئيسي",
		History:        history,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ResponseText != "تم تحديد الكتالوج الرئيسي." {
		t.Fatalf("unexpected output: %#v", output)
	}
	if len(receivedContents) != 3 {
		t.Fatalf("expected 3 turns in history, got %d", len(receivedContents))
	}
	if receivedContents[0].Role != "user" || receivedContents[0].Parts[0].Text != "أريد إضافة منتج" {
		t.Errorf("turn 0 mismatch: %#v", receivedContents[0])
	}
	if receivedContents[1].Role != "model" || receivedContents[1].Parts[0].Text != "في أي كتالوج تحب تضيفه؟" {
		t.Errorf("turn 1 mismatch: %#v", receivedContents[1])
	}
	if receivedContents[2].Role != "user" || receivedContents[2].Parts[0].Text != "الكتالوج الرئيسي" {
		t.Errorf("turn 2 mismatch: %#v", receivedContents[2])
	}
}

func TestMerchantClientChatFunctionCalling(t *testing.T) {
	const apiKey = "test-merchant-gemini-key"
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// Model calls author_catalog_item
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"author_catalog_item","args":{"catalog_id":"cat-1","name":"عطر الشيخ","pricing_mode":"fixed","amount":"8000","currency":"YER"}}}],"role":"model"},"finishReason":"STOP"}]}`))
			return
		}
		// Model receives functionResponse and produces friendly confirmation
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"تمت إضافة عطر الشيخ بنجاح بسعر 8,000 ريال يمني في الكتالوج الرئيسي!"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	executedTool := false
	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{Name: "author_catalog_item", Description: "Author catalog item"},
		},
		executeFn: func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
			if name == "author_catalog_item" {
				executedTool = true
				return ports.AICapabilityResult{
					Data: map[string]any{"success": true, "item_id": "item-123"},
				}, nil
			}
			return ports.AICapabilityResult{}, nil
		},
	}

	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         apiKey,
		Model:          "gemini-2.5-flash",
		RequestTimeout: time.Second,
		Capabilities:   dispatcher,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	output, err := client.Chat(context.Background(), ports.MerchantAIChatInput{
		BusinessID:     "biz-1",
		PrincipalID:    "user-1",
		SessionID:      "sess-1",
		CurrentMessage: "ضيف عطر الشيخ ب 8000 ريال",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if !executedTool {
		t.Fatal("expected tool author_catalog_item to be executed")
	}
	if callCount != 2 {
		t.Fatalf("expected 2 turns (1 tool call + 1 response), got %d", callCount)
	}
	if output.Action != "catalog_updated" {
		t.Fatalf("expected action=catalog_updated, got %s", output.Action)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0] != "author_catalog_item" {
		t.Fatalf("unexpected ToolCalls: %#v", output.ToolCalls)
	}
	if !strings.Contains(output.ResponseText, "عطر الشيخ") {
		t.Fatalf("unexpected response: %s", output.ResponseText)
	}
}

func TestMerchantClientRejectsMissingAPIKey(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "https://example.com", APIKey: "", Model: "gemini-2.5-flash"}); err == nil {
		t.Fatal("expected error on missing API key")
	}
}
