package ports

import (
	"context"
	"time"
)

type ConversationFocus struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	CatalogID *string `json:"catalog_id,omitempty"`
	ItemID    *string `json:"item_id,omitempty"`
	Name      *string `json:"name,omitempty"`
}

type ConversationStateRecord struct {
	BusinessID     string
	ConversationID string
	Focus          *ConversationFocus
	Previous       []ConversationFocus
	Comparison     *ConversationComparison
	Preferences    []StatePreference
	Constraints    []StateConstraint
	Pending        []StatePending
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ConversationComparison struct {
	Type string   `json:"type"`
	IDs  []string `json:"ids"`
}

type StatePreference struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

type StateConstraint struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

type StatePending struct {
	Field string `json:"field"`
	Kind  string `json:"kind"`
}

type ConversationStateRepository interface {
	Get(ctx context.Context, businessID, conversationID string) (ConversationStateRecord, error)
	UpsertValidated(ctx context.Context, record ConversationStateRecord) (ConversationStateRecord, error)
}
