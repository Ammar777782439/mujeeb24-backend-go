package ports

import "context"

type ConversationAccess struct {
	PrincipalID string
	Supervisor  bool
}

type ConversationInboxFilter struct {
	BusinessID string
	Access     ConversationAccess
	Scope      string
	State      string
	Ownership  string
	Channel    string
	Search     string
	CustomerID *string
	Limit      int
	Cursor     string
}

type ConversationInboxOverview struct {
	Total        int
	AssignedToMe int
	Unassigned   int
	StateCounts  map[string]int
}

// GetInboxConversation applies the same visibility rule as ListInbox. Locked
// reads require the caller's transaction and serialize sending vs reassignment.
type ConversationInboxRepository interface {
	ListInbox(context.Context, ConversationInboxFilter) (ConversationPage, error)
	InboxOverview(context.Context, ConversationInboxFilter) (ConversationInboxOverview, error)
	GetInboxConversation(ctx context.Context, businessID, conversationID string, access ConversationAccess, forUpdate bool) (ConversationRecord, error)
}
