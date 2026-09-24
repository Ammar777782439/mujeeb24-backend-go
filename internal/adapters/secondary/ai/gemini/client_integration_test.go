package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// mockRealCapabilityDispatcher is used for integration tests to provide real data to the live Gemini API.
type mockRealCapabilityDispatcher struct {
	pages [][]map[string]any
}

func (m *mockRealCapabilityDispatcher) Definitions() []ports.AICapabilityDefinition {
	return []ports.AICapabilityDefinition{
		{
			Name:        "catalog_data",
			Description: "Search and retrieve catalog items and products.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{
						"type": "string",
						"enum": []string{"list_catalog_items"},
					},
					"limit": map[string]any{
						"type": "integer",
					},
					"cursor": map[string]any{
						"type": "string",
					},
				},
				"required": []string{"operation"},
			},
		},
	}
}

func (m *mockRealCapabilityDispatcher) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
	var params struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(rawParams, &params)

	pageIdx := 0
	if params.Cursor == "page-2" {
		pageIdx = 1
	} else if params.Cursor == "page-3" {
		pageIdx = 2
	}

	if pageIdx >= len(m.pages) {
		fmt.Printf("[TRACE] Tool called: %s (pageIdx: %d). Result: has_more=false\n", name, pageIdx)
		return ports.AICapabilityResult{
			Data: map[string]any{"items": []any{}, "has_more": false, "next_cursor": ""},
		}, nil
	}

	hasMore := pageIdx < len(m.pages)-1
	nextCursor := ""
	if hasMore {
		if pageIdx == 0 {
			nextCursor = "page-2"
		} else if pageIdx == 1 {
			nextCursor = "page-3"
		}
	}
	fmt.Printf("[TRACE] Tool called: %s (pageIdx: %d). Result: has_more=%v, next_cursor=%s\n", name, pageIdx, hasMore, nextCursor)

	return ports.AICapabilityResult{
		Data: map[string]any{
			"items":       m.pages[pageIdx],
			"has_more":    hasMore,
			"next_cursor": nextCursor,
		},
	}, nil
}

func TestLiveGeminiPaginationAndReasoning(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping live integration test. Set GEMINI_API_KEY to run.")
	}

	dispatcher := &mockRealCapabilityDispatcher{
		pages: [][]map[string]any{
			{
				{"id": "item-1", "name": "iPhone 15", "price": 4000, "description": "Apple smartphone"},
				{"id": "item-2", "name": "Samsung S24", "price": 3800, "description": "Android smartphone"},
			},
			{
				{"id": "item-3", "name": "MacBook Pro", "price": 8000, "description": "Apple laptop"},
				{"id": "item-4", "name": "Dell XPS", "price": 7500, "description": "Windows laptop"},
			},
		},
	}

	client, err := NewClient(Config{
		BaseURL:          "https://generativelanguage.googleapis.com",
		APIKey:           apiKey,
		Model:            "gemini-3.5-flash",
		RequestTimeout:   30 * time.Second,
		Capabilities:     dispatcher,
		SafetyTurnBudget: 5,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tests := []struct {
		name       string
		prompt     string
		check      func(t *testing.T, proposal ports.AIDecisionProposal)
	}{
		{
			name:   "Exact Product (stops early)",
			prompt: "هل لديكم آيفون 15 وكم سعره؟",
			check: func(t *testing.T, proposal ports.AIDecisionProposal) {
				if !strings.Contains(proposal.ResponseText, "4000") {
					t.Errorf("Expected price in response, got: %s", proposal.ResponseText)
				}
				if proposal.RequestedAction != "answer" {
					t.Errorf("Expected action answer, got: %s", proposal.RequestedAction)
				}
			},
		},
		{
			name:   "Comparison (requires multiple pages)",
			prompt: "أريد مقارنة بين أجهزة اللابتوب والهواتف الذكية المتوفرة لديكم.",
			check: func(t *testing.T, proposal ports.AIDecisionProposal) {
				if !strings.Contains(strings.ToLower(proposal.ResponseText), "xps") && !strings.Contains(proposal.ResponseText, "8000") {
					t.Errorf("Expected response to mention laptops from page 2, got: %s", proposal.ResponseText)
				}
			},
		},
		{
			name:   "No Match",
			prompt: "هل تبيعون سيارات مرسيدس؟",
			check: func(t *testing.T, proposal ports.AIDecisionProposal) {
				if !strings.Contains(proposal.ResponseText, "لا") && !strings.Contains(proposal.ResponseText, "غير متوفر") && !strings.Contains(proposal.ResponseText, "عذر") {
					t.Errorf("Expected AI to reject or apologize, response: %s", proposal.ResponseText)
				}
			},
		},
		{
			name:   "Exploration (category browsing)",
			prompt: "ما هي المنتجات الموجودة عندكم بشكل عام؟",
			check: func(t *testing.T, proposal ports.AIDecisionProposal) {
				// Should list items from both pages, meaning it paginated
				if !strings.Contains(proposal.ResponseText, "15") && !strings.Contains(strings.ToLower(proposal.ResponseText), "iphone") {
					t.Errorf("Expected response to mention page 1 items, got: %s", proposal.ResponseText)
				}
				if !strings.Contains(strings.ToLower(proposal.ResponseText), "xps") && !strings.Contains(proposal.ResponseText, "8000") {
					t.Errorf("Expected response to mention page 2 items, got: %s", proposal.ResponseText)
				}
			},
		},
		{
			name:   "Alternative (suggest available product)",
			prompt: "هل لديكم آيفون 16؟",
			check: func(t *testing.T, proposal ports.AIDecisionProposal) {
				// We don't have iPhone 16. Should suggest iPhone 15 or Samsung S24
				if strings.Contains(proposal.ResponseText, "نعم") && strings.Contains(proposal.ResponseText, "16") {
					t.Errorf("AI hallucinated iPhone 16, response: %s", proposal.ResponseText)
				}
				if !strings.Contains(proposal.ResponseText, "15") {
					t.Errorf("AI did not suggest iPhone 15 as an alternative, response: %s", proposal.ResponseText)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Printf("\n--- Running Test: %s ---\n", tt.name)
			fmt.Printf("[TRACE] Customer Request: %s\n", tt.prompt)
			proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
				BusinessID:     "test-biz",
				ConversationID: "conv-test",
				Text:           tt.prompt,
				Context:        &ports.AIContext{},
			})
			if err != nil {
				t.Fatalf("Decide failed: %v", err)
			}
			fmt.Printf("[TRACE] Final Response: %s (Action: %s)\n", proposal.ResponseText, proposal.RequestedAction)
			tt.check(t, proposal)
		})
	}
}
