package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ConversationReadCursorRepository struct{ adapter *Adapter }

func NewConversationReadCursorRepository(adapter *Adapter) *ConversationReadCursorRepository {
	return &ConversationReadCursorRepository{adapter: adapter}
}

func (r *ConversationReadCursorRepository) MarkRead(ctx context.Context, businessID, conversationID, principalID string, readAt time.Time) (ports.ConversationReadCursor, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationReadCursor{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(conversationID) == "" || strings.TrimSpace(principalID) == "" || readAt.IsZero() {
		return ports.ConversationReadCursor{}, invalidRepositoryInput("conversation_read_cursor.mark_read", "business, conversation, principal ids, and read time are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationReadCursor{}, err
	}
	const query = `WITH target AS (
		SELECT id FROM communication_messages
		WHERE business_id = $1::uuid
		  AND conversation_reference_id IN (
			SELECT id FROM conversation_references
			WHERE business_id = $1::uuid AND conversation_id = $2::uuid AND is_current
		  )
		  AND visibility = 'public'
		ORDER BY occurred_at DESC, id DESC
		LIMIT 1
	), conversation_exists AS (
		SELECT 1 FROM conversations WHERE business_id = $1::uuid AND id = $2::uuid
	)
	INSERT INTO conversation_read_cursors (
		business_id, conversation_id, principal_id, last_read_message_id, read_at, created_at, updated_at
	)
	SELECT $1::uuid, $2::uuid, $3::uuid, target.id, $4::timestamptz, $4::timestamptz, $4::timestamptz
	FROM conversation_exists
	LEFT JOIN target ON true
	ON CONFLICT (business_id, conversation_id, principal_id) DO UPDATE
	SET last_read_message_id = EXCLUDED.last_read_message_id,
		read_at = EXCLUDED.read_at,
		updated_at = EXCLUDED.updated_at
	RETURNING business_id::text, conversation_id::text, principal_id::text, last_read_message_id::text, read_at, created_at, updated_at`
	var result ports.ConversationReadCursor
	err = executor.QueryRow(ctx, query, businessID, conversationID, principalID, readAt.UTC()).Scan(
		&result.BusinessID, &result.ConversationID, &result.PrincipalID, &result.LastReadMessageID, &result.ReadAt, &result.CreatedAt, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConversationReadCursor{}, &RepositoryError{Operation: "conversation_read_cursor.mark_read", Kind: RepositoryNotFound, Err: err}
	}
	if err != nil {
		return ports.ConversationReadCursor{}, classifyRepositoryWriteError("conversation_read_cursor.mark_read", err)
	}
	return result, nil
}

type CannedReplyRepository struct{ adapter *Adapter }

func NewCannedReplyRepository(adapter *Adapter) *CannedReplyRepository {
	return &CannedReplyRepository{adapter: adapter}
}

type cannedReplyCursor struct {
	UpdatedAt time.Time
	ID        string
}

func (r *CannedReplyRepository) List(ctx context.Context, businessID, status string, limit int, cursor string) (ports.CannedReplyPage, error) {
	if r == nil || r.adapter == nil {
		return ports.CannedReplyPage{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.CannedReplyPage{}, invalidRepositoryInput("canned_reply.list", "business id is required")
	}
	if status != "" && status != "active" && status != "archived" {
		return ports.CannedReplyPage{}, invalidRepositoryInput("canned_reply.list", "status must be active or archived")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 10000 {
		return ports.CannedReplyPage{}, invalidRepositoryInput("canned_reply.list", "limit must not exceed 10000")
	}
	decoded, err := decodeCannedReplyCursor(cursor)
	if err != nil {
		return ports.CannedReplyPage{}, invalidRepositoryInput("canned_reply.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CannedReplyPage{}, err
	}
	var cursorAt, cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.UpdatedAt, decoded.ID
	}
	const query = `SELECT id::text, business_id::text, title, shortcut, body, status, resource_version, created_at, updated_at
	FROM canned_replies
	WHERE business_id = $1::uuid
	  AND ($2 = '' OR status = $2)
	  AND ($3::timestamptz IS NULL OR (updated_at, id) < ($3::timestamptz, $4::uuid))
	ORDER BY updated_at DESC, id DESC
	LIMIT $5`
	rows, err := executor.Query(ctx, query, businessID, status, cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.CannedReplyPage{}, &RepositoryError{Operation: "canned_reply.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.CannedReplyRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanCannedReply(rows)
		if scanErr != nil {
			return ports.CannedReplyPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.CannedReplyPage{}, &RepositoryError{Operation: "canned_reply.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.CannedReplyPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeCannedReplyCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CannedReplyRepository) GetByID(ctx context.Context, businessID, cannedReplyID string) (ports.CannedReplyRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CannedReplyRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(cannedReplyID) == "" {
		return ports.CannedReplyRecord{}, invalidRepositoryInput("canned_reply.get", "business and canned reply ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CannedReplyRecord{}, err
	}
	record, err := scanCannedReply(executor.QueryRow(ctx, `SELECT id::text, business_id::text, title, shortcut, body, status, resource_version, created_at, updated_at FROM canned_replies WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, cannedReplyID))
	if err != nil {
		return ports.CannedReplyRecord{}, classifyRepositoryGetError("canned_reply.get", err)
	}
	return record, nil
}

func (r *CannedReplyRepository) Create(ctx context.Context, create ports.CannedReplyCreate) (ports.CannedReplyRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CannedReplyRecord{}, ErrPoolClosed
	}
	if err := validateCannedReplyCreate(create); err != nil {
		return ports.CannedReplyRecord{}, err
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CannedReplyRecord{}, err
	}
	record, err := scanCannedReply(executor.QueryRow(ctx, `INSERT INTO canned_replies (id, business_id, title, shortcut, body, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8) RETURNING id::text, business_id::text, title, shortcut, body, status, resource_version, created_at, updated_at`, create.ID, create.BusinessID, create.Title, create.Shortcut, create.Body, create.Status, create.CreatedAt.UTC(), create.UpdatedAt.UTC()))
	if err != nil {
		return ports.CannedReplyRecord{}, classifyRepositoryWriteError("canned_reply.create", err)
	}
	return record, nil
}

func (r *CannedReplyRepository) Update(ctx context.Context, update ports.CannedReplyUpdate) (ports.CannedReplyRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CannedReplyRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(update.BusinessID) == "" || strings.TrimSpace(update.CannedReplyID) == "" || update.ExpectedVersion <= 0 || update.UpdatedAt.IsZero() {
		return ports.CannedReplyRecord{}, invalidRepositoryInput("canned_reply.update", "business, canned reply, expected resource version, and update time are required")
	}
	if update.Title == nil && update.Shortcut == nil && update.Body == nil && update.Status == nil {
		return ports.CannedReplyRecord{}, invalidRepositoryInput("canned_reply.update", "at least one field is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CannedReplyRecord{}, err
	}
	record, err := scanCannedReply(executor.QueryRow(ctx, `UPDATE canned_replies SET title = COALESCE($4, title), shortcut = COALESCE($5, shortcut), body = COALESCE($6, body), status = COALESCE($7, status), resource_version = resource_version + 1, updated_at = $8 WHERE business_id = $1::uuid AND id = $2::uuid AND resource_version = $3 RETURNING id::text, business_id::text, title, shortcut, body, status, resource_version, created_at, updated_at`, update.BusinessID, update.CannedReplyID, update.ExpectedVersion, update.Title, update.Shortcut, update.Body, update.Status, update.UpdatedAt.UTC()))
	if err == nil {
		return record, nil
	}
	return ports.CannedReplyRecord{}, classifyCannedReplyUpdateMiss(ctx, executor, update, err)
}

func validateCannedReplyCreate(create ports.CannedReplyCreate) error {
	if uuid.Validate(create.ID) != nil || strings.TrimSpace(create.BusinessID) == "" || strings.TrimSpace(create.Title) == "" || strings.TrimSpace(create.Shortcut) == "" || strings.TrimSpace(create.Body) == "" || (create.Status != "active" && create.Status != "archived") || create.CreatedAt.IsZero() || create.UpdatedAt.IsZero() {
		return invalidRepositoryInput("canned_reply.create", "valid id, business, title, shortcut, body, status, and timestamps are required")
	}
	return nil
}

func classifyCannedReplyUpdateMiss(ctx context.Context, executor SQLExecutor, update ports.CannedReplyUpdate, original error) error {
	if !errors.Is(original, pgx.ErrNoRows) {
		return classifyRepositoryWriteError("canned_reply.update", original)
	}
	var version int64
	err := executor.QueryRow(ctx, `SELECT resource_version FROM canned_replies WHERE business_id = $1::uuid AND id = $2::uuid`, update.BusinessID, update.CannedReplyID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: "canned_reply.update", Kind: RepositoryNotFound, Err: err}
	}
	if err != nil {
		return classifyRepositoryWriteError("canned_reply.update", err)
	}
	if version != update.ExpectedVersion {
		return &RepositoryError{Operation: "canned_reply.update", Kind: RepositoryStale, Err: errors.New("canned reply resource version is stale")}
	}
	return &RepositoryError{Operation: "canned_reply.update", Kind: RepositoryConflict, Err: errors.New("canned reply update was rejected")}
}

type cannedReplyScanner interface {
	Scan(dest ...any) error
}

func scanCannedReply(scanner cannedReplyScanner) (ports.CannedReplyRecord, error) {
	var record ports.CannedReplyRecord
	if err := scanner.Scan(&record.ID, &record.BusinessID, &record.Title, &record.Shortcut, &record.Body, &record.Status, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return ports.CannedReplyRecord{}, err
	}
	return record, nil
}

func encodeCannedReplyCursor(record ports.CannedReplyRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodeCannedReplyCursor(value string) (*cannedReplyCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid canned reply cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 || uuid.Validate(parts[1]) != nil {
		return nil, errors.New("invalid canned reply cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid canned reply cursor")
	}
	return &cannedReplyCursor{UpdatedAt: updatedAt, ID: parts[1]}, nil
}

var _ ports.ConversationReadCursorRepository = (*ConversationReadCursorRepository)(nil)
var _ ports.CannedReplyRepository = (*CannedReplyRepository)(nil)
