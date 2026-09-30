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
	case "updateTeamMemberRole":
		return s.updateTeamMemberRole(ctx, input.(*contract.TeamMemberRoleUpdateInput)), true
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
		Meta:      commandMeta(ctx, actor, input.CommandHeaders),
		Email:     input.Body.Email,
		Role:      input.Body.Role,
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

	out := &contract.Single[contract.TeamInvitation]{}
	out.Body.Data = teamInvitationProjection(result)
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
		Meta:        commandMeta(ctx, actor, input.CommandHeaders),
		PrincipalID: commands.PrincipalID(input.PrincipalID),
		Role:        input.Body.Role,
	})
	if err != nil {
		return mapApplicationError(err)
	}

	out := &contract.Single[contract.TeamMember]{}
	out.Body.Data = teamMemberProjection(result)
	return out
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
		Meta:        commandMeta(ctx, actor, input.CommandHeaders),
		PrincipalID: commands.PrincipalID(input.PrincipalID),
	})
	if err != nil {
		return mapApplicationError(err)
	}

	out := &contract.Single[contract.TeamMember]{}
	out.Body.Data = teamMemberProjection(result)
	return out
}

func teamMemberProjection(view commands.TeamMemberView) contract.TeamMember {
	return contract.TeamMember{
		PrincipalID: contract.UUID(view.PrincipalID),
		Email:       view.Email,
		DisplayName: view.DisplayName,
		Role:        view.Role,
		Status:      view.Status,
	}
}

func teamInvitationProjection(view commands.TeamInvitationView) contract.TeamInvitation {
	return contract.TeamInvitation{
		ID:        contract.UUID(view.ID),
		Email:     view.Email,
		Role:      view.Role,
		Status:    view.Status,
		ExpiresAt: view.ExpiresAt,
	}
}

func teamInvitationCreatedProjection(result commands.TeamInvitationResult) contract.TeamInvitationCreated {
	return contract.TeamInvitationCreated{
		Invitation:      teamInvitationProjection(result.Invitation),
		AcceptanceToken: result.AcceptanceToken,
	}
}
