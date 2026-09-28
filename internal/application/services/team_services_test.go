package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestInviteTeamMemberCreatesHashedNormalizedInvitation(t *testing.T) {
	now := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	repository := &teamServiceRepository{}
	service := InviteTeamMemberCommandService{TeamService: TeamService{
		Repository: repository,
		Now:        func() time.Time { return now },
		NewID:      func() string { return "invitation-1" },
		NewToken:   func() (string, error) { return "one-time-token", nil },
	}}

	result, err := service.Handle(context.Background(), commands.InviteTeamMemberCommand{
		Meta:  commands.CommandMeta{Actor: teamActor(TeamRoleAdmin)},
		Email: "  STAFF@EXAMPLE.TEST ",
		Role:  " Agent ",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if result.AcceptanceToken != "one-time-token" {
		t.Fatalf("expected one-time token, got %q", result.AcceptanceToken)
	}
	if repository.created.Email != "staff@example.test" || repository.created.Role != TeamRoleAgent {
		t.Fatalf("expected normalized invitation, got %#v", repository.created)
	}
	if repository.created.TokenHash == result.AcceptanceToken || repository.created.TokenHash != invitationTokenHash(result.AcceptanceToken) {
		t.Fatalf("invitation token must be stored as a SHA-256 hash only")
	}
	if !repository.created.ExpiresAt.Equal(now.Add(defaultInvitationLifetime)) {
		t.Fatalf("unexpected expiry: %s", repository.created.ExpiresAt)
	}
}

func TestInviteTeamMemberRejectsUnauthorizedAndOwnerRole(t *testing.T) {
	repository := &teamServiceRepository{}
	service := InviteTeamMemberCommandService{TeamService: TeamService{Repository: repository}}

	_, err := service.Handle(context.Background(), commands.InviteTeamMemberCommand{Meta: commands.CommandMeta{Actor: teamActor(TeamRoleAgent)}, Email: "staff@example.test", Role: TeamRoleAgent})
	if err == nil || !hasApplicationCode(err, appErrors.CodeForbidden) || repository.createCalls != 0 {
		t.Fatalf("expected forbidden invite without persistence, err=%v calls=%d", err, repository.createCalls)
	}

	_, err = service.Handle(context.Background(), commands.InviteTeamMemberCommand{Meta: commands.CommandMeta{Actor: teamActor(TeamRoleOwner)}, Email: "staff@example.test", Role: TeamRoleOwner})
	if err == nil || !hasApplicationCode(err, appErrors.CodeValidation) || repository.createCalls != 0 {
		t.Fatalf("expected owner-role rejection without persistence, err=%v calls=%d", err, repository.createCalls)
	}
}

func TestAcceptTeamInvitationHashesTokenBeforePersistence(t *testing.T) {
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	repository := &teamServiceRepository{
		accepted: ports.TeamInvitationRecord{
			ID:        "invitation-1",
			Email:     "staff@example.test",
			Role:      TeamRoleAgent,
			Status:    "accepted",
			ExpiresAt: now.Add(time.Hour),
		},
	}
	service := AcceptTeamInvitationCommandService{TeamService: TeamService{
		Repository: repository,
		Now:        func() time.Time { return now },
	}}

	result, err := service.Handle(context.Background(), commands.AcceptTeamInvitationCommand{
		Meta:            commands.CommandMeta{Actor: teamActor(TeamRoleAgent)},
		AcceptanceToken: "one-time-token",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if repository.acceptedInput.TokenHash != invitationTokenHash("one-time-token") || repository.acceptedInput.TokenHash == "one-time-token" {
		t.Fatalf("acceptance must persist the token hash only")
	}
	if repository.acceptedInput.PrincipalID != "principal-1" || !repository.acceptedInput.AcceptedAt.Equal(now) {
		t.Fatalf("unexpected acceptance input: %#v", repository.acceptedInput)
	}
	if result.Status != "accepted" || result.Email != "staff@example.test" {
		t.Fatalf("unexpected accepted invitation: %#v", result)
	}
}

func TestAcceptTeamInvitationRejectsBlankToken(t *testing.T) {
	repository := &teamServiceRepository{}
	service := AcceptTeamInvitationCommandService{TeamService: TeamService{Repository: repository}}

	_, err := service.Handle(context.Background(), commands.AcceptTeamInvitationCommand{
		Meta:            commands.CommandMeta{Actor: teamActor(TeamRoleAgent)},
		AcceptanceToken: " \t ",
	})
	if err == nil || !hasApplicationCode(err, appErrors.CodeValidation) || repository.acceptCalls != 0 {
		t.Fatalf("expected validation without repository call, err=%v calls=%d", err, repository.acceptCalls)
	}
}

func TestUpdateTeamMemberRoleRequiresManagerAndNonOwnerTargetRole(t *testing.T) {
	memberID := commands.PrincipalID("00000000-0000-0000-0000-000000000012")
	repository := &teamServiceRepository{
		updated: ports.TeamMemberRecord{PrincipalID: string(memberID), Role: TeamRoleViewer, Status: "active"},
	}
	service := UpdateTeamMemberRoleCommandService{TeamService: TeamService{Repository: repository}}

	_, err := service.Handle(context.Background(), commands.UpdateTeamMemberRoleCommand{
		Meta:        commands.CommandMeta{Actor: teamActor(TeamRoleAgent)},
		PrincipalID: memberID,
		Role:        TeamRoleViewer,
	})
	if err == nil || !hasApplicationCode(err, appErrors.CodeForbidden) || repository.updateCalls != 0 {
		t.Fatalf("expected manager guard, err=%v calls=%d", err, repository.updateCalls)
	}

	_, err = service.Handle(context.Background(), commands.UpdateTeamMemberRoleCommand{
		Meta:        commands.CommandMeta{Actor: teamActor(TeamRoleOwner)},
		PrincipalID: memberID,
		Role:        TeamRoleOwner,
	})
	if err == nil || !hasApplicationCode(err, appErrors.CodeValidation) || repository.updateCalls != 0 {
		t.Fatalf("expected owner role rejection, err=%v calls=%d", err, repository.updateCalls)
	}
}

func TestRevokeTeamMemberValidatesPrincipalBeforePersistence(t *testing.T) {
	repository := &teamServiceRepository{}
	service := RevokeTeamMemberCommandService{TeamService: TeamService{Repository: repository}}

	_, err := service.Handle(context.Background(), commands.RevokeTeamMemberCommand{
		Meta:        commands.CommandMeta{Actor: teamActor(TeamRoleAdmin)},
		PrincipalID: "not-a-uuid",
	})
	if err == nil || !hasApplicationCode(err, appErrors.CodeValidation) || repository.revokeCalls != 0 {
		t.Fatalf("expected UUID validation without persistence, err=%v calls=%d", err, repository.revokeCalls)
	}
}

func teamActor(role string) commands.ActorContext {
	return commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1", Role: role}
}

type teamServiceRepository struct {
	created       ports.TeamInvitationCreate
	createCalls   int
	accepted      ports.TeamInvitationRecord
	acceptedInput ports.TeamInvitationAcceptance
	acceptCalls   int
	updated       ports.TeamMemberRecord
	updateInput   ports.TeamMemberRoleChange
	updateCalls   int
	revoked       ports.TeamMemberRecord
	revokeInput   ports.TeamMemberRevocation
	revokeCalls   int
}

func (r *teamServiceRepository) ListMembers(context.Context, string, int, string) (ports.TeamMemberPage, error) {
	return ports.TeamMemberPage{}, nil
}

func (r *teamServiceRepository) ResolveActiveMember(context.Context, string, string) (ports.TeamMemberRecord, error) {
	return ports.TeamMemberRecord{}, nil
}

func (r *teamServiceRepository) CreateInvitation(_ context.Context, create ports.TeamInvitationCreate) (ports.TeamInvitationRecord, error) {
	r.created = create
	r.createCalls++
	return ports.TeamInvitationRecord{ID: create.ID, Email: create.Email, Role: create.Role, Status: "pending", ExpiresAt: create.ExpiresAt}, nil
}

func (r *teamServiceRepository) AcceptInvitation(_ context.Context, acceptance ports.TeamInvitationAcceptance) (ports.TeamInvitationRecord, error) {
	r.acceptedInput = acceptance
	r.acceptCalls++
	return r.accepted, nil
}

func (r *teamServiceRepository) UpdateMemberRole(_ context.Context, change ports.TeamMemberRoleChange) (ports.TeamMemberRecord, error) {
	r.updateInput = change
	r.updateCalls++
	return r.updated, nil
}

func (r *teamServiceRepository) RevokeMember(_ context.Context, revocation ports.TeamMemberRevocation) (ports.TeamMemberRecord, error) {
	r.revokeInput = revocation
	r.revokeCalls++
	return r.revoked, nil
}
