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
        // TargetCatalogID per ADR-041 — required. Populated by the
        // CatalogResolutionService (deterministic) BEFORE the proposal is
        // returned to the HTTP handler. The AI NEVER sets this field itself.
        // If the resolution says NeedsAsk, the operation is converted to
        // "ask_merchant" and Create is left nil.
        TargetCatalogID string                `json:"target_catalog_id"`
        Item            *CatalogItemDraft     `json:"item,omitempty"`
        Variants        []CatalogVariantDraft  `json:"variants,omitempty"`
        Offers          []CatalogOfferDraft   `json:"offers,omitempty"`
        Schema          *AttributeSchemaDraft  `json:"schema,omitempty"` // when needed per contract 11 §4
        Definitions    []AttributeDefinitionDraft `json:"definitions,omitempty"`
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
        // CatalogResolution per ADR-041 — deterministic catalog selection.
        // Set to a non-nil *CatalogResolutionService in bootstrap. If nil,
        // catalog selection is skipped and any create proposal with an empty
        // TargetCatalogID is converted to ask_merchant (failsafe).
        CatalogResolution *CatalogResolutionService
        Now               func() time.Time
        NewID             func() string

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
        // SetTargetCatalog persists the sticky target_catalog_id for the session
        // per ADR-041 layer 2. Called after layer 1 (explicit HTTP param) or
        // layer 3 (single-catalog auto-select) resolves successfully. Subsequent
        // turns read this via GetTargetCatalog and skip the param requirement.
        SetTargetCatalog(ctx context.Context, businessID, sessionID, catalogID string) error
        // GetTargetCatalog reads the sticky target_catalog_id stored by
        // SetTargetCatalog. Returns ("", nil) if no sticky catalog is set
        // (first turn after session creation, or after a catalog was deleted).
        GetTargetCatalog(ctx context.Context, businessID, sessionID string) (catalogID string, err error)
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
        //
        // Per ADR-043 fix: generate the idempotency key HERE (in the agent, after
        // session creation), not in the handler. This way:
        //   - First turn (input.SessionID=""): the agent creates a new session,
        //     then builds the key with the resolved sessionID. Different sessions
        //     → different keys → no collision across "first turn" messages with
        //     the same text.
        //   - Subsequent turns (input.SessionID non-empty): the handler passes
        //     the existing sessionID, and the key is built with it. Double-clicks
        //     within the same session get the same key → correctly deduped.
        //
        // If the merchant provided an explicit Idempotency-Key header (per
        // CommandHeaders), that takes precedence — the merchant is responsible
        // for the key semantics in that case.
        if strings.TrimSpace(input.IdempotencyKey) == "" {
                input.IdempotencyKey = "merchant-ai:" + sessionID + ":" + input.MerchantMessage
        }
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

        // Per ADR-041: catalog selection happens AFTER Gemini returns, BEFORE
        // the proposal is sent to the HTTP handler. This is deterministic —
        // the AI NEVER picks a catalog. The CatalogResolutionService applies
        // 4 priority layers:
        //   1. HTTP param (input.TargetCatalogID)
        //   2. Session sticky (from merchant_ai_sessions.target_catalog_id)
        //   3. Single-catalog auto-select (if only one catalog exists)
        //   4. Failure → convert operation to ask_merchant with a question
        //
        // This is only applied for mutation operations (create/update/delete).
        // For "answer" and "ask_merchant" operations, no catalog is needed.
        if op.Operation == "create" || op.Operation == "update" || op.Operation == "delete" {
                op = a.resolveTargetCatalog(ctx, op, input, sessionID, contractInput.DecisionInput.Context, run, log.Default())
        }

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

