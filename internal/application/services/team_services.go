package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/google/uuid"
)

const (
	defaultInvitationLifetime = 7 * 24 * time.Hour
	maximumInvitationLifetime = 30 * 24 * time.Hour
)

type TeamService struct {
	Repository ports.TeamRepository
	Now        func() time.Time
	NewID      func() string
	NewToken   func() (string, error)
}

type ListTeamMembersQueryService struct {
	TeamService
}

type InviteTeamMemberCommandService struct {
	TeamService
}

type AcceptTeamInvitationCommandService struct {
	TeamService
}

type UpdateTeamMemberRoleCommandService struct {
	TeamService
}

type RevokeTeamMemberCommandService struct {
	TeamService
}

func (s ListTeamMembersQueryService) Handle(ctx context.Context, query queries.ListTeamMembersQuery) (commands.ListResult[commands.TeamMemberView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.TeamMemberView]{}, appErrors.NotImplemented()
	}

	page, err := s.Repository.ListMembers(ctx, string(query.Meta.Actor.BusinessID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.TeamMemberView]{}, mapAIRepositoryError(err)
	}

	return teamMemberList(page), nil
}

func (s InviteTeamMemberCommandService) Handle(ctx context.Context, command commands.InviteTeamMemberCommand) (commands.TeamInvitationResult, error) {
	if s.Repository == nil {
		return commands.TeamInvitationResult{}, appErrors.NotImplemented()
	}
	if !canManageTeam(command.Meta.Actor.Role) {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeForbidden, "only owner or admin can invite team members")
	}

	input, err := normalizeInvitationInput(command)
	if err != nil {
		return commands.TeamInvitationResult{}, err
	}

	token, err := s.newToken()
	if err != nil {
		return commands.TeamInvitationResult{}, err
	}

	record, err := s.Repository.CreateInvitation(ctx, s.invitationCreate(command, input, token))
	if err != nil {
		return commands.TeamInvitationResult{}, mapAIRepositoryError(err)
	}

	return commands.TeamInvitationResult{
		Invitation:      teamInvitationView(record),
		AcceptanceToken: token,
	}, nil
}

func (s AcceptTeamInvitationCommandService) Handle(ctx context.Context, command commands.AcceptTeamInvitationCommand) (commands.TeamInvitationView, error) {
	if s.Repository == nil {
		return commands.TeamInvitationView{}, appErrors.NotImplemented()
	}

	token := strings.TrimSpace(command.AcceptanceToken)
	if token == "" {
		return commands.TeamInvitationView{}, appErrors.New(appErrors.CodeValidation, "invitation acceptance token is required")
	}

	record, err := s.Repository.AcceptInvitation(ctx, ports.TeamInvitationAcceptance{
		TokenHash:   invitationTokenHash(token),
		PrincipalID: string(command.Meta.Actor.PrincipalID),
		AcceptedAt:  s.now(),
	})
	if err != nil {
		return commands.TeamInvitationView{}, mapAIRepositoryError(err)
	}

	return teamInvitationView(record), nil
}

func (s UpdateTeamMemberRoleCommandService) Handle(ctx context.Context, command commands.UpdateTeamMemberRoleCommand) (commands.TeamMemberView, error) {
	if err := s.requireTeamManager(command.Meta.Actor.Role); err != nil {
		return commands.TeamMemberView{}, err
	}

	role := normalizeTeamRole(command.Role)
	if !validTeamRole(role) || role == TeamRoleOwner {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid non-owner team role is required")
	}
	if !validTeamPrincipalID(command.PrincipalID) {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid team principal id is required")
	}

	record, err := s.Repository.UpdateMemberRole(ctx, ports.TeamMemberRoleChange{
		BusinessID:  string(command.Meta.Actor.BusinessID),
		PrincipalID: string(command.PrincipalID),
		Role:        role,
		UpdatedAt:   s.now(),
	})
	if err != nil {
		return commands.TeamMemberView{}, mapTeamRepositoryError(err)
	}

	return teamMemberView(record), nil
}

func (s RevokeTeamMemberCommandService) Handle(ctx context.Context, command commands.RevokeTeamMemberCommand) (commands.TeamMemberView, error) {
	if err := s.requireTeamManager(command.Meta.Actor.Role); err != nil {
		return commands.TeamMemberView{}, err
	}
	if !validTeamPrincipalID(command.PrincipalID) {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid team principal id is required")
	}

	record, err := s.Repository.RevokeMember(ctx, ports.TeamMemberRevocation{
		BusinessID:  string(command.Meta.Actor.BusinessID),
		PrincipalID: string(command.PrincipalID),
		RevokedAt:   s.now(),
	})
	if err != nil {
		return commands.TeamMemberView{}, mapTeamRepositoryError(err)
	}

	return teamMemberView(record), nil
}

