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

type InboundEventStore struct{ adapter *Adapter }

func NewInboundEventStore(adapter *Adapter) *InboundEventStore {
	return &InboundEventStore{adapter: adapter}
}

const inboundEventColumns = `id::text, provider_ref, provider_connection_ref, provider_event_id, dedupe_strategy, business_id::text, connection_id::text, event_type, interaction_kind, provider_message_id, provider_conversation_id, external_user_id, content_reference, external_created_at, received_at, raw_payload_reference, payload_hash, signature_verified, processing_state, processing_owner, processing_lease_token::text, lease_expires_at, attempt_count, last_error_code, next_attempt_at, processing_result_code, processed_at, created_at, updated_at`
const inboundEventSelect = `SELECT ` + inboundEventColumns + ` FROM inbound_event_ledger`

func (s *InboundEventStore) RecordIfAbsent(ctx context.Context, draft ports.InboundEventDraft) (bool, ports.InboundEventRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return false, ports.InboundEventRecord{}, err
	}
	if err := validateInboundDraft(draft); err != nil {
		return false, ports.InboundEventRecord{}, err
	}
	const insertPrefix = `INSERT INTO inbound_event_ledger (id, provider_ref, provider_connection_ref, provider_event_id, dedupe_strategy, business_id, connection_id, event_type, interaction_kind, provider_message_id, provider_conversation_id, external_user_id, content_reference, external_created_at, received_at, raw_payload_reference, payload_hash, signature_verified, processing_state, next_attempt_at, created_at, updated_at) VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $7::uuid, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $21) ON CONFLICT (provider_ref, provider_connection_ref, provider_event_id) DO NOTHING RETURNING `
	var record ports.InboundEventRecord
	var errNoRows error
	errNoRows = executor.QueryRow(ctx, insertPrefix+inboundEventColumns, draft.ID, draft.ProviderRef, draft.ProviderConnectionRef, draft.ProviderEventID, draft.DedupeStrategy, nullableString(draft.BusinessID), nullableString(draft.ConnectionID), draft.EventType, draft.InteractionKind, draft.ProviderMessageID, draft.ProviderConversationID, draft.ExternalUserID, draft.ContentReference, draft.ExternalCreatedAt, draft.ReceivedAt, draft.RawPayloadReference, draft.PayloadHash, draft.SignatureVerified, draft.ProcessingState, draft.NextAttemptAt, draft.CreatedAt).Scan(inboundEventScanArgs(&record)...)
	if errNoRows == nil {
		return true, record, nil
	}
	if !errors.Is(errNoRows, pgx.ErrNoRows) {
		return false, ports.InboundEventRecord{}, classifyRepositoryWriteError("inbound_event.record_if_absent", errNoRows)
	}
	if err := executor.QueryRow(ctx, inboundEventSelect+` WHERE provider_ref = $1 AND provider_connection_ref = $2 AND provider_event_id = $3`, draft.ProviderRef, draft.ProviderConnectionRef, draft.ProviderEventID).Scan(inboundEventScanArgs(&record)...); err != nil {
		return false, ports.InboundEventRecord{}, classifyRepositoryGetError("inbound_event.record_if_absent", err)
	}
	return false, record, nil
}

