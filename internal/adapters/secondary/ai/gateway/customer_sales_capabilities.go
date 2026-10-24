package gateway

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type customerSalesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Messaging tools carry explicit empty selected arrays. Optional catalog refs
// are null, not omitted, so the strict provider schema remains enforceable.
type customerSalesMessageArguments struct {
	ResponseText string                    `json:"response_text"`
	Selected     []ports.SelectedReference `json:"selected"`
}

// record_lead has its own strict contract; it cannot carry transaction lines.
type customerSalesLeadArguments struct {
	Sales struct {
		EvidenceMessageIDs []string `json:"evidence_message_ids"`
	} `json:"sales"`
}

type customerSalesWorkflowArguments struct {
	Sales struct {
		WorkflowID      *string `json:"workflow_id"`
		ExpectedVersion *int64  `json:"expected_version"`
		TransactionType *string `json:"transaction_type"`
		Lines           []struct {
			ItemID                 string  `json:"item_id"`
			OfferID                *string `json:"offer_id"`
			VariantID              *string `json:"variant_id"`
			Quantity               string  `json:"quantity"`
			SelectedAttributesJSON string  `json:"selected_attributes_json"`
		} `json:"lines"`
		TransactionDataJSON string   `json:"transaction_data_json"`
		EvidenceMessageIDs  []string `json:"evidence_message_ids"`
	} `json:"sales"`
}

func decodeCommercialAttributes(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("missing structured sales JSON")
	}
	var attributes map[string]any
	if err := decodeStrictStructuredJSON([]byte(raw), &attributes); err != nil {
		return nil, errors.New("invalid structured sales JSON")
	}
	if attributes == nil {
		return nil, errors.New("structured sales JSON must be an object")
	}
	return attributes, nil
}

func customerSalesTools(capabilities []ports.CustomerSalesCapability) []customerSalesTool {
	tools := make([]customerSalesTool, 0, len(capabilities))
	for _, capability := range capabilities {
		description := ""
		switch capability {
		case ports.CustomerSalesCapabilityRespondWithoutAction:
			description = "Use only for factual answers, explanations, acknowledgments, or greetings that require NO external operation. If the customer wants to proceed, subscribe, purchase, change an order, or speak to a person, this is not a suitable capability. Earlier assistant promises and conversation summaries are not evidence of executed operations. When no offered executor can perform the requested action, choose handoff_to_human."
		case ports.CustomerSalesCapabilityAsk:
			description = "Ask ONLY when an essential fact must be supplied by the customer to understand the goal or use an available executor. Do not reconfirm a clear intent to proceed, defer an unsupported operation by asking questions, or request merchant catalog facts that evaluate_full_catalog can retrieve."
		case ports.CustomerSalesCapabilityHandoff:
			description = "Use when the customer explicitly wants an employee, or requests an external action (including subscription) for which no offered executable capability exists. Short confirmations inherit the earlier customer goal; previous assistant promises are not proof of execution. Request human follow-up without claiming transfer or employee assignment already happened; only Mujeeb can commit waiting_human after authorization."
		case ports.CustomerSalesCapabilityRecordLead, ports.CustomerSalesCapabilityCreateDraft, ports.CustomerSalesCapabilityUpdateDraft:
			tools = append(tools, salesWorkflowTool(capability))
			continue
		case ports.CustomerSalesCapabilityFullCatalog:
			tools = append(tools, customerSalesTool{
				Type:        "function",
				Name:        string(capability),
				Description: "Use when the requested answer or next step needs merchant catalog information (items, prices, offers, options, availability or comparisons) unavailable in the trusted context. This is an internal full-catalog read-only step, not a final answer; after complete coverage choose one offered terminal capability.",
				Parameters:  map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}, "required": []string{}},
			})
			continue
		default:
			continue
		}
		tools = append(tools, customerSalesTool{Type: "function", Name: string(capability), Description: description, Parameters: customerSalesResponseParameters(capability)})
	}
	return tools
}

