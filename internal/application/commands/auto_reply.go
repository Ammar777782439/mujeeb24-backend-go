package commands

type AutoReplyCommand struct {
	Meta                   CommandMeta
	ConversationID         ConversationID
	SourceMessageReference string
	// Only the trusted sales-action worker supplies these continuation fields.
	SalesStartContinuationID string
	SalesStartJobID          string
	SalesStartWorkflowID     string
	SalesStartLeaseToken     string
	Text                     string
	Channel                  string
	ProviderRef              string
}

type AutoReplyResult struct {
	SalesWorkflow     *SalesWorkflowOutcome
	Capability        string
	Decision          AIDecisionView
	Action            string
	OutboundMessageID MessageID
	OutboxEntryID     ID
	Enqueued          bool
}

type AutoReplyHandler = CommandHandler[AutoReplyCommand, AutoReplyResult]

var _ AutoReplyHandler = nil

type SalesWorkflowOutcome struct {
	WorkflowID      ID
	TransactionID   *string
	State           string
	ResourceVersion int64
}
