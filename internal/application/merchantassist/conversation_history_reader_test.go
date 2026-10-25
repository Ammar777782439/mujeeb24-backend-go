package merchantassist

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type summaryStateStub struct {
	calls  int
	result ports.ConversationStateRecord
	err    error
}

func (s *summaryStateStub) Get(_ context.Context, business, conversation string) (ports.ConversationStateRecord, error) {
	s.calls++
	return s.result, s.err
}

type messagePagesStub struct {
	calls int
	pages map[string]ports.MessagePage
	err   error
}

func (s *messagePagesStub) ListByConversation(_ context.Context, _, _ string, limit int, cursor string) (ports.MessagePage, error) {
	s.calls++
	if limit != historyPageSize {
		return ports.MessagePage{}, errors.New("wrong page bound")
	}
	return s.pages[cursor], s.err
}

type fakeContentSanitizer struct {
	calls       int
	err         error
	passthrough bool
}

func (s *fakeContentSanitizer) Sanitize(_ context.Context, _ ContentKind, text string) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	if s.passthrough {
		return text, nil
	}
	return strings.ReplaceAll(text, "PRIVATE-TOKEN", "[REDACTED]"), nil
}
func msg(id string, age time.Duration, text *string) ports.CommunicationMessageRecord {
	return ports.CommunicationMessageRecord{
		ID: id, BusinessID: "business-1", ConversationID: "record-1",
		Visibility: "public", ContentType: "text", Origin: "customer", Direction: "inbound",
		TextContent: text, OccurredAt: readNow.Add(age),
	}
}
func historyState() ports.ConversationStateRecord {
	return ports.ConversationStateRecord{
		BusinessID: "business-1", ConversationID: "record-1", Version: 3,
		Summary: "An old PRIVATE-TOKEN summary", SummaryLastMessageID: "checkpoint",
		SummaryTurnCount: 2,
	}
}
func historyRequest() ConversationHistoryRequest {
	return ConversationHistoryRequest{
		Actor: recordActor(), Business: "business-1", ConversationID: "record-1",
		Task: contracts.TaskExplain,
	}
}
func makeHistoryReader() (*ConversationHistoryReader, *guardFixture, *conversationReadStub, *summaryStateStub, *messagePagesStub, *fakeContentSanitizer) {
	g := &guardFixture{}
	c := &conversationReadStub{result: conversationRecord()}
	s := &summaryStateStub{result: historyState()}
	redactor := &fakeContentSanitizer{}
	privateText := "do not show private note"
	multimedia := "binary file reference"
	m1 := "First PRIVATE-TOKEN"
	m2 := "Second"
	mp := &messagePagesStub{pages: map[string]ports.MessagePage{
		"":      {Items: []ports.CommunicationMessageRecord{msg("new2", -time.Second, &m2), msg("private", -2*time.Second, &privateText)}, HasMore: true, NextCursor: "older"},
		"older": {Items: []ports.CommunicationMessageRecord{msg("media", -3*time.Second, &multimedia), msg("new1", -4*time.Second, &m1), msg("checkpoint", -5*time.Second, nil)}},
	}}
	mp.pages[""].Items[1].Visibility = "private"
	mp.pages["older"].Items[0].ContentType = "file"
	r := &ConversationHistoryReader{
		Conversations: c, State: s, Messages: mp, AIGuard: g, Sanitizer: redactor,
		Now: func() time.Time { return readNow },
	}
	return r, g, c, s, mp, redactor
}

