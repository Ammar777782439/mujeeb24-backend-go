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
	Agent     *merchantcatalogai.Agent
	Execution *merchantcatalogai.ExecutionService
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

	proposalID := ""
	if h.Execution != nil {
		proposalID, err = h.Execution.Prepare(ctx, merchantcatalogai.ExecutionInput{BusinessID: string(actor.BusinessID), PrincipalID: string(actor.PrincipalID), SessionID: result.SessionID, SelectedCatalog: result.SelectedCatalog, Proposal: result.Proposal})
		if err != nil {
			return nil, mapApplicationError(err)
		}
	}
	out := &contract.Single[contract.MerchantCatalogAIResponse]{}
	out.Body.Data = contract.MerchantCatalogAIResponse{
		SessionID:   result.SessionID,
		ProposalID:  proposalID,
		CatalogID:   result.SelectedCatalog.ID,
		CatalogName: result.SelectedCatalog.Name,
		Proposal:    result.Proposal,
	}
	return out, nil
}

func (h *MerchantCatalogAIV2Handler) HandleExecute(ctx context.Context, in *contract.MerchantAIExecuteInput, actor commands.ActorContext) (*contract.Single[contract.MerchantAIExecutionResponse], error) {
	if h == nil || h.Execution == nil {
		return nil, appErrors.NotImplemented()
	}
	switch actor.Role {
	case "owner", "admin":
	default:
		return nil, appErrors.New(appErrors.CodeForbidden, "catalog write permission is required")
	}
	result, err := h.Execution.ExecuteApproved(ctx, string(actor.BusinessID), string(actor.PrincipalID), in.Body.ProposalID)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	out := &contract.Single[contract.MerchantAIExecutionResponse]{}
	out.Body.Data = contract.MerchantAIExecutionResponse{ProposalID: in.Body.ProposalID, Status: "committed", Result: result}
	return out, nil
}
