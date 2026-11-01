package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

func TestConversationAssignmentEligibilityUsesRoleAndCurrentGrants(t *testing.T) {
	for _, tc := range []struct {
		name, role, status string
		permissions        []string
		want               bool
	}{
		{"agent", "agent", "active", []string{"conversations:read", "conversations:reply"}, true},
		{"legacy owner", "owner", "active", []string{"*"}, true},
		{"restricted owner", "owner", "active", []string{"catalog:read"}, false},
		{"read only", "agent", "active", []string{"conversations:read"}, false},
		{"revoked", "agent", "revoked", []string{"*"}, false},
		{"viewer", "viewer", "active", []string{"*"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := eligibleConversationAssignee(ports.TeamMemberRecord{Role: tc.role, Status: tc.status, Permissions: tc.permissions}); got != tc.want {
				t.Fatalf("eligibility=%v want %v", got, tc.want)
			}
		})
	}
}

func TestPrivateNoteReferencePreservesContextAndLegacyNotes(t *testing.T) {
	target := uuid.NewString()
	reference := encodePrivateNoteReference(uuid.NewString(), target, strings.Repeat("ع", 400))
	record := ports.CommunicationMessageRecord{Visibility: "private", ContentReference: reference}
	view := messageView(record)
	if !view.Private || view.RelatedMessageID == nil || *view.RelatedMessageID != target || view.RelatedMessageText == nil || len([]rune(*view.RelatedMessageText)) != 281 {
		t.Fatal("private note context is missing or incorrectly truncated")
	}
	for _, visibility := range []string{"public", "private"} {
		record.Visibility = visibility
		record.ContentReference = "private-note:" + uuid.NewString()
		if messageView(record).RelatedMessageID != nil {
			t.Fatal("legacy notes must not invent a customer message link")
		}
	}
	record.Visibility = "public"
	record.ContentReference = reference
	if messageView(record).RelatedMessageID != nil {
		t.Fatal("note metadata must only be decoded for private notes")
	}
}

func TestPrivateNotesNeverEnterCustomerModelHistoryOrSummary(t *testing.T) {
	publicText, secret := "public customer question", "internal-only handling instructions"
	records := []ports.CommunicationMessageRecord{{ID: "public", Visibility: "public", Direction: "inbound", TextContent: &publicText}, {ID: "note", Visibility: "private", Direction: "outbound", TextContent: &secret}}
	evidence := buildRecentMessageEvidence(records, "", time.Now())
	if len(evidence) != 1 || evidence[0].Reference != "public" {
		t.Fatal("customer AI received private note evidence")
	}
	transcript, err := transcriptChunks(records, 1000, 2)
	if err != nil || strings.Contains(strings.Join(transcript, ""), secret) || strings.Contains(buildTranscript(records), secret) {
		t.Fatal("private note entered model transcript")
	}
	repo := checkpointMessages{pages: map[string]ports.MessagePage{"": {Items: records}}}
	history, err := listConversationHistory(context.Background(), repo, "business", "conversation")
	if err != nil || len(history) != 1 || history[0].ID != "public" {
		t.Fatal("summary checkpoint history contains a private note")
	}
}
