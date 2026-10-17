package gateway

import "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"

// Dynamic merchant-defined values remain JSON data, not open-ended object
// properties in the provider strict schema. Go decodes them before execution.
func salesWorkflowTool(capability ports.CustomerSalesCapability) customerSalesTool {
	description := "Record customer buying interest supported by referenced messages. No sale or external state change is implied."
	switch capability {
	case ports.CustomerSalesCapabilityCreateDraft:
		description = "Create a universal commercial draft for a clear customer request. Select only verified catalog references; exact quantity is a decimal string. Never claim activation, reservation, payment or fulfillment."
	case ports.CustomerSalesCapabilityUpdateDraft:
		description = "Update an explicitly identified workflow and version with grounded customer data. Never select between ambiguous workflows; prior confirmation must be superseded."
	}
	nullableString := map[string]any{"type": []string{"string", "null"}}
	nullableInteger := map[string]any{"type": []string{"integer", "null"}}
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
	sales := map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"workflow_id", "expected_version", "transaction_type", "lines", "transaction_data_json", "evidence_message_ids"},
		"properties": map[string]any{
			"workflow_id":           nullableString,
			"expected_version":      nullableInteger,
			"transaction_type":      map[string]any{"type": []string{"string", "null"}, "enum": []any{"order", "booking", "appointment", "service_request", "reservation", "quote", "subscription", nil}},
			"lines":                 map[string]any{"type": "array", "items": line},
			"transaction_data_json": map[string]any{"type": "string", "description": "JSON object string of transaction requirements supplied by the customer; use {} when none"},
			"evidence_message_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Real current or recent customer message IDs, never invented"},
		},
	}
	return customerSalesTool{Type: "function", Name: string(capability), Description: description,
		Parameters: map[string]any{"type": "object", "additionalProperties": false,
			"required": []string{"sales"}, "properties": map[string]any{"sales": sales}}}
}
