package gemini

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

type customerSalesToolArguments struct {
	Sales        *ports.SalesWorkflowProposal `json:"sales,omitempty"`
	ResponseText string                       `json:"response_text"`
	Selected     []ports.SelectedReference    `json:"selected,omitempty"`
}

func customerSalesTools(capabilities []ports.CustomerSalesCapability) []customerSalesTool {
	tools := make([]customerSalesTool, 0, len(capabilities))
	for _, capability := range capabilities {
		description := ""
		switch capability {
		case ports.CustomerSalesCapabilityRespondWithoutAction:
			description = "Answer, explain, acknowledge, or provide grounded information only when the customer goal can be satisfied without starting, confirming, completing, changing, or promising an external business operation. Never use this capability to ask for confirmation of an unsupported operation or to claim an external state change happened. If the customer wants to proceed and no offered executor can do it, use handoff_to_human."
		case ports.CustomerSalesCapabilityAsk:
			description = "Ask only for genuinely missing customer-supplied information required to understand the goal or use an available capability. Never reconfirm a clear request, and never use a confirmation question to postpone handoff for an unsupported external operation. Do not ask for catalog facts the merchant can provide through evaluate_full_catalog."
		case ports.CustomerSalesCapabilityHandoff:
			description = "Use when the customer explicitly asks for a human, or when the current goal is to start, confirm, complete, or change an external operation that none of the offered executable capabilities can perform. Short confirmations inherit the prior conversational goal. Write a grounded handoff response and never claim the external operation was executed."
		case ports.CustomerSalesCapabilityRecordLead, ports.CustomerSalesCapabilityCreateDraft, ports.CustomerSalesCapabilityUpdateDraft:
			tools = append(tools, salesWorkflowTool(capability))
			continue
		case ports.CustomerSalesCapabilityFullCatalog:
			tools = append(tools, customerSalesTool{Type: "function", Name: string(capability), Description: "Request full catalog access. Use presentation=hierarchy when the customer asks to see ALL products/services or the whole assortment organized by catalog, item, variant and offer. Use presentation=compare when the customer wants matching options, comparisons, suitability, or commercial decisions. Do not send merchant IDs or filtering/query arguments.", Parameters: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"presentation": map[string]any{"type": "string", "enum": []string{"hierarchy", "compare"}}}}})
			continue
		default:
			continue
		}
		tools = append(tools, customerSalesTool{Type: "function", Name: string(capability), Description: description, Parameters: customerSalesResponseParameters(capability)})
	}
	return tools
}

// parseCustomerSalesCapability accepts exactly one offered function call.
// Text, unknown tools, parallel calls and extra arguments fail closed.
func parseCustomerSalesCapability(resp interactionResponse, tools []customerSalesTool) (ports.CapabilityDecision, error) {
	var call *interactionStep
	for i := range resp.Steps {
		step := &resp.Steps[i]
		if step.Type == "function_call" {
			if call != nil {
				return ports.CapabilityDecision{}, errors.New("customer sales requires exactly one capability call")
			}
			call = step
		}
	}
	if call == nil || strings.TrimSpace(call.ID) == "" {
		return ports.CapabilityDecision{}, errors.New("customer sales requires an identified capability call")
	}
	offered := false
	for _, tool := range tools {
		if tool.Name == call.Name {
			offered = true
			break
		}
	}
	if !offered {
		return ports.CapabilityDecision{}, fmt.Errorf("unavailable customer sales capability %q", call.Name)
	}
	if len(call.Arguments) == 0 || strings.TrimSpace(string(call.Arguments)) == "null" {
		return ports.CapabilityDecision{}, errors.New("capability arguments must be an object")
	}
	capability := ports.CustomerSalesCapability(call.Name)
	if capability == ports.CustomerSalesCapabilityFullCatalog {
		var args struct {
			Presentation string `json:"presentation,omitempty"`
		}
		if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
			return ports.CapabilityDecision{}, fmt.Errorf("decode catalog capability: %w", err)
		}
		if args.Presentation != "" && args.Presentation != "hierarchy" && args.Presentation != "compare" {
			return ports.CapabilityDecision{}, errors.New("unsupported catalog presentation")
		}
		return ports.CapabilityDecision{Capability: capability, Presentation: args.Presentation}, nil
	}
	var args customerSalesToolArguments
	if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
		return ports.CapabilityDecision{}, fmt.Errorf("decode capability arguments: %w", err)
	}
	definition, _ := ports.CustomerSalesCapabilityDefinitionFor(capability)
	if definition.Executor == "sales" {
		if args.Sales == nil || args.ResponseText != "" || len(args.Selected) > 0 {
			return ports.CapabilityDecision{}, errors.New("sales capability requires only structured sales arguments")
		}
		decision := ports.CapabilityDecision{Capability: capability, Sales: args.Sales}
		for _, line := range args.Sales.Lines {
			decision.Selected = append(decision.Selected, ports.SelectedReference{ItemID: line.ItemID, OfferID: line.OfferID, VariantID: line.VariantID})
		}
		return decision, nil
	}
	if args.Sales != nil {
		return ports.CapabilityDecision{}, errors.New("messaging capability cannot carry sales arguments")
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
				"properties": map[string]any{"item_id": map[string]any{"type": "string"}, "variant_id": map[string]any{"type": "string"}, "offer_id": map[string]any{"type": "string"}},
				"required":   []string{"item_id"},
			}},
		}, "required": []string{"response_text"},
	}
}

func customerSalesResponseTextDescription(capability ports.CustomerSalesCapability) string {
	switch capability {
	case ports.CustomerSalesCapabilityRespondWithoutAction:
		return "Customer-facing text that answers, explains, acknowledges, or informs only. It must not ask the customer to confirm an unsupported external operation and must not claim, promise, start, confirm, or complete any external state change."
	case ports.CustomerSalesCapabilityAsk:
		return "One customer-facing question for genuinely missing customer-supplied information. It must not reconfirm a request that is already clear and must not postpone handoff for an unsupported external operation."
	case ports.CustomerSalesCapabilityHandoff:
		return "Customer-facing handoff text that states human follow-up is needed. It must not claim the requested external operation has already been executed, confirmed, or completed."
	default:
		return "Grounded customer-facing text."
	}
}
