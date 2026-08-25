package services

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// Quiet Operations Room: resolve durable Mujeeb state first, then make Chatwoot
// calls outside the transaction. Any transport ambiguity is quarantined instead
// of blindly replaying a customer message.
type ChatwootMirrorProcessor struct {
	Store     ports.ChatwootMirrorStore
	Workspace ports.CommunicationWorkspace
	Owner     string
	LeaseFor  time.Duration
	Now       func() time.Time
}

func (p ChatwootMirrorProcessor) Process(ctx context.Context, jobID string) error {
	if p.Store == nil || p.Workspace == nil {
		return errors.New("chatwoot mirror processor dependencies are not configured")
	}
	if p.Owner == "" {
		p.Owner = "worker"
	}
	if p.LeaseFor <= 0 {
		p.LeaseFor = 2 * time.Minute
	}
	if p.Now == nil {
		p.Now = time.Now
	}
	claim, err := p.Store.Claim(ctx, jobID, ports.MirrorLease{Owner: p.Owner, Token: uuid.NewString(), ExpiresAt: p.Now().UTC().Add(p.LeaseFor)})
	if err != nil || !claim.Claimed {
		return err
	}
	job := claim.Record
	delivery, err := p.Store.Resolve(ctx, job.BusinessID, job.ID)
	if err != nil {
		return p.deadLetter(ctx, job, "chatwoot_mirror_mapping_missing")
	}
	contact, err := p.Workspace.CreateContact(ctx, ports.WorkspaceContactDraft{AccountID: delivery.AccountID, InboxID: delivery.InboxID, Name: delivery.CustomerName, Identifier: "mujeeb:" + delivery.BusinessID + ":" + delivery.CustomerIdentifier})
	if err != nil {
		return p.deadLetter(ctx, job, "chatwoot_contact_outcome_unknown")
	}
	conversation, err := p.Workspace.CreateConversation(ctx, ports.WorkspaceConversationDraft{AccountID: delivery.AccountID, InboxID: delivery.InboxID, ContactID: contact.ID, SourceID: "mujeeb:" + delivery.BusinessID + ":" + delivery.ProviderConversationID, Status: "open"})
	if err != nil {
		return p.deadLetter(ctx, job, "chatwoot_conversation_outcome_unknown")
	}
	message, err := p.Workspace.CreateMessage(ctx, ports.WorkspaceMessageDraft{AccountID: delivery.AccountID, ConversationID: conversation.ID, Text: delivery.Text, MessageType: "incoming"})
	if err != nil {
		return p.deadLetter(ctx, job, "chatwoot_message_outcome_unknown")
	}
	_, err = p.Store.MarkCompleted(ctx, ports.ChatwootMirrorCompletion{JobID: job.ID, Owner: leaseOwner(job, p.Owner), Token: leaseToken(job), AccountID: delivery.AccountID, InboxID: delivery.InboxID, ChatwootContactID: strconv.FormatInt(contact.ID, 10), ChatwootConversationID: strconv.FormatInt(conversation.ID, 10), ChatwootMessageID: strconv.FormatInt(message.ID, 10), CompletedAt: p.Now().UTC(), UpdatedAt: p.Now().UTC()})
	return err
}

func (p ChatwootMirrorProcessor) deadLetter(ctx context.Context, job ports.ChatwootMirrorJob, code string) error {
	_, err := p.Store.MoveToDeadLetter(ctx, ports.ChatwootMirrorFailure{JobID: job.ID, Owner: leaseOwner(job, p.Owner), Token: leaseToken(job), FailureCode: code, UpdatedAt: p.Now().UTC()})
	return err
}

func leaseOwner(job ports.ChatwootMirrorJob, fallback string) string {
	if job.LeaseOwner != nil {
		return *job.LeaseOwner
	}
	return fallback
}
func leaseToken(job ports.ChatwootMirrorJob) string {
	if job.LeaseToken != nil {
		return *job.LeaseToken
	}
	return ""
}
