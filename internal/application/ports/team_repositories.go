package ports

import (
	"context"
	"time"
)

type TeamMemberRecord struct {
	BusinessID  string
	PrincipalID string
	Email       string
	DisplayName string
	Role        string
	Permissions []string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TeamMemberPage struct {
	Items      []TeamMemberRecord
	NextCursor string
	HasMore    bool
}

type TeamInvitationRecord struct {
	ID           string
	BusinessID   string
	BusinessName string
	Email        string
	DisplayName  string
	Role         string
	Permissions  []string
	TokenHash    string
	Status       string
	InvitedBy    string
	AcceptedBy   *string
	ExpiresAt    time.Time
	AcceptedAt   *time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	AccountState string
}

type TeamInvitationPage struct {
	Items      []TeamInvitationRecord
	NextCursor string
	HasMore    bool
}

type TeamInvitationCreate struct {
	ID          string
	BusinessID  string
	Email       string
	DisplayName string
	Role        string
	Permissions []string
	TokenHash   string
	InvitedBy   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TeamInvitationAcceptance struct {
	TokenHash   string
	PrincipalID string
	AcceptedAt  time.Time
}

type TeamInvitationNewAccountAcceptance struct {
	TokenHash    string
	PrincipalID  string
	DisplayName  string
	PasswordHash string
	AcceptedAt   time.Time
}

type TeamInvitationRevocation struct {
	BusinessID   string
	InvitationID string
	RevokedAt    time.Time
}

type TeamInvitationResend struct {
	BusinessID      string
	InvitationID    string
	NewInvitationID string
	TokenHash       string
	InvitedBy       string
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

type TeamMemberRoleChange struct {
	BusinessID  string
	PrincipalID string
	Role        string
	UpdatedAt   time.Time
}

type TeamMemberAccessChange struct {
	BusinessID  string
	PrincipalID string
	Role        string
	Permissions []string
	UpdatedAt   time.Time
}

type TeamMemberRevocation struct {
	BusinessID  string
	PrincipalID string
	RevokedAt   time.Time
}

type TeamRepository interface {
	ListMembers(ctx context.Context, businessID string, limit int, cursor string) (TeamMemberPage, error)
	ListInvitations(ctx context.Context, businessID string, limit int, cursor string, now time.Time) (TeamInvitationPage, error)
	ResolveActiveMember(ctx context.Context, businessID, principalID string) (TeamMemberRecord, error)
	CreateInvitation(ctx context.Context, create TeamInvitationCreate) (TeamInvitationRecord, error)
	InspectInvitation(ctx context.Context, tokenHash string, now time.Time) (TeamInvitationRecord, error)
	AcceptInvitation(ctx context.Context, acceptance TeamInvitationAcceptance) (TeamInvitationRecord, error)
	AcceptInvitationNewAccount(ctx context.Context, acceptance TeamInvitationNewAccountAcceptance) (TeamInvitationRecord, error)
	RevokeInvitation(ctx context.Context, revocation TeamInvitationRevocation) (TeamInvitationRecord, error)
	ResendInvitation(ctx context.Context, resend TeamInvitationResend) (TeamInvitationRecord, error)
	UpdateMemberRole(ctx context.Context, change TeamMemberRoleChange) (TeamMemberRecord, error)
	UpdateMemberAccess(ctx context.Context, change TeamMemberAccessChange) (TeamMemberRecord, error)
	RevokeMember(ctx context.Context, revocation TeamMemberRevocation) (TeamMemberRecord, error)
}

type TeamMemberResolver interface {
	ResolveActiveMember(ctx context.Context, businessID, principalID string) (TeamMemberRecord, error)
}
