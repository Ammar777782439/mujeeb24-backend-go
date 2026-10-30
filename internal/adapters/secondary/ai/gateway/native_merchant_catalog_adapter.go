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
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/merchantcatalogtrace"
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
	ctx = merchantcatalogtrace.WithSession(ctx, input.SessionID)
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
Pricing mode belongs exclusively to offers: include pricing_mode in each
new offer's fields, never in create.fields or update.changes.
For every new OfferCreate in create.offers or update.new_offers, supply all
required domain values in the corresponding fields array. In particular,
availability_mode must be one of stock, schedule, supplier_check,
always_available, unknown; availability_status must be one of available,
unavailable, unknown, requires_check, stale; fulfillment_mode must be one of
delivery, pickup, digital, appointment, travel, manual; and status must be one
of draft, active, inactive, expired, archived. They are NOT optional empty
strings. Values inside value_json must be JSON-encoded, e.g. "\"unknown\"".
Unknown availability is NOT proof of stock or immediate availability.
Do not claim delivery or other fulfillment modes without merchant facts.
For a new offer with no merchant-stated amount and no other pricing mode requested,
use pricing_mode=quote_required, price_source=not_stated, omit amount, and set
currency to the trusted business default_currency when available.
Never use starting_from without a numeric amount or copy a different item's price.
If merchant did not name an offer, use name_source=system_default and name="سعر البيع".
If required non-default business facts are still missing, ask with
operation=ask_merchant, status=needs_more_data, and inactive data arrays empty.
Ask specific grouped questions in response_text, with matching critical
missing_information entries; do not silently omit essential customer-facing facts.
Do not request an offer amount merely because it was not stated when a valid
quote_required offer can communicate the pricing truth.
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
A selected template may be extended with NEW, independent attributes after
merchant approval. Match existing definitions by meaning, type AND rules,
not by a similar label alone. Never redefine an existing attr_ field or change
its stored enum to fit an unrelated merchant value. When the merchant describes
a different concept that collides with an existing Arabic label, give the NEW
custom attribute a distinct, precise Arabic label and a fresh snake_case alias;
keep the original attr_ definition and its values unchanged. This is general
to every industry; do not discard merchant details or substitute allowed values.
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
		merchantcatalogtrace.Dump(ctx, fmt.Sprintf("canonical_request_round_%d", round+1), body)
		raw, info, err := a.client.CompletePinned(ctx, req, provider)
		if err != nil {
			merchantcatalogtrace.Event(ctx, "provider_error", fmt.Sprintf("round=%d pinned_provider=%q error=%q", round+1, provider, err.Error()))
			var wireErr *native.MerchantCatalogSchemaError
			var toolNameErr *native.MerchantCatalogToolNameError
			var envelopeErr *native.MerchantCatalogCallEnvelopeError
			if (errors.As(err, &wireErr) || errors.As(err, &toolNameErr) ||
				errors.As(err, &envelopeErr)) && len(req.Tools) == 0 &&
				finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
				// This is an inference-only retry; no tool or commercial mutation ran.
				// Keep the strict transport, do not normalize malformed merchant data.
				finalValidationRetries++
				if envelopeErr != nil {
					trustedProposalInstruction += "\n\n# Final tool-call envelope recovery (one attempt only)\nYour previous final function call was not a well-formed tool call. Submit exactly one function call named submit_merchant_catalog_proposal, with a nonempty call identifier, type=function, and a complete, valid JSON object as its arguments matching the supplied strict schema. Do not call discovery tools here. Do not invent missing business facts, offers or prices.\n"
					continue
				}
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
		merchantcatalogtrace.Event(ctx, "provider_success", fmt.Sprintf("round=%d provider=%q model=%q attempts=%d", round+1, info.Provider, info.Model, len(info.Trace.Attempts)))
		merchantcatalogtrace.Dump(ctx, fmt.Sprintf("canonical_response_round_%d", round+1), raw)
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
		merchantcatalogtrace.Dump(ctx, fmt.Sprintf("canonical_wire_round_%d", round+1), []byte(content))
		proposal, err := catalogWireParse(content)
		if err != nil {
			merchantcatalogtrace.Event(ctx, "wire_rejected", fmt.Sprintf("round=%d error=%q", round+1, err.Error()))
			// Strict structural validation already succeeded, but the model may
			// still supply invalid field JSON, unused definitions, or mutation
			// data in an ask_merchant response. Request a new proposal once,
			// rather than silently correcting or persisting untrusted data.
			if finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
				finalValidationRetries++
				trustedProposalInstruction += "\n\n# Specific canonical parser feedback (one attempt only)\n" + merchantCatalogWireFeedback(content)
				continue
			}
			return merchantcatalogai.Proposal{}, fmt.Errorf("decode strict merchant catalog wire proposal: %w", err)
		}
		merchantcatalogtrace.Event(ctx, "wire_parsed", fmt.Sprintf("round=%d operation=%q status=%q definitions=%d schema_name=%q", round+1, proposal.Operation, proposal.Status, len(proposal.AttributeDefinitions), proposal.AttributeSchemaName))
		proposal = proposal.Normalize()
		if validator, ok := input.Capabilities.(interface {
			ValidateProposalReferences(merchantcatalogai.Proposal) error
		}); ok {
			if err := validator.ValidateProposalReferences(proposal); err != nil {
				merchantcatalogtrace.Event(ctx, "references_rejected", fmt.Sprintf("round=%d error=%q", round+1, err.Error()))
				if finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
					finalValidationRetries++
					trustedProposalInstruction += "\n\n# Canonical catalog reference correction (one attempt only)\nThe previous proposal referenced catalog items, schemas, offers, or variants that Go cannot validate against authorized discovery evidence. Use only exact evidence-backed IDs from this merchant's catalog. If verified references are unavailable, use operation=ask_merchant, status=needs_more_data, and empty mutation fields. Never invent IDs, move data across tenants or retry read-only tools in this final step."
					continue
				}
				return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal references: %w", err)
			}
		}
		if err := proposal.Validate(); err != nil {
			merchantcatalogtrace.Event(ctx, "proposal_rejected", fmt.Sprintf("round=%d error=%q", round+1, err.Error()))
			// A rejected final proposal has no side effects. Give the model one
			// bounded chance to produce a valid proposal or ask for missing data.
			// Do not repair or manufacture commercial fields in Go.
			if finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
				finalValidationRetries++
				trustedProposalInstruction += "\n\n# Canonical proposal validation correction (one attempt only)\n" + merchantCatalogWireFeedback(content) + "\n" + merchantCatalogValidationFeedback(err)
				continue
			}
			return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal: %w", err)
		}
		if validator, ok := input.Capabilities.(interface {
			ValidateProposalSchemaAttributes(context.Context, string, merchantcatalogai.Proposal) error
		}); ok {
			if err := validator.ValidateProposalSchemaAttributes(ctx, input.BusinessID, proposal); err != nil {
				var fieldErr *merchantcatalogai.SchemaAttributePreflightError
				var definitionErr *merchantcatalogai.SchemaDefinitionPreflightError
				if finalValidationRetries == 0 && round+1 < nativeMaxMerchantToolRounds {
					finalValidationRetries++
					feedback := "The proposed item or variant attributes violate its existing merchant catalog template. Read-only schema evidence must be respected. Choose ask_merchant if an essential value or valid choice is unknown. Do not invent values or change the existing attribute schema."
					if errors.As(err, &fieldErr) {
						feedback = "The existing catalog attribute schema rejects " + fieldErr.Path +
							" (data_type=" + fieldErr.DataType + "). For multi_select use a JSON array of allowed choices, even when only one variant option is selected. For select use one allowed scalar. Required item-level attributes cannot be omitted. Use the actual schema definition and enum from authorized discovery evidence. If data is not known, ask_merchant with no mutations. Do not change or invent the schema."
					}
					if errors.As(err, &definitionErr) {
						feedback = fmt.Sprintf("The proposed attribute_definitions[%d] failed the selected merchant template preflight (reason=%s, existing_type=%s, proposed_type=%s). Do not overwrite existing definitions or their rules. Keep existing attr_ keys unchanged. If the merchant meant the SAME concept, use its evidence-backed attr_ key with the stored type and enum, and remove the duplicate definition. If the merchant meant a DIFFERENT concept despite a similar or identical display label, preserve the merchant's requested values by adding a separate custom definition with a NEW, semantically precise Arabic label and a fresh snake_case alias; use that alias in attributes. The selected template may gain independent new definitions after approval without rewriting existing definitions. Do not invent values or silently alter enums. If the merchant specifically wants to change an existing definition's rules or its intent cannot be established, ask_merchant with empty mutation/template payloads.",
							definitionErr.Index, definitionErr.Reason, definitionErr.ExpectedType, definitionErr.ProposedType)
					}
					trustedProposalInstruction += "\n\n# Existing catalog schema preflight correction (one attempt only)\n" + feedback
					continue
				}
				return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal attributes: %w", err)
			}
		}
		merchantcatalogtrace.Event(ctx, "proposal_accepted", fmt.Sprintf("round=%d operation=%q status=%q", round+1, proposal.Operation, proposal.Status))
		return proposal, nil
	}
	return merchantcatalogai.Proposal{}, errors.New("merchant catalog maximum model rounds exceeded")
}

var _ merchantcatalogai.MerchantCatalogAuthoringPort = (*NativeMerchantCatalogAuthoringAdapter)(nil)
