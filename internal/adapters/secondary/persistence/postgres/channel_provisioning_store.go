package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ChannelProvisioningStore struct{ adapter *Adapter }

func NewChannelProvisioningStore(adapter *Adapter) *ChannelProvisioningStore {
	return &ChannelProvisioningStore{adapter: adapter}
}

func (r *ChannelProvisioningStore) CreateOrGet(ctx context.Context, session ports.ChannelProvisioningSession) (ports.ChannelProvisioningSession, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelProvisioningSession{}, ErrPoolClosed
	}
	if strings.TrimSpace(session.ID) == "" {
		session.ID = uuid.NewString()
	}
	if strings.TrimSpace(session.BusinessID) == "" || strings.TrimSpace(session.IdempotencyKey) == "" || strings.TrimSpace(session.ProviderRef) == "" || strings.TrimSpace(session.Channel) == "" || strings.TrimSpace(session.DisplayName) == "" {
		return ports.ChannelProvisioningSession{}, invalidRepositoryInput("channel_provisioning.create_or_get", "business, idempotency, provider, channel, and display name are required")
	}
	now := time.Now().UTC()
	var result ports.ChannelProvisioningSession
	err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		txExecutor, err := r.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		// Serialize creation of an in-flight OAuth session against the
		// last-channel brand cleanup for this tenant (not all merchants).
		var lockedBusinessID string
		if err := txExecutor.QueryRow(txCtx, `SELECT id::text FROM businesses WHERE id=$1::uuid FOR UPDATE`, session.BusinessID).Scan(&lockedBusinessID); err != nil {
			return classifyRepositoryWriteError("channel_provisioning.lock_business", err)
		}
		// Reconcile stale provisioning sessions before creating the new one.
		// Pending/provisioning sessions are always superseded because they are
		// replaced by the new idempotency key. A connected session is only
		// considered active while its linked ChannelConnection is still active.
		// This makes disconnect -> reconnect self-healing even when the disconnect
		// path did not update the historical provisioning session.
		const supersedeQuery = `
			UPDATE channel_provisioning_sessions AS s
			SET status = 'failed', failure_code = 'superseded', updated_at = $1
			WHERE s.business_id = $2::uuid
			  AND s.channel = $3
			  AND s.idempotency_key <> $4
			  AND (
				  s.status IN ('pending_authorization', 'provisioning')
				  OR (
					  s.status = 'connected'
					  AND NOT EXISTS (
						  SELECT 1
						  FROM channel_connections AS c
						  WHERE c.business_id = s.business_id
							AND c.id = s.channel_connection_id
							AND c.status = 'active'
					  )
				  )
			  )`
		if _, err := txExecutor.Exec(txCtx, supersedeQuery, now, session.BusinessID, session.Channel, session.IdempotencyKey); err != nil {
			return err
		}
		const query = `
			INSERT INTO channel_provisioning_sessions (id, business_id, idempotency_key, provider_ref, channel, display_name, status, oauth_state, authorization_url, provider_account_ref, provider_connection_ref, channel_connection_id, failure_code, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, '')::uuid, NULLIF($13, ''), $14, $14)
			ON CONFLICT (business_id, idempotency_key) DO UPDATE SET updated_at = channel_provisioning_sessions.updated_at
			RETURNING id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, ''), created_at, updated_at`
		res, err := scanProvisioningSession(txExecutor.QueryRow(txCtx, query, session.ID, session.BusinessID, session.IdempotencyKey, session.ProviderRef, session.Channel, session.DisplayName, session.Status, session.OAuthState, session.AuthorizationURL, session.ProviderAccountRef, session.ProviderConnectionRef, session.ChannelConnectionID, session.FailureCode, now), "channel_provisioning.create_or_get")
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	return result, nil
}

