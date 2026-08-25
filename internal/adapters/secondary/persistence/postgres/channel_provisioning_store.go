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
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelProvisioningSession{}, err
	}
	const query = `
		INSERT INTO channel_provisioning_sessions (id, business_id, idempotency_key, provider_ref, channel, display_name, status, oauth_state, authorization_url, provider_account_ref, provider_connection_ref, chatwoot_account_id, chatwoot_inbox_id, channel_connection_id, failure_code, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), NULLIF($14, '')::uuid, NULLIF($15, ''), $16, $16)
		ON CONFLICT (business_id, idempotency_key) DO UPDATE SET updated_at = channel_provisioning_sessions.updated_at
		RETURNING id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(chatwoot_account_id, ''), COALESCE(chatwoot_inbox_id, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, '')`
	return scanProvisioningSession(executor.QueryRow(ctx, query, session.ID, session.BusinessID, session.IdempotencyKey, session.ProviderRef, session.Channel, session.DisplayName, session.Status, session.OAuthState, session.AuthorizationURL, session.ProviderAccountRef, session.ProviderConnectionRef, session.ChatwootAccountID, session.ChatwootInboxID, session.ChannelConnectionID, session.FailureCode, now), "channel_provisioning.create_or_get")
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
	const query = `SELECT id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(chatwoot_account_id, ''), COALESCE(chatwoot_inbox_id, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, '') FROM channel_provisioning_sessions WHERE business_id = $1::uuid AND id = $2::uuid`
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
	const query = `SELECT id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(chatwoot_account_id, ''), COALESCE(chatwoot_inbox_id, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, '') FROM channel_provisioning_sessions WHERE oauth_state = $1 ORDER BY updated_at DESC LIMIT 1`
	return scanProvisioningSession(executor.QueryRow(ctx, query, state), "channel_provisioning.get_by_oauth_state")
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
		    chatwoot_account_id = COALESCE($8, chatwoot_account_id),
		    chatwoot_inbox_id = COALESCE($9, chatwoot_inbox_id),
		    channel_connection_id = COALESCE($10::uuid, channel_connection_id),
		    failure_code = COALESCE($11, failure_code),
		    updated_at = $12
		WHERE business_id = $1::uuid AND id = $2::uuid
		RETURNING id::text, business_id::text, idempotency_key, provider_ref, channel, display_name, status, COALESCE(oauth_state, ''), COALESCE(authorization_url, ''), COALESCE(provider_account_ref, ''), COALESCE(provider_connection_ref, ''), COALESCE(chatwoot_account_id, ''), COALESCE(chatwoot_inbox_id, ''), COALESCE(channel_connection_id::text, ''), COALESCE(failure_code, '')`
	return scanProvisioningSession(executor.QueryRow(ctx, query, businessID, id, patch.Status, patch.OAuthState, patch.AuthorizationURL, patch.ProviderAccountRef, patch.ProviderConnectionRef, patch.ChatwootAccountID, patch.ChatwootInboxID, optionalString(patch.ChannelConnectionID), patch.FailureCode, now), "channel_provisioning.mark")
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
	if err := row.Scan(&session.ID, &session.BusinessID, &session.IdempotencyKey, &session.ProviderRef, &session.Channel, &session.DisplayName, &status, &session.OAuthState, &session.AuthorizationURL, &session.ProviderAccountRef, &session.ProviderConnectionRef, &session.ChatwootAccountID, &session.ChatwootInboxID, &session.ChannelConnectionID, &session.FailureCode); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ChannelProvisioningSession{}, &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return ports.ChannelProvisioningSession{}, &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
	}
	session.Status = ports.ChannelProvisioningStatus(status)
	return session, nil
}

var _ ports.ChannelProvisioningStore = (*ChannelProvisioningStore)(nil)
