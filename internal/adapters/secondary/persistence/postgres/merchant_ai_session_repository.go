// Package postgres — Merchant AI Session Repository.
//
// Implements the services.MerchantAISessionReader and
// services.MerchantAISessionWriter interfaces against the merchant_ai_sessions
// and merchant_ai_messages tables per migration 000054.
//
// Per contract 11 §2, this repository is MERCHANT-SPECIFIC and does NOT
// share code paths with Customer Sales AI infrastructure. The B2C
// ConversationRepository (conversations + conversation_state tables) is
// a completely separate persistence concern.
//
// Per contract ⑧ §17, every read/write is tenant-scoped via business_id.
// A mismatch returns "not found" (per contract ⑥ §8: do not leak existence).
//
// Per migration 000054:
//
//      CREATE TABLE merchant_ai_sessions (
//          id           UUID NOT NULL,
//          business_id  UUID NOT NULL,
//          principal_id UUID NOT NULL,
//          created_at   TIMESTAMPTZ NOT NULL,
//          updated_at   TIMESTAMPTZ NOT NULL,
//          PRIMARY KEY (business_id, id),
//          FK business_id → businesses(id) ON DELETE RESTRICT,
//          FK principal_id → principals(id) ON DELETE RESTRICT
//      );
//      CREATE INDEX idx_merchant_ai_sessions_principal_updated
//          ON merchant_ai_sessions (business_id, principal_id, updated_at DESC);
//
//      CREATE TABLE merchant_ai_messages (
//          id          UUID NOT NULL,
//          business_id UUID NOT NULL,
//          session_id  UUID NOT NULL,
//          sender_type VARCHAR(32) NOT NULL,
//          text        TEXT NOT NULL,
//          created_at  TIMESTAMPTZ NOT NULL,
//          PRIMARY KEY (business_id, id),
//          FK (business_id, session_id) → merchant_ai_sessions(business_id, id) ON DELETE CASCADE,
//          CHECK (sender_type IN ('merchant', 'assistant'))
//      );
//      CREATE INDEX idx_merchant_ai_messages_session_created
//          ON merchant_ai_messages (business_id, session_id, created_at ASC);

package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// MerchantAISessionRepository implements services.MerchantAISessionReader
// and services.MerchantAISessionWriter against migration 000054 tables.
type MerchantAISessionRepository struct{ adapter *Adapter }

// NewMerchantAISessionRepository wires the repository to a Postgres Adapter.
func NewMerchantAISessionRepository(adapter *Adapter) *MerchantAISessionRepository {
	return &MerchantAISessionRepository{adapter: adapter}
}

// CreateSession inserts a new merchant_ai_sessions row per migration 000054.
//
// Per migration 000054, the primary key is (business_id, id) and the table
// has FKs to businesses(id) and principals(id). The caller must provide
// valid business_id and principal_id (authenticated context per contract ⑥ §9).
//
// Per contract ⑧ §17, the write is tenant-scoped via business_id.
// Per contract ⑧ §23, no secrets are stored.
func (r *MerchantAISessionRepository) CreateSession(ctx context.Context, businessID, principalID string) (string, error) {
	if r == nil || r.adapter == nil {
		return "", ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return "", invalidRepositoryInput("merchant_ai_session.create", "business_id is required per contract ⑧ §17")
	}
	if strings.TrimSpace(principalID) == "" {
		return "", invalidRepositoryInput("merchant_ai_session.create", "principal_id is required per migration 000054 NOT NULL constraint")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return "", err
	}
	sessionID := uuid.NewString()
	now := time.Now().UTC()
	const query = `INSERT INTO merchant_ai_sessions (id, business_id, principal_id, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)`
	if _, err := executor.Exec(ctx, query, sessionID, businessID, principalID, now, now); err != nil {
		return "", &RepositoryError{Operation: "merchant_ai_session.create", Kind: RepositoryInvalid, Err: fmt.Errorf("insert merchant_ai_session: %w", err)}
	}
	return sessionID, nil
}

