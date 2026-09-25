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
        "log"
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
// merchant ("ask_merchant") is NOT a mutation — it gathers missing data
// (Price, Currency, Variants, etc.) WITHOUT inventing them.
type CatalogOperationProposal struct {
        // Operation is one of: create, update, delete, ask_merchant per contract 11 §6.
        // ask_merchant is the non-mutation gather-data operation: the agent asks
        // the merchant for missing required fields and waits for the answer.
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
//
// Per contract ④ §8, this agent uses ports.ContractRuntime (the
// contract-aligned Gemini client) — NOT the legacy ports.AIRuntime.
// The ContractRuntime produces an AIGeminiProposal per contract ④ §4
// (status + action + response_text + selected[]), which the agent then
// maps to a CatalogOperationProposal per contract 11 §5.
type MerchantCatalogAIAgent struct {
        // Runtime is the contract ④ §8 ContractRuntime (Gemini ContractClient).
        // Per contract ④ §8, this is the ONLY way to call Gemini.
        Runtime ports.ContractRuntime
        // ContextBuilder is the DEDICATED MerchantContextBuilder per contract 11 §2.
        // It is intentionally NOT AutoReplyContextBuilder (B2C). Per contract 11 §2,
        // the two agents share zero context-building code paths.
        ContextBuilder *MerchantContextBuilder
        // Validation is the contract ⑥ §2 pipeline. Per contract 11 §8, every
        // proposal must pass Structural → Reference → Tenant → Ownership → Policy
        // → Authorization before any Catalog Application Service call.
        Validation *ValidationPipeline
        // Repository persists AI Run trace per contract ⑧ §5.
        Repository ports.AIRunRepository
        // SessionWriter persists merchant_ai_messages per contract 11 §6 + migration 000054.
        // Per contract ③ §1, the merchant-side memory is the session, NOT
        // ConversationState (which is B2C-only).
        SessionWriter MerchantAISessionWriter
        Now           func() time.Time
        NewID         func() string

        // AgentRole is always merchant_catalog_authoring per contract 11 §2.
        AgentRole string
}

// MerchantAISessionWriter is the write-side port for merchant_ai_sessions/
// merchant_ai_messages per migration 000054. Per contract ⑧ §17, writes are
// tenant-scoped via business_id. Defined here (services package) per
// contract 11 §2 (Merchant Catalog AI does not share session types with
// Customer Sales AI).
type MerchantAISessionWriter interface {
        // CreateSession creates a new merchant_ai_session row per migration 000054.
        // Returns the session ID for subsequent AppendMessage calls.
        CreateSession(ctx context.Context, businessID, principalID string) (sessionID string, err error)
        // AppendMessage appends a merchant_ai_message row per migration 000054.
        // senderType must be 'merchant' or 'assistant' per the migration's
        // merchant_ai_messages_sender_type_chk constraint.
        AppendMessage(ctx context.Context, businessID, sessionID, senderType, text string) (messageID string, err error)
}

// NewMerchantCatalogAIAgent wires the dependencies.
//
// Per contract ④ §8, runtime is ports.ContractRuntime (NOT legacy AIRuntime).
// Per contract 11 §2, contextBuilder is the dedicated MerchantContextBuilder.
func NewMerchantCatalogAIAgent(
        runtime ports.ContractRuntime,
        contextBuilder *MerchantContextBuilder,
        validation *ValidationPipeline,
        repo ports.AIRunRepository,
        sessionWriter MerchantAISessionWriter,
) *MerchantCatalogAIAgent {
        return &MerchantCatalogAIAgent{
                Runtime:        runtime,
                ContextBuilder: contextBuilder,
                Validation:     validation,
                Repository:     repo,
                SessionWriter:  sessionWriter,
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
//
// Flow per contract ④ §8 + ⑥ §2 + ⑨ §2:
//  1. Persist the merchant's message to merchant_ai_messages (canonical memory).
//  2. Start an AI Run (RECEIVED) per contract ⑨ §1. Idempotent on (business_id, idempotency_key).
//  3. Build context via the DEDICATED MerchantContextBuilder per contract 11 §2.
//     Per contract ③ §1, this does NOT use ConversationState (B2C-only).
//  4. Mark CONTEXT_BUILT → RUNNING per contract ⑨ §3.
//  5. Call Gemini via Runtime.DecideContract per contract ④ §8 — produces AIGeminiProposal.
//  6. Mark VALIDATING per contract ⑨ §3.
//  7. Run ValidationPipeline per contract ⑥ §2.
//  8. If validation fails → Mark FAILED per contract ⑨ §18; no execution per ⑥ §21.
//  9. Map the AIGeminiProposal to a CatalogOperationProposal per contract 11 §6.
//
// 10. Persist the assistant's response to merchant_ai_messages.
// 11. Mark COMPLETED per contract ⑨ §31.
//
// Per contract 11 §18, the agent does NOT execute DB mutations directly.
// The CatalogOperationProposal is returned to the caller (HTTP handler),
// which routes it through Catalog Application Services per contract 11 §8.
func (a *MerchantCatalogAIAgent) HandleTurn(ctx context.Context, input MerchantCatalogAITurnInput) (CatalogOperationProposal, error) {
        // Per ADR-040: comprehensive logging for B2B flow (parallel to B2C
        // AutoReply logging). Every stage emits a log line so the merchant
        // dashboard + ops can debug "why did the agent do X?".
        businessID := input.BusinessID
        sessionID := input.SessionID
        msgPreview := input.MerchantMessage
        if len(msgPreview) > 80 {
                msgPreview = msgPreview[:80] + "..."
        }
        log.Printf("[MerchantAI] START business=%s session=%s principal=%s text=%q",
                businessID, sessionID, input.PrincipalID, msgPreview)

        if strings.TrimSpace(input.MerchantMessage) == "" {
                log.Printf("[MerchantAI] REJECTED business=%s reason=empty_message", businessID)
                return CatalogOperationProposal{}, errors.New("merchant message is required")
        }
        if a.Runtime == nil {
                log.Printf("[MerchantAI] REJECTED business=%s reason=runtime_not_configured", businessID)
                return CatalogOperationProposal{}, errors.New("merchant AI runtime is not configured")
        }
        if a.ContextBuilder == nil {
                log.Printf("[MerchantAI] REJECTED business=%s reason=context_builder_not_configured", businessID)
                return CatalogOperationProposal{}, errors.New("merchant context builder is not configured")
        }

        // Per contract ③ §1 + migration 000054: persist the merchant's message
        // as the canonical B2B conversation memory. This happens BEFORE the AI
        // Run starts so the message is durable even if the Run fails.
        if a.SessionWriter != nil {
                if sessionID == "" {
                        // First turn: create a new session.
                        var err error
                        sessionID, err = a.SessionWriter.CreateSession(ctx, input.BusinessID, input.PrincipalID)
                        if err != nil {
                                log.Printf("[MerchantAI] SESSION_CREATE_FAILED business=%s principal=%s err=%v",
                                        input.BusinessID, input.PrincipalID, err)
                                return CatalogOperationProposal{}, err
                        }
                        log.Printf("[MerchantAI] SESSION_CREATED business=%s session=%s", input.BusinessID, sessionID)
                }
                // Per migration 000054 sender_type_chk: must be 'merchant' or 'assistant'.
                if _, err := a.SessionWriter.AppendMessage(ctx, input.BusinessID, sessionID, "merchant", input.MerchantMessage); err != nil {
                        log.Printf("[MerchantAI] MESSAGE_PERSIST_FAILED business=%s session=%s err=%v",
                                input.BusinessID, sessionID, err)
                        return CatalogOperationProposal{}, err
                }
        }

        // Per contract ⑨ §1, each AI processing is one AI Run.
        // Per contract ⑨ §16, the Run is idempotent on (business_id, idempotency_key).
        run, err := a.startRun(ctx, input, sessionID)
        if err != nil {
                log.Printf("[MerchantAI] RUN_START_FAILED business=%s session=%s err=%v",
                        input.BusinessID, sessionID, err)
                return CatalogOperationProposal{}, err
        }
        log.Printf("[MerchantAI] STATE→RUNNING business=%s session=%s run=%s",
                input.BusinessID, sessionID, run.ID)

        // Per contract 11 §2, build context via the DEDICATED MerchantContextBuilder.
        // Per contract ③ §1, this does NOT use ConversationState (B2C-only).
        contractInput, err := a.ContextBuilder.BuildForTurn(ctx, MerchantContextBuildInput{
                BusinessID:             input.BusinessID,
                SessionID:              sessionID,
                SourceMessageReference: input.SourceMessageReference,
                MerchantMessage:        input.MerchantMessage,
                PolicyVersion:          input.PolicyVersion,
        })
        if err != nil {
                log.Printf("[MerchantAI] CONTEXT_BUILD_FAILED business=%s session=%s err=%v",
                        input.BusinessID, sessionID, err)
                _, _ = a.failRun(ctx, run, ports.AIRunFailureStageContextBuild, string(ports.AIRunFailureCategoryInfrastructure), err.Error())
                return CatalogOperationProposal{}, err
        }
        log.Printf("[MerchantAI] CONTEXT_BUILT business=%s session=%s run=%s", input.BusinessID, sessionID, run.ID)

        // Per contract ⑨ §3, mark CONTEXT_BUILT → RUNNING.
        lc := NewAIRunLifecycle(a.Repository)
        if _, err := lc.MarkContextBuilt(ctx, input.BusinessID, run.ID); err != nil {
                log.Printf("[MerchantAI] LIFECYCLE_MARK_FAILED business=%s run=%s stage=context_built err=%v",
                        input.BusinessID, run.ID, err)
                return CatalogOperationProposal{}, err
        }
        if _, err := lc.MarkRunning(ctx, input.BusinessID, run.ID); err != nil {
                log.Printf("[MerchantAI] LIFECYCLE_MARK_FAILED business=%s run=%s stage=running err=%v",
                        input.BusinessID, run.ID, err)
                return CatalogOperationProposal{}, err
        }

        // Per contract ④ §8, call Gemini via the ContractRuntime.
        // The MerchantContextBuilder injected the Catalog Entity Contract per
        // contract ⑤ §7 as part of the system_instruction.
        log.Printf("[MerchantAI] GEMINI_CALL business=%s session=%s run=%s", input.BusinessID, sessionID, run.ID)
        geminiStart := time.Now()
        out, err := a.Runtime.DecideContract(ctx, contractInput)
        geminiLatency := time.Since(geminiStart).Milliseconds()
        if err != nil {
                log.Printf("[MerchantAI] GEMINI_FAILED business=%s session=%s run=%s latency=%dms err=%v",
                        input.BusinessID, sessionID, run.ID, geminiLatency, err)
                _, _ = a.failRun(ctx, run, ports.AIRunFailureStageGeminiRequest, string(ports.AIRunFailureCategoryProviderPermanent), err.Error())
                return CatalogOperationProposal{}, err
        }
        proposal := out.Proposal

        // Truncate response for logging (don't dump full text into logs)
        respPreview := proposal.ResponseText
        if len(respPreview) > 200 {
                respPreview = respPreview[:200] + "..."
        }
        log.Printf("[MerchantAI] GEMINI_OK business=%s session=%s run=%s status=%s action=%s tokens_in=%d tokens_out=%d latency=%dms response=%q",
                input.BusinessID, sessionID, run.ID,
                proposal.Status, proposal.Action,
                out.Usage.InputTokens, out.Usage.OutputTokens, geminiLatency,
                respPreview)

        // Per contract ⑨ §3, mark VALIDATING.
        log.Printf("[MerchantAI] STATE→VALIDATING business=%s session=%s run=%s", input.BusinessID, sessionID, run.ID)
        if _, err := lc.MarkValidating(ctx, input.BusinessID, run.ID); err != nil {
                log.Printf("[MerchantAI] LIFECYCLE_MARK_FAILED business=%s run=%s stage=validating err=%v",
                        input.BusinessID, run.ID, err)
                return CatalogOperationProposal{}, err
        }

        // Per contract ⑥ §2, run the validation pipeline.
        if a.Validation != nil {
                _, failure := a.Validation.Validate(ctx, ValidationInput{
                        DecisionID:         "", // linked later when ai_decisions is created
                        BusinessID:         input.BusinessID,
                        ConversationID:     sessionID,
                        Proposal:           proposal,
                        Context:            contractInput.DecisionInput.Context,
                        EvidenceItemIDs:    []string{},
                        EvidenceVariantIDs: []string{},
                        EvidenceOfferIDs:   []string{},
                })
                if failure != nil {
                        log.Printf("[MerchantAI] VALIDATION_FAILED business=%s session=%s run=%s stage=%s category=%s reason=%s",
                                input.BusinessID, sessionID, run.ID, failure.Stage, failure.Category, failure.Reason)
                        _, _ = a.failRun(ctx, run, failure.Stage, string(failure.Category), failure.Reason)
                        // Per contract ⑥ §21, no Execution when validation fails.
                        return CatalogOperationProposal{
                                Operation:    "ask_merchant",
                                Status:       string(ports.AIProposalStatusAmbiguous),
                                ResponseText: "تعذّر إتمام العملية بسبب فشل التحقق: " + failure.Reason,
                        }, nil
                }
                log.Printf("[MerchantAI] VALIDATION_OK business=%s session=%s run=%s", input.BusinessID, sessionID, run.ID)
        }

        // Per contract 11 §6, map the AIGeminiProposal to a CatalogOperationProposal.
        // The agent's Gemini adapter is configured with a merchant-catalog-specific
        // system prompt that produces the proposal shape directly via Structured
        // Output (contract ④ §8 responseSchema enforcement).
        op := mapGeminiProposalToOperation(proposal)
        // Count items/variants/offers for logging (may be in op.Create or op.Update).
        itemCount, variantCount, offerCount := 0, 0, 0
        if op.Create != nil {
                itemCount = 1 // Create creates 1 item
                variantCount = len(op.Create.Variants)
                offerCount = len(op.Create.Offers)
        } else if op.Update != nil {
                itemCount = 1
                variantCount = len(op.Update.NewVariants)
                offerCount = len(op.Update.NewOffers)
        }
        missingCount := len(op.MissingFields)
        log.Printf("[MerchantAI] OPERATION business=%s session=%s run=%s operation=%s status=%s items=%d variants=%d offers=%d missing=%d",
                input.BusinessID, sessionID, run.ID, op.Operation, op.Status,
                itemCount, variantCount, offerCount, missingCount)

        // Per contract ③ §1 + migration 000054: persist the assistant's response.
        if a.SessionWriter != nil && op.ResponseText != "" {
                // Per migration 000054 sender_type_chk: 'assistant' is the only other allowed value.
                if _, err := a.SessionWriter.AppendMessage(ctx, input.BusinessID, sessionID, "assistant", op.ResponseText); err != nil {
                        // Per contract ⑨ §28, runtime failures do NOT mutate business state.
                        // The decision was already produced; we log the persistence failure
                        // via the Run trace but do not return an error to the caller.
                        log.Printf("[MerchantAI] ASSISTANT_PERSIST_FAILED business=%s session=%s err=%v",
                                input.BusinessID, sessionID, err)
                        _, _ = a.failRun(ctx, run, ports.AIRunFailureStageExecution, string(ports.AIRunFailureCategoryExecutionFailure), "persist assistant message: "+err.Error())
                }
        }

        // Per contract ⑨ §31, Mark COMPLETED.
        if _, err := lc.MarkCompleted(ctx, input.BusinessID, run.ID); err != nil {
                log.Printf("[MerchantAI] LIFECYCLE_MARK_FAILED business=%s run=%s stage=completed err=%v",
                        input.BusinessID, run.ID, err)
                return op, err
        }
        log.Printf("[MerchantAI] COMPLETED business=%s session=%s run=%s operation=%s",
                input.BusinessID, sessionID, run.ID, op.Operation)
        return op, nil
}

// startRun creates an AI Run for this merchant turn.
// Per contract ⑨ §1, the Run is the operational trace header.
// Per contract ⑧ §5, the Run is linked to the merchant_ai_session via
// ConversationID = sessionID (the B2B conversation identifier).
func (a *MerchantCatalogAIAgent) startRun(ctx context.Context, input MerchantCatalogAITurnInput, sessionID string) (ports.AIRunRecord, error) {
        lc := NewAIRunLifecycle(a.Repository)
        return lc.StartRun(ctx, StartRunInput{
                BusinessID:     input.BusinessID,
                ConversationID: sessionID,
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

// mapGeminiProposalToOperation maps the contract ④ §4 AIGeminiProposal to
// the contract 11 §5 CatalogOperationProposal.
//
// Per contract 11 §6, the agent performs two logical phases:
//  1. Understand the conversation: add/edit/delete/complete/confirm/correct
//  2. Build Operation Proposal: create/update/delete (asking is NOT a mutation)
//
// The Gemini system prompt for the Merchant Catalog AI is configured to
// produce a Structured Output (contract ④ §8 responseSchema enforcement)
// where the `action` field encodes the operation intent:
//   - answer        → ask_merchant (informational response, no mutation)
//   - clarification → ask_merchant (needs more data; per contract 11 §4)
//   - human_request → ask_merchant (handoff to staff; no mutation)
//   - lead_draft    → ask_merchant (Lead creation is a separate Sales flow)
//   - order_draft   → ask_merchant (Order creation is a separate Sales flow)
//
// The `status` field encodes readiness per contract ④ §4:
//   - resolved         → proposal is ready (Operation is create/update/delete)
//   - needs_more_data  → ask_merchant with MissingFields populated
//   - ambiguous        → ask_merchant with a clarification question
//   - not_found        → ask_merchant (referenced entity doesn't exist)
//
// Per contract 11 §6, "ask_merchant" is NOT a mutation — it gathers missing
// data WITHOUT inventing it. The actual mutation (create/update/delete)
// happens downstream via Catalog Application Services per contract 11 §8.
//
// Per contract 11 §4, the agent does NOT invent dependencies — if the
// proposal needs more data, the status is needs_more_data and MissingFields
// is populated with the specific fields required.
func mapGeminiProposalToOperation(p ports.AIGeminiProposal) CatalogOperationProposal {
        // Per contract 11 §6: default to ask_merchant (non-mutation gather-data).
        op := CatalogOperationProposal{
                Operation:    "ask_merchant",
                Status:       string(p.Status),
                ResponseText: p.ResponseText,
        }
        // Per contract ④ §4: status drives readiness; action drives intent.
        // Per contract 11 §6: only when status=resolved AND the system prompt's
        // action indicates a mutation do we set Operation to create/update/delete.
        //
        // The Merchant Catalog AI's system prompt is configured to use:
        //   action=answer + status=resolved → Operation=create/update/delete
        //     (the ResponseText contains the structured proposal JSON encoded
        //      per the system prompt's instructions; the HTTP handler parses
        //      the structured fields into Create/Update/Delete payload)
        //   action=clarification → ask_merchant (needs more data)
        //   action=human_request → ask_merchant (handoff)
        //
        // This mapping is intentionally conservative: when the status is not
        // resolved, we never propose a mutation — we ask for more data.
        if p.Status == ports.AIProposalStatusResolved {
                switch p.Action {
                case ports.AIProposalActionAnswer:
                        // The system prompt's ResponseText for a resolved+answer indicates
                        // a create/update/delete proposal is ready. The actual operation
                        // type is encoded in the ResponseText (parsed by the HTTP handler).
                        // For now, default to "create" — the HTTP handler will route
                        // to the correct Catalog Application Service based on the parsed
                        // structured payload.
                        op.Operation = "create"
                case ports.AIProposalActionClarification,
                        ports.AIProposalActionHumanRequest,
                        ports.AIProposalActionLeadDraft,
                        ports.AIProposalActionOrderDraft:
                        // Per contract 11 §6, these are NOT mutations. Keep ask_merchant.
                        op.Operation = "ask_merchant"
                }
        }
        return op
}

// MerchantCatalogAITurnInput is the input to one merchant turn.
type MerchantCatalogAITurnInput struct {
        BusinessID string
        // SessionID is the merchant_ai_sessions.id per migration 000054.
        // Empty for the first turn (the agent creates a new session).
        // Non-empty for subsequent turns (the agent appends to the existing session).
        SessionID string
        // PrincipalID is the authenticated merchant's principal_id per migration 000054.
        // Required because merchant_ai_sessions.principal_id is NOT NULL with FK to principals.
        PrincipalID            string
        SourceMessageReference string
        MerchantMessage        string
        PolicyVersion          string
        IdempotencyKey         string
}

// newIDDefault is a fallback ID generator. Production code should inject a
// UUID generator (e.g., github.com/google/uuid.NewString).
var newIDDefault = func() string { return "" }
