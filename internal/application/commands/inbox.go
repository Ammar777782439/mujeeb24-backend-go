package commands

import "time"

type MarkConversationReadCommand struct {
	Meta           CommandMeta
	ConversationID ConversationID
}

type ConversationReadResult struct {
	MutationResult
	ConversationID    ConversationID
	LastReadMessageID *MessageID
}

type CannedReplyResult struct {
	MutationResult
	CannedReply CannedReplyView
}

type CreateCannedReplyCommand struct {
	Meta     CommandMeta
	Title    string
	Shortcut string
	Body     string
}

type UpdateCannedReplyCommand struct {
	Meta          CommandMeta
	CannedReplyID CannedReplyID
	Title         *string
	Shortcut      *string
	Body          *string
	Status        *string
}

type SendCannedReplyCommand struct {
	Meta           CommandMeta
	ConversationID ConversationID
	CannedReplyID  CannedReplyID
}

type AutomationRuleView struct {
	ID              AutomationRuleID
	BusinessID      BusinessID
	Name            string
	Status          string
	TriggerKind     string
	Conditions      []byte
	ActionKind      string
	ActionPayload   []byte
	Position        int
	ResourceVersion ResourceVersion
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AutomationRuleResult struct {
	MutationResult
	AutomationRule AutomationRuleView
}

type CreateAutomationRuleCommand struct {
	Meta          CommandMeta
	Name          string
	Conditions    []byte
	ActionKind    string
	ActionPayload []byte
	Position      int
}

type UpdateAutomationRuleCommand struct {
	Meta             CommandMeta
	AutomationRuleID AutomationRuleID
	Name             *string
	Status           *string
	Conditions       []byte
	ActionKind       *string
	ActionPayload    []byte
	Position         *int
}

type ApplyInboundAutomationCommand struct {
	BusinessID     BusinessID
	ConversationID ConversationID
	InboundEventID ID
	Channel        string
	Text           string
}

type MarkConversationReadHandler = CommandHandler[MarkConversationReadCommand, ConversationReadResult]
type CreateCannedReplyHandler = CommandHandler[CreateCannedReplyCommand, CannedReplyResult]
type UpdateCannedReplyHandler = CommandHandler[UpdateCannedReplyCommand, CannedReplyResult]
type SendCannedReplyHandler = CommandHandler[SendCannedReplyCommand, MessageResult]
type CreateAutomationRuleHandler = CommandHandler[CreateAutomationRuleCommand, AutomationRuleResult]
type UpdateAutomationRuleHandler = CommandHandler[UpdateAutomationRuleCommand, AutomationRuleResult]
type ApplyInboundAutomationHandler = CommandHandler[ApplyInboundAutomationCommand, EmptyResult]