// resolveTargetCatalog applies the deterministic ADR-041 catalog selection.
// This method is called AFTER mapGeminiProposalToOperation. It:
//   1. Loads the sticky catalog_id from merchant_ai_sessions (layer 2).
//   2. Calls CatalogResolutionService.Resolve with (HTTP param, sticky,
//      merchant_catalogs evidence).
//   3. If Resolved → populates op.Create.TargetCatalogID and persists
//      the sticky if ShouldSetSticky=true.
//   4. If NeedsAsk → converts the operation to "ask_merchant" with the
//      question from the resolution. The original operation is preserved
//      in the MissingFields for the dashboard to display.
//
// This method is FORBIDDEN from asking the AI to pick — the AI is only
// used to FORMAT the question (already done by CatalogResolutionService).
// The selection itself is pure Go code.
//
// failsafe: if CatalogResolution is nil OR merchant_catalogs is empty,
// the operation is converted to ask_merchant with a fallback question.
// This prevents any hallucinated catalog_id from leaking into a DB write.
func (a *MerchantCatalogAIAgent) resolveTargetCatalog(
        ctx context.Context,
        op CatalogOperationProposal,
        input MerchantCatalogAITurnInput,
        sessionID string,
        aiContext *ports.AIContext,
        run ports.AIRunRecord,
        logger *log.Logger,
) CatalogOperationProposal {
        // Build the merchant_catalogs evidence from the AIContext.
        var merchantCatalogs []ports.MerchantCatalogEntry
        if aiContext != nil {
                merchantCatalogs = aiContext.MerchantCatalogs
        }

        // Failsafe: if no CatalogResolutionService is wired, refuse to write
        // any catalog_id. Convert to ask_merchant with a clear message.
        if a.CatalogResolution == nil {
                logger.Printf("[MerchantAI] CATALOG_RESOLUTION_SKIPPED business=%s run=%s reason=service_not_wired — converting to ask_merchant",
                        input.BusinessID, run.ID)
                return CatalogOperationProposal{
                        Operation:    "ask_merchant",
                        Status:       "needs_more_data",
                        ResponseText: "تعذّر تحديد الكتالوج المستهدف. الرجاء اختيار كتالوج من الـ dashboard أو تحدث مع الدعم.",
                        MissingFields: []CatalogMissingField{{
                                Path:        "target_catalog_id",
                                DisplayName: "الكتالوج المستهدف",
                                DataType:    "uuid",
                                Reason:      "CatalogResolutionService not wired in agent (failsafe per ADR-041)",
                        }},
                }
        }

        // Load the sticky catalog_id from the session (layer 2).
        var sessionSticky string
        if a.SessionWriter != nil && sessionID != "" {
                if sticky, err := a.SessionWriter.GetTargetCatalog(ctx, input.BusinessID, sessionID); err == nil {
                        sessionSticky = sticky
                } else {
                        // Don't fail the whole turn if sticky lookup fails — just log
                        // and proceed with empty sticky (layer 1 or 3 will catch up).
                        logger.Printf("[MerchantAI] STICKY_LOOKUP_FAILED business=%s session=%s err=%v",
                                input.BusinessID, sessionID, err)
                }
        }

        // Run the deterministic resolution.
        resolution := a.CatalogResolution.Resolve(input.TargetCatalogID, sessionSticky, merchantCatalogs)
        logger.Printf("[MerchantAI] CATALOG_RESOLUTION business=%s session=%s run=%s resolved=%v reason=%s catalog_id=%s needs_ask=%v",
                input.BusinessID, sessionID, run.ID,
                resolution.Resolved, resolution.Reason, resolution.CatalogID, resolution.NeedsAsk)

        if resolution.NeedsAsk {
                // Convert the proposed operation to ask_merchant with the resolution's
                // question. This is the AI's ONLY role in catalog selection: deliver
                // the question (already in Arabic with the catalog list).
                missing := []CatalogMissingField{{
                        Path:        "target_catalog_id",
                        DisplayName: "الكتالوج المستهدف",
                        DataType:    "uuid",
                        Reason:      resolution.Reason,
                }}
                return CatalogOperationProposal{
                        Operation:     "ask_merchant",
                        Status:        "needs_more_data",
                        ResponseText:  resolution.AskQuestion,
                        MissingFields: missing,
                }
        }

        // Resolved → populate op.Create.TargetCatalogID (or op.Update if update).
        // Per ADR-041: the AI never sets TargetCatalogID. We do it here in code.
        if op.Operation == "create" {
                if op.Create == nil {
                        // Per contract 11 §6, the create payload was nil from Gemini.
                        // We can't attach a TargetCatalogID to nil, so create a stub.
                        // The HTTP handler / Catalog Application Service will validate
                        // the Item fields downstream.
                        op.Create = &CatalogCreatePayload{}
                }
                op.Create.TargetCatalogID = resolution.CatalogID
        }
        // For update/delete, the target is identified by ItemID in op.Update /
        // op.Delete — the catalog_id is implicit (it's the catalog containing
        // that item). No additional field to set.

        // Persist the sticky if the resolution says to.
        if resolution.ShouldSetSticky && a.SessionWriter != nil && sessionID != "" {
                if err := a.SessionWriter.SetTargetCatalog(ctx, input.BusinessID, sessionID, resolution.CatalogID); err != nil {
                        // Don't fail the turn — just log. The sticky is an optimization,
                        // not a correctness requirement (layer 1 will catch up next turn).
                        logger.Printf("[MerchantAI] STICKY_PERSIST_FAILED business=%s session=%s catalog_id=%s err=%v",
                                input.BusinessID, sessionID, resolution.CatalogID, err)
                } else {
                        logger.Printf("[MerchantAI] STICKY_SET business=%s session=%s catalog_id=%s reason=%s",
                                input.BusinessID, sessionID, resolution.CatalogID, resolution.Reason)
                }
        }

        return op
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
        // Per ADR-044 layer 1: strip the [CREATE]/[UPDATE]/[DELETE] prefix from
        // response_text — the prefix is internal routing metadata, not for the
        // merchant to see. The agent reads it to determine operation type, then
        // removes it from the human-readable text.
        respText := stripOperationPrefix(strings.TrimSpace(p.ResponseText))
        op := CatalogOperationProposal{
                Operation:    "ask_merchant",
                Status:       string(p.Status),
                ResponseText: respText,
        }
        // Per contract 11 §6: when the status is not resolved, we never propose
        // a mutation — we ask for more data.
        if p.Status != ports.AIProposalStatusResolved {
                return op
        }

        // Per ADR-044 layer 2: PREFER the structured `proposal` field if Gemini
        // populated it. This is the source of truth — the prefix in response_text
        // is a fallback for backward compatibility.
        if p.Proposal != nil && p.Proposal.Operation != "" {
                switch p.Proposal.Operation {
                case "create":
                        if p.Proposal.Create != nil {
                                op.Operation = "create"
                                op.Create = mapProposalCreateToPayload(p.Proposal.Create)
                        }
                case "update":
                        if p.Proposal.Update != nil {
                                op.Operation = "update"
                                op.Update = mapProposalUpdateToPayload(p.Proposal.Update)
                        }
                case "delete":
                        if p.Proposal.Delete != nil {
                                op.Operation = "delete"
                                op.Delete = mapProposalDeleteToPayload(p.Proposal.Delete)
                        }
                }
                return op
        }

        // Fallback (no structured proposal): infer operation from the prefix.
        // This path is taken when an older Gemini prompt didn't populate the
        // proposal field. The prefix was already stripped above.
        respTextWithPrefix := strings.TrimSpace(p.ResponseText)
        switch {
        case strings.Contains(respTextWithPrefix, "[CREATE]") || strings.Contains(respTextWithPrefix, "[create]"):
                op.Operation = "create"
        case strings.Contains(respTextWithPrefix, "[UPDATE]") || strings.Contains(respTextWithPrefix, "[update]"):
                op.Operation = "update"
        case strings.Contains(respTextWithPrefix, "[DELETE]") || strings.Contains(respTextWithPrefix, "[delete]"):
                op.Operation = "delete"
        default:
                // Per ADR-041: no operation keyword → keep ask_merchant. This is
                // the safe default for informational queries like "how many
                // catalogs do I have?" — the AI just answers, no mutation.
                op.Operation = "answer"
        }
        return op
}

