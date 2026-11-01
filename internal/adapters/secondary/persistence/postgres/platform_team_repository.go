package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func (r *AuthenticationRepository) CreateOwnerPrincipalAndMembership(ctx context.Context, principal ports.PrincipalRecord, businessID commands.BusinessID, permissions []string, now time.Time) (ports.PrincipalRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PrincipalRecord{}, ErrPoolClosed
	}
	if _, inTx := transactionFromContext(ctx); !inTx {
		return ports.PrincipalRecord{}, ErrTransactionExecutorMissing
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PrincipalRecord{}, err
	}
	encoded, err := json.Marshal(permissions)
	if err != nil {
		return ports.PrincipalRecord{}, err
	}
	// A unique-email conflict is an error. Never change another account's
	// password, profile, status or memberships while creating a business owner.
	_, err = executor.Exec(ctx, `INSERT INTO principals (id,email,display_name,password_hash,status,created_at,updated_at)
 VALUES ($1::uuid,lower($2),$3,$4,'active',$5,$5)`, principal.ID, principal.Email, principal.DisplayName, principal.PasswordHash, now)
	if err != nil {
		return ports.PrincipalRecord{}, classifyRepositoryWriteError("platform_owner.create_principal", err)
	}
	_, err = executor.Exec(ctx, `INSERT INTO business_memberships (business_id,principal_id,role,permissions,status,created_at,updated_at)
 VALUES ($1::uuid,$2::uuid,'owner',$3::jsonb,'active',$4,$4)`, businessID, principal.ID, encoded, now)
	if err != nil {
		return ports.PrincipalRecord{}, classifyRepositoryWriteError("platform_owner.create_membership", err)
	}
	return principal, nil
}

func (r *TeamRepository) UpdateMemberPermissions(ctx context.Context, change ports.TeamMemberPermissionChange) (ports.TeamMemberRecord, ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, ErrPoolClosed
	}
	if _, inTx := transactionFromContext(ctx); !inTx {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, ErrTransactionExecutorMissing
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, err
	}
	// Serialize with lifecycle updates; archived businesses are read-only.
	var status string
	if err := executor.QueryRow(ctx, `SELECT status FROM businesses WHERE id=$1::uuid FOR SHARE`, change.BusinessID).Scan(&status); err != nil {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, classifyRepositoryWriteError("platform_team.business", err)
	}
	if status == "archived" {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, &RepositoryError{Operation: "platform_team.update_permissions", Kind: RepositoryConflict, Err: errors.New("archived business is read-only")}
	}
	before, err := scanTeamMember(executor.QueryRow(ctx, teamMemberSelect+` WHERE m.business_id=$1::uuid AND m.principal_id=$2::uuid AND m.status='active' FOR UPDATE OF m`, change.BusinessID, change.PrincipalID))
	if err != nil {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, err
	}
	encoded, err := json.Marshal(change.Permissions)
	if err != nil {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, err
	}
	_, err = executor.Exec(ctx, `UPDATE business_memberships SET permissions=$3::jsonb,updated_at=$4
 WHERE business_id=$1::uuid AND principal_id=$2::uuid AND status='active'`, change.BusinessID, change.PrincipalID, encoded, change.UpdatedAt)
	if err != nil {
		return ports.TeamMemberRecord{}, ports.TeamMemberRecord{}, classifyRepositoryWriteError("platform_team.update_permissions", err)
	}
	after := before
	after.Permissions = append([]string(nil), change.Permissions...)
	after.UpdatedAt = change.UpdatedAt
	return before, after, nil
}

var _ ports.OwnerPrincipalRepository = (*AuthenticationRepository)(nil)
var _ ports.PlatformTeamRepository = (*TeamRepository)(nil)
