package ports

import (
	"context"
	"time"
)

type MerchantAISessionRecord struct {
	ID          string
	BusinessID  string
	PrincipalID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MerchantAIMessageRecord struct {
	ID         string
	BusinessID string
	SessionID  string
	SenderType string // "merchant" | "assistant"
	Text       string
	CreatedAt  time.Time
}

type MerchantAIMessageDraft struct {
	ID         string
	BusinessID string
	SessionID  string
	SenderType string
	Text       string
	CreatedAt  time.Time
}

type MerchantAISessionRepository interface {
	CreateSession(ctx context.Context, session MerchantAISessionRecord) (MerchantAISessionRecord, error)
	GetSession(ctx context.Context, businessID, principalID, sessionID string) (MerchantAISessionRecord, error)
	AppendMessage(ctx context.Context, draft MerchantAIMessageDraft) (MerchantAIMessageRecord, error)
	ListRecentMessages(ctx context.Context, businessID, sessionID string, limit int) ([]MerchantAIMessageRecord, error)
	TouchSession(ctx context.Context, businessID, sessionID string, updatedAt time.Time) error
}
