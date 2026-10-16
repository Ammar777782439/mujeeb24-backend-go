package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type TeamRepository struct{ adapter *Adapter }

func NewTeamRepository(adapter *Adapter) *TeamRepository { return &TeamRepository{adapter: adapter} }

const teamMemberSelect = `SELECT m.business_id::text, m.principal_id::text, p.email, p.display_name,
	m.role, m.permissions, m.status, m.created_at, m.updated_at
	FROM business_memberships m JOIN principals p ON p.id = m.principal_id`

const teamInvitationReturning = `id::text, business_id::text, email, display_name, role, permissions,
	token_hash, status, invited_by::text, accepted_by::text, expires_at, accepted_at,
	revoked_at, created_at, updated_at`

const qualifiedTeamInvitationSelect = `invitation.id::text, invitation.business_id::text, invitation.email,
	invitation.display_name, invitation.role, invitation.permissions, invitation.token_hash, invitation.status,
	invitation.invited_by::text, invitation.accepted_by::text, invitation.expires_at, invitation.accepted_at,
	invitation.revoked_at, invitation.created_at, invitation.updated_at`

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
	rows, err := executor.Query(ctx, teamMemberSelect+` WHERE m.business_id = $1::uuid AND m.principal_id::text > $2 ORDER BY m.principal_id ASC LIMIT $3`, businessID, decoded, limit+1)
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

func (r *TeamRepository) ListInvitations(ctx context.Context, businessID string, limit int, cursor string, now time.Time) (ports.TeamInvitationPage, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationPage{}, ErrPoolClosed
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	decoded, err := decodeTeamCursor(cursor)
	if err != nil {
		return ports.TeamInvitationPage{}, err
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamInvitationPage{}, err
	}
	if _, err := executor.Exec(ctx, `UPDATE team_invitations SET status = 'expired', updated_at = $2 WHERE business_id = $1::uuid AND status = 'pending' AND expires_at <= $2`, businessID, now); err != nil {
		return ports.TeamInvitationPage{}, classifyRepositoryWriteError("team.expire_invitations", err)
	}
	rows, err := executor.Query(ctx, `SELECT `+teamInvitationReturning+` FROM team_invitations WHERE business_id = $1::uuid AND id::text > $2 ORDER BY id ASC LIMIT $3`, businessID, decoded, limit+1)
	if err != nil {
		return ports.TeamInvitationPage{}, classifyRepositoryWriteError("team.list_invitations", err)
	}
	defer rows.Close()
	items := make([]ports.TeamInvitationRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanTeamInvitation(rows)
		if scanErr != nil {
			return ports.TeamInvitationPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.TeamInvitationPage{}, classifyRepositoryWriteError("team.list_invitations", err)
	}
	page := ports.TeamInvitationPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeTeamCursor(page.Items[len(page.Items)-1].ID)
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
	return scanTeamMember(executor.QueryRow(ctx, teamMemberSelect+` WHERE m.business_id = $1::uuid AND m.principal_id = $2::uuid AND m.status = 'active' AND p.status = 'active'`, businessID, principalID))
}

func (r *TeamRepository) CreateInvitation(ctx context.Context, create ports.TeamInvitationCreate) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	permissions, err := json.Marshal(create.Permissions)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	var record ports.TeamInvitationRecord
	err = r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, executorErr := r.adapter.Executor(txCtx)
		if executorErr != nil {
			return executorErr
		}
		if _, expireErr := executor.Exec(txCtx, `UPDATE team_invitations SET status = 'expired', updated_at = $3 WHERE business_id = $1::uuid AND lower(email) = lower($2) AND status = 'pending' AND expires_at <= $3`, create.BusinessID, create.Email, create.CreatedAt); expireErr != nil {
			return classifyRepositoryWriteError("team.expire_invitation", expireErr)
		}
		row := executor.QueryRow(txCtx, `INSERT INTO team_invitations (
			id, business_id, email, display_name, role, permissions, token_hash,
			status, invited_by, expires_at, created_at, updated_at
		) SELECT $1::uuid, business.id, lower($3), $4, $5, $6::jsonb, $7,
			'pending', $8::uuid, $9, $10, $10
		FROM businesses business WHERE business.id = $2::uuid AND business.status = 'active'
		RETURNING `+teamInvitationReturning,
			create.ID, create.BusinessID, create.Email, create.DisplayName, create.Role, permissions,
			create.TokenHash, create.InvitedBy, create.ExpiresAt, create.CreatedAt)
		var scanErr error
		record, scanErr = scanTeamInvitation(row)
		return scanErr
	})
	if err != nil {
		return ports.TeamInvitationRecord{}, teamRepositoryWriteError("team.create_invitation", err)
	}
	return record, nil
}

