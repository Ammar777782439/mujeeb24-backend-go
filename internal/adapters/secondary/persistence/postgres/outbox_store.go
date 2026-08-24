package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type PostgresOutboxStore struct{ adapter *Adapter }

func NewPostgresOutboxStore(adapter *Adapter) *PostgresOutboxStore {
	return &PostgresOutboxStore{adapter: adapter}
}

const outboxColumns = `id::text, business_id::text, outbound_message_id::text, command_type, dedupe_key, status, attempt_count, available_at, lease_owner, lease_token::text, lease_expires_at, last_error_code, result_code, completed_at, created_at, updated_at`
const outboxSelect = `SELECT ` + outboxColumns + ` FROM outbox_entries`

func (s *PostgresOutboxStore) Enqueue(ctx context.Context, draft ports.OutboxEntryDraft) (ports.OutboxEntryRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.OutboxEntryRecord{}, err
	}
	if draft.ID == "" || draft.BusinessID == "" || draft.OutboundMessageID == "" || draft.CommandType == "" || draft.DedupeKey == "" || draft.AvailableAt.IsZero() || draft.CreatedAt.IsZero() || draft.UpdatedAt.IsZero() {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput("outbox.enqueue", "id, business, outbound message, command, dedupe key, availability, and timestamps are required")
	}
	return scanOutboxWrite(executor.QueryRow(ctx, `INSERT INTO outbox_entries (id, business_id, outbound_message_id, command_type, dedupe_key, status, attempt_count, available_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'pending', 0, $6, $7, $8) RETURNING `+outboxColumns, draft.ID, draft.BusinessID, draft.OutboundMessageID, draft.CommandType, draft.DedupeKey, draft.AvailableAt, draft.CreatedAt, draft.UpdatedAt))
}

