// Package handlers — Merchant Catalog AI HTTP Handler (contract 11 §6).
//
// Exposes the Merchant Catalog AI agent to the dashboard via the B2B
// entrypoint POST /api/v1/businesses/{business_id}/merchant-ai/turns.
//
// Per contract 11 §2, this is the B2B entrypoint — strictly separated from
// the B2C AutoReply flow. The Customer Sales AI has its own HTTP routes
// (conversation message creation); this handler does NOT share code with it.
//
// Per contract 11 §6, the agent performs two phases:
//   1. Understand the conversation (may produce an ask_merchant question)
//   2. Build Operation Proposal (create/update/delete — NOT a mutation until
//      the merchant confirms and the Catalog Application Service runs)
//
// Per contract 11 §18, this handler does NOT execute DB mutations directly.
// It calls the agent's HandleTurn which returns a CatalogOperationProposal.
// The actual mutation (create/update/delete) happens downstream via the
// existing catalog command handlers (createCatalogItem, createOffer, etc.)
// after the merchant confirms the proposal.
//
// Per contract ⑥ §19, Execution only happens after Authorization. The agent
// does NOT authorize; it only proposes. The merchant's confirmation + the
// existing catalog command flow constitute the authorization.

package handlers

import (
        "context"
        "log"
        "strings"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
        appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// MerchantAIHandler is the contract 11 §6 B2B HTTP handler.
//
// Per contract 11 §2, it is a SEPARATE handler from the Customer Sales AI
// command facades. It does NOT share dispatch code with the B2C flow.
type MerchantAIHandler struct {
        Agent *services.MerchantCatalogAIAgent
}

// NewMerchantAIHandler wires the handler.
func NewMerchantAIHandler(agent *services.MerchantCatalogAIAgent) *MerchantAIHandler {
        return &MerchantAIHandler{Agent: agent}
}

// HandleTurn processes one merchant turn per contract 11 §6.
//
// HTTP: POST /api/v1/businesses/{business_id}/merchant-ai/turns
//
// Flow:
//  1. Resolve the authenticated scope (business_id + principal_id) per
//     contract ⑥ §9 (business_id comes from Authenticated Context, NOT
//     from Gemini).
//  2. Build the MerchantCatalogAITurnInput with the merchant's message.
//  3. Call the agent's HandleTurn per contract 11 §6.
//  4. Project the CatalogOperationProposal to a MerchantAIChatResponse.
//
// Per contract ⑧ §17, every operation is tenant-scoped via business_id.
// Per contract ⑥ §21, validation failures return a proposal with
// Status=ambiguous + ResponseText explaining the failure (no HTTP 500 —
// the agent handled the failure gracefully per contract ⑥ §2).
func (h *MerchantAIHandler) HandleTurn(ctx context.Context, in *contract.MerchantAIChatInput, actor commands.ActorContext) (*contract.Single[contract.MerchantAIChatResponse], error) {
        if h == nil || h.Agent == nil {
                log.Printf("[MerchantAIHandler] REJECTED business=%s reason=agent_not_configured", in.BusinessID)
                return nil, appErrors.NotImplemented()
        }
        // Per contract ④ §3, user_message is required. Per contract ⑥ §3,
        // structural validation runs in the ValidationPipeline; we do a cheap
        // pre-check here to avoid starting an AI Run for empty input.
        if strings.TrimSpace(in.Body.Message) == "" {
                log.Printf("[MerchantAIHandler] REJECTED business=%s reason=empty_message", in.BusinessID)
                return nil, appErrors.New(appErrors.CodeValidation, "message is required per contract ④ §3")
        }

        log.Printf("[MerchantAIHandler] REQUEST business=%s session=%s principal=%s msg_len=%d",
                in.BusinessID, in.Body.SessionID, actor.PrincipalID, len(in.Body.Message))

        // Per contract ⑨ §16, idempotency_key prevents duplicate AI Runs.
        // The merchant's Idempotency-Key header (per CommandHeaders) is used.
        idempotencyKey := in.IdempotencyKey
        if idempotencyKey == "" {
                // Fall back to a deterministic key derived from the message + session
                // to prevent accidental duplicates when the merchant double-clicks.
                idempotencyKey = "merchant-ai:" + in.Body.SessionID + ":" + in.Body.Message
        }

        // Per contract 11 §6, call the agent.
        op, err := h.Agent.HandleTurn(ctx, services.MerchantCatalogAITurnInput{
                BusinessID:             string(in.BusinessID),
                SessionID:              in.Body.SessionID,
                PrincipalID:            string(actor.PrincipalID),
                SourceMessageReference: in.XRequestID,
                MerchantMessage:        in.Body.Message,
                PolicyVersion:          "merchant-catalog-ai-v1",
                IdempotencyKey:         idempotencyKey,
        })
        if err != nil {
                log.Printf("[MerchantAIHandler] ERROR business=%s session=%s err=%v",
                        in.BusinessID, in.Body.SessionID, err)
                return nil, mapApplicationError(err)
        }

        log.Printf("[MerchantAIHandler] RESPONSE business=%s session=%s operation=%s status=%s",
                in.BusinessID, in.Body.SessionID, op.Operation, op.Status)

        // Project to the HTTP response per contract 11 §5.
        out := &contract.Single[contract.MerchantAIChatResponse]{}
        out.Body.Data = contract.MerchantAIChatResponse{
                Operation:    op.Operation,
                Status:       op.Status,
                ResponseText: op.ResponseText,
                // SessionID is returned so the dashboard can send the next turn with
                // the same session_id (continuity per contract ③ §1 + migration 000054).
                // We extract it from the input if provided; the agent creates a new
                // session when input.SessionID is empty.
                SessionID: in.Body.SessionID,
        }
        return out, nil
}
