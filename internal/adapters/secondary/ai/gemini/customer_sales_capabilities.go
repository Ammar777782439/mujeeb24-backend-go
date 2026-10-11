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
	ResponseText string                    `json:"response_text"`
	Selected     []ports.SelectedReference `json:"selected,omitempty"`
}

func customerSalesTools(capabilities []ports.CustomerSalesCapability) []customerSalesTool {
	tools := make([]customerSalesTool, 0, len(capabilities))
	for _, capability := range capabilities {
		description := ""
		switch capability {
		case ports.CustomerSalesCapabilityReply:
			description = "Reply using verified current evidence. For facts, comparisons, alternatives or absence requiring catalog knowledge beyond that evidence, request evaluate_full_catalog first."
		case ports.CustomerSalesCapabilityAsk:
			description = "Ask the customer for genuinely missing information. Do not ask the customer for catalog facts the merchant can provide through evaluate_full_catalog."
		case ports.CustomerSalesCapabilityHandoff:
			description = "Request human follow-up when the customer asks for a human or their goal cannot be executed by the available capabilities. Write a grounded customer response; never claim the requested transaction was executed."
		case ports.CustomerSalesCapabilityFullCatalog:
			tools = append(tools, customerSalesTool{Type: "function", Name: string(capability), Description: "Request evaluation of ALL active catalog components and relationships for this merchant when current evidence is insufficient. All token-aware batches must complete before a final capability is chosen. This is not a search; no filters, IDs, tenant, or query arguments are accepted.", Parameters: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}})
			continue
		default:
			continue
		}
		tools = append(tools, customerSalesTool{Type: "function", Name: string(capability), Description: description, Parameters: customerSalesResponseParameters()})
	}
	return tools
}

// parseCustomerSalesCapability accepts exactly one offered function call.
// Text, unknown tools, parallel calls and extra arguments fail closed.
func parseCustomerSalesCapability(resp interactionResponse, tools []customerSalesTool) (ports.CustomerSalesCapability, ports.CustomerSalesProposal, error) {
	var call *interactionStep
	for i := range resp.Steps {
		step := &resp.Steps[i]
		if step.Type == "function_call" {
			if call != nil {
				return "", ports.CustomerSalesProposal{}, errors.New("customer sales requires exactly one capability call")
			}
			call = step
		}
	}
	if call == nil || strings.TrimSpace(call.ID) == "" {
		return "", ports.CustomerSalesProposal{}, errors.New("customer sales requires an identified capability call")
	}
	offered := false
	for _, tool := range tools {
		if tool.Name == call.Name {
			offered = true
			break
		}
	}
	if !offered {
		return "", ports.CustomerSalesProposal{}, fmt.Errorf("unavailable customer sales capability %q", call.Name)
	}
	if len(call.Arguments) == 0 || strings.TrimSpace(string(call.Arguments)) == "null" {
		return "", ports.CustomerSalesProposal{}, errors.New("capability arguments must be an object")
	}
	capability := ports.CustomerSalesCapability(call.Name)
	if capability == ports.CustomerSalesCapabilityFullCatalog {
		var args struct{}
		if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
			return "", ports.CustomerSalesProposal{}, fmt.Errorf("decode catalog capability: %w", err)
		}
		return capability, ports.CustomerSalesProposal{}, nil
	}
	var args customerSalesToolArguments
	if err := decodeStrictStructuredJSON(call.Arguments, &args); err != nil {
		return "", ports.CustomerSalesProposal{}, fmt.Errorf("decode capability arguments: %w", err)
	}
	if strings.TrimSpace(args.ResponseText) == "" {
		return "", ports.CustomerSalesProposal{}, errors.New("capability requires response_text")
	}
	proposal := ports.CustomerSalesProposal{Status: ports.CustomerSalesProposalStatusResolved, ResponseText: args.ResponseText, Selected: args.Selected}
	switch capability {
	case ports.CustomerSalesCapabilityReply:
		proposal.Action = ports.CustomerSalesProposalActionAnswer
	case ports.CustomerSalesCapabilityAsk:
		proposal.Status = ports.CustomerSalesProposalStatusAmbiguous
		proposal.Action = ports.CustomerSalesProposalActionClarification
	case ports.CustomerSalesCapabilityHandoff:
		proposal.Action = ports.CustomerSalesProposalActionHumanRequest
	}
	return capability, proposal, nil
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
func customerSalesResponseParameters() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"response_text": map[string]any{"type": "string"},
			"selected": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"item_id": map[string]any{"type": "string"}, "variant_id": map[string]any{"type": "string"}, "offer_id": map[string]any{"type": "string"}},
				"required":   []string{"item_id"},
			}},
		}, "required": []string{"response_text"},
	}
}
