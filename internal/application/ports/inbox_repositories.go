package ports

import (
	"context"
	"time"
)

// ConversationReadCursor records one authenticated member's last read point
// inside one tenant-scoped conversation. It never changes message visibility.
type ConversationReadCursor struct {
	BusinessID        string
	ConversationID    string
	PrincipalID       string
	LastReadMessageID *string
	ReadAt            time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ConversationReadCursorRepository interface {
	MarkRead(ctx context.Context, businessID, conversationID, principalID string, readAt time.Time) (ConversationReadCursor, error)
}

// CannedReplyRecord is a Mujeeb-owned reusable text response. Saving one has
// no provider side effect; delivery can only happen through OutboundMessage.
type CannedReplyRecord struct {
	ID              string
	BusinessID      string
	Title           string
	Shortcut        string
	Body            string
	Status          string
	ResourceVersion int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CannedReplyPage struct {
	Items      []CannedReplyRecord
	NextCursor string
	HasMore    bool
}

type CannedReplyCreate struct {
	ID         string
	BusinessID string
	Title      string
	Shortcut   string
	Body       string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CannedReplyUpdate struct {
	BusinessID      string
	CannedReplyID   string
	ExpectedVersion int64
	Title           *string
	Shortcut        *string
	Body            *string
	Status          *string
	UpdatedAt       time.Time
}

type CannedReplyRepository interface {
	List(ctx context.Context, businessID, status string, limit int, cursor string) (CannedReplyPage, error)
	GetByID(ctx context.Context, businessID, cannedReplyID string) (CannedReplyRecord, error)
	Create(ctx context.Context, create CannedReplyCreate) (CannedReplyRecord, error)
	Update(ctx context.Context, update CannedReplyUpdate) (CannedReplyRecord, error)
}

// AutomationRuleRecord is intentionally restricted to inbound-message rules.
// Conditions and actions are JSON objects validated by the application layer;
// they are not an expression language and cannot invoke external services.
type AutomationRuleRecord struct {
	ID              string
	BusinessID      string
	Name            string
	Status          string
	TriggerKind     string
	Conditions      []byte
	ActionKind      string
	ActionPayload   []byte
	Position        int
	ResourceVersion int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AutomationRulePage struct {
	Items      []AutomationRuleRecord
	NextCursor string
	HasMore    bool
}

type AutomationRuleCreate struct {
	ID            string
	BusinessID    string
	Name          string
	Status        string
	TriggerKind   string
	Conditions    []byte
	ActionKind    string
	ActionPayload []byte
	Position      int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AutomationRuleUpdate struct {
	BusinessID       string
	AutomationRuleID string
	ExpectedVersion  int64
	Name             *string
	Status           *string
	Conditions       []byte
	ActionKind       *string
	ActionPayload    []byte
	Position         *int
	UpdatedAt        time.Time
}

type AutomationExecutionDraft struct {
	ID             string
	BusinessID     string
	RuleID         string
	InboundEventID string
	Result         string
	ReasonCode     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AutomationExecutionPatch struct {
	ID         string
	Result     string
	ReasonCode string
	UpdatedAt  time.Time
}

type AutomationRuleRepository interface {
	List(ctx context.Context, businessID, status string, limit int, cursor string) (AutomationRulePage, error)
	ListActiveInbound(ctx context.Context, businessID string) ([]AutomationRuleRecord, error)
	GetByID(ctx context.Context, businessID, automationRuleID string) (AutomationRuleRecord, error)
	Create(ctx context.Context, create AutomationRuleCreate) (AutomationRuleRecord, error)
	Update(ctx context.Context, update AutomationRuleUpdate) (AutomationRuleRecord, error)
}

type AutomationExecutionRepository interface {
	RecordIfAbsent(ctx context.Context, draft AutomationExecutionDraft) (created bool, record AutomationExecutionDraft, err error)
	Complete(ctx context.Context, patch AutomationExecutionPatch) error
}
