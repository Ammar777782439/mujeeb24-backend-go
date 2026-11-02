package handlers

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

func (s *Server) dispatchDashboardFollowUps(ctx context.Context, operation string, input any) (any, bool) {
	if operation != "getDashboardFollowUps" {
		return nil, false
	}
	in := input.(*contract.BusinessPath)
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return mapApplicationError(err), true
	}
	if s.deps.GetDashboardFollowUps == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	v, err := s.deps.GetDashboardFollowUps.Handle(ctx, queries.GetDashboardFollowUpsQuery{Meta: queryMeta(actor, "", "")})
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[dto.DashboardFollowUps]{}
	out.Body.Data = dto.DashboardFollowUps{Items: make([]dto.DashboardFollowUp, 0, len(v.Items)), HasMore: v.HasMore, AsOf: v.AsOf}
	for _, x := range v.Items {
		row := dto.DashboardFollowUp{LeadID: dto.UUID(x.LeadID), CustomerID: dto.UUID(x.CustomerID), CustomerName: x.CustomerName, LeadStatus: x.LeadStatus, Channel: x.Channel, AssigneeName: x.AssigneeName, ProductName: x.ProductName, InterestReason: x.InterestReason, LastCustomerMessage: x.LastCustomerMessage, LastCustomerMessageAt: x.LastCustomerMessageAt, NextActionAt: x.NextActionAt, Attention: x.Attention, UpdatedAt: x.UpdatedAt}
		if x.ConversationID != nil {
			v := dto.UUID(*x.ConversationID)
			row.ConversationID = &v
		}
		if x.WorkflowID != nil {
			v := dto.UUID(*x.WorkflowID)
			row.WorkflowID = &v
		}
		out.Body.Data.Items = append(out.Body.Data.Items, row)
	}
	return out, true
}