func TestHistoryReaderCheckpointBoundariesAndRedaction(t *testing.T) {
	r, g, c, s, pages, redactor := makeHistoryReader()
	result, err := r.Read(context.Background(), historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != "complete_after_checkpoint" || result.Summary == nil || len(result.Messages) != 2 {
		t.Fatalf("incorrect covered history: %+v", result)
	}
	if result.Messages[0].Value.MessageRef != "new1" || result.Messages[1].Value.MessageRef != "new2" {
		t.Fatalf("history is not chronological: %+v", result.Messages)
	}
	if result.Messages[0].Evidence.Authority != contracts.AuthorityCustomerStatement ||
		result.Messages[0].Evidence.Completeness != contracts.EvidencePartial ||
		result.Summary.Evidence.Authority != contracts.AuthorityDerived {
		t.Fatalf("incorrect evidence authority: %+v", result)
	}
	for _, e := range []contracts.EvidenceEnvelopeV1{
		result.Messages[0].Evidence, result.Messages[1].Evidence, result.Summary.Evidence,
	} {
		if err := e.Validate(readNow); err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE-TOKEN", "do not show private note", "binary file reference"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("sensitive or private text leaked: %s", secret)
		}
	}
	if redactor.calls != 3 || g.calls != 1 || c.calls != 1 || s.calls != 1 || pages.calls != 2 {
		t.Fatalf("unexpected calls: sanitizer=%d guard=%d conversation=%d state=%d pages=%d",
			redactor.calls, g.calls, c.calls, s.calls, pages.calls)
	}
}
func TestHistoryReaderFailClosedForMissingCheckpoint(t *testing.T) {
	r, _, _, _, pages, _ := makeHistoryReader()
	delete(pages.pages, "older")
	pages.pages["older"] = ports.MessagePage{Items: []ports.CommunicationMessageRecord{msg("not-checkpoint", -3*time.Second, nil)}}
	res, err := r.Read(context.Background(), historyRequest())
	if !errors.Is(err, ErrInvalidSourceEvidence) || len(res.Messages) > 0 || res.Summary != nil {
		t.Fatalf("lost checkpoint must not produce partial context: %v", err)
	}
}
func TestHistoryReaderRejectsPrivateAndCrossTenantResult(t *testing.T) {
	for _, caseName := range []string{"wrong message tenant", "duplicate message", "future summary", "empty checkpoint", "too many newer messages", "bad pagination"} {
		t.Run(caseName, func(t *testing.T) {
			r, _, _, state, pages, _ := makeHistoryReader()
			switch caseName {
			case "wrong message tenant":
				page := pages.pages[""]
				page.Items[0].BusinessID = "other"
				pages.pages[""] = page
			case "duplicate message":
				page := pages.pages["older"]
				page.Items[1].ID = "new2"
				pages.pages["older"] = page
			case "future summary":
				state.result.SummaryLastMessageID = "not-in-history"
			case "empty checkpoint":
				state.result.SummaryLastMessageID = ""
			case "too many newer messages":
				var items []ports.CommunicationMessageRecord
				s := "text"
				for i := 0; i < historyMaxPublicTexts+1; i++ {
					items = append(items, msg(string(rune('a'+i)), time.Duration(-i)*time.Second, &s))
				}
				pages.pages[""] = ports.MessagePage{Items: items}
			case "bad pagination":
				p := pages.pages[""]
				p.NextCursor = ""
				pages.pages[""] = p
			}
			out, err := r.Read(context.Background(), historyRequest())
			if !errors.Is(err, ErrInvalidSourceEvidence) || out.Summary != nil || len(out.Messages) > 0 {
				t.Fatalf("unsafe history accepted: %v %+v", err, out)
			}
		})
	}
}
func TestHistoryReaderMandatorySanitizerAndDenial(t *testing.T) {
	r, g, c, _, pages, sanitizer := makeHistoryReader()
	r.Sanitizer = nil
	if _, err := r.Read(context.Background(), historyRequest()); !errors.Is(err, ErrReadUnavailable) {
		t.Fatal(err)
	}
	if g.calls != 0 || c.calls != 0 || pages.calls != 0 {
		t.Fatal("missing sanitizer must prevent all reads")
	}
	r.Sanitizer = sanitizer
	for _, test := range []struct {
		name string
		edit func(*ConversationHistoryRequest)
	}{
		{"cross-business", func(req *ConversationHistoryRequest) { req.Business = "other" }},
		{"missing conversation permission", func(req *ConversationHistoryRequest) { req.Actor.Permissions = []string{"merchant_ai:use"} }},
		{"missing AI permission", func(req *ConversationHistoryRequest) { req.Actor.Permissions = []string{"conversations:read"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r2, g2, c2, _, p2, _ := makeHistoryReader()
			req := historyRequest()
			test.edit(&req)
			if _, err := r2.Read(context.Background(), req); !errors.Is(err, ErrReadDenied) {
				t.Fatal(err)
			}
			if g2.calls != 0 || c2.calls != 0 || p2.calls != 0 {
				t.Fatal("denied request touched private repositories")
			}
		})
	}
	sanitizer.err = errors.New("unsafe content")
	out, err := r.Read(context.Background(), historyRequest())
	if !errors.Is(err, ErrInvalidSourceEvidence) || out.Summary != nil {
		t.Fatalf("sanitize failure must not return context: %v", err)
	}
}
func TestHistoryReaderUncheckpointedCompleteHistory(t *testing.T) {
	r, _, _, state, pages, _ := makeHistoryReader()
	state.result.Summary = ""
	state.result.SummaryLastMessageID = ""
	msgText := "Latest"
	pages.pages = map[string]ports.MessagePage{"": {Items: []ports.CommunicationMessageRecord{msg("new1", 0, &msgText)}}}
	out, err := r.Read(context.Background(), historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	if out.Coverage != "complete_history" || out.Summary != nil || len(out.Messages) != 1 {
		t.Fatalf("incorrect full history coverage: %+v", out)
	}
}
