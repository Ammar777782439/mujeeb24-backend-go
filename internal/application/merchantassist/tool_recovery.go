package merchantassist

import (
	"encoding/json"
	"errors"
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
	if code == "source_evidence_incomplete" {
		explanation = "This specific source could not be safely verified in full. Never invent missing messages or claim the whole conversation was reviewed. Use only successful earlier tool evidence, look up a different authorized record if useful, or say which information was unavailable. Do not repeat the same failed read."
	}
	if code == "source_unavailable" {
		explanation = "This source could not be read right now. Do not repeat the same failed read. Continue with previously verified evidence if relevant, otherwise explain the temporary limitation without inventing data."
	}
	if code == "permission_denied" {
		explanation = "This source is not authorized for the current actor. Do not retry this denied read, disclose identifiers or claim access; explain the permission limit briefly."
	}
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

func readToolErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrReadDenied):
		return "permission_denied"
	case errors.Is(err, ErrInvalidSourceEvidence):
		return "source_evidence_incomplete"
	case errors.Is(err, ErrInvalidReadRequest):
		return "invalid_arguments"
	default:
		return "source_unavailable"
	}
}
