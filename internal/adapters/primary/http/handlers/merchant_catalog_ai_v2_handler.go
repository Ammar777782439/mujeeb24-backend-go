package handlers

import (
	"context"
	"log"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

type MerchantCatalogAIV2Handler struct {
	Agent *merchantcatalogai.Agent
}

func NewMerchantCatalogAIV2Handler(agent *merchantcatalogai.Agent) *MerchantCatalogAIV2Handler {
	return &MerchantCatalogAIV2Handler{Agent: agent}
}

func (h *MerchantCatalogAIV2Handler) HandleTurn(ctx context.Context, in *contract.MerchantCatalogAIV2Input, actor commands.ActorContext) (*contract.Single[contract.MerchantCatalogAIResponse], error) {
	if h == nil || h.Agent == nil {
		return nil, appErrors.NotImplemented()
	}
	if in == nil || strings.TrimSpace(in.Body.Message) == "" {
		return nil, appErrors.New(appErrors.CodeValidation, "message is required")
	}

	result, err := h.Agent.HandleTurn(ctx, merchantcatalogai.TurnInput{
		BusinessID:        string(actor.BusinessID),
		PrincipalID:       string(actor.PrincipalID),
		SessionID:         in.Body.SessionID,
		Message:           in.Body.Message,
		ExplicitCatalogID: in.Body.TargetCatalogID,
	})
	if err != nil {
		log.Printf("[MerchantCatalogAIV2] business=%s session=%s error=%v", actor.BusinessID, in.Body.SessionID, err)
		return nil, mapApplicationError(err)
	}

	out := &contract.Single[contract.MerchantCatalogAIResponse]{}
	out.Body.Data = contract.MerchantCatalogAIResponse{
		SessionID:   result.SessionID,
		CatalogID:   result.SelectedCatalog.ID,
		CatalogName: result.SelectedCatalog.Name,
		Proposal:    result.Proposal,
	}
	return out, nil
}
