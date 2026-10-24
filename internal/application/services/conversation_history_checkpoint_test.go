package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// The real PostgreSQL repository returns newest-first pages. The summarizer
// MUST normalize all pages before building context for Gemini OR DeepSeek.
type checkpointMessages struct {
	pages map[string]ports.MessagePage
}

func (m checkpointMessages) Record(context.Context, ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord, error) {
	return ports.CommunicationMessageRecord{}, nil
}
func (m checkpointMessages) ListByConversation(_ context.Context, _, _ string, _ int, cursor string) (ports.MessagePage, error) {
	return m.pages[cursor], nil
}
func checkpointMsg(id, direction, text string, age time.Duration) ports.CommunicationMessageRecord {
	return ports.CommunicationMessageRecord{ID: id, Direction: direction, Origin: "customer", TextContent: &text, OccurredAt: time.Now().Add(age)}
}
func TestConversationHistoryIsChronologicalAcrossPages(t *testing.T) {
	m := checkpointMessages{pages: map[string]ports.MessagePage{
		"":      {Items: []ports.CommunicationMessageRecord{checkpointMsg("m4", "outbound", "four", 0), checkpointMsg("m3", "inbound", "three", -time.Minute)}, HasMore: true, NextCursor: "older"},
		"older": {Items: []ports.CommunicationMessageRecord{checkpointMsg("m2", "outbound", "two", -2*time.Minute), checkpointMsg("m1", "inbound", "one", -3*time.Minute)}},
	}}
	out, err := listConversationHistory(context.Background(), m, "biz", "conv")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 || out[0].ID != "m1" || out[1].ID != "m2" || out[2].ID != "m3" || out[3].ID != "m4" {
		t.Fatalf("incorrect message chronology: %#v", out)
	}
}
func TestConversationHistoryFailsClosedOnBrokenCursor(t *testing.T) {
	m := checkpointMessages{pages: map[string]ports.MessagePage{
		"": {Items: []ports.CommunicationMessageRecord{checkpointMsg("one", "inbound", "text", 0)}, HasMore: true, NextCursor: ""},
	}}
	if _, err := listConversationHistory(context.Background(), m, "biz", "conv"); err == nil {
		t.Fatal("accepted an incomplete conversation history")
	}
}
func TestTranscriptChunksNeverTruncateArabicMessages(t *testing.T) {
	messages := []ports.CommunicationMessageRecord{
		checkpointMsg("m1", "inbound", "عندي متجر صغير وأريد مقارنة الأسعار", -time.Minute),
		checkpointMsg("m2", "outbound", "المساعد وعد بالتحويل ولم يثبت التنفيذ", 0),
	}
	parts, err := transcriptChunks(messages, 130, 3)
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.Join(parts, "")
	if !strings.Contains(combined, "عندي متجر صغير وأريد مقارنة الأسعار") || !strings.Contains(combined, "المساعد وعد بالتحويل ولم يثبت التنفيذ") {
		t.Fatalf("message content lost: %q", combined)
	}
	if !strings.Contains(combined, "المساعد:") {
		t.Fatalf("assistant not attributed: %q", combined)
	}
}
func TestTranscriptChunksRejectUnboundedSingleMessage(t *testing.T) {
	msgs := []ports.CommunicationMessageRecord{checkpointMsg("long", "inbound", strings.Repeat("أ", 3000), 0)}
	if _, err := transcriptChunks(msgs, 3900, 8); err == nil {
		t.Fatal("oversized message must not be cut mid-UTF8 or silently discarded")
	}
}
func TestTranscriptChunksBoundInferenceCalls(t *testing.T) {
	var msgs []ports.CommunicationMessageRecord
	for i := 0; i < 12; i++ {
		msgs = append(msgs, checkpointMsg("id", "inbound", strings.Repeat("أ", 40), time.Duration(-i)*time.Minute))
	}
	if _, err := transcriptChunks(msgs, 110, 2); err == nil {
		t.Fatal("unbounded model calls accepted")
	}
}

