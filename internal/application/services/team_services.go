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

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultInvitationLifetime     = 7 * 24 * time.Hour
	maximumInvitationLifetime     = 30 * 24 * time.Hour
	minimumEmployeePasswordLength = 12
	maximumBcryptPasswordBytes    = 72
)

type TeamService struct {
	Repository ports.TeamRepository
	Now        func() time.Time
	NewID      func() string
	NewToken   func() (string, error)
}

type ListTeamMembersQueryService struct{ TeamService }
type ListTeamInvitationsQueryService struct{ TeamService }
type InspectTeamInvitationQueryService struct{ TeamService }
type InviteTeamMemberCommandService struct{ TeamService }
type PlatformInviteTeamMemberCommandService struct{ TeamService }
type AcceptTeamInvitationCommandService struct{ TeamService }
type AcceptNewTeamInvitationCommandService struct{ TeamService }
type RevokeTeamInvitationCommandService struct{ TeamService }
type ResendTeamInvitationCommandService struct{ TeamService }
type UpdateTeamMemberRoleCommandService struct{ TeamService }
type UpdateTeamMemberAccessCommandService struct{ TeamService }
type RevokeTeamMemberCommandService struct{ TeamService }

func (s ListTeamMembersQueryService) Handle(ctx context.Context, query queries.ListTeamMembersQuery) (commands.ListResult[commands.TeamMemberView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.TeamMemberView]{}, appErrors.NotImplemented()
	}
	if !canReadTeam(query.Meta.Actor) {
		return commands.ListResult[commands.TeamMemberView]{}, appErrors.New(appErrors.CodeForbidden, "team read permission is required")
	}
	page, err := s.Repository.ListMembers(ctx, string(query.Meta.Actor.BusinessID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.TeamMemberView]{}, mapTeamRepositoryError(err)
	}
	return teamMemberList(page), nil
}

func (s ListTeamInvitationsQueryService) Handle(ctx context.Context, query queries.ListTeamInvitationsQuery) (commands.ListResult[commands.TeamInvitationView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.TeamInvitationView]{}, appErrors.NotImplemented()
	}
	if !canReadTeam(query.Meta.Actor) {
		return commands.ListResult[commands.TeamInvitationView]{}, appErrors.New(appErrors.CodeForbidden, "team read permission is required")
	}
	page, err := s.Repository.ListInvitations(ctx, string(query.Meta.Actor.BusinessID), query.Limit, query.Cursor, s.now())
	if err != nil {
		return commands.ListResult[commands.TeamInvitationView]{}, mapTeamRepositoryError(err)
	}
	items := make([]commands.TeamInvitationView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, teamInvitationView(item))
	}
	return commands.ListResult[commands.TeamInvitationView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s InspectTeamInvitationQueryService) Handle(ctx context.Context, query queries.InspectTeamInvitationQuery) (commands.TeamInvitationView, error) {
	if s.Repository == nil {
		return commands.TeamInvitationView{}, appErrors.NotImplemented()
	}
	token := strings.TrimSpace(query.AcceptanceToken)
	if token == "" {
		return commands.TeamInvitationView{}, appErrors.New(appErrors.CodeValidation, "invitation acceptance token is required")
	}
	now := query.Now.UTC()
	if now.IsZero() {
		now = s.now()
	}
	record, err := s.Repository.InspectInvitation(ctx, invitationTokenHash(token), now)
	if err != nil {
		return commands.TeamInvitationView{}, mapTeamRepositoryError(err)
	}
	return teamInvitationView(record), nil
}

func (s InviteTeamMemberCommandService) Handle(ctx context.Context, command commands.InviteTeamMemberCommand) (commands.TeamInvitationResult, error) {
	if !canManageTeam(command.Meta.Actor.Role) {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeForbidden, "only owner or admin can invite team members")
	}
	return s.invite(ctx, command, true)
}

func (s PlatformInviteTeamMemberCommandService) Handle(ctx context.Context, command commands.InviteTeamMemberCommand) (commands.TeamInvitationResult, error) {
	return s.invite(ctx, command, false)
}

func (s TeamService) invite(ctx context.Context, command commands.InviteTeamMemberCommand, enforceGrantBoundary bool) (commands.TeamInvitationResult, error) {
	if s.Repository == nil {
		return commands.TeamInvitationResult{}, appErrors.NotImplemented()
	}
	input, err := normalizeInvitationInput(command)
	if err != nil {
		return commands.TeamInvitationResult{}, err
	}
	if enforceGrantBoundary && !authorization.CanGrant(command.Meta.Actor, input.permissions) {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeForbidden, "cannot grant a permission the actor does not hold")
	}
	token, err := s.newToken()
	if err != nil {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeExternalDependency, "failed to generate invitation token")
	}
	record, err := s.Repository.CreateInvitation(ctx, s.invitationCreate(command, input, token))
	if err != nil {
		return commands.TeamInvitationResult{}, mapTeamRepositoryError(err)
	}
	return commands.TeamInvitationResult{Invitation: teamInvitationView(record), AcceptanceToken: token}, nil
}

