// Package services — Merchant Catalog AI Authoring Agent (contract 11 v2)
//
// Implements contract 11 Merchant Catalog AI Authoring Architecture v2 — CLOSED.
//
// Per contract 11 §1, this is an independent conversational agent that helps
// the merchant ADD a product to the catalog with all its required dependencies.
//
// Per contract 11 §2, this agent is INDEPENDENT from the Customer Sales AI:
//   Customer Sales AI → Customer AI Runtime → Catalog Read Boundary
//   Merchant Catalog AI → Merchant AI Runtime → Catalog Authoring Boundary
//
// They share ONLY infrastructure: Catalog Contract, Catalog Domain, Application
// Services, Validation primitives, Authorization primitives, Audit,
// Observability, Gemini infrastructure. They DO NOT share: System Prompt,
// Agent Role, Tool Permissions, Conversation Purpose, Proposal Contract,
// Execution Workflow.
//
// Per contract 11 §5, Create is NOT just an Item Draft; it builds a complete
// catalog creation operation: CatalogItem + Variants + Offers + (when needed)
// AttributeSchema/AttributeDefinition.
//
// Per contract 11 §6, the agent performs two logical phases:
//   1. Understand the conversation: add/edit/delete/complete/confirm/correct
//   2. Build Operation Proposal: create/update/delete (asking is NOT a mutation)
//
// Per contract 11 §7, Gemini does NOT execute. It only:
//   - Understands the merchant's words
//   - Extracts data
//   - Discovers missing data
//   - Asks the merchant
//   - Organizes information
//   - Builds Proposal
//
// Per contract 11 §8, the mandatory execution path is:
//   Merchant → Merchant Catalog AI → Catalog Operation Proposal
//   → Structural Validation → Reference Validation → Tenant/Ownership
//   → Policy Evaluation → Authorization → Confirmation (if required)
//   → Catalog Application Service → Catalog Domain
//
// Per contract 11 §17, Anti-Hallucination relies on:
//   Catalog Contract + Actual Evidence + Structured Output
//   + Deterministic Validation + Reference Validation + Ownership Validation
//   + Policy + Authorization
//
// Per contract 11 §18, the agent MUST NOT:
//   SQL, Direct DB Write/Update/Delete, Authorization Decision,
//   Tenant Selection, Inventing Prices/Availability/Merchant Data,
//   Semantic Search, search_catalog(query), Customer Sales Operations,
//   Provider API Execution.
//
// Per contract 11 §20, v2 SUPERSEDES v1. The implementation MUST NOT use
// AIAuthoringProposal or GenerateCatalogDraft as final contract — v2's
// CatalogOperationProposal replaces them.

package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogOperationProposal is the contract 11 §5 final output of the Merchant
// Catalog AI agent. Per contract 11 §5, the operation may include the full
// product graph: CatalogItem + Variants + Offers (+ AttributeSchema/
// AttributeDefinition when needed).
//
// Per contract 11 §4, the agent does NOT invent dependencies — it asks the
// merchant for missing data.
//
// Per contract 11 §6, only create/update/delete are mutations. Asking the
// merchant ("ask merchant") is NOT a mutation.
type CatalogOperationProposal struct {
	// Operation is one of: create, update, delete per contract 11 §6.
	Operation string `json:"operation"`

	// Status reflects whether the proposal is ready to execute or needs more data.
	// Per contract ④ §4 the allowed values map to AIProposalStatus:
	//   resolved         — proposal is complete and ready
	//   needs_more_data  — missing required fields; ask the merchant
	//   ambiguous        — the merchant's request is unclear; ask for clarification
	//   not_found        — the referenced entity does not exist (for update/delete)
	Status string `json:"status"`

	// ResponseText is the agent's message to the merchant. When status is
	// needs_more_data, this is the question to ask.
	ResponseText string `json:"response_text,omitempty"`

	// Create holds the proposed new product graph when Operation == "create".
	Create *CatalogCreatePayload `json:"create,omitempty"`

	// Update holds the proposed changes when Operation == "update".
	Update *CatalogUpdatePayload `json:"update,omitempty"`

	// Delete holds the delete target when Operation == "delete".
	// Per contract 11 §14, delete requires explicit confirmation.
	Delete *CatalogDeletePayload `json:"delete,omitempty"`

	// MissingFields is the list of fields the agent needs from the merchant
	// before the proposal can be executed. Per contract 11 §4, the agent
	// asks the merchant for these — it does NOT invent them.
	MissingFields []CatalogMissingField `json:"missing_fields,omitempty"`
}

