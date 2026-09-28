package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SupportRepository implements ports.SupportRepository against Postgres.
//
// Per Platform Administration Contract V1 §37-44:
//   - Support is a Platform domain — Platform Admin reads/writes support data
//     via Platform Scope (not Merchant Scope).
//   - Tickets transition forward-only: OPEN → IN_PROGRESS → RESOLVED → CLOSED.
//   - Messages are APPEND ONLY.
//   - business_id is a cross-reference only — does NOT grant merchant
//     membership.
type SupportRepository struct {
	adapter *Adapter
}

func NewSupportRepository(adapter *Adapter) *SupportRepository {
	return &SupportRepository{adapter: adapter}
}

const supportTicketSelectColumns = `id::text, business_id::text, subject, status, priority, category, created_by::text, created_by_type, resolved_at, resolved_by::text, closed_at, closed_by::text, last_message_at, created_at, updated_at`

func scanSupportTicket(scanner interface {
	Scan(dest ...any) error
}, record *ports.SupportTicketRecord) error {
	return scanner.Scan(
		&record.ID, &record.BusinessID, &record.Subject, &record.Status, &record.Priority, &record.Category,
		&record.CreatedBy, &record.CreatedByType,
		&record.ResolvedAt, &record.ResolvedBy,
		&record.ClosedAt, &record.ClosedBy,
		&record.LastMessageAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
}

func (r *SupportRepository) CreateTicket(ctx context.Context, create ports.SupportTicketCreate) (ports.SupportTicketRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportTicketRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.BusinessID) == "" || strings.TrimSpace(create.Subject) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.create", "id, business_id, and subject are required")
	}
	if strings.TrimSpace(create.CreatedBy) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.create", "created_by is required")
	}
	priority := strings.TrimSpace(strings.ToUpper(create.Priority))
	if priority == "" {
		priority = "NORMAL"
	}
	if priority != "NORMAL" && priority != "HIGH" && priority != "CRITICAL" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.create", "priority must be NORMAL, HIGH, or CRITICAL")
	}
	category := strings.TrimSpace(strings.ToUpper(create.Category))
	if category == "" {
		category = "OTHER"
	}
	if category != "ACCOUNT" && category != "CHANNEL" && category != "AI" && category != "CATALOG" && category != "SALES" && category != "BILLING" && category != "OTHER" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.create", "category must be one of ACCOUNT / CHANNEL / AI / CATALOG / SALES / BILLING / OTHER")
	}
	createdByType := strings.TrimSpace(strings.ToUpper(create.CreatedByType))
	if createdByType != "BUSINESS_USER" && createdByType != "PLATFORM_ADMIN" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.create", "created_by_type must be BUSINESS_USER or PLATFORM_ADMIN")
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportTicketRecord{}, err
	}
	var record ports.SupportTicketRecord
	err = executor.QueryRow(ctx,
		`INSERT INTO support_tickets (id, business_id, subject, status, priority, category, created_by, created_by_type, created_at, updated_at)
		 VALUES ($1::uuid, $2::uuid, $3, 'OPEN', $4, $5, $6::uuid, $7, $8, $8)
		 RETURNING `+supportTicketSelectColumns,
		create.ID, create.BusinessID, create.Subject, priority, category,
		create.CreatedBy, createdByType, create.Now,
	).Scan(
		&record.ID, &record.BusinessID, &record.Subject, &record.Status, &record.Priority, &record.Category,
		&record.CreatedBy, &record.CreatedByType,
		&record.ResolvedAt, &record.ResolvedBy,
		&record.ClosedAt, &record.ClosedBy,
		&record.LastMessageAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return ports.SupportTicketRecord{}, classifyRepositoryWriteError("support_ticket.create", err)
	}
	return record, nil
}