func (s AcceptTeamInvitationCommandService) Handle(ctx context.Context, command commands.AcceptTeamInvitationCommand) (commands.TeamInvitationView, error) {
	if s.Repository == nil {
		return commands.TeamInvitationView{}, appErrors.NotImplemented()
	}
	token := strings.TrimSpace(command.AcceptanceToken)
	if token == "" || command.Meta.Actor.PrincipalID == "" {
		return commands.TeamInvitationView{}, appErrors.New(appErrors.CodeValidation, "invitation token and authenticated principal are required")
	}
	record, err := s.Repository.AcceptInvitation(ctx, ports.TeamInvitationAcceptance{
		TokenHash: invitationTokenHash(token), PrincipalID: string(command.Meta.Actor.PrincipalID), AcceptedAt: s.now(),
	})
	if err != nil {
		return commands.TeamInvitationView{}, mapTeamRepositoryError(err)
	}
	return teamInvitationView(record), nil
}

func (s AcceptNewTeamInvitationCommandService) Handle(ctx context.Context, command commands.AcceptNewTeamInvitationCommand) (commands.TeamInvitationView, error) {
	if s.Repository == nil {
		return commands.TeamInvitationView{}, appErrors.NotImplemented()
	}
	token := strings.TrimSpace(command.AcceptanceToken)
	displayName := strings.TrimSpace(command.DisplayName)
	passwordBytes := []byte(command.Password)
	if token == "" || displayName == "" || len(passwordBytes) < minimumEmployeePasswordLength || len(passwordBytes) > maximumBcryptPasswordBytes {
		return commands.TeamInvitationView{}, appErrors.New(appErrors.CodeValidation, "valid invitation token, display name, and password between 12 and 72 bytes are required")
	}
	passwordHash, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	if err != nil {
		return commands.TeamInvitationView{}, appErrors.New(appErrors.CodeExternalDependency, "failed to prepare employee credentials")
	}
	record, err := s.Repository.AcceptInvitationNewAccount(ctx, ports.TeamInvitationNewAccountAcceptance{
		TokenHash: invitationTokenHash(token), PrincipalID: s.newID(), DisplayName: displayName,
		PasswordHash: string(passwordHash), AcceptedAt: s.now(),
	})
	if err != nil {
		return commands.TeamInvitationView{}, mapTeamRepositoryError(err)
	}
	return teamInvitationView(record), nil
}

func (s RevokeTeamInvitationCommandService) Handle(ctx context.Context, command commands.RevokeTeamInvitationCommand) (commands.TeamInvitationView, error) {
	if err := s.requireTeamManager(command.Meta.Actor.Role); err != nil {
		return commands.TeamInvitationView{}, err
	}
	if uuid.Validate(string(command.InvitationID)) != nil {
		return commands.TeamInvitationView{}, appErrors.New(appErrors.CodeValidation, "valid invitation id is required")
	}
	record, err := s.Repository.RevokeInvitation(ctx, ports.TeamInvitationRevocation{
		BusinessID: string(command.Meta.Actor.BusinessID), InvitationID: string(command.InvitationID), RevokedAt: s.now(),
	})
	if err != nil {
		return commands.TeamInvitationView{}, mapTeamRepositoryError(err)
	}
	return teamInvitationView(record), nil
}

func (s ResendTeamInvitationCommandService) Handle(ctx context.Context, command commands.ResendTeamInvitationCommand) (commands.TeamInvitationResult, error) {
	if err := s.requireTeamManager(command.Meta.Actor.Role); err != nil {
		return commands.TeamInvitationResult{}, err
	}
	if uuid.Validate(string(command.InvitationID)) != nil {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeValidation, "valid invitation id is required")
	}
	lifetime := invitationLifetime(command.ExpiresIn)
	if lifetime > maximumInvitationLifetime {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeValidation, "invitation expiry cannot exceed 30 days")
	}
	token, err := s.newToken()
	if err != nil {
		return commands.TeamInvitationResult{}, appErrors.New(appErrors.CodeExternalDependency, "failed to generate invitation token")
	}
	now := s.now()
	record, err := s.Repository.ResendInvitation(ctx, ports.TeamInvitationResend{
		BusinessID: string(command.Meta.Actor.BusinessID), InvitationID: string(command.InvitationID),
		NewInvitationID: s.newID(), TokenHash: invitationTokenHash(token), InvitedBy: string(command.Meta.Actor.PrincipalID),
		ExpiresAt: now.Add(lifetime), CreatedAt: now,
	})
	if err != nil {
		return commands.TeamInvitationResult{}, mapTeamRepositoryError(err)
	}
	return commands.TeamInvitationResult{Invitation: teamInvitationView(record), AcceptanceToken: token}, nil
}

