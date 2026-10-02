package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlatformAuditRepository persists Platform Audit Events.
//
// Per Platform Administration Contract §48:
//   - APPEND ONLY — no Update / Delete methods exist.
//   - NEVER stores secrets (password, JWT, refresh token, provider secret,
//     social token, customer message content).
//
// Tenant isolation: Platform Audit is Platform-scoped. The business_id column
// is nullable and used for cross-reference only — Platform Admin can read ALL
// platform audit events regardless of business_id (they are Platform Scope, not
// Merchant Scope). This is the explicit design — per Contract §45, Platform
// Audit is separate from Merchant Audit.
type PlatformAuditRepository struct {
	adapter *Adapter
}

func NewPlatformAuditRepository(adapter *Adapter) *PlatformAuditRepository {
	return &PlatformAuditRepository{adapter: adapter}
}

func (r *PlatformAuditRepository) Append(ctx context.Context, draft ports.PlatformAuditDraft) (ports.PlatformAuditEvent, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformAuditEvent{}, ErrPoolClosed
	}
	if strings.TrimSpace(draft.Action) == "" || strings.TrimSpace(draft.TargetType) == "" {
		return ports.PlatformAuditEvent{}, invalidRepositoryInput("platform_audit.append", "action and target_type are required")
	}
	if draft.OccurredAt.IsZero() {
		draft.OccurredAt = time.Now().UTC()
	}
	if len(draft.Metadata) == 0 {
		draft.Metadata = []byte(`{}`)
	}
	if !json.Valid(draft.Metadata) {
		return ports.PlatformAuditEvent{}, invalidRepositoryInput("platform_audit.append", "metadata must be valid JSON")
	}
	eventID := uuid.NewString()
	result := strings.TrimSpace(draft.Result)
	if result == "" {
		result = "SUCCESS"
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformAuditEvent{}, err
	}
	var record ports.PlatformAuditEvent
	err = executor.QueryRow(ctx,
		`INSERT INTO platform_audit_events (id, actor_platform_admin_id, action, target_type, target_id, business_id, result, failure_code, correlation_id, occurred_at, metadata, created_at)
                 VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''), NULLIF($6, '')::uuid, $7, NULLIF($8, ''), NULLIF($9, ''), $10, $11, $10)
                 RETURNING id::text, actor_platform_admin_id::text, action, target_type, target_id, business_id::text, result, failure_code, correlation_id, occurred_at, metadata`,
		eventID, platformAuditNullableString(draft.ActorPlatformAdminID), draft.Action, draft.TargetType, platformAuditNullableString(draft.TargetID), platformAuditNullableString(draft.BusinessID), result, platformAuditNullableString(draft.FailureCode), platformAuditNullableString(draft.CorrelationID), draft.OccurredAt, draft.Metadata,
	).Scan(&record.ID, &record.ActorPlatformAdminID, &record.Action, &record.TargetType, &record.TargetID, &record.BusinessID, &record.Result, &record.FailureCode, &record.CorrelationID, &record.OccurredAt, &record.Metadata)
	if err != nil {
		return ports.PlatformAuditEvent{}, classifyRepositoryWriteError("platform_audit.append", err)
	}
	return record, nil
}

func (r *PlatformAuditRepository) List(ctx context.Context, filter ports.PlatformAuditFilter) (ports.PlatformAuditPage, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformAuditPage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	decoded, err := decodePlatformAuditCursor(filter.Cursor)
	if err != nil {
		return ports.PlatformAuditPage{}, invalidRepositoryInput("platform_audit.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformAuditPage{}, err
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.OccurredAt, decoded.ID
	}
	rows, err := executor.Query(ctx,
		`SELECT id::text, actor_platform_admin_id::text, action, target_type, target_id, business_id::text, result, failure_code, correlation_id, occurred_at, metadata
                 FROM platform_audit_events
                 WHERE ($1::uuid IS NULL OR actor_platform_admin_id = $1::uuid)
                   AND ($2 = '' OR action = $2)
                   AND ($3 = '' OR target_type = $3)
                   AND ($4 = '' OR target_id = $4)
                   AND ($5 = '' OR business_id::text = $5)
                   AND ($6 = '' OR result = $6)
                   AND ($7::timestamptz IS NULL OR occurred_at >= $7)
                   AND ($8::timestamptz IS NULL OR occurred_at <= $8)
                   AND ($9::timestamptz IS NULL OR (occurred_at, id) < ($9, $10::uuid))
                 ORDER BY occurred_at DESC, id DESC
                 LIMIT $11`,
		platformAuditNullableString(filter.ActorPlatformAdminID), filter.Action, filter.TargetType, filter.TargetID, filter.BusinessID, filter.Result,
		filter.OccurredFrom, filter.OccurredTo, cursorAt, cursorID, filter.Limit+1,
	)
	if err != nil {
		return ports.PlatformAuditPage{}, &RepositoryError{Operation: "platform_audit.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.PlatformAuditEvent, 0, filter.Limit)
	for rows.Next() {
		var record ports.PlatformAuditEvent
		if err := rows.Scan(&record.ID, &record.ActorPlatformAdminID, &record.Action, &record.TargetType, &record.TargetID, &record.BusinessID, &record.Result, &record.FailureCode, &record.CorrelationID, &record.OccurredAt, &record.Metadata); err != nil {
			return ports.PlatformAuditPage{}, &RepositoryError{Operation: "platform_audit.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.PlatformAuditPage{}, &RepositoryError{Operation: "platform_audit.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.PlatformAuditPage{Items: items}
	if len(items) > filter.Limit {
		page.HasMore = true
		page.Items = items[:filter.Limit]
		page.NextCursor = encodePlatformAuditCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *PlatformAuditRepository) GetByID(ctx context.Context, eventID string) (ports.PlatformAuditEvent, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformAuditEvent{}, ErrPoolClosed
	}
	if strings.TrimSpace(eventID) == "" {
		return ports.PlatformAuditEvent{}, invalidRepositoryInput("platform_audit.get", "event id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformAuditEvent{}, err
	}
	var record ports.PlatformAuditEvent
	err = executor.QueryRow(ctx,
		`SELECT id::text, actor_platform_admin_id::text, action, target_type, target_id, business_id::text, result, failure_code, correlation_id, occurred_at, metadata
                 FROM platform_audit_events WHERE id = $1::uuid`,
		eventID,
	).Scan(&record.ID, &record.ActorPlatformAdminID, &record.Action, &record.TargetType, &record.TargetID, &record.BusinessID, &record.Result, &record.FailureCode, &record.CorrelationID, &record.OccurredAt, &record.Metadata)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PlatformAuditEvent{}, &RepositoryError{Operation: "platform_audit.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.PlatformAuditEvent{}, &RepositoryError{Operation: "platform_audit.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func platformAuditNullableString(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

type platformAuditCursor struct {
	OccurredAt time.Time
	ID         string
}

func encodePlatformAuditCursor(record ports.PlatformAuditEvent) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.OccurredAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodePlatformAuditCursor(value string) (*platformAuditCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid platform audit cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid platform audit cursor")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid platform audit cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, fmt.Errorf("invalid platform audit cursor")
	}
	return &platformAuditCursor{OccurredAt: occurredAt, ID: parts[1]}, nil
}

var _ ports.PlatformAuditRepository = (*PlatformAuditRepository)(nil)
