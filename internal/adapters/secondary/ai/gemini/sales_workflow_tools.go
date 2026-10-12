package gemini

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"

func salesWorkflowTool(capability ports.CustomerSalesCapability) customerSalesTool {
	description := "Record an actual buying request supported by customer message evidence. This starts a separate sales workflow, not an informational reply. No confirmed sale is implied."
	required := []string{"evidence_message_ids"}
	if capability == ports.CustomerSalesCapabilityCreateDraft {
		description = "Create a universal commercial draft for a clear customer request. Use verified catalog selections and exact decimal quantities. Do not invent price, customer identity, fulfillment, or missing customer information. Draft creation may request clickable customer confirmation; it never activates, pays, reserves or fulfills an external operation."
		required = append(required, "transaction_type", "lines")
	}
	if capability == ports.CustomerSalesCapabilityUpdateDraft {
		description = "Update the explicitly selected open workflow using its current exposed id and version. Provide complete replacement draft lines and collected transaction data. Never choose between ambiguous workflows. Changing the agreement supersedes previous confirmation buttons."
		required = append(required, "workflow_id", "expected_version", "transaction_type", "lines")
	}
	line := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"item_id", "quantity"}, "properties": map[string]any{
		"item_id": map[string]any{"type": "string"}, "offer_id": map[string]any{"type": "string"}, "variant_id": map[string]any{"type": "string"}, "quantity": map[string]any{"type": "string", "description": "Exact positive decimal, at most 4 fractional digits"}, "selected_attributes": map[string]any{"type": "object"},
	}}
	sales := map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": map[string]any{
		"workflow_id": map[string]any{"type": "string"}, "expected_version": map[string]any{"type": "integer"}, "transaction_type": map[string]any{"type": "string", "enum": []string{"order", "booking", "appointment", "service_request", "reservation", "quote", "subscription"}}, "lines": map[string]any{"type": "array", "items": line}, "transaction_data": map[string]any{"type": "object"}, "evidence_message_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "References from the current customer message or grounded recent customer messages; never invented"},
	}}
	return customerSalesTool{Type: "function", Name: string(capability), Description: description, Parameters: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"sales"}, "properties": map[string]any{"sales": sales}}}
}
