package handlers

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

func (s *Server) dispatchTeamCommand(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "createTeamInvitation":
		return s.createTeamInvitation(ctx, input.(*contract.TeamInvitationCreateInput)), true
	case "acceptTeamInvitation":
		return s.acceptTeamInvitation(ctx, input.(*contract.TeamInvitationAcceptInput)), true
	case "acceptNewTeamInvitation":
		return s.acceptNewTeamInvitation(ctx, input.(*contract.TeamInvitationAcceptNewInput)), true
	case "revokeTeamInvitation":
		return s.revokeTeamInvitation(ctx, input.(*contract.TeamInvitationRevokeInput)), true
	case "resendTeamInvitation":
		return s.resendTeamInvitation(ctx, input.(*contract.TeamInvitationResendInput)), true
	case "updateTeamMemberRole":
		return s.updateTeamMemberRole(ctx, input.(*contract.TeamMemberRoleUpdateInput)), true
	case "updateTeamMemberAccess":
		return s.updateTeamMemberAccess(ctx, input.(*contract.TeamMemberAccessUpdateInput)), true
	case "revokeTeamMember":
		return s.revokeTeamMember(ctx, input.(*contract.TeamMemberRevokeInput)), true
	default:
		return nil, false
	}
}

func (s *Server) createTeamInvitation(ctx context.Context, input *contract.TeamInvitationCreateInput) any {
	if s.deps.InviteTeamMember == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.InviteTeamMember.Handle(ctx, commands.InviteTeamMemberCommand{
		Meta: commandMeta(ctx, actor, input.CommandHeaders), Email: input.Body.Email,
		DisplayName: input.Body.DisplayName, Role: input.Body.Role, Permissions: input.Body.Permissions,
		ExpiresIn: time.Duration(input.Body.ExpiresInHours) * time.Hour,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	out := &contract.Single[contract.TeamInvitationCreated]{}
	out.Body.Data = teamInvitationCreatedProjection(result)
	return out
}

func (s *Server) acceptTeamInvitation(ctx context.Context, input *contract.TeamInvitationAcceptInput) any {
	if s.deps.AcceptTeamInvitation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	principalID, ok := middleware.PrincipalID(ctx)
	if !ok {
		return mapApplicationError(appErrors.New(appErrors.CodeUnauthenticated, "authenticated principal is required"))
	}
	result, err := s.deps.AcceptTeamInvitation.Handle(ctx, commands.AcceptTeamInvitationCommand{
		Meta:            commandMeta(ctx, commands.ActorContext{PrincipalID: principalID}, input.CommandHeaders),
		AcceptanceToken: input.Body.AcceptanceToken,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamInvitation(result)
}

func (s *Server) acceptNewTeamInvitation(ctx context.Context, input *contract.TeamInvitationAcceptNewInput) any {
	if s.deps.AcceptNewTeamInvitation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	result, err := s.deps.AcceptNewTeamInvitation.Handle(ctx, commands.AcceptNewTeamInvitationCommand{
		AcceptanceToken: input.Body.AcceptanceToken, DisplayName: input.Body.DisplayName, Password: input.Body.Password,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamInvitation(result)
}

func (s *Server) revokeTeamInvitation(ctx context.Context, input *contract.TeamInvitationRevokeInput) any {
	if s.deps.RevokeTeamInvitation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.RevokeTeamInvitation.Handle(ctx, commands.RevokeTeamInvitationCommand{
		Meta: commandMeta(ctx, actor, input.CommandHeaders), InvitationID: commands.ID(input.InvitationID),
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamInvitation(result)
}

func (s *Server) resendTeamInvitation(ctx context.Context, input *contract.TeamInvitationResendInput) any {
	if s.deps.ResendTeamInvitation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.ResendTeamInvitation.Handle(ctx, commands.ResendTeamInvitationCommand{
		Meta: commandMeta(ctx, actor, input.CommandHeaders), InvitationID: commands.ID(input.InvitationID),
		ExpiresIn: time.Duration(input.Body.ExpiresInHours) * time.Hour,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	out := &contract.Single[contract.TeamInvitationCreated]{}
	out.Body.Data = teamInvitationCreatedProjection(result)
	return out
}

func (s *Server) updateTeamMemberRole(ctx context.Context, input *contract.TeamMemberRoleUpdateInput) any {
	if s.deps.UpdateTeamMemberRole == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.UpdateTeamMemberRole.Handle(ctx, commands.UpdateTeamMemberRoleCommand{
		Meta: commandMeta(ctx, actor, input.CommandHeaders), PrincipalID: commands.PrincipalID(input.PrincipalID), Role: input.Body.Role,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamMember(result)
}

func (s *Server) updateTeamMemberAccess(ctx context.Context, input *contract.TeamMemberAccessUpdateInput) any {
	if s.deps.UpdateTeamMemberAccess == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.UpdateTeamMemberAccess.Handle(ctx, commands.UpdateTeamMemberAccessCommand{
		Meta: commandMeta(ctx, actor, input.CommandHeaders), PrincipalID: commands.PrincipalID(input.PrincipalID),
		Role: input.Body.Role, Permissions: input.Body.Permissions,
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamMember(result)
}

func (s *Server) revokeTeamMember(ctx context.Context, input *contract.TeamMemberRevokeInput) any {
	if s.deps.RevokeTeamMember == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}
	result, err := s.deps.RevokeTeamMember.Handle(ctx, commands.RevokeTeamMemberCommand{
		Meta: commandMeta(ctx, actor, input.CommandHeaders), PrincipalID: commands.PrincipalID(input.PrincipalID),
	})
	if err != nil {
		return mapApplicationError(err)
	}
	return singleTeamMember(result)
}

func teamMemberProjection(view commands.TeamMemberView) contract.TeamMember {
	return contract.TeamMember{
		PrincipalID: contract.UUID(view.PrincipalID), Email: view.Email, DisplayName: view.DisplayName,
		Role: view.Role, Permissions: append([]string(nil), view.Permissions...), Status: view.Status,
	}
}

func teamInvitationProjection(view commands.TeamInvitationView) contract.TeamInvitation {
	result := contract.TeamInvitation{
		ID: contract.UUID(view.ID), BusinessID: contract.UUID(view.BusinessID), BusinessName: view.BusinessName,
		Email: view.Email, DisplayName: view.DisplayName, Role: view.Role,
		Permissions: append([]string(nil), view.Permissions...), Status: view.Status,
		InvitedBy: contract.UUID(view.InvitedBy), ExpiresAt: view.ExpiresAt,
		AcceptedAt: view.AcceptedAt, RevokedAt: view.RevokedAt, CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt, AccountState: view.AccountState,
	}
	if view.AcceptedBy != nil {
		acceptedBy := contract.UUID(*view.AcceptedBy)
		result.AcceptedBy = &acceptedBy
	}
	return result
}

func teamInvitationCreatedProjection(result commands.TeamInvitationResult) contract.TeamInvitationCreated {
	return contract.TeamInvitationCreated{Invitation: teamInvitationProjection(result.Invitation), AcceptanceToken: result.AcceptanceToken}
}

func singleTeamInvitation(view commands.TeamInvitationView) *contract.Single[contract.TeamInvitation] {
	out := &contract.Single[contract.TeamInvitation]{}
	out.Body.Data = teamInvitationProjection(view)
	return out
}

func singleTeamMember(view commands.TeamMemberView) *contract.Single[contract.TeamMember] {
	out := &contract.Single[contract.TeamMember]{}
	out.Body.Data = teamMemberProjection(view)
	return out
}
