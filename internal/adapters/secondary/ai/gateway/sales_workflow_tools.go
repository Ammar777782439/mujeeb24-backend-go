package gateway

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"

// Dynamic merchant-defined values remain JSON data, not open-ended object
// properties in the provider strict schema. Go decodes them before execution.
func salesWorkflowTool(capability ports.CustomerSalesCapability) customerSalesTool {
	description := "Record early customer interest ONLY, not a commercial draft. When the customer clearly asks to proceed with an available product or subscription, prefer create_transaction_draft, or update_transaction_draft for an active commercial draft. Never attach transaction lines or claim a sale."
	switch capability {
	case ports.CustomerSalesCapabilityCreateDraft:
		description = "Create a commercial DRAFT for a clear customer request to proceed, with grounded catalog references and decimal quantities. For an existing lead without a transaction, supply its trusted workflow_id and expected_version to promote the lead to a draft. If a commercial draft already exists, choose update_transaction_draft instead. This is not activation, payment or fulfillment."
	case ports.CustomerSalesCapabilityUpdateDraft:
		description = "Update an existing commercial draft using its exact workflow_id and expected_version from trusted sales context. Include the complete intended catalog lines snapshot and transaction_type; transaction_data_json may contain ONLY newly provided customer fields, since the server preserves previously verified nonempty fields. Never guess a workflow or version."
	}

	// A lead is *not* a transaction. Excluding lines at schema level prevents
	// the model from producing a syntactically valid but unexecutable lead.
	if capability == ports.CustomerSalesCapabilityRecordLead {
		lead := map[string]any{"type": "object", "additionalProperties": false,
			"required": []string{"evidence_message_ids"},
			"properties": map[string]any{
				"evidence_message_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Real current or recent customer message IDs, never invented"},
			},
		}
		return customerSalesTool{Type: "function", Name: string(capability), Description: description,
			Parameters: map[string]any{"type": "object", "additionalProperties": false,
				"required": []string{"sales"}, "properties": map[string]any{"sales": lead}}}
	}
	nullableString := map[string]any{"type": []string{"string", "null"}}
	nullableInteger := map[string]any{"type": []string{"integer", "null"}}
	// DeepSeek strict schemas cannot enforce array cardinality: Go still checks
	// non-empty lines, decimal quantity and trusted catalog references.
	line := map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"item_id", "offer_id", "variant_id", "quantity", "selected_attributes_json"},
		"properties": map[string]any{
			"item_id":                  map[string]any{"type": "string"},
			"offer_id":                 nullableString,
			"variant_id":               nullableString,
			"quantity":                 map[string]any{"type": "string", "description": "Exact positive decimal, at most 4 fractional digits"},
			"selected_attributes_json": map[string]any{"type": "string", "description": "JSON object string of only grounded, item-specific selected attributes; use {} when empty"},
		},
	}
	// Update MUST identify a real workflow. Create may either start a new
	// workflow (null ID) or promote a trusted lead with ID/version.
	workflowID := nullableString
	expectedVersion := nullableInteger
	if capability == ports.CustomerSalesCapabilityUpdateDraft {
		workflowID = map[string]any{"type": "string", "description": "Existing active workflow ID from trusted sales context; cannot be guessed"}
		expectedVersion = map[string]any{"type": "integer", "description": "Current resource_version from the same workflow in trusted sales context"}
	}
	sales := map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"workflow_id", "expected_version", "transaction_type", "lines", "transaction_data_json", "evidence_message_ids"},
		"properties": map[string]any{
			"workflow_id":           workflowID,
			"expected_version":      expectedVersion,
			"transaction_type":      map[string]any{"type": []string{"string", "null"}, "enum": []any{"order", "booking", "appointment", "service_request", "reservation", "quote", "subscription", nil}},
			"lines":                 map[string]any{"type": "array", "items": line, "description": "Complete intended lines snapshot for this draft; never invent catalog IDs or quantities"},
			"transaction_data_json": map[string]any{"type": "string", "description": "JSON object using exact attribute_key values from trusted merchant sales requirements. Use only grounded customer-supplied fields; on update, unchanged fields may be omitted and are preserved by the server. Use {} when no new fields"},
			"evidence_message_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Real current or recent customer message IDs, never invented"},
		},
	}
	return customerSalesTool{Type: "function", Name: string(capability), Description: description,
		Parameters: map[string]any{"type": "object", "additionalProperties": false,
			"required": []string{"sales"}, "properties": map[string]any{"sales": sales}}}
}
