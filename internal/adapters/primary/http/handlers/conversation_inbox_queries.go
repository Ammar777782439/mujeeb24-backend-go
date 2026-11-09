package handlers

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

func (s *Server) dispatchInboxQuery(ctx context.Context, operation string, input any) (any, bool) {
	switch operation {
	case "getConversationAttachmentReadURL":
		in := input.(*contract.ConversationAttachmentReadInput)
		if s.deps.GetConversationAttachmentReadURL == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetConversationAttachmentReadURL.Handle(ctx, queries.GetConversationAttachmentReadURLQuery{
			Meta: queryMeta(actor, "", ""), ConversationID: commands.ConversationID(in.ConversationID),
			MessageID: commands.MessageID(in.MessageID), AttachmentID: string(in.AttachmentID),
		})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.ConversationAttachmentReadURLView]{}
		out.Body.Data = contract.ConversationAttachmentReadURLView{
			URL: view.URL, ExpiresInSeconds: view.ExpiresInSeconds, MimeType: view.MimeType, SizeBytes: view.SizeBytes,
		}
		return out, true
	case "getConversationSummary":
		in := input.(*contract.ConversationInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.GetConversationSummary == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.GetConversationSummary.Handle(ctx, queries.GetConversationSummaryQuery{Meta: queryMeta(actor, "", ""), ConversationID: commands.ConversationID(in.ConversationID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[dto.ConversationSummary]{}
		out.Body.Data = dto.ConversationSummary{ConversationID: dto.UUID(result.ConversationID), Status: result.Status, Text: result.Text, CoveredInboundTurns: result.CoveredInboundTurns}
		return out, true
	case "getConversationInboxOverview":
		in := input.(*contract.ConversationListInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.GetConversationInboxOverview == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.GetConversationInboxOverview.Handle(ctx, queries.GetConversationInboxOverviewQuery{Meta: queryMeta(actor, "", ""), Scope: in.Scope, Search: in.Search})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[dto.ConversationInboxOverview]{}
		out.Body.Data = dto.ConversationInboxOverview{Total: result.Total, AssignedToMe: result.AssignedToMe, Unassigned: result.Unassigned, StateCounts: result.StateCounts}
		return out, true
	}
	return nil, false
}
