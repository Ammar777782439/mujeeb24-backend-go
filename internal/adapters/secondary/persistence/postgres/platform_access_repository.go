package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type PlatformAccessRepository struct {
	adapter *Adapter
}

func NewPlatformAccessRepository(adapter *Adapter) *PlatformAccessRepository {
	return &PlatformAccessRepository{adapter: adapter}
}

func (r *PlatformAccessRepository) IsActiveSuperAdmin(ctx context.Context, principalID string) (bool, error) {
	if r == nil || r.adapter == nil {
		return false, ErrPoolClosed
	}

	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return false, err
	}

	var active bool
	err = executor.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM platform_super_admins
			WHERE principal_id = $1::uuid
				AND status = 'active'
		)
	`, principalID).Scan(&active)
	return active, err
}

func (r *PlatformAccessRepository) EnsurePlatformSuperAdmin(ctx context.Context, input ports.PlatformSuperAdminBootstrap) (commands.PrincipalID, error) {
	if r == nil || r.adapter == nil {
		return "", ErrPoolClosed
	}
	if err := validatePlatformSuperAdminBootstrap(input); err != nil {
		return "", err
	}

	var principalID commands.PrincipalID
	err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		resolvedID, err := r.upsertBootstrapPrincipal(txCtx, input)
		if err != nil {
			return err
		}
		if err := r.activateSuperAdmin(txCtx, resolvedID, input); err != nil {
			return err
		}
		principalID = resolvedID
		return nil
	})
	return principalID, err
}

func validatePlatformSuperAdminBootstrap(input ports.PlatformSuperAdminBootstrap) error {
	if input.Principal != "" && strings.TrimSpace(input.Email) != "" && strings.TrimSpace(input.Name) != "" && strings.TrimSpace(input.Hash) != "" && !input.Now.IsZero() {
		return nil
	}
	return &RepositoryError{
		Operation: "platform.bootstrap_super_admin",
		Kind:      RepositoryInvalid,
		Err:       errors.New("principal, email, name, password hash, and time are required"),
	}
}

func (r *PlatformAccessRepository) upsertBootstrapPrincipal(ctx context.Context, input ports.PlatformSuperAdminBootstrap) (commands.PrincipalID, error) {
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return "", err
	}

	var principalID commands.PrincipalID
	err = executor.QueryRow(ctx, `
		INSERT INTO principals (
			id, email, display_name, password_hash, status, created_at, updated_at
		)
		VALUES ($1::uuid, lower($2), $3, $4, 'active', $5, $5)
		ON CONFLICT (lower(email)) DO UPDATE
		SET display_name = EXCLUDED.display_name,
			password_hash = EXCLUDED.password_hash,
			status = 'active',
			updated_at = EXCLUDED.updated_at
		RETURNING id::text
	`, input.Principal, input.Email, input.Name, input.Hash, input.Now).Scan(&principalID)
	if err != nil {
		return "", &RepositoryError{Operation: "platform.bootstrap_principal", Kind: RepositoryInvalid, Err: err}
	}
	return principalID, nil
}

func (r *PlatformAccessRepository) activateSuperAdmin(ctx context.Context, principalID commands.PrincipalID, input ports.PlatformSuperAdminBootstrap) error {
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}

	_, err = executor.Exec(ctx, `
		INSERT INTO platform_super_admins (
			principal_id, status, created_at, updated_at, revoked_at
		)
		VALUES ($1::uuid, 'active', $2, $2, NULL)
		ON CONFLICT (principal_id) DO UPDATE
		SET status = 'active',
			updated_at = EXCLUDED.updated_at,
			revoked_at = NULL
	`, principalID, input.Now)
	if err != nil {
		return &RepositoryError{Operation: "platform.bootstrap_super_admin", Kind: RepositoryInvalid, Err: err}
	}
	return nil
}

var _ ports.PlatformAccessRepository = (*PlatformAccessRepository)(nil)
var _ ports.PlatformSuperAdminBootstrapRepository = (*PlatformAccessRepository)(nil)
