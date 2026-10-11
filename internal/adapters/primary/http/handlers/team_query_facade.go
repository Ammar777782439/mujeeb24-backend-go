package handlers

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

func (s *Server) dispatchTeamQuery(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "listTeamMembers":
		return s.listTeamMembers(ctx, input.(*contract.TeamMemberListInput)), true
	case "listTeamInvitations":
		return s.listTeamInvitations(ctx, input.(*contract.TeamInvitationListInput)), true
	case "inspectTeamInvitation":
		return s.inspectTeamInvitation(ctx, input.(*contract.TeamInvitationInspectInput)), true
	default:
		return nil, false
	}
}

func (s *Server) listTeamMembers(ctx context.Context, input *contract.TeamMemberListInput) any {
	if s.deps.ListTeamMembers == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.ListTeamMembers.Handle(ctx, queries.ListTeamMembersQuery{
		Meta: queryMeta(actor, "", ""), Limit: input.Limit, Cursor: input.Cursor,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	out := &contract.List[contract.TeamMember]{}
	out.Body.Data = make([]contract.TeamMember, 0, len(result.Items))
	for _, member := range result.Items {
		out.Body.Data = append(out.Body.Data, teamMemberProjection(member))
	}
	out.Body.Pagination = contract.Page{NextCursor: optionalString(result.NextCursor), HasMore: result.HasMore}
	return out
}

func (s *Server) listTeamInvitations(ctx context.Context, input *contract.TeamInvitationListInput) any {
	if s.deps.ListTeamInvitations == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.ListTeamInvitations.Handle(ctx, queries.ListTeamInvitationsQuery{
		Meta: queryMeta(actor, "", ""), Limit: input.Limit, Cursor: input.Cursor,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	out := &contract.List[contract.TeamInvitation]{}
	out.Body.Data = make([]contract.TeamInvitation, 0, len(result.Items))
	for _, invitation := range result.Items {
		out.Body.Data = append(out.Body.Data, teamInvitationProjection(invitation))
	}
	out.Body.Pagination = contract.Page{NextCursor: optionalString(result.NextCursor), HasMore: result.HasMore}
	return out
}

func (s *Server) inspectTeamInvitation(ctx context.Context, input *contract.TeamInvitationInspectInput) any {
	if s.deps.InspectTeamInvitation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	result, err := s.deps.InspectTeamInvitation.Handle(ctx, queries.InspectTeamInvitationQuery{
		AcceptanceToken: input.Body.AcceptanceToken, Now: time.Now().UTC(),
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamInvitation(result)
}
