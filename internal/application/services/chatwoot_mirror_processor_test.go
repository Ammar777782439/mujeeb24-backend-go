package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestChatwootMirrorProcessorCompletesAfterWorkspaceSuccess(t *testing.T) {
	store := &mirrorStoreFake{job: ports.ChatwootMirrorJob{ID: "job-1", BusinessID: "business-1", CommunicationMessageID: "message-1"}, delivery: testMirrorDelivery()}
	workspace := &workspaceFake{}
	now := time.Date(2026, 8, 25, 17, 0, 0, 0, time.UTC)
	processor := ChatwootMirrorProcessor{Store: store, Workspace: workspace, Owner: "worker-1", Now: func() time.Time { return now }}
	if err := processor.Process(context.Background(), "job-1"); err != nil {
		t.Fatalf("process: %v", err)
	}
	if store.completed.JobID != "job-1" || store.completed.AccountID != 9 || store.completed.InboxID != 10 || store.completed.ChatwootConversationID != "22" || store.completed.ChatwootMessageID != "33" {
		t.Fatalf("unexpected completion: %#v", store.completed)
	}
	if store.failure.FailureCode != "" || len(workspace.calls) != 3 || workspace.calls[0] != "contact" || workspace.calls[1] != "conversation" || workspace.calls[2] != "message" {
		t.Fatalf("unexpected calls=%v failure=%#v", workspace.calls, store.failure)
	}
}

func TestChatwootMirrorProcessorQuarantinesUnknownWorkspaceOutcome(t *testing.T) {
	store := &mirrorStoreFake{job: ports.ChatwootMirrorJob{ID: "job-2", BusinessID: "business-1", CommunicationMessageID: "message-1"}, delivery: testMirrorDelivery()}
	workspace := &workspaceFake{messageErr: errors.New("timeout")}
	processor := ChatwootMirrorProcessor{Store: store, Workspace: workspace, Owner: "worker-1"}
	if err := processor.Process(context.Background(), "job-2"); err != nil {
		t.Fatalf("process: %v", err)
	}
	if store.completed.JobID != "" || store.failure.FailureCode != "chatwoot_message_outcome_unknown" {
		t.Fatalf("expected unknown outcome dead-letter, completion=%#v failure=%#v", store.completed, store.failure)
	}
}

func testMirrorDelivery() ports.ChatwootMirrorDelivery {
	return ports.ChatwootMirrorDelivery{BusinessID: "business-1", JobID: "job-1", CommunicationMessageID: "message-1", CustomerName: "عميل", CustomerIdentifier: "external-user-1", ProviderConversationID: "provider-conversation-1", Text: "مرحبا", AccountID: 9, InboxID: 10}
}

type mirrorStoreFake struct {
	job       ports.ChatwootMirrorJob
	delivery  ports.ChatwootMirrorDelivery
	completed ports.ChatwootMirrorCompletion
	failure   ports.ChatwootMirrorFailure
}

func (f *mirrorStoreFake) Enqueue(context.Context, ports.ChatwootMirrorDraft) (ports.ChatwootMirrorJob, error) {
	return f.job, nil
}
func (f *mirrorStoreFake) ListClaimable(context.Context, int) ([]ports.ChatwootMirrorJob, error) {
	return nil, nil
}
func (f *mirrorStoreFake) Claim(_ context.Context, _ string, lease ports.MirrorLease) (ports.MirrorClaimResult, error) {
	f.job.LeaseOwner, f.job.LeaseToken = &lease.Owner, &lease.Token
	return ports.MirrorClaimResult{Claimed: true, Record: f.job}, nil
}
func (f *mirrorStoreFake) Resolve(context.Context, string, string) (ports.ChatwootMirrorDelivery, error) {
	return f.delivery, nil
}
func (f *mirrorStoreFake) MarkCompleted(_ context.Context, completion ports.ChatwootMirrorCompletion) (ports.ChatwootMirrorJob, error) {
	f.completed = completion
	return f.job, nil
}
func (f *mirrorStoreFake) MoveToDeadLetter(_ context.Context, failure ports.ChatwootMirrorFailure) (ports.ChatwootMirrorJob, error) {
	f.failure = failure
	return f.job, nil
}

type workspaceFake struct {
	calls           []string
	contactErr      error
	conversationErr error
	messageErr      error
}

func (f *workspaceFake) CreateContact(_ context.Context, _ ports.WorkspaceContactDraft) (ports.WorkspaceContact, error) {
	f.calls = append(f.calls, "contact")
	if f.contactErr != nil {
		return ports.WorkspaceContact{}, f.contactErr
	}
	return ports.WorkspaceContact{ID: 11}, nil
}
func (f *workspaceFake) CreateConversation(_ context.Context, _ ports.WorkspaceConversationDraft) (ports.WorkspaceConversation, error) {
	f.calls = append(f.calls, "conversation")
	if f.conversationErr != nil {
		return ports.WorkspaceConversation{}, f.conversationErr
	}
	return ports.WorkspaceConversation{ID: 22}, nil
}
func (f *workspaceFake) CreateMessage(_ context.Context, _ ports.WorkspaceMessageDraft) (ports.WorkspaceMessage, error) {
	f.calls = append(f.calls, "message")
	if f.messageErr != nil {
		return ports.WorkspaceMessage{}, f.messageErr
	}
	return ports.WorkspaceMessage{ID: 33}, nil
}