// parseCustomerSalesCapability accepts one identified, offered capability.
// Backend validation and execution are never delegated to the model.
func parseCustomerSalesCapability(resp capabilityResponse, tools []customerSalesTool) (ports.CapabilityDecision, error) {
	if len(resp.Calls) != 1 || strings.TrimSpace(resp.Calls[0].ID) == "" {
		return ports.CapabilityDecision{}, errors.New("customer sales requires exactly one identified capability call")
	}
	call := resp.Calls[0]
	var offered *customerSalesTool
	for i := range tools {
		if tools[i].Name == call.Name {
			offered = &tools[i]
			break
		}
	}
	if offered == nil {
		return ports.CapabilityDecision{}, fmt.Errorf("unavailable customer sales capability %q", call.Name)
	}
	if err := validateStrictArguments(call.Arguments, offered.Parameters); err != nil {
		return ports.CapabilityDecision{}, fmt.Errorf("strict capability arguments: %w", err)
	}
	capability := ports.CustomerSalesCapability(call.Name)
	if capability == ports.CustomerSalesCapabilityFullCatalog {
		var args struct{}
		if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
			return ports.CapabilityDecision{}, fmt.Errorf("decode catalog capability: %w", err)
		}
		return ports.CapabilityDecision{Capability: capability}, nil
	}
	definition, ok := ports.CustomerSalesCapabilityDefinitionFor(capability)
	if !ok {
		return ports.CapabilityDecision{}, errors.New("capability definition missing")
	}
	if definition.Executor == "sales" {
		if capability == ports.CustomerSalesCapabilityRecordLead {
			var args customerSalesLeadArguments
			if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
				return ports.CapabilityDecision{}, fmt.Errorf("decode lead capability arguments: %w", err)
			}
			return ports.CapabilityDecision{
				Capability: capability,
				Sales: &ports.SalesWorkflowProposal{
					EvidenceMessageIDs: args.Sales.EvidenceMessageIDs,
				},
			}, nil
		}
		var args customerSalesWorkflowArguments
		if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
			return ports.CapabilityDecision{}, fmt.Errorf("decode sales capability arguments: %w", err)
		}
		// Decode explicitly typed wire fields. JSON strings for dynamic
		// transaction data avoid open-ended objects in strict tool schemas.
		details, err := decodeCommercialAttributes(args.Sales.TransactionDataJSON)
		if err != nil {
			return ports.CapabilityDecision{}, err
		}
		sales := &ports.SalesWorkflowProposal{
			TransactionData: details, EvidenceMessageIDs: args.Sales.EvidenceMessageIDs,
		}
		if args.Sales.WorkflowID != nil {
			sales.WorkflowID = *args.Sales.WorkflowID
		}
		if args.Sales.ExpectedVersion != nil {
			sales.ExpectedVersion = *args.Sales.ExpectedVersion
		}
		if args.Sales.TransactionType != nil {
			sales.TransactionType = *args.Sales.TransactionType
		}
		for _, line := range args.Sales.Lines {
			attributes, err := decodeCommercialAttributes(line.SelectedAttributesJSON)
			if err != nil {
				return ports.CapabilityDecision{}, err
			}
			sales.Lines = append(sales.Lines, ports.SalesWorkflowLine{
				ItemID: line.ItemID, OfferID: line.OfferID, VariantID: line.VariantID,
				Quantity: line.Quantity, SelectedAttributes: attributes,
			})
		}
		decision := ports.CapabilityDecision{Capability: capability, Sales: sales}
		for _, line := range sales.Lines {
			decision.Selected = append(decision.Selected, ports.SelectedReference{
				ItemID: line.ItemID, OfferID: line.OfferID, VariantID: line.VariantID,
			})
		}
		return decision, nil
	}
	var args customerSalesMessageArguments
	if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
		return ports.CapabilityDecision{}, fmt.Errorf("decode capability arguments: %w", err)
	}
	if strings.TrimSpace(args.ResponseText) == "" {
		return ports.CapabilityDecision{}, errors.New("capability requires response_text")
	}
	return ports.CapabilityDecision{Capability: capability, ResponseText: args.ResponseText, Selected: args.Selected}, nil
}

// Token counting uses equivalent GenerateContent function declarations.
func customerSalesTokenTools(tools []customerSalesTool) []map[string]any {
	declarations := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		declarations = append(declarations, map[string]any{"name": tool.Name, "description": tool.Description, "parametersJsonSchema": tool.Parameters})
	}
	return []map[string]any{{"functionDeclarations": declarations}}
}

// All terminal capabilities share grounded customer text and catalog references.
func customerSalesResponseParameters(capability ports.CustomerSalesCapability) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"response_text": map[string]any{"type": "string", "description": customerSalesResponseTextDescription(capability)},
			"selected": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"item_id": map[string]any{"type": "string"}, "variant_id": map[string]any{"type": []string{"string", "null"}}, "offer_id": map[string]any{"type": []string{"string", "null"}}},
				"required":   []string{"item_id", "variant_id", "offer_id"},
			}},
		}, "required": []string{"response_text", "selected"},
	}
}

func customerSalesResponseTextDescription(capability ports.CustomerSalesCapability) string {
	switch capability {
	case ports.CustomerSalesCapabilityRespondWithoutAction:
		return "Customer-facing text that answers, explains, acknowledges, or informs only. It must not ask the customer to confirm an unsupported external operation and must not claim, promise, start, confirm, or complete any external state change."
	case ports.CustomerSalesCapabilityAsk:
		return "One customer-facing question for genuinely missing customer-supplied information. It must not reconfirm a request that is already clear and must not postpone handoff for an unsupported external operation."
	case ports.CustomerSalesCapabilityHandoff:
		return "Customer-facing request for human follow-up. Do not claim that an employee was assigned, a transfer was committed, or a subscription or other external action succeeded before a trusted execution result."
	default:
		return "Grounded customer-facing text."
	}
}