func (s *InboundEventStore) Get(ctx context.Context, businessID, eventID string) (ports.InboundEventRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.InboundEventRecord{}, err
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(eventID) == "" {
		return ports.InboundEventRecord{}, invalidRepositoryInput("inbound_event.get", "business and event ids are required")
	}
	var record ports.InboundEventRecord
	if err := executor.QueryRow(ctx, inboundEventSelect+` WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, eventID).Scan(inboundEventScanArgs(&record)...); err != nil {
		return ports.InboundEventRecord{}, classifyRepositoryGetError("inbound_event.get", err)
	}
	return record, nil
}

func (s *InboundEventStore) List(ctx context.Context, filter ports.InboundEventFilter) (ports.InboundEventPage, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.InboundEventPage{}, err
	}
	if strings.TrimSpace(filter.BusinessID) == "" {
		return ports.InboundEventPage{}, invalidRepositoryInput("inbound_event.list", "business id is required")
	}
	limit, cursor, err := salesPageArgs(filter.Limit, filter.Cursor, "inbound_event.list")
	if err != nil {
		return ports.InboundEventPage{}, err
	}
	var cursorAt any
	var cursorID any
	if cursor != nil {
		cursorAt, cursorID = cursor.At, cursor.ID
	}
	rows, err := executor.Query(ctx, inboundEventSelect+` WHERE business_id = $1::uuid AND ($2 = '' OR connection_id = $2::uuid) AND ($3 = '' OR provider_ref = $3) AND ($4 = '' OR processing_state = $4) AND ($5::timestamptz IS NULL OR (received_at, id) < ($5::timestamptz, $6::uuid)) ORDER BY received_at DESC, id DESC LIMIT $7`, filter.BusinessID, filter.ConnectionID, filter.ProviderRef, filter.ProcessingState, cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.InboundEventPage{}, classifyRepositoryWriteError("inbound_event.list", err)
	}
	defer rows.Close()
	items := make([]ports.InboundEventRecord, 0, limit)
	for rows.Next() {
		var item ports.InboundEventRecord
		if err := rows.Scan(inboundEventScanArgs(&item)...); err != nil {
			return ports.InboundEventPage{}, classifyRepositoryGetError("inbound_event.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.InboundEventPage{}, classifyRepositoryGetError("inbound_event.list", err)
	}
	page := ports.InboundEventPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeSalesCursor(last.ReceivedAt, last.ID)
	}
	return page, nil
}

func (s *InboundEventStore) Claim(ctx context.Context, eventID string, lease ports.InboundEventLease) (ports.InboundEventClaimResult, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.InboundEventClaimResult{}, err
	}
	if strings.TrimSpace(eventID) == "" || strings.TrimSpace(lease.Owner) == "" || strings.TrimSpace(lease.Token) == "" || lease.ExpiresAt.IsZero() || !lease.ExpiresAt.After(time.Now().UTC()) {
		return ports.InboundEventClaimResult{}, invalidRepositoryInput("inbound_event.claim", "event id, owner, token, and lease expiry are required")
	}
	var record ports.InboundEventRecord
	claimErr := executor.QueryRow(ctx, `UPDATE inbound_event_ledger SET processing_state = 'processing', processing_owner = $2, processing_lease_token = $3::uuid, lease_expires_at = $4, attempt_count = attempt_count + 1, updated_at = now() WHERE id = $1::uuid AND ((processing_state IN ('received', 'retryable_failed') AND (next_attempt_at IS NULL OR next_attempt_at <= now())) OR (processing_state = 'processing' AND lease_expires_at <= now())) RETURNING `+inboundEventColumns, eventID, lease.Owner, lease.Token, lease.ExpiresAt).Scan(inboundEventScanArgs(&record)...)
	if claimErr == nil {
		return ports.InboundEventClaimResult{Claimed: true, Record: record}, nil
	}
	if !errors.Is(claimErr, pgx.ErrNoRows) {
		return ports.InboundEventClaimResult{}, classifyRepositoryWriteError("inbound_event.claim", claimErr)
	}
	if err := executor.QueryRow(ctx, inboundEventSelect+` WHERE id = $1::uuid`, eventID).Scan(inboundEventScanArgs(&record)...); err != nil {
		return ports.InboundEventClaimResult{}, classifyRepositoryGetError("inbound_event.claim", err)
	}
	return ports.InboundEventClaimResult{Claimed: false, Record: record}, nil
}

func (s *InboundEventStore) MarkProcessed(ctx context.Context, eventID string, completion ports.InboundEventCompletion) (ports.InboundEventRecord, error) {
	if completion.ResultCode == "" || completion.Owner == "" || completion.Token == "" || completion.ProcessedAt.IsZero() || completion.UpdatedAt.IsZero() {
		return ports.InboundEventRecord{}, invalidRepositoryInput("inbound_event.mark_processed", "owner, token, result, and timestamps are required")
	}
	return s.updateWithLease(ctx, "inbound_event.mark_processed", eventID, completion.Owner, completion.Token, `processing_state = 'processed', processing_owner = NULL, processing_lease_token = NULL, lease_expires_at = NULL, processing_result_code = $4, processed_at = $5, last_error_code = NULL, next_attempt_at = NULL, updated_at = $6`, completion.ResultCode, completion.ProcessedAt, completion.UpdatedAt)
}

func (s *InboundEventStore) MarkRetryableFailure(ctx context.Context, eventID string, failure ports.InboundEventFailure) (ports.InboundEventRecord, error) {
	if failure.ErrorCode == "" || failure.Owner == "" || failure.Token == "" || failure.NextAttempt == nil || failure.UpdatedAt.IsZero() {
		return ports.InboundEventRecord{}, invalidRepositoryInput("inbound_event.mark_retryable_failure", "owner, token, error, next attempt, and timestamp are required")
	}
	return s.updateWithLease(ctx, "inbound_event.mark_retryable_failure", eventID, failure.Owner, failure.Token, `processing_state = 'retryable_failed', processing_owner = NULL, processing_lease_token = NULL, lease_expires_at = NULL, processing_result_code = NULL, processed_at = NULL, last_error_code = $4, next_attempt_at = $5, updated_at = $6`, failure.ErrorCode, *failure.NextAttempt, failure.UpdatedAt)
}

func (s *InboundEventStore) MoveToDeadLetter(ctx context.Context, eventID string, failure ports.InboundEventFailure) (ports.InboundEventRecord, error) {
	if failure.ErrorCode == "" || failure.Owner == "" || failure.Token == "" || failure.UpdatedAt.IsZero() {
		return ports.InboundEventRecord{}, invalidRepositoryInput("inbound_event.dead_letter", "owner, token, error, and timestamp are required")
	}
	return s.updateWithLease(ctx, "inbound_event.dead_letter", eventID, failure.Owner, failure.Token, `processing_state = 'dead_letter', processing_owner = NULL, processing_lease_token = NULL, lease_expires_at = NULL, processing_result_code = NULL, processed_at = NULL, last_error_code = $4, next_attempt_at = NULL, updated_at = $5`, failure.ErrorCode, failure.UpdatedAt)
}

func (s *InboundEventStore) updateWithLease(ctx context.Context, operation, eventID, owner, token, setClause string, values ...any) (ports.InboundEventRecord, error) {
	executor, err := s.executor(ctx)
	if err != nil {
		return ports.InboundEventRecord{}, err
	}
	if eventID == "" || owner == "" || token == "" {
		return ports.InboundEventRecord{}, invalidRepositoryInput(operation, "event id, owner, and token are required")
	}
	args := []any{eventID, owner, token}
	args = append(args, values...)
	var record ports.InboundEventRecord
	query := `UPDATE inbound_event_ledger SET ` + setClause + ` WHERE id = $1::uuid AND processing_state = 'processing' AND processing_owner = $2 AND processing_lease_token = $3::uuid AND lease_expires_at > now() RETURNING ` + inboundEventColumns
	if err := executor.QueryRow(ctx, query, args...).Scan(inboundEventScanArgs(&record)...); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return ports.InboundEventRecord{}, classifyRepositoryWriteError(operation, err)
		}
		return ports.InboundEventRecord{}, classifyLeaseMutationMiss(ctx, executor, operation, eventID, owner, token)
	}
	return record, nil
}

func (s *InboundEventStore) executor(ctx context.Context) (SQLExecutor, error) {
	if s == nil || s.adapter == nil {
		return nil, ErrPoolClosed
	}
	return s.adapter.Executor(ctx)
}

func validateInboundDraft(draft ports.InboundEventDraft) error {
	if draft.ID == "" || draft.ProviderRef == "" || draft.ProviderConnectionRef == "" || draft.ProviderEventID == "" || draft.DedupeStrategy == "" || draft.EventType == "" || draft.ProcessingState == "" || draft.RawPayloadReference == "" || draft.PayloadHash == "" || draft.ReceivedAt.IsZero() || draft.CreatedAt.IsZero() || draft.UpdatedAt.IsZero() {
		return invalidRepositoryInput("inbound_event.record_if_absent", "identity, event type, state, payload reference/hash, and timestamps are required")
	}
	if draft.BusinessID == nil && draft.ConnectionID != nil || draft.BusinessID != nil && draft.ConnectionID == nil && draft.ProviderRef != "chatwoot" {
		return invalidRepositoryInput("inbound_event.record_if_absent", "business and connection must be provided together unless provider is chatwoot")
	}
	if !draft.SignatureVerified && draft.ProcessingState != "unresolved" && draft.ProcessingState != "rejected" {
		return invalidRepositoryInput("inbound_event.record_if_absent", "unverified event must be unresolved or rejected")
	}
	return nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func inboundEventScanArgs(record *ports.InboundEventRecord) []any {
	return []any{&record.ID, &record.ProviderRef, &record.ProviderConnectionRef, &record.ProviderEventID, &record.DedupeStrategy, &record.BusinessID, &record.ConnectionID, &record.EventType, &record.InteractionKind, &record.ProviderMessageID, &record.ProviderConversationID, &record.ExternalUserID, &record.ContentReference, &record.ExternalCreatedAt, &record.ReceivedAt, &record.RawPayloadReference, &record.PayloadHash, &record.SignatureVerified, &record.ProcessingState, &record.ProcessingOwner, &record.ProcessingLeaseToken, &record.LeaseExpiresAt, &record.AttemptCount, &record.LastErrorCode, &record.NextAttemptAt, &record.ProcessingResultCode, &record.ProcessedAt, &record.CreatedAt, &record.UpdatedAt}
}

func scanInboundEvent(row interface{ Scan(...any) error }) (ports.InboundEventRecord, error) {
	var record ports.InboundEventRecord
	if err := row.Scan(inboundEventScanArgs(&record)...); err != nil {
		return ports.InboundEventRecord{}, classifyRepositoryGetError("inbound_event.scan", err)
	}
	return record, nil
}

func classifyLeaseMutationMiss(ctx context.Context, executor SQLExecutor, operation, eventID, owner, token string) error {
	var state string
	var currentOwner *string
	var currentToken *string
	if err := executor.QueryRow(ctx, `SELECT processing_state, processing_owner, processing_lease_token::text FROM inbound_event_ledger WHERE id = $1::uuid`, eventID).Scan(&state, &currentOwner, &currentToken); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return classifyRepositoryWriteError(operation, err)
	}
	if state != "processing" || currentOwner == nil || currentToken == nil || *currentOwner != owner || *currentToken != token {
		return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: fmt.Errorf("event is not owned by active lease")}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: fmt.Errorf("event lease mutation was rejected")}
}

var _ ports.EventStore = (*InboundEventStore)(nil)
