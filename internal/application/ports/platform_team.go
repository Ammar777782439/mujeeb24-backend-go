package ports

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"time"
)

// OwnerPrincipalRepository creates a NEW identity; it must never reset an existing account.
// The caller owns the transaction that also activates the business and writes its audit.
type OwnerPrincipalRepository interface {
	CreateOwnerPrincipalAndMembership(context.Context, PrincipalRecord, commands.BusinessID, []string, time.Time) (PrincipalRecord, error)
}

// PlatformTeamRepository exposes only team governance, never merchant content.
// Permission changes preserve role and status, including the last-owner invariant.
type PlatformTeamRepository interface {
	ListMembers(context.Context, string, int, string) (TeamMemberPage, error)
	ListInvitations(context.Context, string, int, string, time.Time) (TeamInvitationPage, error)
	UpdateMemberPermissions(context.Context, TeamMemberPermissionChange) (TeamMemberRecord, TeamMemberRecord, error)
}

type TeamMemberPermissionChange struct {
	BusinessID  string
	PrincipalID string
	Permissions []string
	UpdatedAt   time.Time
}