func (r *SupportRepository) GetTicketByID(ctx context.Context, ticketID string) (ports.SupportTicketRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportTicketRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(ticketID) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.get", "ticket id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportTicketRecord{}, err
	}
	var record ports.SupportTicketRecord
	err = executor.QueryRow(ctx,
		`SELECT `+supportTicketSelectColumns+` FROM support_tickets WHERE id = $1::uuid`,
		ticketID,
	).Scan(
		&record.ID, &record.BusinessID, &record.Subject, &record.Status, &record.Priority, &record.Category,
		&record.CreatedBy, &record.CreatedByType,
		&record.ResolvedAt, &record.ResolvedBy,
		&record.ClosedAt, &record.ClosedBy,
		&record.LastMessageAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *SupportRepository) ListTickets(ctx context.Context, filter ports.SupportTicketListFilter) (ports.SupportTicketPage, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportTicketPage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	decoded, err := decodeSupportTicketCursor(filter.Cursor)
	if err != nil {
		return ports.SupportTicketPage{}, invalidRepositoryInput("support_ticket.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportTicketPage{}, err
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.CreatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx,
		`SELECT `+supportTicketSelectColumns+`
		 FROM support_tickets
		 WHERE ($1 = '' OR business_id::text = $1)
		   AND ($2 = '' OR status = $2)
		   AND ($3 = '' OR priority = $3)
		   AND ($4 = '' OR category = $4)
		   AND ($5::timestamptz IS NULL OR created_at >= $5)
		   AND ($6::timestamptz IS NULL OR created_at <= $6)
		   AND ($7::timestamptz IS NULL OR (created_at, id) < ($7, $8::uuid))
		 ORDER BY created_at DESC, id DESC
		 LIMIT $9`,
		strings.TrimSpace(filter.BusinessID), strings.TrimSpace(filter.Status), strings.TrimSpace(filter.Priority), strings.TrimSpace(filter.Category),
		filter.CreatedFrom, filter.CreatedTo, cursorAt, cursorID, filter.Limit+1,
	)
	if err != nil {
		return ports.SupportTicketPage{}, &RepositoryError{Operation: "support_ticket.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.SupportTicketRecord, 0, filter.Limit)
	for rows.Next() {
		var record ports.SupportTicketRecord
		if err := scanSupportTicket(rows, &record); err != nil {
			return ports.SupportTicketPage{}, &RepositoryError{Operation: "support_ticket.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.SupportTicketPage{}, &RepositoryError{Operation: "support_ticket.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.SupportTicketPage{Items: items}
	if len(items) > filter.Limit {
		page.HasMore = true
		page.Items = items[:filter.Limit]
		page.NextCursor = encodeSupportTicketCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

// StartTicket transitions OPEN → IN_PROGRESS. Per Contract §43: this happens
// when a Platform Admin replies. Returns Conflict if not OPEN.
func (r *SupportRepository) StartTicket(ctx context.Context, ticketID, startedBy string, now time.Time) (ports.SupportTicketRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportTicketRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(ticketID) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.start", "ticket id is required")
	}
	if strings.TrimSpace(startedBy) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.start", "started_by is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportTicketRecord{}, err
	}
	var record ports.SupportTicketRecord
	err = executor.QueryRow(ctx,
		`UPDATE support_tickets SET status = 'IN_PROGRESS', updated_at = $2
		 WHERE id = $1::uuid AND status = 'OPEN'
		 RETURNING `+supportTicketSelectColumns,
		ticketID, now,
	).Scan(
		&record.ID, &record.BusinessID, &record.Subject, &record.Status, &record.Priority, &record.Category,
		&record.CreatedBy, &record.CreatedByType,
		&record.ResolvedAt, &record.ResolvedBy,
		&record.ClosedAt, &record.ClosedBy,
		&record.LastMessageAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.GetTicketByID(ctx, ticketID)
			if getErr != nil {
				return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.start", Kind: RepositoryNotFound, Err: getErr}
			}
			return ports.SupportTicketRecord{}, &RepositoryError{
				Operation: "support_ticket.start",
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("ticket %s is in status %s (cannot transition to IN_PROGRESS)", ticketID, current.Status),
			}
		}
		return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.start", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

// ResolveTicket transitions IN_PROGRESS | WAITING_MERCHANT → RESOLVED.
// Per Contract §43: support has fixed the issue. Returns Conflict if not
// in a resolvable state.
func (r *SupportRepository) ResolveTicket(ctx context.Context, ticketID, resolvedBy string, now time.Time) (ports.SupportTicketRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportTicketRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(ticketID) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.resolve", "ticket id is required")
	}
	if strings.TrimSpace(resolvedBy) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.resolve", "resolved_by is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportTicketRecord{}, err
	}
	var record ports.SupportTicketRecord
	err = executor.QueryRow(ctx,
		`UPDATE support_tickets
		 SET status = 'RESOLVED', resolved_at = $2, resolved_by = $3::uuid, updated_at = $2
		 WHERE id = $1::uuid AND status IN ('IN_PROGRESS', 'WAITING_MERCHANT')
		 RETURNING `+supportTicketSelectColumns,
		ticketID, now, resolvedBy,
	).Scan(
		&record.ID, &record.BusinessID, &record.Subject, &record.Status, &record.Priority, &record.Category,
		&record.CreatedBy, &record.CreatedByType,
		&record.ResolvedAt, &record.ResolvedBy,
		&record.ClosedAt, &record.ClosedBy,
		&record.LastMessageAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.GetTicketByID(ctx, ticketID)
			if getErr != nil {
				return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.resolve", Kind: RepositoryNotFound, Err: getErr}
			}
			return ports.SupportTicketRecord{}, &RepositoryError{
				Operation: "support_ticket.resolve",
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("ticket %s is in status %s (cannot transition to RESOLVED)", ticketID, current.Status),
			}
		}
		return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.resolve", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

// CloseTicket transitions RESOLVED → CLOSED. Per Contract §39: CLOSED is
// terminal — no reopen. Returns Conflict if not RESOLVED.
func (r *SupportRepository) CloseTicket(ctx context.Context, ticketID, closedBy string, now time.Time) (ports.SupportTicketRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportTicketRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(ticketID) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.close", "ticket id is required")
	}
	if strings.TrimSpace(closedBy) == "" {
		return ports.SupportTicketRecord{}, invalidRepositoryInput("support_ticket.close", "closed_by is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportTicketRecord{}, err
	}
	var record ports.SupportTicketRecord
	err = executor.QueryRow(ctx,
		`UPDATE support_tickets
		 SET status = 'CLOSED', closed_at = $2, closed_by = $3::uuid, updated_at = $2
		 WHERE id = $1::uuid AND status = 'RESOLVED'
		 RETURNING `+supportTicketSelectColumns,
		ticketID, now, closedBy,
	).Scan(
		&record.ID, &record.BusinessID, &record.Subject, &record.Status, &record.Priority, &record.Category,
		&record.CreatedBy, &record.CreatedByType,
		&record.ResolvedAt, &record.ResolvedBy,
		&record.ClosedAt, &record.ClosedBy,
		&record.LastMessageAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.GetTicketByID(ctx, ticketID)
			if getErr != nil {
				return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.close", Kind: RepositoryNotFound, Err: getErr}
			}
			return ports.SupportTicketRecord{}, &RepositoryError{
				Operation: "support_ticket.close",
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("ticket %s is in status %s (cannot transition to CLOSED — only RESOLVED can)", ticketID, current.Status),
			}
		}
		return ports.SupportTicketRecord{}, &RepositoryError{Operation: "support_ticket.close", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

// ----------------------------------------------------------------------------
// Messages — append-only per Contract §42
// ----------------------------------------------------------------------------

const supportMessageSelectColumns = `id::text, ticket_id::text, business_id::text, author_type, author_id::text, body, created_at`

func scanSupportMessage(scanner interface {
	Scan(dest ...any) error
}, record *ports.SupportMessageRecord) error {
	return scanner.Scan(
		&record.ID, &record.TicketID, &record.BusinessID,
		&record.AuthorType, &record.AuthorID, &record.Body,
		&record.CreatedAt,
	)
}

// AppendMessage inserts a new message + bumps last_message_at on the ticket
// in the same SQL transaction. Per Contract §42: messages are append-only —
// there is NO Update / Delete method on this port.
func (r *SupportRepository) AppendMessage(ctx context.Context, create ports.SupportMessageCreate) (ports.SupportMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportMessageRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.TicketID) == "" || strings.TrimSpace(create.BusinessID) == "" {
		return ports.SupportMessageRecord{}, invalidRepositoryInput("support_message.append", "id, ticket_id, and business_id are required")
	}
	if strings.TrimSpace(create.Body) == "" {
		return ports.SupportMessageRecord{}, invalidRepositoryInput("support_message.append", "body is required")
	}
	if strings.TrimSpace(create.AuthorID) == "" {
		return ports.SupportMessageRecord{}, invalidRepositoryInput("support_message.append", "author_id is required")
	}
	authorType := strings.TrimSpace(strings.ToUpper(create.AuthorType))
	if authorType != "BUSINESS_USER" && authorType != "PLATFORM_ADMIN" {
		return ports.SupportMessageRecord{}, invalidRepositoryInput("support_message.append", "author_type must be BUSINESS_USER or PLATFORM_ADMIN")
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportMessageRecord{}, err
	}
	var record ports.SupportMessageRecord
	err = executor.QueryRow(ctx,
		`WITH inserted AS (
		   INSERT INTO support_messages (id, ticket_id, business_id, author_type, author_id, body, created_at)
		   VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::uuid, $6, $7)
		   RETURNING `+supportMessageSelectColumns+`
		 )
		 UPDATE support_tickets
		   SET last_message_at = (SELECT created_at FROM inserted), updated_at = $7
		   WHERE id = $2::uuid
		 RETURNING (SELECT `+supportMessageSelectColumns+` FROM inserted)`,
		create.ID, create.TicketID, create.BusinessID, authorType, create.AuthorID, create.Body, create.Now,
	).Scan(
		&record.ID, &record.TicketID, &record.BusinessID,
		&record.AuthorType, &record.AuthorID, &record.Body,
		&record.CreatedAt,
	)
	if err != nil {
		return ports.SupportMessageRecord{}, classifyRepositoryWriteError("support_message.append", err)
	}
	return record, nil
}

func (r *SupportRepository) ListMessages(ctx context.Context, filter ports.SupportMessageListFilter) (ports.SupportMessagePage, error) {
	if r == nil || r.adapter == nil {
		return ports.SupportMessagePage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	decoded, err := decodeSupportMessageCursor(filter.Cursor)
	if err != nil {
		return ports.SupportMessagePage{}, invalidRepositoryInput("support_message.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SupportMessagePage{}, err
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.CreatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx,
		`SELECT `+supportMessageSelectColumns+`
		 FROM support_messages
		 WHERE ($1 = '' OR ticket_id::text = $1)
		   AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3::uuid))
		 ORDER BY created_at DESC, id DESC
		 LIMIT $4`,
		strings.TrimSpace(filter.TicketID), cursorAt, cursorID, filter.Limit+1,
	)
	if err != nil {
		return ports.SupportMessagePage{}, &RepositoryError{Operation: "support_message.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.SupportMessageRecord, 0, filter.Limit)
	for rows.Next() {
		var record ports.SupportMessageRecord
		if err := scanSupportMessage(rows, &record); err != nil {
			return ports.SupportMessagePage{}, &RepositoryError{Operation: "support_message.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.SupportMessagePage{}, &RepositoryError{Operation: "support_message.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.SupportMessagePage{Items: items}
	if len(items) > filter.Limit {
		page.HasMore = true
		page.Items = items[:filter.Limit]
		page.NextCursor = encodeSupportMessageCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

// ----------------------------------------------------------------------------
// Cursors
// ----------------------------------------------------------------------------

type supportTicketCursor struct {
	CreatedAt time.Time
	ID        string
}

func encodeSupportTicketCursor(record ports.SupportTicketRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodeSupportTicketCursor(value string) (*supportTicketCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid support ticket cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid support ticket cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid support ticket cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, fmt.Errorf("invalid support ticket cursor")
	}
	return &supportTicketCursor{CreatedAt: createdAt, ID: parts[1]}, nil
}

type supportMessageCursor struct {
	CreatedAt time.Time
	ID        string
}

func encodeSupportMessageCursor(record ports.SupportMessageRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodeSupportMessageCursor(value string) (*supportMessageCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid support message cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid support message cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid support message cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, fmt.Errorf("invalid support message cursor")
	}
	return &supportMessageCursor{CreatedAt: createdAt, ID: parts[1]}, nil
}

var _ ports.SupportRepository = (*SupportRepository)(nil)
