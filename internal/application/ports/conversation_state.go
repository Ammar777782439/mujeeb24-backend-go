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
        // Per ADR-039: Summary + Sliding Window hybrid context strategy.
        // Summary holds the LLM-generated running summary of older conversation
        // turns (anything older than the sliding window of recent messages).
        // SummaryTurnCount is the turn count at the time the summary was last
        // generated. The SummaryService regenerates the summary every N turns
        // (default N=4) and increments SummaryTurnCount accordingly.
        Summary            string
        SummaryTurnCount   int
        Version            int64
        CreatedAt          time.Time
        UpdatedAt          time.Time
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