func (r *TeamRepository) InspectInvitation(ctx context.Context, tokenHash string, now time.Time) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	if _, err := executor.Exec(ctx, `UPDATE team_invitations SET status = 'expired', updated_at = $2 WHERE token_hash = $1 AND status = 'pending' AND expires_at <= $2`, tokenHash, now); err != nil {
		return ports.TeamInvitationRecord{}, classifyRepositoryWriteError("team.inspect_expire", err)
	}
	return scanTeamInvitationMetadata(executor.QueryRow(ctx, `SELECT `+qualifiedTeamInvitationSelect+`, business.name,
		CASE WHEN EXISTS (SELECT 1 FROM principals p WHERE lower(p.email) = lower(invitation.email)) THEN 'existing' ELSE 'new' END
		FROM team_invitations invitation JOIN businesses business ON business.id = invitation.business_id
		WHERE invitation.token_hash = $1`, tokenHash))
}

func (r *TeamRepository) AcceptInvitation(ctx context.Context, acceptance ports.TeamInvitationAcceptance) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	var record ports.TeamInvitationRecord
	err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, executorErr := r.adapter.Executor(txCtx)
		if executorErr != nil {
			return executorErr
		}
		if _, updateErr := executor.Exec(txCtx, `UPDATE team_invitations SET status = 'expired', updated_at = $2 WHERE token_hash = $1 AND status = 'pending' AND expires_at <= $2`, acceptance.TokenHash, acceptance.AcceptedAt); updateErr != nil {
			return classifyRepositoryWriteError("team.accept_expire", updateErr)
		}
		var businessID string
		if queryErr := executor.QueryRow(txCtx, `SELECT business_id::text FROM team_invitations WHERE token_hash=$1 AND status='pending' AND expires_at>$2 FOR UPDATE`, acceptance.TokenHash, acceptance.AcceptedAt).Scan(&businessID); queryErr != nil {
			return &RepositoryError{Operation: "team.accept_invitation", Kind: RepositoryConflict, Err: errors.New("invitation is not pending")}
		}
		if guardErr := r.ensureSeatAvailable(txCtx, executor, businessID, acceptance.PrincipalID, acceptance.AcceptedAt); guardErr != nil {
			return guardErr
		}
		var scanErr error
		record, scanErr = scanTeamInvitation(executor.QueryRow(txCtx, `WITH accepted_invitation AS (
		UPDATE team_invitations invitation
		SET status = 'accepted', accepted_by = $2::uuid, accepted_at = $3, updated_at = $3
		FROM principals principal, businesses business
		WHERE invitation.token_hash = $1 AND invitation.status = 'pending' AND invitation.expires_at > $3
			AND principal.id = $2::uuid AND principal.status = 'active'
			AND lower(principal.email) = lower(invitation.email)
			AND business.id = invitation.business_id AND business.status = 'active'
		RETURNING invitation.*
	), activated_membership AS (
		INSERT INTO business_memberships (business_id, principal_id, role, permissions, status, created_at, updated_at)
		SELECT business_id, accepted_by, role, permissions, 'active', $3, $3 FROM accepted_invitation
		ON CONFLICT (business_id, principal_id) DO UPDATE SET role = EXCLUDED.role, permissions = EXCLUDED.permissions,
			status = 'active', updated_at = EXCLUDED.updated_at RETURNING business_id
	)
	SELECT `+teamInvitationReturning+` FROM accepted_invitation
	WHERE EXISTS (SELECT 1 FROM activated_membership)`, acceptance.TokenHash, acceptance.PrincipalID, acceptance.AcceptedAt))
		return scanErr
	})
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	return record, nil
}

