package ports

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

type PrincipalRecord struct {
	ID           commands.PrincipalID
	Email        string
	DisplayName  string
	PasswordHash string
	Status       string
}

type BusinessMembershipRecord struct {
	BusinessID  commands.BusinessID
	PrincipalID commands.PrincipalID
	Business    commands.BusinessView
	Role        string
	Permissions []string
	Status      string
}

type RefreshSessionRecord struct {
	ID          string
	PrincipalID commands.PrincipalID
	TokenHash   string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	UsedAt      *time.Time
}

type PrincipalRepository interface {
	GetByEmail(ctx context.Context, email string) (PrincipalRecord, error)
	GetByID(ctx context.Context, principalID commands.PrincipalID) (PrincipalRecord, error)
	ListActiveMemberships(ctx context.Context, principalID commands.PrincipalID, limit int, cursor string) ([]BusinessMembershipRecord, string, bool, error)
	ResolveActiveMembership(ctx context.Context, principalID commands.PrincipalID, businessID commands.BusinessID) (BusinessMembershipRecord, error)
}

type PrincipalBootstrapRepository interface {
	EnsurePrincipalAndMembership(ctx context.Context, principal PrincipalRecord, businessID commands.BusinessID, role string, permissions []string, now time.Time) (PrincipalRecord, error)
}

type RefreshSessionRepository interface {
	Create(ctx context.Context, session RefreshSessionRecord, now time.Time) error
	Consume(ctx context.Context, tokenHash string, now time.Time) (RefreshSessionRecord, error)
	Revoke(ctx context.Context, principalID commands.PrincipalID, sessionID string, now time.Time) error
}

type AccessTokenIssuer interface {
	IssueAccessToken(ctx context.Context, principalID commands.PrincipalID, now time.Time) (string, time.Time, error)
	VerifyAccessToken(ctx context.Context, token string) (commands.PrincipalID, error)
}
