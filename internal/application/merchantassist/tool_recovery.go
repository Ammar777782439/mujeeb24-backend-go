package merchantassist

import (
	"encoding/json"
)

// A model's invalid read request is an unsuccessful tool result, not a
// merchant-facing HTTP error. Do not include an unverified ID, private record,
// available tenant IDs, or a permission list in error feedback.
func readToolFailure(code, kind string, allowed map[string]ToolDefinition) string {
	candidates := []string{}
	switch kind {
	case "customer":
		candidates = []string{"find_customers"}
	case "conversation":
		candidates = []string{"list_conversations", "find_customers", "customer_activity"}
	case "transaction":
		candidates = []string{"list_transactions", "transaction_customer_details"}
	case "lead":
		candidates = []string{"list_leads"}
	default:
		candidates = []string{"platform_guide", "dashboard_overview", "find_customers", "list_conversations", "list_leads", "list_transactions"}
	}
	available := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if _, ok := allowed[name]; ok {
			available = append(available, name)
		}
	}
	explanation := "The requested read was not executed. Choose an offered tool to discover a verified record, or ask the merchant a short clarifying question. Never guess a record ID or claim a failed read succeeded."
	if code == "invalid_arguments" {
		explanation = "The requested read was not executed because its arguments did not match the Go tool schema. Retry with the advertised schema or ask the merchant for clarification."
	}
	if code == "tool_unavailable" || code == "batch_rejected" {
		explanation = "The requested read was not executed. Use only the tools offered in this conversation, or explain the permission/coverage limit without inventing data."
	}
	result, _ := json.Marshal(map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":                    code,
			"guidance":                explanation,
			"allowed_discovery_tools": available,
		},
	})
	return string(result)
}