// stripOperationPrefix removes the [CREATE]/[UPDATE]/[DELETE]/[create]/[update]/[delete]
// prefix from response_text per ADR-044 layer 1. The prefix is internal
// routing metadata; the merchant should never see it in the chat.
//
// Examples:
//   "[CREATE] تم تجهيز مسودة..." → "تم تجهيز مسودة..."
//   "[UPDATE] تم تعديل السعر..." → "تم تعديل السعر..."
//
// Per ADR-042, the prefix is also encoded by the prompt as an instruction
// to Gemini. After Gemini returns, the agent strips it before returning
// the proposal to the HTTP handler (which then sends it to the frontend).
func stripOperationPrefix(text string) string {
        for _, prefix := range []string{"[CREATE]", "[UPDATE]", "[DELETE]", "[create]", "[update]", "[delete]"} {
                if strings.HasPrefix(text, prefix) {
                        return strings.TrimSpace(strings.TrimPrefix(text, prefix))
                }
        }
        return text
}

// mapProposalCreateToPayload converts the ports-level proposal create struct
// to the agent's CatalogCreatePayload. Per ADR-041, TargetCatalogID is
// intentionally NOT mapped here — it's populated by resolveTargetCatalog
// after the CatalogResolutionService runs.
func mapProposalCreateToPayload(p *ports.CatalogProposalCreate) *CatalogCreatePayload {
        if p == nil {
                return nil
        }
        out := &CatalogCreatePayload{}
        if p.Item != nil {
                out.Item = &CatalogItemDraft{
                        Name:                 p.Item.Name,
                        ItemType:             p.Item.ItemType,
                        ShortDescription:     p.Item.ShortDescription,
                        LongDescription:      p.Item.LongDescription,
                        PricingMode:          p.Item.PricingMode,
                        AvailabilityMode:     p.Item.AvailabilityMode,
                        FulfillmentMode:      p.Item.FulfillmentMode,
                        RequiresConfirmation: p.Item.RequiresConfirmation,
                        Attributes:           p.Item.Attributes,
                }
        }
        for _, v := range p.Variants {
                out.Variants = append(out.Variants, CatalogVariantDraft{
                        Name:       v.Name,
                        Attributes: v.Attributes,
                })
        }
        for _, o := range p.Offers {
                out.Offers = append(out.Offers, CatalogOfferDraft{
                        VariantNameRef:     o.VariantNameRef,
                        Name:               o.Name,
                        PricingMode:        o.PricingMode,
                        Amount:             o.Amount,
                        Currency:           o.Currency,
                        PricingUnit:        o.PricingUnit,
                        PriceSource:        o.PriceSource,
                        AvailabilityMode:   o.AvailabilityMode,
                        AvailabilityStatus: o.AvailabilityStatus,
                        FulfillmentMode:    o.FulfillmentMode,
                        ValidityFrom:       o.ValidityFrom,
                        ValidityUntil:      o.ValidityUntil,
                })
        }
        // Note: Schema/Definitions are not carried over from the AI proposal
        // payload (omitted per ADR-044 layer 2 scope).
        return out
}

