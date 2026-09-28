package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type KnowledgeDocumentRepository struct{ adapter *Adapter }

type BusinessPolicyRepository struct{ adapter *Adapter }

func NewKnowledgeDocumentRepository(adapter *Adapter) *KnowledgeDocumentRepository {
	return &KnowledgeDocumentRepository{adapter: adapter}
}

func NewBusinessPolicyRepository(adapter *Adapter) *BusinessPolicyRepository {
	return &BusinessPolicyRepository{adapter: adapter}
}

func (r *KnowledgeDocumentRepository) ListPublished(ctx context.Context, businessID, search string, now time.Time, limit int) ([]ports.KnowledgeDocumentRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return nil, &RepositoryError{Operation: "knowledge.list_published", Kind: RepositoryInvalid, Err: errors.New("business id is required")}
	}
	limit = boundedKnowledgePolicyLimit(limit)
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, `
		SELECT id::text, business_id::text, knowledge_key, title, content, content_type,
		       source_reference, authority, status, version, valid_from, valid_until,
		       created_at, updated_at
		FROM knowledge_documents
		WHERE business_id = $1::uuid
		  AND status = 'published'
		  AND valid_from <= $3
		  AND ($2 = '' OR knowledge_key ILIKE '%' || $2 || '%' OR title ILIKE '%' || $2 || '%' OR content ILIKE '%' || $2 || '%')
		ORDER BY updated_at DESC, id DESC
		LIMIT $4`, businessID, strings.TrimSpace(search), now.UTC(), limit)
	if err != nil {
		return nil, &RepositoryError{Operation: "knowledge.list_published", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	result := make([]ports.KnowledgeDocumentRecord, 0, limit)
	for rows.Next() {
		var record ports.KnowledgeDocumentRecord
		if err := rows.Scan(&record.ID, &record.BusinessID, &record.KnowledgeKey, &record.Title, &record.Content, &record.ContentType, &record.SourceReference, &record.Authority, &record.Status, &record.Version, &record.ValidFrom, &record.ValidUntil, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, &RepositoryError{Operation: "knowledge.list_published", Kind: RepositoryInvalid, Err: err}
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "knowledge.list_published", Kind: RepositoryInvalid, Err: err}
	}
	return result, nil
}

func (r *BusinessPolicyRepository) ListPublished(ctx context.Context, businessID, search string, now time.Time, limit int) ([]ports.BusinessPolicyRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return nil, &RepositoryError{Operation: "business_policy.list_published", Kind: RepositoryInvalid, Err: errors.New("business id is required")}
	}
	limit = boundedKnowledgePolicyLimit(limit)
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, `
		SELECT id::text, business_id::text, policy_key, category, title, summary, rules,
		       authority, status, version, valid_from, valid_until, created_at, updated_at
		FROM business_policy_versions
		WHERE business_id = $1::uuid
		  AND status = 'published'
		  AND valid_from <= $3
		  AND ($2 = '' OR policy_key ILIKE '%' || $2 || '%' OR category ILIKE '%' || $2 || '%' OR title ILIKE '%' || $2 || '%' OR summary ILIKE '%' || $2 || '%')
		ORDER BY updated_at DESC, id DESC
		LIMIT $4`, businessID, strings.TrimSpace(search), now.UTC(), limit)
	if err != nil {
		return nil, &RepositoryError{Operation: "business_policy.list_published", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	result := make([]ports.BusinessPolicyRecord, 0, limit)
	for rows.Next() {
		var record ports.BusinessPolicyRecord
		if err := rows.Scan(&record.ID, &record.BusinessID, &record.PolicyKey, &record.Category, &record.Title, &record.Summary, &record.Rules, &record.Authority, &record.Status, &record.Version, &record.ValidFrom, &record.ValidUntil, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, &RepositoryError{Operation: "business_policy.list_published", Kind: RepositoryInvalid, Err: err}
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "business_policy.list_published", Kind: RepositoryInvalid, Err: err}
	}
	return result, nil
}

func boundedKnowledgePolicyLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 10000 {
		return 100
	}
	return limit
}

var _ ports.KnowledgeDocumentRepository = (*KnowledgeDocumentRepository)(nil)
var _ ports.BusinessPolicyRepository = (*BusinessPolicyRepository)(nil)
var _ = pgx.ErrNoRows
