package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

// ChannelHistoryRepository owns the PostgreSQL-only transaction. It does not
// disconnect a provider, modify subscriptions, or delete merchant catalogs.
type ChannelHistoryRepository struct{ adapter *Adapter }

func NewChannelHistoryRepository(adapter *Adapter) *ChannelHistoryRepository {
	return &ChannelHistoryRepository{adapter: adapter}
}

func (r *ChannelHistoryRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func (r *ChannelHistoryRepository) Preview(ctx context.Context, businessID, connectionID string) (ports.ChannelHistorySummary, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.ChannelHistorySummary{}, err
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(connectionID) == "" {
		return ports.ChannelHistorySummary{}, invalidRepositoryInput("channel_history.preview", "business and connection ids required")
	}
	var exists string
	err = executor.QueryRow(ctx, `SELECT id::text FROM channel_connections WHERE business_id=$1::uuid AND id=$2::uuid`, businessID, connectionID).Scan(&exists)
	if err != nil {
		return ports.ChannelHistorySummary{}, classifyRepositoryGetError("channel_history.preview", err)
	}
	result := ports.ChannelHistorySummary{BusinessID: businessID, ConnectionID: connectionID}
	const counts = `SELECT
        (SELECT count(DISTINCT conversation_id) FROM conversation_references WHERE business_id=$1::uuid AND connection_id=$2::uuid),
        (SELECT count(*) FROM communication_messages m
           JOIN conversation_references r ON r.business_id=m.business_id AND r.id=m.conversation_reference_id
           WHERE r.business_id=$1::uuid AND r.connection_id=$2::uuid)`
	if err := executor.QueryRow(ctx, counts, businessID, connectionID).Scan(&result.Conversations, &result.Messages); err != nil {
		return ports.ChannelHistorySummary{}, classifyRepositoryWriteError("channel_history.preview_counts", err)
	}
	return result, nil
}

func (r *ChannelHistoryRepository) Purge(ctx context.Context, businessID, connectionID, key string, expectedVersion int64) (ports.ChannelHistorySummary, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelHistorySummary{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(connectionID) == "" || strings.TrimSpace(key) == "" || expectedVersion <= 0 {
		return ports.ChannelHistorySummary{}, invalidRepositoryInput("channel_history.purge", "business, connection, key and version required")
	}
	var result ports.ChannelHistorySummary
	err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := r.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		// Lock only this account. Two concurrent purges cannot race each other.
		var version int64
		err = executor.QueryRow(txCtx, `SELECT resource_version FROM channel_connections
            WHERE business_id=$1::uuid AND id=$2::uuid FOR UPDATE`, businessID, connectionID).Scan(&version)
		if err != nil {
			return classifyRepositoryGetError("channel_history.purge_connection", err)
		}
		if version != expectedVersion {
			return &RepositoryError{Operation: "channel_history.purge", Kind: RepositoryStale, Err: errors.New("channel connection version changed")}
		}
		result = ports.ChannelHistorySummary{BusinessID: businessID, ConnectionID: connectionID}
		var lastKey string
		var lastAt time.Time
		err = executor.QueryRow(txCtx, `SELECT idempotency_key, cutoff, deleted_conversations, deleted_messages
            FROM channel_history_purges WHERE business_id=$1::uuid AND connection_id=$2::uuid`,
			businessID, connectionID).Scan(&lastKey, &lastAt, &result.Conversations, &result.Messages)
		if err == nil && key == lastKey {
			result.PurgedAt = &lastAt
			return nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return classifyRepositoryGetError("channel_history.purge_idempotency", err)
		}

		// Temp tables survive only inside this transaction. Store IDs, never bodies.
		const ids = `CREATE TEMP TABLE purge_refs ON COMMIT DROP AS
            SELECT r.id, r.conversation_id FROM conversation_references r
            WHERE r.business_id=$1::uuid AND r.connection_id=$2::uuid`
		if _, err = executor.Exec(txCtx, ids, businessID, connectionID); err != nil {
			return classifyRepositoryWriteError("channel_history.purge_refs", err)
		}
		// Refuse to delete half a conversation belonging to multiple accounts.
		var crossAccount bool
		if err = executor.QueryRow(txCtx, `SELECT EXISTS(
            SELECT 1 FROM conversation_references r
            JOIN purge_refs p ON p.conversation_id=r.conversation_id
            WHERE r.business_id=$1::uuid AND r.connection_id IS DISTINCT FROM $2::uuid
        )`, businessID, connectionID).Scan(&crossAccount); err != nil {
			return classifyRepositoryGetError("channel_history.cross_account", err)
		}
		if crossAccount {
			return &RepositoryError{Operation: "channel_history.purge", Kind: RepositoryConflict, Err: errors.New("conversation belongs to multiple channel connections")}
		}
		if _, err = executor.Exec(txCtx, `CREATE TEMP TABLE purge_conversations ON COMMIT DROP AS
            SELECT DISTINCT conversation_id AS id FROM purge_refs`); err != nil {
			return classifyRepositoryWriteError("channel_history.conversations", err)
		}
		if _, err = executor.Exec(txCtx, `CREATE TEMP TABLE purge_events ON COMMIT DROP AS
            SELECT id, raw_payload_reference FROM inbound_event_ledger
            WHERE (business_id=$1::uuid AND connection_id=$2::uuid)
            OR (business_id IS NULL AND provider_ref = (
                SELECT provider_ref FROM channel_connections WHERE business_id=$1::uuid AND id=$2::uuid)
                AND provider_connection_ref = (
                SELECT provider_account_ref FROM channel_connections WHERE business_id=$1::uuid AND id=$2::uuid)
            )`, businessID, connectionID); err != nil {
			return classifyRepositoryWriteError("channel_history.events", err)
		}
		// Shared provider webhook deliveries must not be erased across tenants.
		var sharedPayload bool
		err = executor.QueryRow(txCtx, `SELECT EXISTS(SELECT 1 FROM purge_events p
            JOIN inbound_event_ledger e ON e.raw_payload_reference=p.raw_payload_reference
            WHERE p.raw_payload_reference LIKE 'db://inbound_webhook_payloads/%'
              AND NOT EXISTS(SELECT 1 FROM purge_events own WHERE own.id=e.id))`).Scan(&sharedPayload)
		if err != nil {
			return classifyRepositoryGetError("channel_history.shared_payload", err)
		}
		if sharedPayload {
			return &RepositoryError{Operation: "channel_history.purge", Kind: RepositoryConflict, Err: errors.New("provider webhook contains events for another connection")}
		}
		if _, err = executor.Exec(txCtx, `CREATE TEMP TABLE purge_runs ON COMMIT DROP AS
            SELECT id FROM ai_runs WHERE business_id=$1::uuid
                AND (conversation_id IN (SELECT id FROM purge_conversations)
                  OR source_event_id IN (SELECT id FROM purge_events))`, businessID); err != nil {
			return classifyRepositoryWriteError("channel_history.runs", err)
		}
		// Never erase a message currently being transmitted or AI still running.
		var busy bool
		err = executor.QueryRow(txCtx, `SELECT
            EXISTS(SELECT 1 FROM outbox_entries o JOIN outbound_messages m
                ON m.business_id=o.business_id AND m.id=o.outbound_message_id
                WHERE m.business_id=$1::uuid AND m.connection_id=$2::uuid AND o.status='processing')
            OR EXISTS(SELECT 1 FROM ai_runs a JOIN purge_runs r ON a.id=r.id
                WHERE a.status NOT IN ('completed','failed','cancelled'))`, businessID, connectionID).Scan(&busy)
		if err != nil {
			return classifyRepositoryGetError("channel_history.busy", err)
		}
		if busy {
			return &RepositoryError{Operation: "channel_history.purge", Kind: RepositoryConflict, Err: errors.New("connection has in-flight messages or AI tasks; retry when idle")}
		}
		if err = executor.QueryRow(txCtx, `SELECT
             (SELECT count(*) FROM purge_conversations),
             (SELECT count(*) FROM communication_messages m WHERE m.business_id=$1::uuid
                 AND m.conversation_reference_id IN (SELECT id FROM purge_refs))`,
			businessID).Scan(&result.Conversations, &result.Messages); err != nil {
			return classifyRepositoryGetError("channel_history.counts", err)
		}
		// Remove content and its dependencies in child-before-parent order.
		queries := []string{
			`DELETE FROM communication_messages WHERE business_id=$1::uuid
                AND conversation_reference_id IN (SELECT id FROM purge_refs)`,
			`DELETE FROM outbound_delivery_status_updates WHERE business_id=$1::uuid
                AND (inbound_event_id IN (SELECT id FROM purge_events) OR outbound_message_id IN
                    (SELECT id FROM outbound_messages WHERE business_id=$1::uuid AND connection_id=$2::uuid))`,
			`DELETE FROM outbox_entries WHERE business_id=$1::uuid AND outbound_message_id IN
                (SELECT id FROM outbound_messages WHERE business_id=$1::uuid AND connection_id=$2::uuid)`,
			`DELETE FROM outbound_messages WHERE business_id=$1::uuid AND connection_id=$2::uuid`,
			`DELETE FROM automation_executions WHERE business_id=$1::uuid AND inbound_event_id IN
                (SELECT id FROM purge_events)`,
			`DELETE FROM ai_decisions WHERE business_id=$1::uuid
                AND conversation_id IN (SELECT id FROM purge_conversations)`,
			`DELETE FROM ai_tool_calls WHERE ai_run_id IN (SELECT id FROM purge_runs)`,
			`DELETE FROM ai_gemini_interactions WHERE ai_run_id IN (SELECT id FROM purge_runs)`,
			`DELETE FROM ai_catalog_batches WHERE ai_run_id IN (SELECT id FROM purge_runs)`,
			`DELETE FROM ai_run_attempts WHERE ai_run_id IN (SELECT id FROM purge_runs)`,
			// Preserve token usage for SaaS accounting while removing linked conversation text.
			`UPDATE ai_runs SET conversation_id=NULL, source_event_id=NULL, message_id=NULL,
                gemini_interaction_id=NULL, previous_interaction_id=NULL, failure_reason=NULL
                WHERE business_id=$1::uuid AND id IN (SELECT id FROM purge_runs)`,
			`UPDATE inbound_event_ledger SET raw_payload_reference='purged://history',
                provider_message_id=NULL, provider_conversation_id=NULL, external_user_id=NULL,
                content_reference=NULL, processing_owner=NULL, processing_lease_token=NULL,
                lease_expires_at=NULL, next_attempt_at=NULL, last_error_code=NULL,
                processing_state=CASE WHEN dedupe_strategy='dedupe_uncertain' THEN 'rejected' ELSE 'processed' END,
                processing_result_code='history_purged',
                processed_at=CASE WHEN dedupe_strategy='dedupe_uncertain' THEN NULL ELSE now() END,
                updated_at=now()
                WHERE id IN (SELECT id FROM purge_events)`,
			`UPDATE external_identities SET profile_snapshot_reference=NULL WHERE business_id=$1::uuid AND connection_id=$2::uuid`,
			`DELETE FROM inbound_webhook_payloads WHERE 'db://inbound_webhook_payloads/' || id::text IN
                (SELECT raw_payload_reference FROM purge_events)`,
			`DELETE FROM conversation_labels WHERE business_id=$1::uuid
                AND conversation_id IN (SELECT id FROM purge_conversations)`,
			`DELETE FROM conversation_read_cursors WHERE business_id=$1::uuid
                AND conversation_id IN (SELECT id FROM purge_conversations)`,
			`DELETE FROM conversation_state WHERE business_id=$1::uuid
                AND conversation_id IN (SELECT id FROM purge_conversations)`,
			`UPDATE leads SET source_conversation_reference_id=NULL WHERE business_id=$1::uuid
                AND source_conversation_reference_id IN (SELECT id FROM purge_refs)`,
			`UPDATE commercial_transactions SET source_conversation_reference_id=NULL WHERE business_id=$1::uuid
                AND source_conversation_reference_id IN (SELECT id FROM purge_refs)`,
			`UPDATE lead_attributions SET source_conversation_id=NULL WHERE business_id=$1::uuid
                AND source_conversation_id IN (SELECT id FROM purge_conversations)`,
			`DELETE FROM conversation_references WHERE business_id=$1::uuid
                AND id IN (SELECT id FROM purge_refs)`,
			`DELETE FROM conversations WHERE business_id=$1::uuid
                AND id IN (SELECT id FROM purge_conversations)`,
		}
		for index, query := range queries {
			// The fixed statements have different parameter arities. pgx
			// rejects unused arguments, even for DELETE without placeholders.
			var args []any
			if strings.Contains(query, "$1") {
				args = append(args, businessID)
			}
			if strings.Contains(query, "$2") {
				args = append(args, connectionID)
			}
			if _, err := executor.Exec(txCtx, query, args...); err != nil {
				return classifyRepositoryWriteError(fmt.Sprintf("channel_history.purge_step_%d", index), err)
			}
		}
		cutoff := time.Now().UTC()
		_, err = executor.Exec(txCtx, `INSERT INTO channel_history_purges
            (business_id, connection_id, cutoff, idempotency_key, deleted_conversations, deleted_messages)
            VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6)
            ON CONFLICT (business_id,connection_id) DO UPDATE SET
                cutoff=EXCLUDED.cutoff, idempotency_key=EXCLUDED.idempotency_key,
                deleted_conversations=EXCLUDED.deleted_conversations,
                deleted_messages=EXCLUDED.deleted_messages, updated_at=now()`,
			businessID, connectionID, cutoff, key, result.Conversations, result.Messages)
		if err != nil {
			return classifyRepositoryWriteError("channel_history.watermark", err)
		}
		result.PurgedAt = &cutoff
		return nil
	})
	return result, err
}

func (r *ChannelHistoryRepository) ShouldIgnore(ctx context.Context, businessID, connectionID string, occurredAt *time.Time) (bool, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return false, err
	}
	var cutoff time.Time
	err = executor.QueryRow(ctx, `SELECT cutoff FROM channel_history_purges
        WHERE business_id=$1::uuid AND connection_id=$2::uuid`, businessID, connectionID).Scan(&cutoff)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, classifyRepositoryGetError("channel_history.guard", err)
	}
	// Unknown timestamps must not resurrect purged history.
	return occurredAt == nil || !occurredAt.After(cutoff), nil
}

var _ ports.ChannelHistoryStore = (*ChannelHistoryRepository)(nil)
var _ ports.ChannelHistoryGuard = (*ChannelHistoryRepository)(nil)