func (s TeamService) requireTeamManager(role string) error {
	if s.Repository == nil {
		return appErrors.NotImplemented()
	}
	if !canManageTeam(role) {
		return appErrors.New(appErrors.CodeForbidden, "only owner or admin can manage team members")
	}
	return nil
}

type normalizedInvitationInput struct {
	email    string
	role     string
	lifetime time.Duration
}

func normalizeInvitationInput(command commands.InviteTeamMemberCommand) (normalizedInvitationInput, error) {
	email := strings.ToLower(strings.TrimSpace(command.Email))
	role := normalizeTeamRole(command.Role)
	if !validInvitationEmail(email) || !validTeamRole(role) || role == TeamRoleOwner {
		return normalizedInvitationInput{}, appErrors.New(appErrors.CodeValidation, "valid email and non-owner team role are required")
	}

	lifetime := invitationLifetime(command.ExpiresIn)
	if lifetime > maximumInvitationLifetime {
		return normalizedInvitationInput{}, appErrors.New(appErrors.CodeValidation, "invitation expiry cannot exceed 30 days")
	}

	return normalizedInvitationInput{email: email, role: role, lifetime: lifetime}, nil
}

func invitationLifetime(requested time.Duration) time.Duration {
	if requested <= 0 {
		return defaultInvitationLifetime
	}
	return requested
}

func (s TeamService) invitationCreate(command commands.InviteTeamMemberCommand, input normalizedInvitationInput, token string) ports.TeamInvitationCreate {
	now := s.now()
	return ports.TeamInvitationCreate{
		ID:          s.newID(),
		BusinessID:  string(command.Meta.Actor.BusinessID),
		Email:       input.email,
		Role:        input.role,
		Permissions: []string{},
		TokenHash:   invitationTokenHash(token),
		InvitedBy:   string(command.Meta.Actor.PrincipalID),
		ExpiresAt:   now.Add(input.lifetime),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (s TeamService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s TeamService) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return uuid.NewString()
}

func (s TeamService) newToken() (string, error) {
	if s.NewToken != nil {
		return s.NewToken()
	}
	return newInvitationToken()
}

func teamMemberList(page ports.TeamMemberPage) commands.ListResult[commands.TeamMemberView] {
	items := make([]commands.TeamMemberView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, teamMemberView(item))
	}
	return commands.ListResult[commands.TeamMemberView]{
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
}

func teamMemberView(record ports.TeamMemberRecord) commands.TeamMemberView {
	return commands.TeamMemberView{
		PrincipalID: commands.PrincipalID(record.PrincipalID),
		Email:       record.Email,
		DisplayName: record.DisplayName,
		Role:        record.Role,
		Status:      record.Status,
	}
}

func teamInvitationView(record ports.TeamInvitationRecord) commands.TeamInvitationView {
	return commands.TeamInvitationView{
		ID:        commands.ID(record.ID),
		Email:     record.Email,
		Role:      record.Role,
		Status:    record.Status,
		ExpiresAt: record.ExpiresAt,
	}
}

func validInvitationEmail(value string) bool {
	at := strings.IndexByte(value, '@')
	return at > 0 && at < len(value)-1 && !strings.ContainsAny(value, " \t\n")
}

func newInvitationToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func invitationTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func validTeamPrincipalID(id commands.PrincipalID) bool {
	return uuid.Validate(string(id)) == nil
}

func mapTeamRepositoryError(err error) error {
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "team member or invitation was not found")
	case "conflict":
		return appErrors.New(appErrors.CodeInvalidState, "team operation is not allowed in its current state")
	case "invalid":
		return appErrors.New(appErrors.CodeValidation, "team persistence rejected the request")
	default:
		return err
	}
}

var _ queries.ListTeamMembersHandler = ListTeamMembersQueryService{}
var _ commands.InviteTeamMemberHandler = InviteTeamMemberCommandService{}
var _ commands.AcceptTeamInvitationHandler = AcceptTeamInvitationCommandService{}
var _ commands.UpdateTeamMemberRoleHandler = UpdateTeamMemberRoleCommandService{}
var _ commands.RevokeTeamMemberHandler = RevokeTeamMemberCommandService{}
