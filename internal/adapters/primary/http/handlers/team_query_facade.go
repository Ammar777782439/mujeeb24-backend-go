package handlers

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

func (s *Server) dispatchTeamQuery(ctx context.Context, operationID string, input any) (any, bool) {
	if operationID != "listTeamMembers" {
		return nil, false
	}
	return s.listTeamMembers(ctx, input.(*contract.TeamMemberListInput)), true
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
		Meta:   queryMeta(actor, "", ""),
		Limit:  input.Limit,
		Cursor: input.Cursor,
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
