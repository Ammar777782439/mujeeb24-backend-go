// Package services — Merchant Catalog AI Context Builder (contract 11 §2 + ④ §3)
//
// Implements a DEDICATED context builder for the Merchant Catalog AI agent,
// strictly separated from the Customer Sales AI's AutoReplyContextBuilder.
//
// Per contract 11 §2, Customer Sales AI and Merchant Catalog AI share ONLY
// infrastructure (Catalog Contract, Validation primitives, Gemini infra).
// They DO NOT share: System Prompt, Agent Role, Tool Permissions,
// Conversation Purpose, Proposal Contract, Execution Workflow, OR Context
// Builder.
//
// Per contract ③ §1, ConversationState is the B2C customer-side memory
// (focus/previous/comparison/preferences/constraints/pending). It MUST NOT
// be used by the Merchant Catalog AI agent — the merchant-side memory is the
// merchant_ai_sessions + merchant_ai_messages tables (migration 000054).
//
// Per contract ④ §3, the input to Gemini includes: business_context,
// conversation_context, conversation_state, catalog_evidence, user_message.
// For the Merchant Catalog AI, "user_message" is the merchant's message
// (not a customer's). "conversation_context" is the merchant_ai_messages
// history. "conversation_state" is ABSENT (B2B does not use ConversationState).
// "catalog_evidence" is the catalog scope the merchant is authoring against.
//
// Per contract ⑤ §7, the Catalog Entity Contract is sent as part of the
// system_instruction once per AI Runtime invocation. This builder injects it
// into the ContractRuntimeInput.EntityContractPayload.
//
// Per contract ⑥ §11, Mujeeb does NOT re-interpret customer/merchant intent
// during validation. The context builder only assembles evidence; the
// ValidationPipeline runs after Gemini returns.

package services

