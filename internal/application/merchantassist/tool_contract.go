package merchantassist

import (
	"context"
	"encoding/json"
)

// MaxReadToolsPerBatch is an application-level safety limit, not a claim that
// DeepSeek will honor parallel_tool_calls=false. Independent reads are executed
// sequentially in Go, and the complete batch is replayed to the model.
const MaxReadToolsPerBatch = 8

// ToolDefinition is a Go-owned capability contract. The provider receives
// exactly these read-only functions, never database or mutation privileges.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	ID               string
	Name             string
	Arguments        json.RawMessage
	ThoughtSignature string
}

type ToolMessage struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ToolCompletion struct {
	FinishReason string
	Content      string
	Calls        []ToolCall
	Provider     string
}

// ToolModel is independent of provider-specific API envelopes.
type ToolModel interface {
	CompleteTools(context.Context, []ToolMessage, []ToolDefinition, string) (ToolCompletion, error)
}

func schemaObject(properties map[string]any, required ...string) map[string]any {
	// Required must encode as [] for parameterless native tool functions.
	// A nil variadic []string encodes as JSON null, which is rejected by
	// both our strict schema compiler and DeepSeek strict tool calling.
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": properties, "required": required,
	}
}

func toolDefinitions(available []string) []ToolDefinition {
	definitions := map[string]ToolDefinition{
		"platform_guide":       {"platform_guide", "Explain Mujeeb 24 sections and verified links; explain that Merchant Catalog AI is a separate approval-controlled product authoring workflow. This assistant never creates or edits products.", schemaObject(map[string]any{})},
		"dashboard_overview":   {"dashboard_overview", "Read business dashboard aggregates: open conversations, human waiting, new customers, new leads, transactions needing review. New customers is not total customers.", schemaObject(map[string]any{})},
		"find_customers":       {"find_customers", "Find a named customer or obtain a complete customer count and status breakdown. Use an empty search to include all customers. Results include exact total across fully scanned pages, sample and completeness.", schemaObject(map[string]any{"search": map[string]any{"type": "string"}}, "search")},
		"customer_activity":    {"customer_activity", "Inspect a previously discovered customer's verified conversations and transactions. Use the exact customer record_id returned by find_customers; never guess an ID.", schemaObject(map[string]any{"record_id": map[string]any{"type": "string"}}, "record_id")},
		"list_conversations":   {"list_conversations", "Read complete conversation state totals and a bounded sample. Only waiting_human proves need for human intervention; open alone does not.", schemaObject(map[string]any{})},
		"list_leads":           {"list_leads", "Read total leads by status and a bounded sample with IDs for follow-up.", schemaObject(map[string]any{})},
		"list_transactions":    {"list_transactions", "Read full transaction/order state totals with sample and record IDs.", schemaObject(map[string]any{})},
		"list_catalogs":        {"list_catalogs", "Read catalog status totals and a bounded sample. Merchant catalog authoring is a separate workflow; this tool cannot create or edit items.", schemaObject(map[string]any{})},
		"list_channels":        {"list_channels", "Read business connection status by channel.", schemaObject(map[string]any{})},
		"subscription":         {"subscription", "Read subscription and available AI reply quotas for the current business.", schemaObject(map[string]any{})},
		"published_policies":   {"published_policies", "Search reviewed, published merchant policy evidence; search is required.", schemaObject(map[string]any{"search": map[string]any{"type": "string"}}, "search")},
		"published_knowledge":  {"published_knowledge", "Search published merchant knowledge evidence; search is required.", schemaObject(map[string]any{"search": map[string]any{"type": "string"}}, "search")},
		"conversation_history": {"conversation_history", "Summarize a verified current or previously listed conversation's sanitized messages and state. No private notes.", schemaObject(map[string]any{"record_id": map[string]any{"type": "string"}}, "record_id")},
		"transaction_record":   {"transaction_record", "Read a verified selected or listed transaction record for details; no approvals or mutations.", schemaObject(map[string]any{"record_id": map[string]any{"type": "string"}}, "record_id")},
		"customer_record":      {"customer_record", "Read a verified customer record; no customer profile mutations.", schemaObject(map[string]any{"record_id": map[string]any{"type": "string"}}, "record_id")},
		"lead_evidence":        {"lead_evidence", "Read verified sales lead and score evidence; no sales mutations.", schemaObject(map[string]any{"record_id": map[string]any{"type": "string"}}, "record_id")},
	}
	out := make([]ToolDefinition, 0, len(available))
	for _, name := range available {
		if spec, ok := definitions[name]; ok {
			out = append(out, spec)
		}
	}
	return out
}
