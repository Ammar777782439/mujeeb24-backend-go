package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeepSeekFinalEvidenceKeepsAllReadResultsWithoutToolHistory(t *testing.T) {
	history := []nativeMerchantChatMessage{
		{Role: "system", Content: "trusted rules"},
		{Role: "user", Content: "أضف منتج جلكسي اس 8"},
		{Role: "assistant", ToolCalls: []nativeMerchantChatCall{
			{ID: "read-1", Type: "function", Function: nativeMerchantChatFunction{
				Name: "merchant_catalog_list_items", Arguments: `{"limit":50}`,
			}},
			{ID: "read-2", Type: "function", Function: nativeMerchantChatFunction{
				Name: "merchant_catalog_list_attribute_schemas", Arguments: `{"name":null}`,
			}},
		}},
		{Role: "tool", ToolCallID: "read-1", Content: `{"data":{"catalog_item":"Coffee"},"has_more":true,"next_cursor":"next-1"}`},
		{Role: "tool", ToolCallID: "read-2", Content: `{"data":{"schema_id":"schema-1"},"has_more":false,"next_cursor":""}`},
	}
	fresh, count, err := deepSeekMerchantFinalEvidence(history)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(fresh) != 2 ||
		fresh[0].Role != "system" || fresh[1].Role != "user" ||
		len(fresh[1].ToolCalls) != 0 {
		t.Fatalf("unexpected final conversation shape: count=%d messages=%+v", count, fresh)
	}
	context, ok := fresh[1].Content.(string)
	if !ok {
		t.Fatal("final user context not a string")
	}
	for _, expected := range []string{
		"جلكسي اس 8", "Coffee", "schema-1", "read-1", "read-2",
		"merchant_catalog_list_items", "merchant_catalog_list_attribute_schemas",
		"next-1", "has_more",
	} {
		if !strings.Contains(context, expected) {
			t.Errorf("missing preserved evidence %q", expected)
		}
	}
	payload, err := json.Marshal(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "tool_calls") || strings.Contains(string(payload), "tool_call_id") ||
		strings.Contains(string(payload), `"role":"tool"`) || strings.Contains(string(payload), `"role":"assistant"`) {
		t.Fatal("stale tool-call role history leaked into DeepSeek final request")
	}
}

func TestDeepSeekFinalEvidenceRejectsTamperedToolResultSequence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result string
		id     string
	}{
		{name: "missing_tool_reply", id: ""},
		{name: "mismatched_call_id", id: "other", result: `{"data":{}}`},
		{name: "invalid_json_data", id: "call-1", result: "{no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history := []nativeMerchantChatMessage{
				{Role: "system", Content: "trusted"},
				{Role: "user", Content: "message"},
				{Role: "assistant", ToolCalls: []nativeMerchantChatCall{{
					ID: "call-1", Type: "function", Function: nativeMerchantChatFunction{
						Name: "read_catalog", Arguments: `{"query":"coffee"}`,
					},
				}}},
				{Role: "tool", ToolCallID: tc.id, Content: tc.result},
			}
			if _, _, err := deepSeekMerchantFinalEvidence(history); err == nil {
				t.Fatal("accepted tampered discovery evidence")
			}
		})
	}
}