import (
        "context"
        "errors"
        "strings"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// MerchantContextBuilder is the DEDICATED context builder for the Merchant
// Catalog AI agent per contract 11 §2.
//
// It is intentionally a separate type from AutoReplyContextBuilder to enforce
// the absolute isolation between Customer Sales AI (B2C) and Merchant Catalog
// AI (B2B). They share zero code paths; only the port interfaces
// (ports.AIContextBuilder) are common.
//
// Per contract ③ §1, this builder does NOT use ConversationState. The
// merchant-side conversation memory lives in merchant_ai_sessions/
// merchant_ai_messages (migration 000054), loaded via MerchantAISessionRepository.
//
// Per contract ⑤ §7, the Catalog Entity Contract is injected once and reused
// for every call. The builder stores the pre-serialized JSON payload and
// attaches it to every ContractRuntimeInput.
type MerchantContextBuilder struct {
        // BusinessRepository reads the merchant's Business row for system context.
        // Per contract ④ §3, business_context is part of the Gemini input.
        Businesses ports.BusinessRepository
        // SessionRepository reads the merchant_ai_messages history per contract 11 §6.
        // Per contract ④ §3, conversation_context is the multi-turn message history.
        Sessions MerchantAISessionReader
        // CatalogRepository reads the merchant's existing catalog for evidence.
        // Per contract ④ §3, catalog_evidence is the catalog scope the merchant
        // is authoring against (read-only, tenant-scoped per contract ⑤ §13).
        Catalogs ports.CatalogRepository
        // EntityContractPayload is the JSON-encoded Catalog Entity Contract per
        // contract ⑤ §7. Built once at bootstrap via BuildCatalogEntityContractPayload
        // and reused for every call. Injected into ContractRuntimeInput.
        EntityContractPayload []byte
        // MaxMessages is the maximum number of merchant_ai_messages to include
        // in conversation_context. Per contract ③ §6, we do NOT send the entire
        // merchant session history — only the recent relevant turns.
        MaxMessages int
        // Now is the time provider (UTC per contract ⑧ §13).
        Now func() time.Time
}

// MerchantAISessionReader is the read-side port for merchant_ai_sessions/
// merchant_ai_messages per migration 000054. Defined here (services package)
// because the contract 11 §2 mandates that Merchant Catalog AI does NOT
// share infrastructure types with Customer Sales AI beyond the strict
// infrastructure list (Catalog Contract, Validation, Audit, Observability,
// Gemini infra). The session reader is merchant-specific, so it lives here.
//
// Per contract ⑧ §17, reads are tenant-scoped via business_id.
type MerchantAISessionReader interface {
        // ListMessages returns the merchant_ai_messages for a session, ordered
        // by created_at ASC (oldest first). Per migration 000054, the table has
        // idx_merchant_ai_messages_session_created on (business_id, session_id, created_at ASC).
        ListMessages(ctx context.Context, businessID, sessionID string, limit int) ([]MerchantAIMessage, error)
}

// MerchantAIMessage mirrors a row in merchant_ai_messages per migration 000054.
//
// Per migration 000054 merchant_ai_messages:
//
//      id          UUID NOT NULL
//      business_id UUID NOT NULL
//      session_id  UUID NOT NULL
//      sender_type VARCHAR(32) NOT NULL — CHECK (sender_type IN ('merchant', 'assistant'))
//      text        TEXT NOT NULL
//      created_at  TIMESTAMPTZ NOT NULL
type MerchantAIMessage struct {
        ID         string
        BusinessID string
        SessionID  string
        SenderType string // 'merchant' or 'assistant' per migration 000054 sender_type_chk
        Text       string
        CreatedAt  time.Time
}

// NewMerchantContextBuilder wires the dedicated B2B context builder.
//
// Per contract 11 §2, this constructor is SEPARATE from NewAutoReplyContextBuilder.
// They share no code paths; the B2C and B2B agents are completely independent.
func NewMerchantContextBuilder(
        businesses ports.BusinessRepository,
        sessions MerchantAISessionReader,
        catalogs ports.CatalogRepository,
) *MerchantContextBuilder {
        return &MerchantContextBuilder{
                Businesses:  businesses,
                Sessions:    sessions,
                Catalogs:    catalogs,
                MaxMessages: 12, // reasonable default per contract ③ §6 (no fixed max in Domain)
                Now:         func() time.Time { return time.Now().UTC() },
        }
}

// BuildForTurn assembles the ContractRuntimeInput for one merchant turn.
//
// Per contract ④ §3, the input includes:
//   - business_context: the merchant's Business row (from PostgreSQL)
//   - conversation_context: recent merchant_ai_messages history (NOT
//     ConversationState — that's B2C only per contract ③ §1)
//   - catalog_evidence: the merchant's existing catalog scope (read-only,
//     tenant-scoped per contract ⑤ §13)
//   - user_message: the merchant's current message
//   - EntityContractPayload: the Catalog Entity Contract per contract ⑤ §7
//
// Per contract ⑤ §7, the Entity Contract is sent once per AI Runtime
// invocation. We attach the pre-serialized payload to every call.
//
// Per contract ⑥ §11, this builder does NOT interpret merchant intent —
// it only assembles evidence. Intent interpretation is Gemini's job per
// contract ④ §2.
func (b *MerchantContextBuilder) BuildForTurn(ctx context.Context, input MerchantContextBuildInput) (ports.ContractRuntimeInput, error) {
        if b == nil {
                return ports.ContractRuntimeInput{}, errors.New("merchant context builder is not configured")
        }
        if strings.TrimSpace(input.BusinessID) == "" {
                return ports.ContractRuntimeInput{}, errors.New("business_id is required for merchant context per contract ⑧ §17")
        }
        if strings.TrimSpace(input.MerchantMessage) == "" {
                return ports.ContractRuntimeInput{}, errors.New("merchant_message is required per contract ④ §3")
        }
        if b.Businesses == nil {
                return ports.ContractRuntimeInput{}, errors.New("business repository is not configured")
        }

        // Per contract ④ §3: business_context — the merchant's Business row.
        business, err := b.Businesses.GetByID(ctx, input.BusinessID)
        if err != nil {
                return ports.ContractRuntimeInput{}, err
        }
        if business.ID != input.BusinessID {
                // Per contract ⑥ §8: do not leak cross-tenant resources.
                return ports.ContractRuntimeInput{}, errors.New("business scope mismatch per contract ⑥ §8")
        }

        // Per contract ④ §3: conversation_context — recent merchant_ai_messages.
        // Per contract ③ §1, we do NOT use ConversationState (B2C-only).
        var recentMessages []MerchantAIMessage
        if b.Sessions != nil && input.SessionID != "" {
                limit := b.MaxMessages
                if limit <= 0 {
                        limit = 12
                }
                msgs, err := b.Sessions.ListMessages(ctx, input.BusinessID, input.SessionID, limit)
                if err != nil {
                        return ports.ContractRuntimeInput{}, err
                }
                recentMessages = msgs
        }

        // Assemble the AIContext (the conversation_context + business_context +
        // catalog_evidence envelope). Per contract ④ §3, this is the input Gemini
        // receives alongside the user_message.
        aiContext := &ports.AIContext{
                SchemaVersion: AIContextSchemaVersion,
                Freshness:     AIContextFresh,
                Business: ports.AIContextBusiness{
                        Reference:       business.ID,
                        Name:            business.Name,
                        VerticalType:    business.VerticalType,
                        Locale:          business.Locale,
                        DefaultCurrency: business.DefaultCurrency,
                },
                GeneratedAt: b.Now(),
                ExpiresAt:   b.Now().Add(2 * time.Minute),
        }
        // Per ADR-041: load ALL the merchant's catalogs and send them to
        // Gemini as evidence. This is REQUIRED (not optional) so the AI:
        //   (a) knows what catalogs exist (can answer "how many catalogs?")
        //   (b) can formulate a question to the merchant when the
        //       CatalogResolutionService says "ask merchant which catalog"
        //
        // The AI is FORBIDDEN from picking a catalog itself — the
        // deterministic CatalogResolutionService handles selection. The
        // AI only reads the evidence to inform the merchant.
        if b.Catalogs != nil {
                catalogPage, catalogErr := b.Catalogs.ListCatalogs(ctx, input.BusinessID, "active", 100, "")
                if catalogErr == nil {
                        aiContext.MerchantCatalogs = make([]ports.MerchantCatalogEntry, 0, len(catalogPage.Items))
                        for _, c := range catalogPage.Items {
                                itemsPage, countErr := b.Catalogs.ListCatalogItems(ctx, input.BusinessID, c.ID, "", "active", 1, "")
                                itemsCount := 0
                                if countErr == nil {
                                        itemsCount = len(itemsPage.Items)
                                        if itemsPage.HasMore {
                                                itemsCount++ // lower bound; a count API would be exact
                                        }
                                }
                                aiContext.MerchantCatalogs = append(aiContext.MerchantCatalogs, ports.MerchantCatalogEntry{
                                        ID:         c.ID,
                                        Name:       c.Name,
                                        Status:     c.Status,
                                        ItemsCount: itemsCount,
                                })
                        }
                }
                // If catalogErr != nil we silently skip — the AI can still operate
                // without catalog evidence. The CatalogResolutionService will catch
                // the empty-list case and ask the merchant.
        }

        // Per contract ④ §3: catalog_evidence — the merchant's existing catalog
        // scope (read-only, tenant-scoped per contract ⑤ §13). This is what the
        // agent can reference when the merchant asks to "update" or "delete" an
        // existing item. For "create" flows, this may be empty.
        // NOTE: We do NOT call Catalogs here to avoid coupling every turn to a
        // catalog read. The CatalogBatchController (contract ② §1) handles
        // catalog evidence assembly when Gemini indicates it needs catalog data.
        // This builder injects the Catalog Entity Contract (contract ⑤ §7) so
        // Gemini always knows the entity definitions before seeing actual data.

        // Per contract ④ §3: user_message — the merchant's current message.
        decisionInput := ports.AIDecisionInput{
                BusinessID:             input.BusinessID,
                ConversationID:         input.SessionID, // merchant_ai_sessions.id acts as the conversation_id for B2B
                SourceMessageReference: input.SourceMessageReference,
                Text:                   input.MerchantMessage,
                Channel:                "merchant_dashboard", // not facebook/instagram/whatsapp — B2B channel
                PolicyVersion:          input.PolicyVersion,
                Context:                aiContext,
        }

        // Per contract ④ §3: encode recent merchant messages into the AIContext.
        // Per contract ④ §3, conversation_context is the multi-turn history.
        // We map MerchantAIMessage → AIRecentMessageEvidence.
        if len(recentMessages) > 0 {
                aiContext.RecentMessages = make([]ports.AIRecentMessageEvidence, 0, len(recentMessages))
                for _, m := range recentMessages {
                        direction := "inbound"
                        origin := "merchant"
                        if m.SenderType == "assistant" {
                                direction = "outbound"
                                origin = "ai"
                        }
                        aiContext.RecentMessages = append(aiContext.RecentMessages, ports.AIRecentMessageEvidence{
                                Reference:     m.ID,
                                Direction:     direction,
                                Origin:        origin,
                                Text:          m.Text,
                                OccurredAt:    m.CreatedAt,
                                EvidenceState: AIContextFresh,
                                SchemaVersion: AIEvidenceSchemaVersion,
                        })
                }
        }

        // Per contract ③ §4: GeminiInteractionContext — for the Merchant Catalog
        // AI, we do NOT use previous_interaction_id chaining per contract 11 §2
        // (the B2B agent is stateless across turns; merchant_ai_messages is the
        // canonical memory). Per contract ② §8, batches also do not chain. So
        // PreviousInteractionID is always empty here.
        geminiInteraction := ports.GeminiInteractionContext{
                PreviousInteractionID: "",
                Store:                 false, // do not use Gemini server-side history for B2B
        }

        // Per contract ⑤ §7: EntityContractPayload — the pre-serialized Catalog
        // Entity Contract. Built once at bootstrap via BuildCatalogEntityContractPayload.
        return ports.ContractRuntimeInput{
                DecisionInput:         decisionInput,
                GeminiInteraction:     geminiInteraction,
                EntityContractPayload: b.EntityContractPayload,
        }, nil
}

// MerchantContextBuildInput is the input to BuildForTurn.
type MerchantContextBuildInput struct {
        BusinessID             string
        SessionID              string // merchant_ai_sessions.id; empty for the first turn
        SourceMessageReference string
        MerchantMessage        string
        PolicyVersion          string
}
