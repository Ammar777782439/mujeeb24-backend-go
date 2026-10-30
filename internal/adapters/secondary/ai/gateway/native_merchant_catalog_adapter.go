package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// NativeMerchantCatalogAuthoringAdapter uses stateless Chat Completions through
// the gateway. Every assistant tool call and server-executed tool result is
// replayed explicitly; no Google previous_interaction_id is ever reused.
type NativeMerchantCatalogAuthoringAdapter struct {
	client          *native.CompletionClient
	model           string
	maxOutputTokens int
	maxRequestBytes int

	usageReporter func(context.Context, string, string, ports.CustomerSalesUsageTelemetry) error
	AIGuard       ports.BusinessAIExecutionGuard
}

const (
	nativeMaxMerchantToolRounds    = 12
	nativeMaxMerchantToolCalls     = 32
	nativeMaxMerchantCallsPerRound = 8
)

func NewNativeMerchantCatalogAuthoringAdapter(client *native.CompletionClient, model string, maxOutputTokens, maxRequestBytes int) (*NativeMerchantCatalogAuthoringAdapter, error) {
	if client == nil || strings.TrimSpace(model) == "" || maxOutputTokens <= 0 || maxRequestBytes <= 0 {
		return nil, errors.New("native merchant catalog requires client, model, and positive limits")
	}
	return &NativeMerchantCatalogAuthoringAdapter{client: client, model: model, maxOutputTokens: maxOutputTokens, maxRequestBytes: maxRequestBytes}, nil
}

// SetAIExecutionGuard blocks every merchant model round for disabled businesses.
func (a *NativeMerchantCatalogAuthoringAdapter) SetAIExecutionGuard(guard ports.BusinessAIExecutionGuard) {
	if a != nil {
		a.AIGuard = guard
	}
}

// SetUsageReporter persists one native gateway charge for every successful
// Chat Completions round, including tool rounds that precede the proposal.
func (a *NativeMerchantCatalogAuthoringAdapter) SetUsageReporter(report func(context.Context, string, string, ports.CustomerSalesUsageTelemetry) error) {
	if a != nil {
		a.usageReporter = report
	}
}

type nativeMerchantChatFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type nativeMerchantChatCall struct {
	ID               string                     `json:"id"`
	Type             string                     `json:"type"`
	Function         nativeMerchantChatFunction `json:"function"`
	ThoughtSignature string                     `json:"thought_signature,omitempty"`
}
type nativeMerchantChatMessage struct {
	Role       string                   `json:"role"`
	Content    any                      `json:"content,omitempty"`
	ToolCalls  []nativeMerchantChatCall `json:"tool_calls,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
}
type nativeMerchantChatRequest struct {
	Model             string                      `json:"model"`
	Messages          []nativeMerchantChatMessage `json:"messages"`
	Tools             []map[string]any            `json:"tools,omitempty"`
	ToolChoice        string                      `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool                       `json:"parallel_tool_calls,omitempty"`
	ResponseFormat    any                         `json:"response_format,omitempty"`
	MaxTokens         int                         `json:"max_tokens"`
	Stream            bool                        `json:"stream"`
}
type nativeMerchantChatResponse struct {
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role      string                   `json:"role"`
			Content   json.RawMessage          `json:"content"`
			ToolCalls []nativeMerchantChatCall `json:"tool_calls"`
			Refusal   *string                  `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
}

func (a *NativeMerchantCatalogAuthoringAdapter) Propose(ctx context.Context, input merchantcatalogai.MerchantCatalogAuthoringInput) (merchantcatalogai.Proposal, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.Message) == "" ||
		strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.PrincipalID) == "" {
		return merchantcatalogai.Proposal{}, errors.New("merchant catalog gateway input is incomplete")
	}
	if a.AIGuard != nil {
		if err := a.AIGuard.Check(ctx, input.BusinessID); err != nil {
			return merchantcatalogai.Proposal{}, err
		}
	}
	system, contextualInput, err := buildMerchantCatalogAuthoringPrompt(input)
	if err != nil {
		return merchantcatalogai.Proposal{}, err
	}

	// DeepSeek constrains the whole commercial transport hierarchy. Only
	// individual field values remain JSON strings, decoded and validated in Go.
	// Never serialize the complete proposal as an unconstrained string.
	wireSchema := merchantCatalogWireSchema()
	proposalFormat := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "merchant_catalog_wire",
			"strict": true,
			"schema": wireSchema,
		},
	}
	trustedProposalInstruction := system + "\n\n" + `# Strict merchant catalog wire transport
Return the final proposal using the merchant_catalog_wire tool schema.
The transport tool does NOT include schema_version. Go sets the canonical Proposal V3
schema_version from its own contract; do not invent a version field.
All schema object fields are required. For an inactive operation, use empty
fields/variants/offers arrays and empty IDs/strings, never invented data.
For create: populate create.fields, create.variants, create.offers.
For update: populate update.item_id and only requested changes in
update.changes, update.existing_variants, update.new_variants,
update.existing_offers, update.new_offers.
For delete: populate delete.id. For read-only answers: operation=ask_merchant
with all mutation payloads empty. When asking for missing information, use
status=needs_more_data and operation=ask_merchant, and keep root
attribute_schema_name="" and attribute_definitions=[].
Do not draft attribute definitions or a new template until an actual
create/update proposal is ready for review.
When selecting an existing template, use only an ID obtained from
merchant_catalog_list_attribute_schemas. Populate create.attribute_schema_id
or update.attribute_schema_id as appropriate. Use exact existing definition
keys (attr_) from that template. Do not invent IDs, Arabic labels or templates.
For a new template, set root attribute_schema_name to an Arabic merchant-facing
name that YOU understand from the merchant's product. Go will only persist
your provided name after approval; it never translates or authors labels.
Set attribute_schema_name="" when an existing template is selected.
For every custom English snake_case attribute key appearing in item or variant attributes,
add one entry to attribute_definitions with {key,label,data_type,validation_rules_json};
label must be merchant-facing Arabic and the data type must match the JSON value.
validation_rules_json must be a nonempty JSON object string, e.g. "{}" when
there are no constraints. For select and multi_select, provide an evidence-backed
nonempty enum array. Never send an empty validation_rules_json string.
Use the exact same semantic key in both places. Never create attr_ keys;
those come from Go after the merchant approves. Existing attr_ keys from
real catalog evidence need no new definition.
Each fields array contains {name,value_json}; value_json is the JSON
encoding of that one field value (e.g. "600.0000" encoded as a JSON string,
or attributes as a JSON object). Field names must match the schema enum.
Do not omit merchant-requested changes, invent IDs, or move price into item
attributes. Existing offers are under update.existing_offers only.
A resolved create proposal must contain at least one valid offer. If there is
not enough reliable information to create the requested item, return
operation=ask_merchant and status=needs_more_data with all mutation payloads
empty. Never invent a price, a template ID, or an offer to satisfy validation.
The response_text must never claim that a product was saved or registered;
final proposals are for merchant review only. If details are missing, ask for
those details without claiming the product was created.
No proposal is executed until the merchant approves it.`
	req := nativeMerchantChatRequest{
		Model:     a.model,
		Messages:  []nativeMerchantChatMessage{{Role: "system", Content: system}, {Role: "user", Content: contextualInput}},
		MaxTokens: a.maxOutputTokens,
		Stream:    false,
	}
	allowed := map[string]map[string]any{}
	if input.Capabilities != nil {
		for _, def := range input.Capabilities.Definitions() {
			if def.Name == "" {
				return merchantcatalogai.Proposal{}, errors.New("unnamed merchant discovery tool")
			}
			if _, exists := allowed[def.Name]; exists {
				return merchantcatalogai.Proposal{}, errors.New("duplicate merchant discovery tool")
			}
			strictParams, err := strictDiscoverySchema(def.Parameters)
			if err != nil {
				return merchantcatalogai.Proposal{}, fmt.Errorf("invalid merchant discovery tool %s: %w", def.Name, err)
			}
			allowed[def.Name] = strictParams
			req.Tools = append(req.Tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": def.Name, "description": def.Description,
					"parameters": strictParams, "strict": true,
				},
			})
		}
	}
	if len(req.Tools) > 0 {
		req.ToolChoice = "auto"
	}

	totalCalls := 0
	seenCalls := map[string]struct{}{}
	finalValidationRetries := 0
	provider := "" // Set after the first successful inference, then keep it for this turn.
	for round := 0; round < nativeMaxMerchantToolRounds; round++ {
		if err := ctx.Err(); err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		// Build the correct provider request before checking its byte budget.
		if len(req.Tools) > 0 {
			req.ResponseFormat = nil
		} else {
			req.ToolChoice = ""
			req.ResponseFormat = proposalFormat
			req.Messages[0].Content = trustedProposalInstruction
		}
		body, err := json.Marshal(req)
		if err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("marshal merchant catalog request: %w", err)
		}
		if len(body) > a.maxRequestBytes {
			return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog request exceeds context byte limit: %d > %d", len(body), a.maxRequestBytes)
		}
		if a.AIGuard != nil {
			if err := a.AIGuard.Check(ctx, input.BusinessID); err != nil {
				return merchantcatalogai.Proposal{}, err
			}
		}
		raw, info, err := a.client.CompletePinned(ctx, req, provider)
		if err != nil {
			var wireErr *native.MerchantCatalogSchemaError
			var toolNameErr *native.MerchantCatalogToolNameError
			if (errors.As(err, &wireErr) || errors.As(err, &toolNameErr)) && len(req.Tools) == 0 &&
				finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
				// This is an inference-only retry; no tool or commercial mutation ran.
				// Keep the strict transport, do not normalize malformed merchant data.
				finalValidationRetries++
				if toolNameErr != nil {
					trustedProposalInstruction += "\n\n# Final tool name recovery (one attempt only)\nThe previous response selected a function that was NOT offered for this final step. You MUST call ONLY submit_merchant_catalog_proposal. Do NOT call any merchant_catalog_list_* read tools in this final step. Use the exact declared function name, one tool call only, and obey its strict argument schema. Do not invent product information or prices.\n"
					continue
				}
				trustedProposalInstruction += "\n\n# Strict wire recovery (one attempt only)\nThe previous forced merchant_catalog_wire tool call did not satisfy the\nrequired strict object structure. Reissue one COMPLETE tool call.\nEvery object must contain every schema-required key, even inactive payloads.\nFor operation=ask_merchant: status=needs_more_data when asking for facts;\ncreate={\"fields\":[],\"variants\":[],\"offers\":[]};\nupdate={\"item_id\":\"\",\"attribute_schema_id\":\"\",\"changes\":[],\n\"existing_variants\":[],\"new_variants\":[],\"existing_offers\":[],\"new_offers\":[]};\ndelete={\"id\":\"\",\"reason_given\":\"\"}.\nUse empty arrays for unused collections and empty strings for unused IDs.\nDo not include create fields or offers when operation=ask_merchant.\nDo not fabricate missing prices, variant choices, catalog references,\nor execution results. The response must match the supplied tool schema.\n"
				continue
			}
			return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog native request: %w", err)
		}
		provider = info.Provider
		var response nativeMerchantChatResponse
		if err := json.Unmarshal(raw, &response); err != nil {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog native provider returned invalid chat schema")
		}
		if a.usageReporter != nil {
			tokens := response.Usage.PromptTokensDetails.CachedTokens
			if tokens > response.Usage.PromptTokens {
				tokens = response.Usage.PromptTokens
			}
			usage := ports.CustomerSalesUsageTelemetry{
				Provider: info.Provider, Model: info.Model,
				InputTokens:  response.Usage.PromptTokens,
				CachedTokens: tokens, OutputTokens: response.Usage.CompletionTokens,
				ModelRequests: len(info.Trace.Attempts),
			}
			if err := a.usageReporter(ctx, input.BusinessID, input.SessionID, usage); err != nil {
				return merchantcatalogai.Proposal{}, fmt.Errorf("record merchant gateway spend: %w", err)
			}
		}
		if len(response.Choices) != 1 {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog requires exactly one model choice")
		}
		choice := response.Choices[0]
		if choice.Message.Role != "assistant" || choice.Message.Refusal != nil {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog model returned refusal or unexpected role")
		}
		if len(choice.Message.ToolCalls) > 0 {
			if choice.FinishReason != "tool_calls" {
				return merchantcatalogai.Proposal{}, errors.New("merchant tool calls had invalid completion reason")
			}
			if len(choice.Message.ToolCalls) > nativeMaxMerchantCallsPerRound || totalCalls+len(choice.Message.ToolCalls) > nativeMaxMerchantToolCalls {
				return merchantcatalogai.Proposal{}, errors.New("merchant discovery call budget exceeded")
			}
			if input.Capabilities == nil {
				return merchantcatalogai.Proposal{}, errors.New("merchant catalog read-only capabilities are unavailable")
			}
			var text string
			if len(choice.Message.Content) > 0 && string(choice.Message.Content) != "null" {
				if err := json.Unmarshal(choice.Message.Content, &text); err != nil || strings.TrimSpace(text) != "" {
					return merchantcatalogai.Proposal{}, errors.New("merchant model mixed tool calls with unverified content")
				}
			}
			// Verify the entire batch before executing its first read.
			for _, call := range choice.Message.ToolCalls {
				if call.Type != "function" || call.ID == "" || call.Function.Name == "" {
					return merchantcatalogai.Proposal{}, errors.New("invalid merchant catalog tool call")
				}
				toolSchema, ok := allowed[call.Function.Name]
				if !ok {
					return merchantcatalogai.Proposal{}, errors.New("merchant model requested an unauthorized tool")
				}
				if err := validateStrictArguments([]byte(call.Function.Arguments), toolSchema); err != nil {
					return merchantcatalogai.Proposal{}, fmt.Errorf("invalid merchant tool parameters: %w", err)
				}
				if _, duplicate := seenCalls[call.ID]; duplicate {
					return merchantcatalogai.Proposal{}, errors.New("duplicate merchant tool call ID")
				}
				seenCalls[call.ID] = struct{}{}
				var args map[string]json.RawMessage
				if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil || args == nil {
					return merchantcatalogai.Proposal{}, errors.New("merchant tool arguments must be a JSON object")
				}
			}
			req.Messages = append(req.Messages, nativeMerchantChatMessage{Role: "assistant", ToolCalls: choice.Message.ToolCalls})
			for _, call := range choice.Message.ToolCalls {
				result, err := input.Capabilities.Execute(ctx, merchantcatalogai.MerchantCatalogDiscoveryExecutionContext{
					BusinessID: input.BusinessID, ConversationID: input.SessionID, PrincipalID: input.PrincipalID,
				}, call.Function.Name, []byte(call.Function.Arguments))
				if err != nil {
					return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog read tool %s: %w", call.Function.Name, err)
				}
				// The model must receive the server's pagination contract, not
				// only the page rows. Provider adapters share this canonical contract.
				encoded, err := json.Marshal(struct {
					Data       any    `json:"data"`
					HasMore    bool   `json:"has_more"`
					NextCursor string `json:"next_cursor"`
				}{Data: result.Data, HasMore: result.HasMore, NextCursor: result.NextCursor})
				if err != nil {
					return merchantcatalogai.Proposal{}, errors.New("unable to serialize merchant read-only tool result")
				}
				req.Messages = append(req.Messages, nativeMerchantChatMessage{Role: "tool", ToolCallID: call.ID, Content: string(encoded)})
			}
			totalCalls += len(choice.Message.ToolCalls)
			continue
		}
		if choice.FinishReason != "stop" {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog model did not finish proposal")
		}
		if len(req.Tools) > 0 {
			// A discovery response is NOT a final commercial proposal.
			// End read-only discovery and request one separately validated
			// structured transport envelope on the next inference round.
			req.Tools = nil
			req.ToolChoice = ""
			continue
		}
		var content string
		if err := json.Unmarshal(choice.Message.Content, &content); err != nil || strings.TrimSpace(content) == "" {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog returned no strict structured result")
		}
		proposal, err := catalogWireParse(content)
		if err != nil {
			// Strict structural validation already succeeded, but the model may
			// still supply invalid field JSON, unused definitions, or mutation
			// data in an ask_merchant response. Request a new proposal once,
			// rather than silently correcting or persisting untrusted data.
			if finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
				finalValidationRetries++
				trustedProposalInstruction += `

# Final proposal semantic recovery (one attempt only)
The previous merchant_catalog_wire proposal failed the canonical parser.
Return a new complete strict tool call with every required field.
Every validation_rules_json must be a nonempty valid JSON object string
such as "{}" when no validation constraints apply.
Do not invent attributes, labels, rules, template IDs, offers or prices.
When asking the merchant for missing details, use operation=ask_merchant
and status=needs_more_data, attribute_schema_name="", attribute_definitions=[],
create={"fields":[],"variants":[],"offers":[]},
update={"item_id":"","attribute_schema_id":"","changes":[],
"existing_variants":[],"new_variants":[],"existing_offers":[],"new_offers":[]},
and delete={"id":"","reason_given":""}.
Never claim the product has already been saved.
`
				continue
			}
			return merchantcatalogai.Proposal{}, fmt.Errorf("decode strict merchant catalog wire proposal: %w", err)
		}
		proposal = proposal.Normalize()
		if validator, ok := input.Capabilities.(interface {
			ValidateProposalReferences(merchantcatalogai.Proposal) error
		}); ok {
			if err := validator.ValidateProposalReferences(proposal); err != nil {
				return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal references: %w", err)
			}
		}
		if err := proposal.Validate(); err != nil {
			// A rejected final proposal has no side effects. Give the model one
			// bounded chance to produce a valid proposal or ask for missing data.
			// Do not repair or manufacture commercial fields in Go.
			if finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
				finalValidationRetries++
				trustedProposalInstruction += "\n\nYour previous final proposal failed canonical validation. Rebuild the complete strict wire. If a valid create offer cannot be established from merchant facts, return ask_merchant with needs_more_data and no mutation fields. Do not invent prices or references."
				continue
			}
			return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal: %w", err)
		}
		return proposal, nil
	}
	return merchantcatalogai.Proposal{}, errors.New("merchant catalog maximum model rounds exceeded")
}

var _ merchantcatalogai.MerchantCatalogAuthoringPort = (*NativeMerchantCatalogAuthoringAdapter)(nil)
