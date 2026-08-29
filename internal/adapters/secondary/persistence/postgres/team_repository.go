package postgres

import (
	"context"
	"encoding/json"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type TeamRepository struct {
	adapter *Adapter
}

func NewTeamRepository(adapter *Adapter) *TeamRepository {
	return &TeamRepository{adapter: adapter}
}

func (r *TeamRepository) ListMembers(ctx context.Context, businessID string, limit int, cursor string) (ports.TeamMemberPage, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberPage{}, ErrPoolClosed
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	decoded, err := decodeTeamCursor(cursor)
	if err != nil {
		return ports.TeamMemberPage{}, err
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberPage{}, err
	}
	rows, err := executor.Query(ctx, `SELECT m.business_id::text, m.principal_id::text, p.email, p.display_name, m.role, m.permissions, m.status, m.created_at, m.updated_at FROM business_memberships m JOIN principals p ON p.id = m.principal_id WHERE m.business_id = $1::uuid AND m.principal_id::text > $2 ORDER BY m.principal_id ASC LIMIT $3`, businessID, decoded, limit+1)
	if err != nil {
		return ports.TeamMemberPage{}, &RepositoryError{Operation: "team.list_members", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.TeamMemberRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanTeamMember(rows)
		if scanErr != nil {
			return ports.TeamMemberPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.TeamMemberPage{}, &RepositoryError{Operation: "team.list_members", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.TeamMemberPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeTeamCursor(page.Items[len(page.Items)-1].PrincipalID)
	}
	return page, nil
}

func (r *TeamRepository) ResolveActiveMember(ctx context.Context, businessID, principalID string) (ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}
	return scanTeamMember(executor.QueryRow(ctx, `SELECT m.business_id::text, m.principal_id::text, p.email, p.display_name, m.role, m.permissions, m.status, m.created_at, m.updated_at FROM business_memberships m JOIN principals p ON p.id = m.principal_id WHERE m.business_id = $1::uuid AND m.principal_id = $2::uuid AND m.status = 'active' AND p.status = 'active'`, businessID, principalID))
}

func (r *TeamRepository) CreateInvitation(ctx context.Context, create ports.TeamInvitationCreate) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	permissions, err := json.Marshal(create.Permissions)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	return scanTeamInvitation(executor.QueryRow(ctx, `
		INSERT INTO team_invitations (
			id, business_id, email, role, permissions, token_hash,
			status, invited_by, expires_at, created_at, updated_at
		)
		VALUES ($1::uuid, $2::uuid, lower($3), $4, $5::jsonb, $6, 'pending', $7::uuid, $8, $9, $10)
		RETURNING id::text, business_id::text, email, role, permissions, token_hash,
			status, invited_by::text, accepted_by::text, expires_at, accepted_at,
			revoked_at, created_at, updated_at
	`, create.ID, create.BusinessID, create.Email, create.Role, permissions, create.TokenHash, create.InvitedBy, create.ExpiresAt, create.CreatedAt, create.UpdatedAt))
}

func (r *TeamRepository) AcceptInvitation(ctx context.Context, acceptance ports.TeamInvitationAcceptance) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}

	return scanTeamInvitation(executor.QueryRow(ctx, `
		WITH accepted_invitation AS (
			UPDATE team_invitations invitation
			SET status = 'accepted',
				accepted_by = $2::uuid,
				accepted_at = $3,
				updated_at = $3
			FROM principals principal
			WHERE invitation.token_hash = $1
				AND invitation.status = 'pending'
				AND invitation.expires_at > $3
				AND principal.id = $2::uuid
				AND principal.status = 'active'
				AND lower(principal.email) = lower(invitation.email)
			RETURNING invitation.id, invitation.business_id, invitation.email, invitation.role,
				invitation.permissions, invitation.token_hash, invitation.status, invitation.invited_by,
				invitation.accepted_by, invitation.expires_at, invitation.accepted_at,
				invitation.revoked_at, invitation.created_at, invitation.updated_at
		), activated_membership AS (
			INSERT INTO business_memberships (
				business_id, principal_id, role, permissions, status, created_at, updated_at
			)
			SELECT business_id, accepted_by, role, permissions, 'active', $3, $3
			FROM accepted_invitation
			ON CONFLICT (business_id, principal_id) DO UPDATE
			SET role = EXCLUDED.role,
				permissions = EXCLUDED.permissions,
				status = 'active',
				updated_at = EXCLUDED.updated_at
			RETURNING business_id
		)
		SELECT id::text, business_id::text, email, role, permissions, token_hash, status,
			invited_by::text, accepted_by::text, expires_at, accepted_at, revoked_at,
			created_at, updated_at
		FROM accepted_invitation
		WHERE EXISTS (SELECT 1 FROM activated_membership)
	`, acceptance.TokenHash, acceptance.PrincipalID, acceptance.AcceptedAt))
}

func (r *TeamRepository) UpdateMemberRole(ctx context.Context, change ports.TeamMemberRoleChange) (ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}

	return scanTeamMember(executor.QueryRow(ctx, `
		WITH updated_member AS (
			UPDATE business_memberships
			SET role = $3,
				updated_at = $4
			WHERE business_id = $1::uuid
				AND principal_id = $2::uuid
				AND status = 'active'
				AND role <> 'owner'
			RETURNING business_id, principal_id, role, permissions, status, created_at, updated_at
		)
		SELECT member.business_id::text, member.principal_id::text, principal.email,
			principal.display_name, member.role, member.permissions, member.status,
			member.created_at, member.updated_at
		FROM updated_member member
		JOIN principals principal ON principal.id = member.principal_id
	`, change.BusinessID, change.PrincipalID, change.Role, change.UpdatedAt))
}

func (r *TeamRepository) RevokeMember(ctx context.Context, revocation ports.TeamMemberRevocation) (ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}

	record, err := scanTeamMember(executor.QueryRow(ctx, `
		WITH active_owners AS MATERIALIZED (
			SELECT principal_id
			FROM business_memberships
			WHERE business_id = $1::uuid
				AND role = 'owner'
				AND status = 'active'
			FOR UPDATE
		), revoked_member AS (
			UPDATE business_memberships membership
			SET status = 'revoked',
				updated_at = $3
			WHERE membership.business_id = $1::uuid
				AND membership.principal_id = $2::uuid
				AND membership.status = 'active'
				AND (membership.role <> 'owner' OR (SELECT count(*) FROM active_owners) > 1)
			RETURNING membership.business_id, membership.principal_id, membership.role,
				membership.permissions, membership.status, membership.created_at, membership.updated_at
		)
		SELECT member.business_id::text, member.principal_id::text, principal.email,
			principal.display_name, member.role, member.permissions, member.status,
			member.created_at, member.updated_at
		FROM revoked_member member
		JOIN principals principal ON principal.id = member.principal_id
	`, revocation.BusinessID, revocation.PrincipalID, revocation.RevokedAt))
	if IsRepositoryKind(err, RepositoryNotFound) {
		return ports.TeamMemberRecord{}, &RepositoryError{
			Operation: "team.revoke_member",
			Kind:      RepositoryConflict,
			Err:       err,
		}
	}
	return record, err
}

var _ ports.TeamRepository = (*TeamRepository)(nil)
