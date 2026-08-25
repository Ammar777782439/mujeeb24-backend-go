package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AuthenticationRepository struct{ adapter *Adapter }

func NewAuthenticationRepository(adapter *Adapter) *AuthenticationRepository {
	return &AuthenticationRepository{adapter: adapter}
}

func (r *AuthenticationRepository) GetByEmail(ctx context.Context, email string) (ports.PrincipalRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PrincipalRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(email) == "" {
		return ports.PrincipalRecord{}, &RepositoryError{Operation: "auth.principal_by_email", Kind: RepositoryInvalid, Err: errors.New("email is required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PrincipalRecord{}, err
	}
	const query = `SELECT id::text, email, display_name, password_hash, status FROM principals WHERE lower(email) = lower($1)`
	return scanPrincipal(executor.QueryRow(ctx, query, email), "auth.principal_by_email")
}

func (r *AuthenticationRepository) GetByID(ctx context.Context, principalID commands.PrincipalID) (ports.PrincipalRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PrincipalRecord{}, ErrPoolClosed
	}
	if principalID == "" {
		return ports.PrincipalRecord{}, &RepositoryError{Operation: "auth.principal_by_id", Kind: RepositoryInvalid, Err: errors.New("principal id is required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PrincipalRecord{}, err
	}
	const query = `SELECT id::text, email, display_name, password_hash, status FROM principals WHERE id = $1::uuid`
	return scanPrincipal(executor.QueryRow(ctx, query, principalID), "auth.principal_by_id")
}

func (r *AuthenticationRepository) ListActiveMemberships(ctx context.Context, principalID commands.PrincipalID, limit int, cursor string) ([]ports.BusinessMembershipRecord, string, bool, error) {
	if r == nil || r.adapter == nil {
		return nil, "", false, ErrPoolClosed
	}
	if principalID == "" {
		return nil, "", false, &RepositoryError{Operation: "auth.list_memberships", Kind: RepositoryInvalid, Err: errors.New("principal id is required")}
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	decodedCursor, err := decodeMembershipCursor(cursor)
	if err != nil {
		return nil, "", false, err
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, "", false, err
	}
	const query = `SELECT m.business_id::text, m.principal_id::text, b.id::text, b.name, b.slug, b.status, b.vertical_type, b.timezone, b.default_currency, b.locale, b.created_at, b.updated_at, m.role, m.permissions, m.status
		FROM business_memberships m JOIN businesses b ON b.id = m.business_id
		WHERE m.principal_id = $1::uuid AND m.status = 'active' AND b.id::text > $2
		ORDER BY b.id ASC LIMIT $3`
	rows, err := executor.Query(ctx, query, principalID, decodedCursor, limit+1)
	if err != nil {
		return nil, "", false, &RepositoryError{Operation: "auth.list_memberships", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.BusinessMembershipRecord, 0, limit)
	for rows.Next() {
		item, err := scanMembership(rows)
		if err != nil {
			return nil, "", false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, &RepositoryError{Operation: "auth.list_memberships", Kind: RepositoryInvalid, Err: err}
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	next := ""
	if hasMore {
		next = encodeMembershipCursor(string(items[len(items)-1].BusinessID))
	}
	return items, next, hasMore, nil
}

func encodeMembershipCursor(businessID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(businessID))
}

func decodeMembershipCursor(cursor string) (string, error) {
	if strings.TrimSpace(cursor) == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", &RepositoryError{Operation: "auth.list_memberships", Kind: RepositoryInvalid, Err: errors.New("invalid membership cursor")}
	}
	value := string(decoded)
	if _, err := uuid.Parse(value); err != nil {
		return "", &RepositoryError{Operation: "auth.list_memberships", Kind: RepositoryInvalid, Err: errors.New("invalid membership cursor")}
	}
	return value, nil
}

func (r *AuthenticationRepository) ResolveActiveMembership(ctx context.Context, principalID commands.PrincipalID, businessID commands.BusinessID) (ports.BusinessMembershipRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.BusinessMembershipRecord{}, ErrPoolClosed
	}
	if principalID == "" || businessID == "" {
		return ports.BusinessMembershipRecord{}, &RepositoryError{Operation: "auth.resolve_membership", Kind: RepositoryInvalid, Err: errors.New("principal and business ids are required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.BusinessMembershipRecord{}, err
	}
	const query = `SELECT m.business_id::text, m.principal_id::text, b.id::text, b.name, b.slug, b.status, b.vertical_type, b.timezone, b.default_currency, b.locale, b.created_at, b.updated_at, m.role, m.permissions, m.status
		FROM business_memberships m JOIN businesses b ON b.id = m.business_id
		WHERE m.principal_id = $1::uuid AND m.business_id = $2::uuid AND m.status = 'active'`
	return scanMembership(executor.QueryRow(ctx, query, principalID, businessID))
}

func (r *AuthenticationRepository) EnsurePrincipalAndMembership(ctx context.Context, principal ports.PrincipalRecord, businessID commands.BusinessID, role string, permissions []string, now time.Time) (ports.PrincipalRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PrincipalRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(principal.Email) == "" || strings.TrimSpace(principal.DisplayName) == "" || strings.TrimSpace(principal.PasswordHash) == "" || businessID == "" {
		return ports.PrincipalRecord{}, &RepositoryError{Operation: "auth.bootstrap", Kind: RepositoryInvalid, Err: errors.New("principal identity, password hash, and business id are required")}
	}
	if principal.ID == "" {
		principal.ID = commands.PrincipalID(uuid.NewString())
	}
	if principal.Status == "" {
		principal.Status = "active"
	}
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return ports.PrincipalRecord{}, err
	}
	var output ports.PrincipalRecord
	err = r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := r.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		const upsertPrincipal = `INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
			VALUES ($1::uuid, lower($2), $3, $4, $5, $6, $6)
			ON CONFLICT (lower(email)) DO UPDATE SET display_name = EXCLUDED.display_name, password_hash = EXCLUDED.password_hash, status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
			RETURNING id::text, email, display_name, password_hash, status`
		output, err = scanPrincipal(executor.QueryRow(txCtx, upsertPrincipal, principal.ID, principal.Email, principal.DisplayName, principal.PasswordHash, principal.Status, now), "auth.bootstrap")
		if err != nil {
			return err
		}
		const upsertMembership = `INSERT INTO business_memberships (business_id, principal_id, role, permissions, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, 'active', $5, $5)
			ON CONFLICT (business_id, principal_id) DO UPDATE SET role = EXCLUDED.role, permissions = EXCLUDED.permissions, status = 'active', updated_at = EXCLUDED.updated_at`
		_, err = executor.Exec(txCtx, upsertMembership, businessID, output.ID, role, permissionsJSON, now)
		return err
	})
	return output, err
}

func (r *AuthenticationRepository) Create(ctx context.Context, session ports.RefreshSessionRecord, now time.Time) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if session.ID == "" || session.PrincipalID == "" || session.TokenHash == "" || !session.ExpiresAt.After(now) {
		return &RepositoryError{Operation: "auth.create_refresh", Kind: RepositoryInvalid, Err: errors.New("invalid refresh session")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	_, err = executor.Exec(ctx, `INSERT INTO refresh_sessions (id, principal_id, token_hash, expires_at, created_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5)`, session.ID, session.PrincipalID, session.TokenHash, session.ExpiresAt, now)
	if err != nil {
		return &RepositoryError{Operation: "auth.create_refresh", Kind: RepositoryConflict, Err: err}
	}
	return nil
}

func (r *AuthenticationRepository) Consume(ctx context.Context, tokenHash string, now time.Time) (ports.RefreshSessionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.RefreshSessionRecord{}, ErrPoolClosed
	}
	if tokenHash == "" {
		return ports.RefreshSessionRecord{}, &RepositoryError{Operation: "auth.consume_refresh", Kind: RepositoryInvalid, Err: errors.New("token hash is required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.RefreshSessionRecord{}, err
	}
	const query = `UPDATE refresh_sessions SET used_at = $2, revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL AND used_at IS NULL AND expires_at > $2 RETURNING id::text, principal_id::text, token_hash, expires_at, revoked_at, used_at`
	var session ports.RefreshSessionRecord
	if err := executor.QueryRow(ctx, query, tokenHash, now).Scan(&session.ID, &session.PrincipalID, &session.TokenHash, &session.ExpiresAt, &session.RevokedAt, &session.UsedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.RefreshSessionRecord{}, &RepositoryError{Operation: "auth.consume_refresh", Kind: RepositoryNotFound, Err: err}
		}
		return ports.RefreshSessionRecord{}, &RepositoryError{Operation: "auth.consume_refresh", Kind: RepositoryInvalid, Err: err}
	}
	return session, nil
}

func (r *AuthenticationRepository) Revoke(ctx context.Context, principalID commands.PrincipalID, sessionID string, now time.Time) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if principalID == "" || sessionID == "" {
		return &RepositoryError{Operation: "auth.revoke_refresh", Kind: RepositoryInvalid, Err: errors.New("principal and session id are required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	command, err := executor.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = $3 WHERE id = $1::uuid AND principal_id = $2::uuid AND revoked_at IS NULL`, sessionID, principalID, now)
	if err != nil {
		return &RepositoryError{Operation: "auth.revoke_refresh", Kind: RepositoryInvalid, Err: err}
	}
	if command.RowsAffected() == 0 {
		return &RepositoryError{Operation: "auth.revoke_refresh", Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanPrincipal(row rowScanner, operation string) (ports.PrincipalRecord, error) {
	var record ports.PrincipalRecord
	if err := row.Scan(&record.ID, &record.Email, &record.DisplayName, &record.PasswordHash, &record.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PrincipalRecord{}, &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return ports.PrincipalRecord{}, &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}
func scanMembership(row rowScanner) (ports.BusinessMembershipRecord, error) {
	var record ports.BusinessMembershipRecord
	var permissions []byte
	if err := row.Scan(&record.BusinessID, &record.PrincipalID, &record.Business.ID, &record.Business.Name, &record.Business.Slug, &record.Business.Status, &record.Business.VerticalType, &record.Business.Timezone, &record.Business.DefaultCurrency, &record.Business.Locale, &record.Business.CreatedAt, &record.Business.UpdatedAt, &record.Role, &permissions, &record.Status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.BusinessMembershipRecord{}, &RepositoryError{Operation: "auth.membership", Kind: RepositoryNotFound, Err: err}
		}
		return ports.BusinessMembershipRecord{}, &RepositoryError{Operation: "auth.membership", Kind: RepositoryInvalid, Err: err}
	}
	if err := json.Unmarshal(permissions, &record.Permissions); err != nil {
		return ports.BusinessMembershipRecord{}, &RepositoryError{Operation: "auth.membership", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

var _ ports.PrincipalRepository = (*AuthenticationRepository)(nil)
var _ ports.PrincipalBootstrapRepository = (*AuthenticationRepository)(nil)
var _ ports.RefreshSessionRepository = (*AuthenticationRepository)(nil)
