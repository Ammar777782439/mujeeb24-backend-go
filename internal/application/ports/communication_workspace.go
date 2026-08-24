package ports

import (
	"context"
	"time"
)

type CommunicationWorkspace interface {
	CreateContact(context.Context, WorkspaceContactDraft) (WorkspaceContact, error)
	CreateConversation(context.Context, WorkspaceConversationDraft) (WorkspaceConversation, error)
	CreateMessage(context.Context, WorkspaceMessageDraft) (WorkspaceMessage, error)
}

type WorkspaceContactDraft struct {
	AccountID       int64
	InboxID         int64
	Name            string
	Identifier      string
	PhoneNumber     string
	AvatarURL       string
	AdditionalAttrs []byte
}

type WorkspaceContact struct {
	ID         int64
	AccountID  int64
	InboxID    int64
	Identifier string
}

type WorkspaceConversationDraft struct {
	AccountID       int64
	InboxID         int64
	ContactID       int64
	SourceID        string
	InitialText     string
	Status          string
	AdditionalAttrs []byte
	CustomAttrs     []byte
}

type WorkspaceConversation struct {
	ID        int64
	AccountID int64
	InboxID   int64
}

type WorkspaceMessageDraft struct {
	AccountID      int64
	ConversationID int64
	Text           string
	MessageType    string
	Private        bool
}

type WorkspaceMessage struct {
	ID             int64
	AccountID      int64
	ConversationID int64
	Text           string
	MessageType    string
	Status         string
	CreatedAt      time.Time
	Private        bool
}
