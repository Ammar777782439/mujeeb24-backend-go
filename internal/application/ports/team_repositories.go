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
	ID          string
	BusinessID  string
	Email       string
	Role        string
	Permissions []string
	TokenHash   string
	Status      string
	InvitedBy   string
	AcceptedBy  *string
	ExpiresAt   time.Time
	AcceptedAt  *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TeamInvitationCreate struct {
	ID          string
	BusinessID  string
	Email       string
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

type TeamMemberRoleChange struct {
	BusinessID  string
	PrincipalID string
	Role        string
	UpdatedAt   time.Time
}

type TeamMemberRevocation struct {
	BusinessID  string
	PrincipalID string
	RevokedAt   time.Time
}

type TeamRepository interface {
	ListMembers(ctx context.Context, businessID string, limit int, cursor string) (TeamMemberPage, error)
	ResolveActiveMember(ctx context.Context, businessID, principalID string) (TeamMemberRecord, error)
	CreateInvitation(ctx context.Context, create TeamInvitationCreate) (TeamInvitationRecord, error)
	AcceptInvitation(ctx context.Context, acceptance TeamInvitationAcceptance) (TeamInvitationRecord, error)
	UpdateMemberRole(ctx context.Context, change TeamMemberRoleChange) (TeamMemberRecord, error)
	RevokeMember(ctx context.Context, revocation TeamMemberRevocation) (TeamMemberRecord, error)
}