func (r *TeamRepository) AcceptInvitationNewAccount(ctx context.Context, acceptance ports.TeamInvitationNewAccountAcceptance) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	if _, err := executor.Exec(ctx, `UPDATE team_invitations SET status = 'expired', updated_at = $2 WHERE token_hash = $1 AND status = 'pending' AND expires_at <= $2`, acceptance.TokenHash, acceptance.AcceptedAt); err != nil {
		return ports.TeamInvitationRecord{}, classifyRepositoryWriteError("team.accept_new_expire", err)
	}
	var accepted ports.TeamInvitationRecord
	err = r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, executorErr := r.adapter.Executor(txCtx)
		if executorErr != nil {
			return executorErr
		}
		invitation, scanErr := scanTeamInvitation(executor.QueryRow(txCtx, `SELECT `+teamInvitationReturning+` FROM team_invitations WHERE token_hash = $1 FOR UPDATE`, acceptance.TokenHash))
		if scanErr != nil {
			return scanErr
		}
		if invitation.Status != "pending" || !invitation.ExpiresAt.After(acceptance.AcceptedAt) {
			return &RepositoryError{Operation: "team.accept_new", Kind: RepositoryConflict, Err: errors.New("invitation is not pending")}
		}
		var businessStatus string
		if err := executor.QueryRow(txCtx, `SELECT status FROM businesses WHERE id = $1::uuid FOR SHARE`, invitation.BusinessID).Scan(&businessStatus); err != nil || businessStatus != "active" {
			return &RepositoryError{Operation: "team.accept_new", Kind: RepositoryConflict, Err: errors.New("business is not active")}
		}
		if guardErr := r.ensureSeatAvailable(txCtx, executor, invitation.BusinessID, acceptance.PrincipalID, acceptance.AcceptedAt); guardErr != nil {
			return guardErr
		}
		var existingID string
		err := executor.QueryRow(txCtx, `SELECT id::text FROM principals WHERE lower(email) = lower($1)`, invitation.Email).Scan(&existingID)
		if err == nil {
			return &RepositoryError{Operation: "team.accept_new", Kind: RepositoryConflict, Err: errors.New("principal already exists")}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return classifyRepositoryWriteError("team.accept_new.lookup_principal", err)
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
			VALUES ($1::uuid, lower($2), $3, $4, 'active', $5, $5)`, acceptance.PrincipalID, invitation.Email, acceptance.DisplayName, acceptance.PasswordHash, acceptance.AcceptedAt); err != nil {
			return classifyRepositoryWriteError("team.accept_new.create_principal", err)
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO business_memberships (business_id, principal_id, role, permissions, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, 'active', $5, $5)`, invitation.BusinessID, acceptance.PrincipalID, invitation.Role, mustJSON(invitation.Permissions), acceptance.AcceptedAt); err != nil {
			return classifyRepositoryWriteError("team.accept_new.create_membership", err)
		}
		accepted, scanErr = scanTeamInvitation(executor.QueryRow(txCtx, `UPDATE team_invitations SET status = 'accepted', accepted_by = $2::uuid,
			accepted_at = $3, updated_at = $3 WHERE id = $1::uuid AND status = 'pending' RETURNING `+teamInvitationReturning,
			invitation.ID, acceptance.PrincipalID, acceptance.AcceptedAt))
		return scanErr
	})
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	return accepted, nil
}

