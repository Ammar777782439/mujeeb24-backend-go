package commands

type AutoReplyCommand struct {
	Meta                   CommandMeta
	ConversationID         ConversationID
	SourceMessageReference string
	Text                   string
	Channel                string
	ProviderRef            string
}

type AutoReplyResult struct {
	Decision          AIDecisionView
	Action            string
	OutboundMessageID MessageID
	OutboxEntryID     ID
	Enqueued          bool
}

type AutoReplyHandler = CommandHandler[AutoReplyCommand, AutoReplyResult]

var _ AutoReplyHandler = nil

type ChatwootAutoReplyCommand struct {
	Meta                   CommandMeta
	AccountID              string
	InboxID                string
	ChatwootConversationID string
	MujeebConversationID   ConversationID
	SourceMessageReference string
	Text                   string
}

type ChatwootAutoReplyResult struct {
	Executed      bool
	Blocked       bool
	BlockedReason string
	AutoReply     AutoReplyResult
}
