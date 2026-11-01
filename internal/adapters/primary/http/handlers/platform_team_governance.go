package handlers

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func platformGovernanceActor(ctx context.Context) commands.ActorContext {
	adminID, _ := middleware.PlatformAdminID(ctx)
	return commands.ActorContext{PrincipalID: adminID, Role: "platform_super_admin"}
}

func (s *Server) platformListTeamMembers(ctx context.Context, in *dto.PlatformTeamListInput) (any, bool) {
	if s.platformDeps.TeamGovernance == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	result, err := s.platformDeps.TeamGovernance.ListMembers(ctx, platformGovernanceActor(ctx), string(in.BusinessID), in.Limit, in.Cursor)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[contract.TeamMember]{}
	out.Body.Data = make([]contract.TeamMember, 0, len(result.Items))
	for _, member := range result.Items {
		out.Body.Data = append(out.Body.Data, teamMemberProjection(member))
	}
	out.Body.Pagination = contract.Page{NextCursor: optionalString(result.NextCursor), HasMore: result.HasMore}
	return out, true
}

func (s *Server) platformListTeamInvitations(ctx context.Context, in *dto.PlatformTeamListInput) (any, bool) {
	if s.platformDeps.TeamGovernance == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	result, err := s.platformDeps.TeamGovernance.ListInvitations(ctx, platformGovernanceActor(ctx), string(in.BusinessID), in.Limit, in.Cursor)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.List[contract.TeamInvitation]{}
	out.Body.Data = make([]contract.TeamInvitation, 0, len(result.Items))
	for _, invitation := range result.Items {
		out.Body.Data = append(out.Body.Data, teamInvitationProjection(invitation))
	}
	out.Body.Pagination = contract.Page{NextCursor: optionalString(result.NextCursor), HasMore: result.HasMore}
	return out, true
}

func (s *Server) platformUpdateTeamMemberPermissions(ctx context.Context, in *dto.PlatformTeamMemberPermissionsInput) (any, bool) {
	if s.platformDeps.TeamGovernance == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	result, err := s.platformDeps.TeamGovernance.UpdatePermissions(ctx, platformGovernanceActor(ctx), ports.TeamMemberPermissionChange{BusinessID: string(in.BusinessID), PrincipalID: string(in.PrincipalID), Permissions: in.Body.Permissions}, in.Body.Reason, in.XCorrelationID)
	if err != nil {
		return mapApplicationError(err), true
	}
	return singleTeamMember(result), true
}