func TestLoadMessagesAfterCheckpointNeverLosesUncoveredTurns(t *testing.T) {
	m := checkpointMessages{pages: map[string]ports.MessagePage{
		"": {Items: []ports.CommunicationMessageRecord{
			checkpointMsg("m6", "inbound", "أريد حجز موعد", 0),
			checkpointMsg("m5", "outbound", "هذا مجرد وعد بالاتصال", -time.Minute),
			checkpointMsg("m4", "inbound", "طلب آخر", -2*time.Minute),
		}, HasMore: true, NextCursor: "older"},
		"older": {Items: []ports.CommunicationMessageRecord{
			checkpointMsg("m3", "outbound", "أهلاً", -3*time.Minute),
			checkpointMsg("m2", "inbound", "مرحباً", -4*time.Minute),
		}},
	}}
	got, err := listMessagesAfterCheckpoint(context.Background(), m, "biz", "conv", "m2")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].ID != "m3" || got[1].ID != "m4" || got[2].ID != "m5" || got[3].ID != "m6" {
		t.Fatalf("uncovered messages must arrive in chronological order: %#v", got)
	}
}
func TestLoadMessagesAfterCheckpointRejectsUnknownAnchor(t *testing.T) {
	m := checkpointMessages{pages: map[string]ports.MessagePage{
		"": {Items: []ports.CommunicationMessageRecord{checkpointMsg("m3", "inbound", "test", 0)}}}}
	if _, err := listMessagesAfterCheckpoint(context.Background(), m, "biz", "conv", "missing"); err == nil {
		t.Fatal("missing summary checkpoint must not silently discard context")
	}
}

type initiallyMissingSummaryState struct {
	ensured int
	saved   bool
}

func (s *initiallyMissingSummaryState) Get(context.Context, string, string) (ports.ConversationStateRecord, error) {
	return ports.ConversationStateRecord{}, errNotFoundStub{}
}
func (s *initiallyMissingSummaryState) UpsertValidated(_ context.Context, r ports.ConversationStateRecord) (ports.ConversationStateRecord, error) {
	return r, nil
}
func (s *initiallyMissingSummaryState) EnsureSummaryState(_ context.Context, businessID, conversationID string) (ports.ConversationStateRecord, error) {
	s.ensured++
	return ports.ConversationStateRecord{BusinessID: businessID, ConversationID: conversationID, Version: 1}, nil
}
func (s *initiallyMissingSummaryState) SaveSummaryIfVersion(_ context.Context, _, _ string, version int64, summary, lastID string, coveredInbound int) (bool, error) {
	s.saved = version == 1 && lastID == "m6" && summary != "" && coveredInbound == 3
	return s.saved, nil
}
func TestSummaryColdStartContinuesAfterCreatingMissingState(t *testing.T) {
	repo := &initiallyMissingSummaryState{}
	var msgs []ports.CommunicationMessageRecord
	for i := 10; i >= 1; i-- {
		id := fmt.Sprintf("m%d", i)
		direction := "outbound"
		if i%2 == 1 {
			direction = "inbound"
		}
		msgs = append(msgs, checkpointMsg(id, direction, id, time.Duration(-i)*time.Minute))
	}
	provider := &summaryGenerationStub{output: "العميل ما زال بحاجة إلى معلومات عن موعد خدمة ولم يتأكد تنفيذ أي إجراء"}
	service := &ConversationSummaryService{StateRepository: repo, Messages: checkpointMessages{pages: map[string]ports.MessagePage{
		"": {Items: msgs},
	}}, SummaryPort: provider}
	out, err := service.MaybeSummarize(context.Background(), "business", "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Summarized || repo.ensured != 1 || !repo.saved || provider.calls != 1 {
		t.Fatalf("cold-start summary did not commit: result=%#v, state=%#v, provider_calls=%d", out, repo, provider.calls)
	}
}