func (r *ChannelProvisioningStore) GetByID(ctx context.Context, businessID, id string) (ports.ChannelProvisioningSession, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelProvisioningSession{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(id) == "" {
		return ports.ChannelProvisioningSession{}, invalidRepositoryInput("channel_provisioning.get_by_id", "business and id are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	const query = `SELECT id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, ''), created_at, updated_at FROM channel_provisioning_sessions WHERE business_id = $1::uuid AND id = $2::uuid`
	return scanProvisioningSession(executor.QueryRow(ctx, query, businessID, id), "channel_provisioning.get_by_id")
}

func (r *ChannelProvisioningStore) GetByOAuthState(ctx context.Context, state string) (ports.ChannelProvisioningSession, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelProvisioningSession{}, ErrPoolClosed
	}
	if strings.TrimSpace(state) == "" {
		return ports.ChannelProvisioningSession{}, invalidRepositoryInput("channel_provisioning.get_by_oauth_state", "oauth state is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	const query = `SELECT id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, ''), created_at, updated_at FROM channel_provisioning_sessions WHERE oauth_state = $1 ORDER BY updated_at DESC LIMIT 1`
	return scanProvisioningSession(executor.QueryRow(ctx, query, state), "channel_provisioning.get_by_oauth_state")
}

// SupersedeConnectedByChannelConnection marks the provisioning session that produced
// a connection as superseded before a replacement OAuth flow starts. The old
// session must leave the active-session partial index so a new provisioning
// session for the same channel can be created without weakening idempotency.
func (r *ChannelProvisioningStore) SupersedeConnectedByChannelConnection(ctx context.Context, businessID, channelConnectionID string) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	businessID = strings.TrimSpace(businessID)
	channelConnectionID = strings.TrimSpace(channelConnectionID)
	if businessID == "" || channelConnectionID == "" {
		return invalidRepositoryInput("channel_provisioning.supersede_connected", "business and channel connection ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	const query = `
		UPDATE channel_provisioning_sessions
		SET status = 'failed',
		    failure_code = 'superseded',
		    updated_at = $3
		WHERE business_id = $1::uuid
		  AND channel_connection_id = $2::uuid
		  AND status = 'connected'`
	_, err = executor.Exec(ctx, query, businessID, channelConnectionID, time.Now().UTC())
	return err
}

func (r *ChannelProvisioningStore) MarkProvisioning(ctx context.Context, businessID, id string, patch ports.ChannelProvisioningPatch) (ports.ChannelProvisioningSession, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelProvisioningSession{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(id) == "" || patch.Status == "" {
		return ports.ChannelProvisioningSession{}, invalidRepositoryInput("channel_provisioning.mark", "business, id, and status are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	now := time.Now().UTC()
	const query = `
		UPDATE channel_provisioning_sessions
		SET status = $3,
		    oauth_state = COALESCE($4, oauth_state),
		    authorization_url = COALESCE($5, authorization_url),
		    provider_account_ref = COALESCE($6, provider_account_ref),
		    provider_connection_ref = COALESCE($7, provider_connection_ref),
		    channel_connection_id = COALESCE($8::uuid, channel_connection_id),
		    failure_code = COALESCE($9, failure_code),
		    updated_at = $10
		WHERE business_id = $1::uuid AND id = $2::uuid
		RETURNING id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, ''), created_at, updated_at`
	return scanProvisioningSession(executor.QueryRow(ctx, query, businessID, id, patch.Status, patch.OAuthState, patch.AuthorizationURL, patch.ProviderAccountRef, patch.ProviderConnectionRef, optionalString(patch.ChannelConnectionID), patch.FailureCode, now), "channel_provisioning.mark")
}

func optionalString(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return *value
}

func scanProvisioningSession(row pgx.Row, operation string) (ports.ChannelProvisioningSession, error) {
	var session ports.ChannelProvisioningSession
	var status string
	if err := row.Scan(&session.ID, &session.BusinessID, &session.IdempotencyKey, &session.ProviderRef, &session.Channel, &session.DisplayName, &status, &session.OAuthState, &session.AuthorizationURL, &session.ProviderAccountRef, &session.ProviderConnectionRef, &session.ChannelConnectionID, &session.FailureCode, &session.CreatedAt, &session.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ChannelProvisioningSession{}, &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return ports.ChannelProvisioningSession{}, &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
	}
	session.Status = ports.ChannelProvisioningStatus(status)
	return session, nil
}

var _ ports.ChannelProvisioningStore = (*ChannelProvisioningStore)(nil)
