package ports

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

// PlatformAccessRepository is intentionally separate from business membership.
// An active platform administrator is not automatically a member of any tenant.
type PlatformAccessRepository interface {
	IsActiveSuperAdmin(ctx context.Context, principalID string) (bool, error)
}

type PlatformSuperAdminBootstrap struct {
	Principal commands.PrincipalID
	Email     string
	Name      string
	Hash      string
	Now       time.Time
}

type PlatformSuperAdminBootstrapRepository interface {
	EnsurePlatformSuperAdmin(ctx context.Context, bootstrap PlatformSuperAdminBootstrap) (commands.PrincipalID, error)
}
