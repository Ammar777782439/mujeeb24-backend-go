package ports

import (
	"context"
	"time"
)

type KnowledgeDocumentRecord struct {
	ID              string
	BusinessID      string
	KnowledgeKey    string
	Title           string
	Content         string
	ContentType     string
	SourceReference string
	Authority       string
	Status          string
	Version         int
	ValidFrom       time.Time
	ValidUntil      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type BusinessPolicyRecord struct {
	ID         string
	BusinessID string
	PolicyKey  string
	Category   string
	Title      string
	Summary    string
	Rules      []byte
	Authority  string
	Status     string
	Version    int
	ValidFrom  time.Time
	ValidUntil *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type KnowledgeDocumentRepository interface {
	ListPublished(ctx context.Context, businessID, search string, now time.Time, limit int) ([]KnowledgeDocumentRecord, error)
}

type BusinessPolicyRepository interface {
	ListPublished(ctx context.Context, businessID, search string, now time.Time, limit int) ([]BusinessPolicyRecord, error)
}