func (s *PostgresOutboxStore) Get(ctx context.Context, businessID, entryID string) (ports.OutboxEntryRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.OutboxEntryRecord{}, err
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(entryID) == "" {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput("outbox.get", "business and entry ids are required")
	}
	return scanOutbox(executor.QueryRow(ctx, outboxSelect+` WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, entryID))
}

func (s *PostgresOutboxStore) List(ctx context.Context, filter ports.OutboxFilter) (ports.OutboxPage, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.OutboxPage{}, err
	}
	if strings.TrimSpace(filter.BusinessID) == "" {
		return ports.OutboxPage{}, invalidRepositoryInput("outbox.list", "business id is required")
	}
	limit, cursor, err := salesPageArgs(filter.Limit, filter.Cursor, "outbox.list")
	if err != nil {
		return ports.OutboxPage{}, err
	}
	var cursorAt any
	var cursorID any
	if cursor != nil {
		cursorAt, cursorID = cursor.At, cursor.ID
	}
	rows, err := executor.Query(ctx, outboxSelect+` WHERE business_id = $1::uuid AND ($2 = '' OR status = $2) AND ($3::timestamptz IS NULL OR (created_at, id) < ($3::timestamptz, $4::uuid)) ORDER BY created_at DESC, id DESC LIMIT $5`, filter.BusinessID, filter.Status, cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.OutboxPage{}, classifyRepositoryWriteError("outbox.list", err)
	}
	defer rows.Close()
	items := make([]ports.OutboxEntryRecord, 0, limit)
	for rows.Next() {
		var item ports.OutboxEntryRecord
		if err := rows.Scan(outboxScanArgs(&item)...); err != nil {
			return ports.OutboxPage{}, classifyRepositoryGetError("outbox.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.OutboxPage{}, classifyRepositoryGetError("outbox.list", err)
	}
	page := ports.OutboxPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeSalesCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *PostgresOutboxStore) Claim(ctx context.Context, entryID string, lease ports.OutboxLease) (ports.OutboxClaimResult, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.OutboxClaimResult{}, err
	}
	if strings.TrimSpace(entryID) == "" || strings.TrimSpace(lease.Owner) == "" || strings.TrimSpace(lease.Token) == "" || lease.ExpiresAt.IsZero() || !lease.ExpiresAt.After(time.Now().UTC()) {
		return ports.OutboxClaimResult{}, invalidRepositoryInput("outbox.claim", "entry id, owner, token, and future lease expiry are required")
	}
	var record ports.OutboxEntryRecord
	claimErr := executor.QueryRow(ctx, `UPDATE outbox_entries SET status = 'processing', lease_owner = $2, lease_token = $3::uuid, lease_expires_at = $4, attempt_count = attempt_count + 1, updated_at = now() WHERE id = $1::uuid AND ((status IN ('pending', 'retryable_failed') AND available_at <= now()) OR (status = 'processing' AND lease_expires_at <= now())) RETURNING `+outboxColumns, entryID, lease.Owner, lease.Token, lease.ExpiresAt).Scan(outboxScanArgs(&record)...)
	if claimErr == nil {
		return ports.OutboxClaimResult{Claimed: true, Record: record}, nil
	}
	if !errors.Is(claimErr, pgx.ErrNoRows) {
		return ports.OutboxClaimResult{}, classifyRepositoryWriteError("outbox.claim", claimErr)
	}
	if err := executor.QueryRow(ctx, outboxSelect+` WHERE id = $1::uuid`, entryID).Scan(outboxScanArgs(&record)...); err != nil {
		return ports.OutboxClaimResult{}, classifyRepositoryGetError("outbox.claim", err)
	}
	return ports.OutboxClaimResult{Claimed: false, Record: record}, nil
}

func (s *PostgresOutboxStore) MarkCompleted(ctx context.Context, entryID string, completion ports.OutboxCompletion) (ports.OutboxEntryRecord, error) {
	if completion.Owner == "" || completion.Token == "" || completion.ResultCode == "" || completion.CompletedAt.IsZero() || completion.UpdatedAt.IsZero() {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput("outbox.mark_completed", "owner, token, result, and timestamps are required")
	}
	return s.updateWithLease(ctx, "outbox.mark_completed", entryID, completion.Owner, completion.Token, `status = 'completed', lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, result_code = $4, completed_at = $5, updated_at = $6`, completion.ResultCode, completion.CompletedAt, completion.UpdatedAt)
}

func (s *PostgresOutboxStore) MarkRetryableFailure(ctx context.Context, entryID string, failure ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	if failure.Owner == "" || failure.Token == "" || failure.ErrorCode == "" || failure.NextAttempt == nil || failure.UpdatedAt.IsZero() {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput("outbox.mark_retryable_failure", "owner, token, error, next attempt, and timestamp are required")
	}
	return s.updateWithLease(ctx, "outbox.mark_retryable_failure", entryID, failure.Owner, failure.Token, `status = 'retryable_failed', lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, result_code = NULL, completed_at = NULL, last_error_code = $4, available_at = $5, updated_at = $6`, failure.ErrorCode, *failure.NextAttempt, failure.UpdatedAt)
}

func (s *PostgresOutboxStore) MoveToDeadLetter(ctx context.Context, entryID string, failure ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	if failure.Owner == "" || failure.Token == "" || failure.ErrorCode == "" || failure.UpdatedAt.IsZero() {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput("outbox.dead_letter", "owner, token, error, and timestamp are required")
	}
	return s.updateWithLease(ctx, "outbox.dead_letter", entryID, failure.Owner, failure.Token, `status = 'dead_letter', lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, result_code = NULL, completed_at = NULL, last_error_code = $4, updated_at = $5`, failure.ErrorCode, failure.UpdatedAt)
}

func (s *PostgresOutboxStore) Requeue(ctx context.Context, entryID string, nextAttemptAt, updatedAt time.Time) (ports.OutboxEntryRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.OutboxEntryRecord{}, err
	}
	if entryID == "" || nextAttemptAt.IsZero() || updatedAt.IsZero() {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput("outbox.requeue", "entry id and timestamps are required")
	}
	var record ports.OutboxEntryRecord
	err = executor.QueryRow(ctx, `UPDATE outbox_entries SET status = 'pending', available_at = $2, lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, result_code = NULL, completed_at = NULL, updated_at = $3 WHERE id = $1::uuid AND status = 'dead_letter' RETURNING `+outboxColumns, entryID, nextAttemptAt, updatedAt).Scan(outboxScanArgs(&record)...)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ports.OutboxEntryRecord{}, classifyRepositoryWriteError("outbox.requeue", err)
	}
	return ports.OutboxEntryRecord{}, classifyOutboxMutationMiss(ctx, executor, "outbox.requeue", entryID)
}

func (s *PostgresOutboxStore) updateWithLease(ctx context.Context, operation, entryID, owner, token, setClause string, values ...any) (ports.OutboxEntryRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.OutboxEntryRecord{}, err
	}
	if entryID == "" {
		return ports.OutboxEntryRecord{}, invalidRepositoryInput(operation, "entry id is required")
	}
	args := []any{entryID, owner, token}
	args = append(args, values...)
	var record ports.OutboxEntryRecord
	query := `UPDATE outbox_entries SET ` + setClause + ` WHERE id = $1::uuid AND status = 'processing' AND lease_owner = $2 AND lease_token = $3::uuid AND lease_expires_at > now() RETURNING ` + outboxColumns
	if err := executor.QueryRow(ctx, query, args...).Scan(outboxScanArgs(&record)...); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return ports.OutboxEntryRecord{}, classifyRepositoryWriteError(operation, err)
		}
		return ports.OutboxEntryRecord{}, classifyOutboxMutationMiss(ctx, executor, operation, entryID)
	}
	return record, nil
}

func (s *PostgresOutboxStore) executor(ctx context.Context) (SQLExecutor, error) {
	if s == nil || s.adapter == nil {
		return nil, ErrPoolClosed
	}
	return s.adapter.Executor(ctx)
}

func outboxScanArgs(record *ports.OutboxEntryRecord) []any {
	return []any{&record.ID, &record.BusinessID, &record.OutboundMessageID, &record.CommandType, &record.DedupeKey, &record.Status, &record.AttemptCount, &record.AvailableAt, &record.LeaseOwner, &record.LeaseToken, &record.LeaseExpiresAt, &record.LastErrorCode, &record.ResultCode, &record.CompletedAt, &record.CreatedAt, &record.UpdatedAt}
}

func scanOutbox(row interface{ Scan(...any) error }) (ports.OutboxEntryRecord, error) {
	return scanOutboxWithClassifier(row, classifyRepositoryGetError)
}

func scanOutboxWrite(row interface{ Scan(...any) error }) (ports.OutboxEntryRecord, error) {
	return scanOutboxWithClassifier(row, classifyRepositoryWriteError)
}

func scanOutboxWithClassifier(row interface{ Scan(...any) error }, classify func(string, error) error) (ports.OutboxEntryRecord, error) {
	var record ports.OutboxEntryRecord
	if err := row.Scan(outboxScanArgs(&record)...); err != nil {
		return ports.OutboxEntryRecord{}, classify("outbox.scan", err)
	}
	return record, nil
}

func classifyOutboxMutationMiss(ctx context.Context, executor SQLExecutor, operation, entryID string) error {
	var status string
	var owner, token *string
	if err := executor.QueryRow(ctx, `SELECT status, lease_owner, lease_token::text FROM outbox_entries WHERE id = $1::uuid`, entryID).Scan(&status, &owner, &token); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return classifyRepositoryWriteError(operation, err)
	}
	if status != "processing" || owner == nil || token == nil {
		return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: fmt.Errorf("outbox entry status %s is not an active lease", status)}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: fmt.Errorf("outbox lease owner or token is invalid")}
}

var _ ports.OutboxStore = (*PostgresOutboxStore)(nil)
