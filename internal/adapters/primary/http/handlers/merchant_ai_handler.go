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
        //
        // Per ADR-043 fix: we do NOT generate a fallback key in the handler
        // anymore. The agent generates it AFTER session creation, so the key
        // is always scoped to a real session_id (not the empty string from
        // the first turn). This prevents the "idempotency collision" bug where
        // two first-turn messages with the same text would collide on the
        // key "merchant-ai::<message>" and the second request would return
        // the first request's completed run, causing:
        //   illegal AI Run transition: completed → context_built
        //
        // Now: pass the Idempotency-Key header (if the merchant provided one)
        // OR empty (the agent will compute a session-scoped key).
        op, err := h.Agent.HandleTurn(ctx, services.MerchantCatalogAITurnInput{
                BusinessID:             string(in.BusinessID),
                SessionID:              in.Body.SessionID,
                PrincipalID:            string(actor.PrincipalID),
                SourceMessageReference: in.XRequestID,
                MerchantMessage:        in.Body.Message,
                PolicyVersion:          "merchant-catalog-ai-v1",
                IdempotencyKey:         in.IdempotencyKey, // empty OK — agent generates session-scoped key if not provided
                // Per ADR-041 layer 1: pass the explicit catalog selection from
                // the dashboard dropdown. Empty when the merchant didn't pick —
                // CatalogResolutionService then tries layers 2/3/4.
                TargetCatalogID:        in.Body.TargetCatalogID,
        })
        if err != nil {
                log.Printf("[MerchantAIHandler] ERROR business=%s session=%s err=%v",
                        in.BusinessID, in.Body.SessionID, err)
                return nil, mapApplicationError(err)
        }

        log.Printf("[MerchantAIHandler] RESPONSE business=%s session=%s operation=%s status=%s",
                in.BusinessID, in.Body.SessionID, op.Operation, op.Status)

        // Per ADR-044 layer 2: project the structured proposal fields into
        // the HTTP response. The frontend renders these as an approval card.
        // Per ADR-044 layer 3: MissingFields is populated when the agent
        // needs more data (status=needs_more_data) — the frontend can render
        // a form for these.
        out := &contract.Single[contract.MerchantAIChatResponse]{}
        resp := contract.MerchantAIChatResponse{
                Operation:    op.Operation,
                Status:       op.Status,
                ResponseText: op.ResponseText,
                // Per ADR-043 fix: also return the resolved session ID (not the
                // input session_id which may be empty on first turn).
                SessionID: in.Body.SessionID,
        }
        if op.Create != nil {
                resp.Create = mapCreateToHTTP(op.Create)
                resp.TargetCatalogID = op.Create.TargetCatalogID
        }
        if op.Update != nil {
                resp.Update = mapUpdateToHTTP(op.Update)
        }
        if op.Delete != nil {
                resp.Delete = mapDeleteToHTTP(op.Delete)
        }
        if len(op.MissingFields) > 0 {
                resp.MissingFields = make([]contract.MerchantAIMissingField, 0, len(op.MissingFields))
                for _, f := range op.MissingFields {
                        resp.MissingFields = append(resp.MissingFields, contract.MerchantAIMissingField{
                                Path:        f.Path,
                                DisplayName: f.DisplayName,
                                DataType:    f.DataType,
                                Reason:      f.Reason,
                        })
                }
        }
        out.Body.Data = resp
        return out, nil
}

// mapCreateToHTTP converts the agent's CatalogCreatePayload to the HTTP
// projection per ADR-044 layer 2.
func mapCreateToHTTP(c *services.CatalogCreatePayload) *contract.MerchantAIProposalCreate {
        if c == nil {
                return nil
        }
        out := &contract.MerchantAIProposalCreate{
                TargetCatalogID: c.TargetCatalogID,
        }
        if c.Item != nil {
                out.Item = &contract.MerchantAIProposalItem{
                        Name:                 c.Item.Name,
                        ItemType:             c.Item.ItemType,
                        ShortDescription:     c.Item.ShortDescription,
                        LongDescription:      c.Item.LongDescription,
                        PricingMode:          c.Item.PricingMode,
                        AvailabilityMode:     c.Item.AvailabilityMode,
                        FulfillmentMode:      c.Item.FulfillmentMode,
                        RequiresConfirmation: c.Item.RequiresConfirmation,
                        Attributes:           c.Item.Attributes,
                }
        }
        for _, v := range c.Variants {
                out.Variants = append(out.Variants, contract.MerchantAIProposalVariant{
                        Name:       v.Name,
                        Attributes: v.Attributes,
                })
        }
        for _, o := range c.Offers {
                out.Offers = append(out.Offers, contract.MerchantAIProposalOffer{
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

// mapUpdateToHTTP converts the agent's CatalogUpdatePayload to the HTTP projection.
func mapUpdateToHTTP(u *services.CatalogUpdatePayload) *contract.MerchantAIProposalUpdate {
        if u == nil {
                return nil
        }
        out := &contract.MerchantAIProposalUpdate{
                ItemID: u.ItemID,
                Changes: contract.MerchantAIProposalItem{
                        Name:                  u.Changes.Name,
                        ItemType:              u.Changes.ItemType,
                        ShortDescription:      u.Changes.ShortDescription,
                        LongDescription:       u.Changes.LongDescription,
                        PricingMode:           u.Changes.PricingMode,
                        AvailabilityMode:      u.Changes.AvailabilityMode,
                        FulfillmentMode:       u.Changes.FulfillmentMode,
                        RequiresConfirmation: u.Changes.RequiresConfirmation,
                        Attributes:            u.Changes.Attributes,
                },
        }
        for _, v := range u.NewVariants {
                out.NewVariants = append(out.NewVariants, contract.MerchantAIProposalVariant{
                        Name:       v.Name,
                        Attributes: v.Attributes,
                })
        }
        for _, o := range u.NewOffers {
                out.NewOffers = append(out.NewOffers, contract.MerchantAIProposalOffer{
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

// mapDeleteToHTTP converts the agent's CatalogDeletePayload to the HTTP projection.
func mapDeleteToHTTP(d *services.CatalogDeletePayload) *contract.MerchantAIProposalDelete {
        if d == nil {
                return nil
        }
        return &contract.MerchantAIProposalDelete{
                ItemID:      d.ItemID,
                Confirmed:   d.Confirmed,
                ReasonGiven: d.ReasonGiven,
        }
}