func (s UpdateTeamMemberRoleCommandService) Handle(ctx context.Context, command commands.UpdateTeamMemberRoleCommand) (commands.TeamMemberView, error) {
	if err := s.requireTeamManager(command.Meta.Actor.Role); err != nil {
		return commands.TeamMemberView{}, err
	}
	role := normalizeTeamRole(command.Role)
	if !validTeamRole(role) || role == TeamRoleOwner || !validTeamPrincipalID(command.PrincipalID) {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid principal and non-owner team role are required")
	}
	record, err := s.Repository.UpdateMemberRole(ctx, ports.TeamMemberRoleChange{
		BusinessID: string(command.Meta.Actor.BusinessID), PrincipalID: string(command.PrincipalID), Role: role, UpdatedAt: s.now(),
	})
	if err != nil {
		return commands.TeamMemberView{}, mapTeamRepositoryError(err)
	}
	return teamMemberView(record), nil
}

func (s UpdateTeamMemberAccessCommandService) Handle(ctx context.Context, command commands.UpdateTeamMemberAccessCommand) (commands.TeamMemberView, error) {
	if err := s.requireTeamManager(command.Meta.Actor.Role); err != nil {
		return commands.TeamMemberView{}, err
	}
	role := normalizeTeamRole(command.Role)
	permissions, ok := authorization.NormalizePermissions(command.Permissions)
	if !validTeamRole(role) || role == TeamRoleOwner || !validTeamPrincipalID(command.PrincipalID) || !ok || len(permissions) == 0 {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid principal, non-owner role, and known permissions are required")
	}
	if !authorization.CanGrant(command.Meta.Actor, permissions) {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeForbidden, "cannot grant a permission the actor does not hold")
	}
	record, err := s.Repository.UpdateMemberAccess(ctx, ports.TeamMemberAccessChange{
		BusinessID: string(command.Meta.Actor.BusinessID), PrincipalID: string(command.PrincipalID),
		Role: role, Permissions: permissions, UpdatedAt: s.now(),
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
	if !validTeamPrincipalID(command.PrincipalID) || command.PrincipalID == command.Meta.Actor.PrincipalID {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid different team principal id is required")
	}
	record, err := s.Repository.RevokeMember(ctx, ports.TeamMemberRevocation{
		BusinessID: string(command.Meta.Actor.BusinessID), PrincipalID: string(command.PrincipalID), RevokedAt: s.now(),
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

func canReadTeam(actor commands.ActorContext) bool {
	return canManageTeam(actor.Role) || authorization.Has(actor, authorization.PermissionTeamRead)
}

type normalizedInvitationInput struct {
	email       string
	displayName string
	role        string
	permissions []string
	lifetime    time.Duration
}

func normalizeInvitationInput(command commands.InviteTeamMemberCommand) (normalizedInvitationInput, error) {
	email := strings.ToLower(strings.TrimSpace(command.Email))
	displayName := strings.TrimSpace(command.DisplayName)
	role := normalizeTeamRole(command.Role)
	permissions, ok := authorization.NormalizePermissions(command.Permissions)
	if !validInvitationEmail(email) || displayName == "" || !validTeamRole(role) || role == TeamRoleOwner || !ok || len(permissions) == 0 {
		return normalizedInvitationInput{}, appErrors.New(appErrors.CodeValidation, "valid employee identity, non-owner role, and known permissions are required")
	}
	lifetime := invitationLifetime(command.ExpiresIn)
	if lifetime > maximumInvitationLifetime {
		return normalizedInvitationInput{}, appErrors.New(appErrors.CodeValidation, "invitation expiry cannot exceed 30 days")
	}
	return normalizedInvitationInput{email: email, displayName: displayName, role: role, permissions: permissions, lifetime: lifetime}, nil
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
		ID: s.newID(), BusinessID: string(command.Meta.Actor.BusinessID), Email: input.email, DisplayName: input.displayName,
		Role: input.role, Permissions: input.permissions, TokenHash: invitationTokenHash(token),
		InvitedBy: string(command.Meta.Actor.PrincipalID), ExpiresAt: now.Add(input.lifetime), CreatedAt: now, UpdatedAt: now,
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
	return commands.ListResult[commands.TeamMemberView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}
}

func teamMemberView(record ports.TeamMemberRecord) commands.TeamMemberView {
	return commands.TeamMemberView{
		PrincipalID: commands.PrincipalID(record.PrincipalID), Email: record.Email, DisplayName: record.DisplayName,
		Role: record.Role, Permissions: append([]string(nil), record.Permissions...), Status: record.Status,
	}
}

func teamInvitationView(record ports.TeamInvitationRecord) commands.TeamInvitationView {
	view := commands.TeamInvitationView{
		ID: commands.ID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), BusinessName: record.BusinessName,
		Email: record.Email, DisplayName: record.DisplayName, Role: record.Role,
		Permissions: append([]string(nil), record.Permissions...), Status: record.Status,
		InvitedBy: commands.PrincipalID(record.InvitedBy), ExpiresAt: record.ExpiresAt,
		AcceptedAt: record.AcceptedAt, RevokedAt: record.RevokedAt, CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt, AccountState: record.AccountState,
	}
	if record.AcceptedBy != nil {
		acceptedBy := commands.PrincipalID(*record.AcceptedBy)
		view.AcceptedBy = &acceptedBy
	}
	return view
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

func validTeamPrincipalID(id commands.PrincipalID) bool { return uuid.Validate(string(id)) == nil }

// Only map known, business-actionable repository conflicts. Never put raw
// PostgreSQL errors, SQL values, personal details or schema names in the API.
func mapTeamRepositoryError(err error) error {
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "team member or invitation was not found")
	case "conflict":
		switch {
		case strings.Contains(err.Error(), "an active subscription is required to add a team seat"):
			return appErrors.New(appErrors.CodeInvalidState, "An active subscription is required before accepting a team invitation.")
		case strings.Contains(err.Error(), "active seat limit "):
			return appErrors.New(appErrors.CodeInvalidState, "The subscription has no available employee seat.")
		case strings.Contains(err.Error(), "principal already exists"):
			return appErrors.New(appErrors.CodeInvalidState, "An account already exists for the invited email. Sign in to accept this invitation.")
		case strings.Contains(err.Error(), "invitation is not pending"):
			return appErrors.New(appErrors.CodeInvalidState, "The invitation is no longer pending. Ask for a new invitation.")
		case strings.Contains(err.Error(), "business is not active"):
			return appErrors.New(appErrors.CodeInvalidState, "The business is not active and cannot accept new members.")
		case strings.Contains(err.Error(), "accepted invitation cannot be resent"):
			return appErrors.New(appErrors.CodeInvalidState, "An accepted invitation cannot be resent.")
		case strings.Contains(err.Error(), "a newer pending invitation already exists"):
			return appErrors.New(appErrors.CodeInvalidState,
				"A newer pending invitation already exists for this email. Refresh the invitation list.")
		default:
			return appErrors.New(appErrors.CodeInvalidState, "The team invitation cannot be completed in its current state.")
		}
	case "invalid":
		// The persistence layer could have rejected a database constraint,
		// schema mismatch, or infrastructure command. This is NOT proof that
		// the merchant entered invalid values. Server-side logs/request_id
		// must be inspected to identify the specific underlying SQLSTATE.
		return appErrors.New(appErrors.CodeExternalDependency,
			"Team invitation could not be saved. Please contact support with the request ID.")
	default:
		return err
	}
}

var _ queries.ListTeamInvitationsHandler = ListTeamInvitationsQueryService{}
var _ queries.InspectTeamInvitationHandler = InspectTeamInvitationQueryService{}
var _ commands.InviteTeamMemberHandler = InviteTeamMemberCommandService{}
var _ commands.InviteTeamMemberHandler = PlatformInviteTeamMemberCommandService{}
var _ commands.AcceptTeamInvitationHandler = AcceptTeamInvitationCommandService{}
var _ commands.AcceptNewTeamInvitationHandler = AcceptNewTeamInvitationCommandService{}
var _ commands.RevokeTeamInvitationHandler = RevokeTeamInvitationCommandService{}
var _ commands.ResendTeamInvitationHandler = ResendTeamInvitationCommandService{}
var _ commands.UpdateTeamMemberRoleHandler = UpdateTeamMemberRoleCommandService{}
var _ commands.UpdateTeamMemberAccessHandler = UpdateTeamMemberAccessCommandService{}
var _ commands.RevokeTeamMemberHandler = RevokeTeamMemberCommandService{}
