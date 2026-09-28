package commands

type BeginChannelConnectionCommand struct {
	Meta        CommandMeta
	Provider    string
	Channel     string
	DisplayName string
}
type ReconnectChannelCommand struct {
	Meta         CommandMeta
	ConnectionID ConnectionID
	Reason       string
}
type DisconnectChannelCommand struct {
	Meta         CommandMeta
	ConnectionID ConnectionID
	Reason       string
}
type BeginChannelConnectionResult struct {
	MutationResult
	Connection   ChannelConnectionView
	Provisioning ChannelProvisioningView
}
type ChannelConnectionResult struct {
	MutationResult
	Connection ChannelConnectionView
}
type BeginChannelConnectionHandler = CommandHandler[BeginChannelConnectionCommand, BeginChannelConnectionResult]
type ReconnectChannelHandler = CommandHandler[ReconnectChannelCommand, ChannelConnectionResult]
type DisconnectChannelHandler = CommandHandler[DisconnectChannelCommand, ChannelConnectionResult]

type UpdateConversationCommand struct {
	Meta           CommandMeta
	ConversationID ConversationID
	State          *string
	Ownership      *string
	AIModeOverride *string
	Priority       *string
}
type AssignConversationCommand struct {
	Meta                CommandMeta
	ConversationID      ConversationID
	AssigneePrincipalID PrincipalID
}
type UpdateConversationLabelsCommand struct {
	Meta           CommandMeta
	ConversationID ConversationID
	Add            []string
	Remove         []string
}
type AddPrivateNoteCommand struct {
	Meta           CommandMeta
	ConversationID ConversationID
	Text           string
}
type CreateOutboundMessageCommand struct {
	Meta           CommandMeta
	ConversationID ConversationID
	Text           string
}
type ConversationResult struct {
	MutationResult
	Conversation ConversationView
}
type MessageResult struct {
	MutationResult
	Message MessageView
}
type UpdateConversationHandler = CommandHandler[UpdateConversationCommand, ConversationResult]
type AssignConversationHandler = CommandHandler[AssignConversationCommand, ConversationResult]
type UpdateConversationLabelsHandler = CommandHandler[UpdateConversationLabelsCommand, ConversationResult]
type AddPrivateNoteHandler = CommandHandler[AddPrivateNoteCommand, MessageResult]
type CreateOutboundMessageHandler = CommandHandler[CreateOutboundMessageCommand, MessageResult]
