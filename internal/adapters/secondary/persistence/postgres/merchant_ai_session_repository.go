package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type MerchantAISessionRepository struct {
	adapter *Adapter
}

func NewMerchantAISessionRepository(adapter *Adapter) *MerchantAISessionRepository {
	return &MerchantAISessionRepository{adapter: adapter}
}

func (r *MerchantAISessionRepository) CreateSession(ctx context.Context, session ports.MerchantAISessionRecord) (ports.MerchantAISessionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.MerchantAISessionRecord{}, &RepositoryError{Operation: "merchant_ai_session.create", Kind: RepositoryInvalid, Err: errors.New("repository is not configured")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.MerchantAISessionRecord{}, &RepositoryError{Operation: "merchant_ai_session.create", Kind: RepositoryInvalid, Err: err}
	}
	id := strings.TrimSpace(session.ID)
	businessID := strings.TrimSpace(session.BusinessID)
	principalID := strings.TrimSpace(session.PrincipalID)
	if id == "" || businessID == "" || principalID == "" {
		return ports.MerchantAISessionRecord{}, invalidRepositoryInput("merchant_ai_session.create", "id, business_id, and principal_id are required")
	}

	createdAt := session.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := session.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	const query = `INSERT INTO merchant_ai_sessions (id, business_id, principal_id, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
RETURNING id, business_id, principal_id, created_at, updated_at`

	var rec ports.MerchantAISessionRecord
	scanErr := executor.QueryRow(ctx, query, id, businessID, principalID, createdAt, updatedAt).Scan(
		&rec.ID, &rec.BusinessID, &rec.PrincipalID, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if scanErr != nil {
		return ports.MerchantAISessionRecord{}, classifyRepositoryWriteError("merchant_ai_session.create", scanErr)
	}
	return rec, nil
}

func (r *MerchantAISessionRepository) GetSession(ctx context.Context, businessID, principalID, sessionID string) (ports.MerchantAISessionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.MerchantAISessionRecord{}, &RepositoryError{Operation: "merchant_ai_session.get", Kind: RepositoryInvalid, Err: errors.New("repository is not configured")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.MerchantAISessionRecord{}, &RepositoryError{Operation: "merchant_ai_session.get", Kind: RepositoryInvalid, Err: err}
	}
	businessID = strings.TrimSpace(businessID)
	principalID = strings.TrimSpace(principalID)
	sessionID = strings.TrimSpace(sessionID)
	if businessID == "" || principalID == "" || sessionID == "" {
		return ports.MerchantAISessionRecord{}, invalidRepositoryInput("merchant_ai_session.get", "business_id, principal_id, and session_id are required")
	}

	const query = `SELECT id, business_id, principal_id, created_at, updated_at
FROM merchant_ai_sessions
WHERE id = $1::uuid AND business_id = $2::uuid AND principal_id = $3::uuid`

	var rec ports.MerchantAISessionRecord
	scanErr := executor.QueryRow(ctx, query, sessionID, businessID, principalID).Scan(
		&rec.ID, &rec.BusinessID, &rec.PrincipalID, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if scanErr != nil {
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return ports.MerchantAISessionRecord{}, &RepositoryError{Operation: "merchant_ai_session.get", Kind: RepositoryNotFound, Err: scanErr}
		}
		return ports.MerchantAISessionRecord{}, &RepositoryError{Operation: "merchant_ai_session.get", Kind: RepositoryInvalid, Err: scanErr}
	}
	return rec, nil
}

func (r *MerchantAISessionRepository) AppendMessage(ctx context.Context, draft ports.MerchantAIMessageDraft) (ports.MerchantAIMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.MerchantAIMessageRecord{}, &RepositoryError{Operation: "merchant_ai_message.append", Kind: RepositoryInvalid, Err: errors.New("repository is not configured")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.MerchantAIMessageRecord{}, &RepositoryError{Operation: "merchant_ai_message.append", Kind: RepositoryInvalid, Err: err}
	}
	id := strings.TrimSpace(draft.ID)
	businessID := strings.TrimSpace(draft.BusinessID)
	sessionID := strings.TrimSpace(draft.SessionID)
	text := strings.TrimSpace(draft.Text)
	senderType := strings.ToLower(strings.TrimSpace(draft.SenderType))

	if id == "" || businessID == "" || sessionID == "" || text == "" {
		return ports.MerchantAIMessageRecord{}, invalidRepositoryInput("merchant_ai_message.append", "id, business_id, session_id, and text are required")
	}
	if senderType != "merchant" && senderType != "assistant" {
		return ports.MerchantAIMessageRecord{}, invalidRepositoryInput("merchant_ai_message.append", "sender_type must be merchant or assistant")
	}

	createdAt := draft.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	const query = `INSERT INTO merchant_ai_messages (id, business_id, session_id, sender_type, text, created_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6)
RETURNING id, business_id, session_id, sender_type, text, created_at`

	var rec ports.MerchantAIMessageRecord
	scanErr := executor.QueryRow(ctx, query, id, businessID, sessionID, senderType, text, createdAt).Scan(
		&rec.ID, &rec.BusinessID, &rec.SessionID, &rec.SenderType, &rec.Text, &rec.CreatedAt,
	)
	if scanErr != nil {
		return ports.MerchantAIMessageRecord{}, classifyRepositoryWriteError("merchant_ai_message.append", scanErr)
	}
	return rec, nil
}

func (r *MerchantAISessionRepository) ListRecentMessages(ctx context.Context, businessID, sessionID string, limit int) ([]ports.MerchantAIMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, &RepositoryError{Operation: "merchant_ai_message.list_recent", Kind: RepositoryInvalid, Err: errors.New("repository is not configured")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, &RepositoryError{Operation: "merchant_ai_message.list_recent", Kind: RepositoryInvalid, Err: err}
	}
	businessID = strings.TrimSpace(businessID)
	sessionID = strings.TrimSpace(sessionID)
	if businessID == "" || sessionID == "" {
		return nil, invalidRepositoryInput("merchant_ai_message.list_recent", "business_id and session_id are required")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	const query = `SELECT id, business_id, session_id, sender_type, text, created_at
FROM merchant_ai_messages
WHERE business_id = $1::uuid AND session_id = $2::uuid
ORDER BY created_at DESC
LIMIT $3`

	rows, err := executor.Query(ctx, query, businessID, sessionID, limit)
	if err != nil {
		return nil, &RepositoryError{Operation: "merchant_ai_message.list_recent", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()

	var descList []ports.MerchantAIMessageRecord
	for rows.Next() {
		var rec ports.MerchantAIMessageRecord
		if err := rows.Scan(&rec.ID, &rec.BusinessID, &rec.SessionID, &rec.SenderType, &rec.Text, &rec.CreatedAt); err != nil {
			return nil, &RepositoryError{Operation: "merchant_ai_message.list_recent", Kind: RepositoryInvalid, Err: err}
		}
		descList = append(descList, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "merchant_ai_message.list_recent", Kind: RepositoryInvalid, Err: err}
	}

	// Reverse to chronological order (oldest to newest)
	count := len(descList)
	chronoList := make([]ports.MerchantAIMessageRecord, count)
	for i, msg := range descList {
		chronoList[count-1-i] = msg
	}
	return chronoList, nil
}

func (r *MerchantAISessionRepository) TouchSession(ctx context.Context, businessID, sessionID string, updatedAt time.Time) error {
	if r == nil || r.adapter == nil {
		return &RepositoryError{Operation: "merchant_ai_session.touch", Kind: RepositoryInvalid, Err: errors.New("repository is not configured")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return &RepositoryError{Operation: "merchant_ai_session.touch", Kind: RepositoryInvalid, Err: err}
	}
	businessID = strings.TrimSpace(businessID)
	sessionID = strings.TrimSpace(sessionID)
	if businessID == "" || sessionID == "" {
		return invalidRepositoryInput("merchant_ai_session.touch", "business_id and session_id are required")
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	const query = `UPDATE merchant_ai_sessions SET updated_at = $1 WHERE id = $2::uuid AND business_id = $3::uuid`
	cmdTag, err := executor.Exec(ctx, query, updatedAt.UTC(), sessionID, businessID)
	if err != nil {
		return classifyRepositoryWriteError("merchant_ai_session.touch", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return &RepositoryError{Operation: "merchant_ai_session.touch", Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
	}
	return nil
}

var _ ports.MerchantAISessionRepository = (*MerchantAISessionRepository)(nil)