// AppendMessage inserts a new merchant_ai_messages row per migration 000054.
//
// Per migration 000054 merchant_ai_messages_sender_type_chk, senderType must
// be 'merchant' or 'assistant'. The FK to merchant_ai_sessions ensures the
// session exists with matching business_id (tenant-scoped per contract ⑧ §17).
//
// Per contract ③ §1, this is the canonical B2B conversation memory. The
// merchant_ai_messages table is the source of truth for merchant-side
// multi-turn history (NOT ConversationState, which is B2C-only per contract ③).
func (r *MerchantAISessionRepository) AppendMessage(ctx context.Context, businessID, sessionID, senderType, text string) (string, error) {
	if r == nil || r.adapter == nil {
		return "", ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(sessionID) == "" {
		return "", invalidRepositoryInput("merchant_ai_message.append", "business_id and session_id are required per contract ⑧ §17")
	}
	// Per migration 000054 merchant_ai_messages_sender_type_chk.
	if senderType != "merchant" && senderType != "assistant" {
		return "", invalidRepositoryInput("merchant_ai_message.append", fmt.Sprintf("sender_type must be 'merchant' or 'assistant' per migration 000054, got %q", senderType))
	}
	if strings.TrimSpace(text) == "" {
		return "", invalidRepositoryInput("merchant_ai_message.append", "text is required per migration 000054 TEXT NOT NULL")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return "", err
	}
	messageID := uuid.NewString()
	now := time.Now().UTC()
	const query = `INSERT INTO merchant_ai_messages (id, business_id, session_id, sender_type, text, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6)`
	if _, err := executor.Exec(ctx, query, messageID, businessID, sessionID, senderType, text, now); err != nil {
		return "", &RepositoryError{Operation: "merchant_ai_message.append", Kind: RepositoryInvalid, Err: fmt.Errorf("insert merchant_ai_message: %w", err)}
	}
	return messageID, nil
}

// ListMessages returns the merchant_ai_messages for a session, ordered by
// created_at ASC (oldest first) per migration 000054
// idx_merchant_ai_messages_session_created.
//
// Per contract ⑧ §17, the read is tenant-scoped via business_id. A mismatch
// returns an empty list (do not leak existence of other tenants' sessions
// per contract ⑥ §8).
//
// Per contract ③ §6, we do NOT send the entire merchant session history —
// only the most recent `limit` messages. The caller (MerchantContextBuilder)
// passes a reasonable limit (default 12).
func (r *MerchantAISessionRepository) ListMessages(ctx context.Context, businessID, sessionID string, limit int) ([]services.MerchantAIMessage, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, invalidRepositoryInput("merchant_ai_message.list", "business_id and session_id are required per contract ⑧ §17")
	}
	if limit <= 0 {
		limit = 12
	}
	if limit > 100 {
		limit = 100
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	// Per migration 000054 idx_merchant_ai_messages_session_created
	// (business_id, session_id, created_at ASC), this query uses the index
	// efficiently: it filters by (business_id, session_id) and orders by
	// created_at ASC. We take the last `limit` rows by using a subquery
	// with DESC + outer ASC.
	const query = `SELECT id::text, business_id::text, session_id::text, sender_type, text, created_at FROM merchant_ai_messages WHERE business_id = $1::uuid AND session_id = $2::uuid ORDER BY created_at DESC LIMIT $3`
	rows, err := executor.Query(ctx, query, businessID, sessionID, limit)
	if err != nil {
		return nil, &RepositoryError{Operation: "merchant_ai_message.list", Kind: RepositoryInvalid, Err: fmt.Errorf("query merchant_ai_messages: %w", err)}
	}
	defer rows.Close()
	var out []services.MerchantAIMessage
	for rows.Next() {
		var m services.MerchantAIMessage
		if err := rows.Scan(&m.ID, &m.BusinessID, &m.SessionID, &m.SenderType, &m.Text, &m.CreatedAt); err != nil {
			return nil, &RepositoryError{Operation: "merchant_ai_message.list", Kind: RepositoryInvalid, Err: fmt.Errorf("scan merchant_ai_message: %w", err)}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "merchant_ai_message.list", Kind: RepositoryInvalid, Err: err}
	}
	// Reverse to chronological (oldest first) for the context builder.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Compile-time assertions that the repository satisfies both interfaces.
var _ services.MerchantAISessionReader = (*MerchantAISessionRepository)(nil)
var _ services.MerchantAISessionWriter = (*MerchantAISessionRepository)(nil)