// mapProposalUpdateToPayload converts the ports-level proposal update struct
// to the agent's CatalogUpdatePayload.
func mapProposalUpdateToPayload(p *ports.CatalogProposalUpdate) *CatalogUpdatePayload {
        if p == nil {
                return nil
        }
        out := &CatalogUpdatePayload{
                ItemID:  p.ItemID,
                Changes: CatalogItemDraft{
                        Name:                  p.Changes.Name,
                        ItemType:              p.Changes.ItemType,
                        ShortDescription:      p.Changes.ShortDescription,
                        LongDescription:       p.Changes.LongDescription,
                        PricingMode:           p.Changes.PricingMode,
                        AvailabilityMode:      p.Changes.AvailabilityMode,
                        FulfillmentMode:       p.Changes.FulfillmentMode,
                        RequiresConfirmation: p.Changes.RequiresConfirmation,
                        Attributes:            p.Changes.Attributes,
                },
        }
        for _, v := range p.NewVariants {
                out.NewVariants = append(out.NewVariants, CatalogVariantDraft{
                        Name:       v.Name,
                        Attributes: v.Attributes,
                })
        }
        for _, o := range p.NewOffers {
                out.NewOffers = append(out.NewOffers, CatalogOfferDraft{
                        VariantNameRef:     o.VariantNameRef,
                        Name:               o.Name,
                        PricingMode:        o.PricingMode,
                        Amount:             o.Amount,
                        Currency:           o.Currency,
                        PricingUnit:        o.PricingUnit,
                        PriceSource:        o.PriceSource,
                        AvailabilityMode:   o.AvailabilityMode,
                        AvailabilityStatus: o.AvailabilityStatus,
                        FulfillmentMode:    o.FulfillmentMode,
                        ValidityFrom:       o.ValidityFrom,
                        ValidityUntil:      o.ValidityUntil,
                })
        }
        return out
}

// mapProposalDeleteToPayload converts the ports-level proposal delete struct
// to the agent's CatalogDeletePayload.
func mapProposalDeleteToPayload(p *ports.CatalogProposalDelete) *CatalogDeletePayload {
        if p == nil {
                return nil
        }
        return &CatalogDeletePayload{
                ItemID:      p.ItemID,
                Confirmed:   p.Confirmed,
                ReasonGiven: p.ReasonGiven,
        }
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
        // TargetCatalogID is per ADR-041 layer 1 (HTTP parameter). The
        // dashboard sets it when the merchant picks a catalog from a
        // dropdown before sending the message. Empty when the merchant
        // hasn't explicitly picked — the CatalogResolutionService then
        // tries layers 2/3/4.
        TargetCatalogID string
}

// newIDDefault is a fallback ID generator. Production code should inject a
// UUID generator (e.g., github.com/google/uuid.NewString).
var newIDDefault = func() string { return "" }
