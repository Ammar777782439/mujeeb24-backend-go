package services

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type pagedMessageHistory struct {
	ports.MessageRepository
	pages map[string]ports.MessagePage
}

func (r pagedMessageHistory) ListByConversation(_ context.Context, _, _ string, _ int, cursor string) (ports.MessagePage, error) {
	return r.pages[cursor], nil
}

func TestMessagesAfterSummaryCheckpointAreOldestToNewestAcrossPages(t *testing.T) {
	now := time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC)
	current := "provider-current"
	message := func(id string, seconds int, provider *string) ports.CommunicationMessageRecord {
		return ports.CommunicationMessageRecord{
			ID: id, ProviderMessageID: provider, Direction: "inbound", Origin: "customer",
			TextContent: &id, OccurredAt: now.Add(time.Duration(seconds) * time.Second),
		}
	}
	repo := pagedMessageHistory{pages: map[string]ports.MessagePage{
		"": {Items: []ports.CommunicationMessageRecord{
			message("current-id", 120, &current),
			message("newer-same-second", 60, nil),
		}, HasMore: true, NextCursor: "second"},
		"second": {Items: []ports.CommunicationMessageRecord{
			message("older-same-second", 60, nil),
			message("checkpoint-id", 0, nil),
		}},
	}}
	records, err := listMessagesAfterCheckpoint(context.Background(), repo, "business-1", "conversation-1", "checkpoint-id")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, record := range records {
		got = append(got, record.ID)
	}
	want := []string{"older-same-second", "newer-same-second", "current-id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pagination lost chronology or changed timestamp ties: got=%v want=%v", got, want)
	}
	visible := buildRecentMessageEvidence(records, "provider-current", now)
	got = nil
	for _, message := range visible {
		got = append(got, message.Reference)
	}
	if !reflect.DeepEqual(got, want[:2]) {
		t.Fatalf("evidence not chronological or included current provider message twice: got=%v", got)
	}
}

func TestBoundedNewestFirstHistoryIsReversedForModel(t *testing.T) {
	now := time.Now().UTC()
	newerText, olderText := "بعدها", "قبلها"
	messages := []ports.CommunicationMessageRecord{
		{ID: "newer", Direction: "inbound", Origin: "customer", TextContent: &newerText, OccurredAt: now},
		{ID: "older", Direction: "inbound", Origin: "customer", TextContent: &olderText, OccurredAt: now.Add(-time.Minute)},
	}
	// MessageRepository.ListByConversation returns ORDER BY occurred_at,
	// created_at, id DESC. AutoReplyContextBuilder reverses that result.
	messages[0], messages[1] = messages[1], messages[0]
	result := buildRecentMessageEvidence(messages, "", now)
	if len(result) != 2 || result[0].Reference != "older" || result[1].Reference != "newer" {
		t.Fatalf("model did not receive oldest-to-newest history: %+v", result)
	}
}