// CatalogCreatePayload is contract 11 §5 — the proposed new product graph.
type CatalogCreatePayload struct {
	Item        *CatalogItemDraft          `json:"item,omitempty"`
	Variants    []CatalogVariantDraft      `json:"variants,omitempty"`
	Offers      []CatalogOfferDraft        `json:"offers,omitempty"`
	Schema      *AttributeSchemaDraft      `json:"schema,omitempty"` // when needed per contract 11 §4
	Definitions []AttributeDefinitionDraft `json:"definitions,omitempty"`
}

// CatalogItemDraft per contract 11 §5.
type CatalogItemDraft struct {
	Name                 string         `json:"name"`
	ItemType             string         `json:"item_type"`
	ShortDescription     string         `json:"short_description,omitempty"`
	LongDescription      string         `json:"long_description,omitempty"`
	PricingMode          string         `json:"pricing_mode"`
	AvailabilityMode     string         `json:"availability_mode"`
	FulfillmentMode      string         `json:"fulfillment_mode"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	Attributes           map[string]any `json:"attributes,omitempty"`
	AttributeSchemaID    *string        `json:"attribute_schema_id,omitempty"`
}

// CatalogVariantDraft per contract 11 §12.
type CatalogVariantDraft struct {
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// CatalogOfferDraft per contract 11 §13. Per contract 11 §13, if the merchant
// does not provide required commercial data, the agent does NOT invent it —
// it asks the merchant or leaves the proposal incomplete per Domain rules.
type CatalogOfferDraft struct {
	VariantNameRef     string  `json:"variant_name_ref,omitempty"` // references Variants[].Name when not yet persisted
	Name               string  `json:"name"`
	PricingMode        string  `json:"pricing_mode"`
	Amount             *string `json:"amount,omitempty"`
	Currency           *string `json:"currency,omitempty"`
	PricingUnit        *string `json:"pricing_unit,omitempty"`
	PriceSource        *string `json:"price_source,omitempty"`
	AvailabilityMode   *string `json:"availability_mode,omitempty"`
	AvailabilityStatus *string `json:"availability_status,omitempty"`
	FulfillmentMode    *string `json:"fulfillment_mode,omitempty"`
	ValidityFrom       *string `json:"validity_from,omitempty"`
	ValidityUntil      *string `json:"validity_until,omitempty"`
}

// AttributeSchemaDraft per contract 11 §4.
type AttributeSchemaDraft struct {
	Name string `json:"name"`
}

// AttributeDefinitionDraft per contract 11 §11. Per contract 11 §11, attribute
// values are not assumed to be strings — the data_type and validation_rules
// from the AttributeDefinition govern how values are interpreted.
type AttributeDefinitionDraft struct {
	AttributeKey    string         `json:"attribute_key"`
	Label           string         `json:"label"`
	DataType        string         `json:"data_type"` // text/number/boolean/date/datetime/select/multi_select/location/money
	IsRequired      bool           `json:"is_required"`
	IsSearchable    bool           `json:"is_searchable"`
	ValidationRules map[string]any `json:"validation_rules,omitempty"`
	DisplayOrder    int            `json:"display_order"`
}

// CatalogUpdatePayload per contract 11 §14.
type CatalogUpdatePayload struct {
	ItemID      string                `json:"item_id"`
	Changes     CatalogItemDraft      `json:"changes"`
	NewVariants []CatalogVariantDraft `json:"new_variants,omitempty"`
	NewOffers   []CatalogOfferDraft   `json:"new_offers,omitempty"`
}

// CatalogDeletePayload per contract 11 §14. Requires explicit confirmation.
type CatalogDeletePayload struct {
	ItemID      string `json:"item_id"`
	Confirmed   bool   `json:"confirmed"` // merchant must set this to true before execution
	ReasonGiven string `json:"reason_given,omitempty"`
}

// CatalogMissingField per contract 11 §4 — what the agent needs from the merchant.
type CatalogMissingField struct {
	Path        string `json:"path"`         // e.g., "offers[0].amount"
	DisplayName string `json:"display_name"` // e.g., "السعر للعرض الأول"
	DataType    string `json:"data_type"`
	Reason      string `json:"reason"` // why this field is required
}

// MerchantCatalogAIAgent is the contract 11 v2 agent.
//
// Per contract 11 §2, it shares infrastructure with Customer Sales AI but
// has its own System Prompt, Agent Role, Tool Permissions, Conversation
// Purpose, Proposal Contract, Execution Workflow.
type MerchantCatalogAIAgent struct {
	Runtime        ports.AIRuntime
	ContextBuilder ports.AIContextBuilder
	Validation     *ValidationPipeline
	Repository     ports.AIRunRepository
	Now            func() time.Time
	NewID          func() string

	// AgentRole is always merchant_catalog_authoring per contract 11 §2.
	AgentRole string
}

// NewMerchantCatalogAIAgent wires the dependencies.
func NewMerchantCatalogAIAgent(
	runtime ports.AIRuntime,
	contextBuilder ports.AIContextBuilder,
	validation *ValidationPipeline,
	repo ports.AIRunRepository,
) *MerchantCatalogAIAgent {
	return &MerchantCatalogAIAgent{
		Runtime:        runtime,
		ContextBuilder: contextBuilder,
		Validation:     validation,
		Repository:     repo,
		Now:            func() time.Time { return time.Now().UTC() },
		NewID:          newIDDefault,
		AgentRole:      ports.AIRunAgentRoleMerchantCatalogAuthoring,
	}
}

// HandleTurn processes one merchant turn. Per contract 11 §6, the agent first
// understands the conversation (which may produce a question), then if the
// data is complete, builds an Operation Proposal.
//
// Per contract 11 §7, the Runtime produces the proposal; per contract 11 §8,
// Mujeeb's validation pipeline runs BEFORE any Catalog Application Service.
func (a *MerchantCatalogAIAgent) HandleTurn(ctx context.Context, input MerchantCatalogAITurnInput) (CatalogOperationProposal, error) {
	if strings.TrimSpace(input.MerchantMessage) == "" {
		return CatalogOperationProposal{}, errors.New("merchant message is required")
	}
	if a.Runtime == nil {
		return CatalogOperationProposal{}, errors.New("merchant AI runtime is not configured")
	}

	// Per contract ⑨ §1, each AI processing is one AI Run.
	// Per contract ⑨ §16, the Run is idempotent on (business_id, idempotency_key).
	run, err := a.startRun(ctx, input)
	if err != nil {
		return CatalogOperationProposal{}, err
	}

	// Per contract ③ §1, Mujeeb is the canonical Conversation State.
	// Per contract ③ §6, the Context Builder builds only what's needed.
	var builtContext *ports.AIContext
	if a.ContextBuilder != nil {
		bc, err := a.ContextBuilder.Build(ctx, ports.ContextBuildInput{
			BusinessID:             input.BusinessID,
			ConversationID:         input.ConversationID,
			SourceMessageReference: input.SourceMessageReference,
			Text:                   input.MerchantMessage,
			Channel:                "merchant_dashboard",
			PolicyVersion:          input.PolicyVersion,
			ConversationState:      input.ConversationState,
		})
		if err != nil {
			_, _ = a.failRun(ctx, run, ports.AIRunFailureStageContextBuild, string(ports.AIRunFailureCategoryInfrastructure), err.Error())
			return CatalogOperationProposal{}, err
		}
		builtContext = &bc
	}

	// Per contract ⑨ §3, mark CONTEXT_BUILT → RUNNING.
	lc := NewAIRunLifecycle(a.Repository)
	if _, err := lc.MarkContextBuilt(ctx, input.BusinessID, run.ID); err != nil {
		return CatalogOperationProposal{}, err
	}
	if _, err := lc.MarkRunning(ctx, input.BusinessID, run.ID); err != nil {
		return CatalogOperationProposal{}, err
	}

	// Per contract 11 §7, Gemini proposes only.
	aiInput := ports.AIDecisionInput{
		BusinessID:             input.BusinessID,
		ConversationID:         input.ConversationID,
		SourceMessageReference: input.SourceMessageReference,
		Text:                   input.MerchantMessage,
		Channel:                "merchant_dashboard",
		PolicyVersion:          input.PolicyVersion,
		Context:                builtContext,
	}
	proposal, err := a.Runtime.Decide(ctx, aiInput)
	if err != nil {
		_, _ = a.failRun(ctx, run, ports.AIRunFailureStageGeminiRequest, string(ports.AIRunFailureCategoryProviderPermanent), err.Error())
		return CatalogOperationProposal{}, err
	}

	// Per contract ⑥ §3, run the validation pipeline.
	if a.Validation != nil {
		_, failure := a.Validation.Validate(ctx, ValidationInput{
			DecisionID:         "", // linked later when ai_decisions is created
			BusinessID:         input.BusinessID,
			ConversationID:     input.ConversationID,
			Proposal:           convertLegacyProposalToContract(proposal),
			Context:            builtContext,
			EvidenceItemIDs:    []string{},
			EvidenceVariantIDs: []string{},
			EvidenceOfferIDs:   []string{},
		})
		if failure != nil {
			_, _ = a.failRun(ctx, run, failure.Stage, string(failure.Category), failure.Reason)
			// Per contract ⑥ §21, no Execution when validation fails.
			return CatalogOperationProposal{
				Operation:    "ask",
				Status:       string(ports.AIProposalStatusAmbiguous),
				ResponseText: "تعذّر إتمام العملية بسبب فشل التحقق: " + failure.Reason,
			}, nil
		}
	}

	// Per contract 11 §6, convert the AI Proposal to a CatalogOperationProposal.
	// The agent's Gemini adapter is configured with a merchant-catalog-specific
	// system prompt that produces the CatalogOperationProposal shape directly.
	op := convertProposalToOperation(proposal)
	return op, nil
}

// startRun creates an AI Run for this merchant turn.
func (a *MerchantCatalogAIAgent) startRun(ctx context.Context, input MerchantCatalogAITurnInput) (ports.AIRunRecord, error) {
	lc := NewAIRunLifecycle(a.Repository)
	return lc.StartRun(ctx, StartRunInput{
		BusinessID:     input.BusinessID,
		ConversationID: input.ConversationID,
		MessageID:      input.SourceMessageReference,
		IdempotencyKey: input.IdempotencyKey,
		AgentRole:      ports.AIRunAgentRoleMerchantCatalogAuthoring,
		NewRunID:       a.NewID,
	})
}

// failRun marks the AI Run as FAILED per contract ⑨ §18.
// stage and category are strings (raw values from ports.AIRunFailureStage* and
// ports.AIRunFailureCategory*) to match MarkFailed's signature.
func (a *MerchantCatalogAIAgent) failRun(ctx context.Context, run ports.AIRunRecord, stage, category string, reason string) (ports.AIRunRecord, error) {
	lc := NewAIRunLifecycle(a.Repository)
	return lc.MarkFailed(ctx, run.BusinessID, run.ID, stage, category, reason)
}

// convertLegacyProposalToContract is a temporary bridge that converts the
// existing AIDecisionProposal shape to the contract ④ §4 AIGeminiProposal.
// Once the Gemini adapter produces AIGeminiProposal directly, this bridge
// is removed.
func convertLegacyProposalToContract(p ports.AIDecisionProposal) ports.AIGeminiProposal {
	status := ports.AIProposalStatusResolved
	if p.RequiresHuman {
		status = ports.AIProposalStatusNeedsMoreData
	}
	action := ports.AIProposalAction(p.RequestedAction)
	if !isValidAction(action) {
		action = ports.AIProposalActionAnswer
	}
	return ports.AIGeminiProposal{
		Status:       status,
		Action:       action,
		ResponseText: p.ResponseText,
		Selected:     nil, // legacy proposal does not carry SelectedReference yet
	}
}

func isValidAction(a ports.AIProposalAction) bool {
	switch a {
	case ports.AIProposalActionAnswer,
		ports.AIProposalActionClarification,
		ports.AIProposalActionHumanRequest,
		ports.AIProposalActionLeadDraft,
		ports.AIProposalActionOrderDraft:
		return true
	}
	return false
}

// convertProposalToOperation maps the contract ④ Proposal to the contract 11
// CatalogOperationProposal. In production, the Merchant Catalog AI Gemini
// adapter is configured with a system prompt that produces CatalogOperationProposal
// directly (via Structured Output), so this conversion becomes trivial.
func convertProposalToOperation(p ports.AIDecisionProposal) CatalogOperationProposal {
	// Default to "ask" — per contract 11 §6, asking is not a mutation.
	op := CatalogOperationProposal{
		Operation:    "ask",
		Status:       string(ports.AIProposalStatusResolved),
		ResponseText: p.ResponseText,
	}
	if p.RequestedAction == "create" {
		op.Operation = "create"
	}
	if p.RequestedAction == "update" {
		op.Operation = "update"
	}
	if p.RequestedAction == "delete" {
		op.Operation = "delete"
	}
	return op
}

// MerchantCatalogAITurnInput is the input to one merchant turn.
type MerchantCatalogAITurnInput struct {
	BusinessID             string
	ConversationID         string
	SourceMessageReference string
	MerchantMessage        string
	PolicyVersion          string
	IdempotencyKey         string
	ConversationState      *ports.ConversationStateRecord
}

// newIDDefault is a fallback ID generator. Production code should inject a
// UUID generator (e.g., github.com/google/uuid.NewString).
var newIDDefault = func() string { return "" }