func (r *TeamRepository) ensureSeatAvailable(ctx context.Context, executor SQLExecutor, businessID, principalID string, now time.Time) error {
	if _, err := executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, businessID); err != nil {
		return &RepositoryError{Operation: "team.seat_entitlement.lock", Kind: RepositoryInvalid, Err: err}
	}
	var seatLimit int
	if err := executor.QueryRow(ctx,
		`SELECT seat_limit + COALESCE((SELECT sum(seat_delta) FROM subscription_addons WHERE subscription_id=subscriptions.id AND status='ACTIVE'),0) FROM subscriptions
		 WHERE business_id=$1::uuid AND status='ACTIVE' AND period_start <= $2 AND period_end > $2`,
		businessID, now).Scan(&seatLimit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: "team.seat_entitlement", Kind: RepositoryConflict, Err: errors.New("an active subscription is required to add a team seat")}
		}
		return &RepositoryError{Operation: "team.seat_entitlement", Kind: RepositoryInvalid, Err: err}
	}
	var activeSeats int
	if err := executor.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM business_memberships WHERE business_id=$1::uuid AND status='active' AND principal_id<>$2::uuid`,
		businessID, principalID).Scan(&activeSeats); err != nil {
		return &RepositoryError{Operation: "team.seat_entitlement", Kind: RepositoryInvalid, Err: err}
	}
	if activeSeats >= seatLimit {
		return &RepositoryError{Operation: "team.seat_entitlement", Kind: RepositoryConflict, Err: fmt.Errorf("active seat limit %d has been reached", seatLimit)}
	}
	return nil
}

func (r *TeamRepository) RevokeInvitation(ctx context.Context, revocation ports.TeamInvitationRevocation) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamInvitationRecord{}, err
	}
	if _, err := executor.Exec(ctx, `UPDATE team_invitations SET status = 'expired', updated_at = $3 WHERE business_id = $1::uuid AND id = $2::uuid AND status = 'pending' AND expires_at <= $3`, revocation.BusinessID, revocation.InvitationID, revocation.RevokedAt); err != nil {
		return ports.TeamInvitationRecord{}, classifyRepositoryWriteError("team.revoke_invitation_expire", err)
	}
	return scanTeamInvitation(executor.QueryRow(ctx, `UPDATE team_invitations SET status = 'revoked', revoked_at = $3, updated_at = $3
		WHERE business_id = $1::uuid AND id = $2::uuid AND status = 'pending' AND expires_at > $3
		RETURNING `+teamInvitationReturning, revocation.BusinessID, revocation.InvitationID, revocation.RevokedAt))
}

func (r *TeamRepository) ResendInvitation(ctx context.Context, resend ports.TeamInvitationResend) (ports.TeamInvitationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamInvitationRecord{}, ErrPoolClosed
	}
	var created ports.TeamInvitationRecord
	err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, executorErr := r.adapter.Executor(txCtx)
		if executorErr != nil {
			return executorErr
		}
		previous, scanErr := scanTeamInvitation(executor.QueryRow(txCtx, `SELECT `+teamInvitationReturning+` FROM team_invitations WHERE business_id = $1::uuid AND id = $2::uuid FOR UPDATE`, resend.BusinessID, resend.InvitationID))
		if scanErr != nil {
			return scanErr
		}
		if previous.Status == "accepted" {
			return &RepositoryError{Operation: "team.resend_invitation", Kind: RepositoryConflict, Err: errors.New("accepted invitation cannot be resent")}
		}
		if previous.Status == "pending" {
			status := "revoked"
			if !previous.ExpiresAt.After(resend.CreatedAt) {
				status = "expired"
			}
			_, err := executor.Exec(txCtx, `UPDATE team_invitations SET status = $3, revoked_at = CASE WHEN $3 = 'revoked' THEN $4 ELSE NULL END, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid`, resend.BusinessID, resend.InvitationID, status, resend.CreatedAt)
			if err != nil {
				return classifyRepositoryWriteError("team.resend_invitation.close_old", err)
			}
		}
		created, scanErr = scanTeamInvitation(executor.QueryRow(txCtx, `INSERT INTO team_invitations (
			id, business_id, email, display_name, role, permissions, token_hash, status, invited_by,
			expires_at, created_at, updated_at
		) VALUES ($1::uuid, $2::uuid, lower($3), $4, $5, $6::jsonb, $7, 'pending', $8::uuid, $9, $10, $10)
		RETURNING `+teamInvitationReturning, resend.NewInvitationID, resend.BusinessID, previous.Email,
			previous.DisplayName, previous.Role, mustJSON(previous.Permissions), resend.TokenHash,
			resend.InvitedBy, resend.ExpiresAt, resend.CreatedAt))
		return scanErr
	})
	if err != nil {
		return ports.TeamInvitationRecord{}, teamRepositoryWriteError("team.resend_invitation", err)
	}
	return created, nil
}

func (r *TeamRepository) UpdateMemberRole(ctx context.Context, change ports.TeamMemberRoleChange) (ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}
	return scanTeamMember(executor.QueryRow(ctx, `WITH updated_member AS (
		UPDATE business_memberships SET role = $3, updated_at = $4
		WHERE business_id = $1::uuid AND principal_id = $2::uuid AND status = 'active' AND role <> 'owner'
		RETURNING business_id, principal_id, role, permissions, status, created_at, updated_at
	) SELECT member.business_id::text, member.principal_id::text, principal.email, principal.display_name,
		member.role, member.permissions, member.status, member.created_at, member.updated_at
	FROM updated_member member JOIN principals principal ON principal.id = member.principal_id`,
		change.BusinessID, change.PrincipalID, change.Role, change.UpdatedAt))
}

func (r *TeamRepository) UpdateMemberAccess(ctx context.Context, change ports.TeamMemberAccessChange) (ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ErrPoolClosed
	}
	permissions, err := json.Marshal(change.Permissions)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}
	return scanTeamMember(executor.QueryRow(ctx, `WITH updated_member AS (
		UPDATE business_memberships SET role = $3, permissions = $4::jsonb, updated_at = $5
		WHERE business_id = $1::uuid AND principal_id = $2::uuid AND status = 'active' AND role <> 'owner'
		RETURNING business_id, principal_id, role, permissions, status, created_at, updated_at
	) SELECT member.business_id::text, member.principal_id::text, principal.email, principal.display_name,
		member.role, member.permissions, member.status, member.created_at, member.updated_at
	FROM updated_member member JOIN principals principal ON principal.id = member.principal_id`,
		change.BusinessID, change.PrincipalID, change.Role, permissions, change.UpdatedAt))
}

func (r *TeamRepository) RevokeMember(ctx context.Context, revocation ports.TeamMemberRevocation) (ports.TeamMemberRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.TeamMemberRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.TeamMemberRecord{}, err
	}
	record, err := scanTeamMember(executor.QueryRow(ctx, `WITH active_owners AS MATERIALIZED (
		SELECT principal_id FROM business_memberships WHERE business_id = $1::uuid AND role = 'owner' AND status = 'active' FOR UPDATE
	), revoked_member AS (
		UPDATE business_memberships membership SET status = 'revoked', updated_at = $3
		WHERE membership.business_id = $1::uuid AND membership.principal_id = $2::uuid AND membership.status = 'active'
			AND (membership.role <> 'owner' OR (SELECT count(*) FROM active_owners) > 1)
		RETURNING membership.business_id, membership.principal_id, membership.role, membership.permissions,
			membership.status, membership.created_at, membership.updated_at
	) SELECT member.business_id::text, member.principal_id::text, principal.email, principal.display_name,
		member.role, member.permissions, member.status, member.created_at, member.updated_at
	FROM revoked_member member JOIN principals principal ON principal.id = member.principal_id`,
		revocation.BusinessID, revocation.PrincipalID, revocation.RevokedAt))
	if IsRepositoryKind(err, RepositoryNotFound) {
		return ports.TeamMemberRecord{}, &RepositoryError{Operation: "team.revoke_member", Kind: RepositoryConflict, Err: err}
	}
	return record, err
}

func mustJSON(values []string) []byte {
	encoded, _ := json.Marshal(values)
	return encoded
}

func teamRepositoryWriteError(operation string, err error) error {
	var repositoryErr *RepositoryError
	if errors.As(err, &repositoryErr) {
		return err
	}
	return classifyRepositoryWriteError(operation, err)
}

var _ ports.TeamRepository = (*TeamRepository)(nil)
