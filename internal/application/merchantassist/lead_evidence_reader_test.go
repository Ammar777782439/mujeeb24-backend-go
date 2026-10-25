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

type leadEvidenceStub struct {
	getCalls   int
	attrCalls  int
	scoreCalls int
	rec        ports.LeadRecord
	attrs      ports.LeadAttributionPage
	scores     ports.LeadScorePage
}

func (s *leadEvidenceStub) Get(_ context.Context, _, _ string) (ports.LeadRecord, error) {
	s.getCalls++
	return s.rec, nil
}
func (s *leadEvidenceStub) ListAttributions(_ context.Context, _, _ string, limit int, cursor string) (ports.LeadAttributionPage, error) {
	s.attrCalls++
	if limit != leadEvidencePageLimit || cursor != "" {
		return ports.LeadAttributionPage{}, errors.New("unbounded")
	}
	return s.attrs, nil
}
func (s *leadEvidenceStub) ListScores(_ context.Context, _, _ string, limit int, cursor string) (ports.LeadScorePage, error) {
	s.scoreCalls++
	if limit != leadEvidencePageLimit || cursor != "" {
		return ports.LeadScorePage{}, errors.New("unbounded")
	}
	return s.scores, nil
}
func leadEvidenceFixture() (*LeadEvidenceReader, *leadEvidenceStub, *guardFixture) {
	s := &leadEvidenceStub{
		rec: leadRecord(),
		attrs: ports.LeadAttributionPage{Items: []ports.LeadAttributionRecord{{
			ID: "attr-1", BusinessID: "business-1", LeadID: "record-1",
			SourceChannel: ptrString("whatsapp"), SourceConversationID: ptrString("conversation-1"),
			CapturedAt: readNow.Add(-time.Minute), CampaignReference: ptrString("confidential_campaign"),
		}}},
		scores: ports.LeadScorePage{Items: []ports.LeadScoreRecord{{
			ID: "score-1", BusinessID: "business-1", LeadID: "record-1", Band: "high",
			Value: "88.00", CalculatedAt: readNow.Add(-time.Minute),
			Factors:        []byte(`{"secret_internal_factor":"do-not-send"}`),
			ModelReference: ptrString("internal-model"),
		}}},
	}
	g := &guardFixture{}
	return &LeadEvidenceReader{Leads: s, AIGuard: g, Now: func() time.Time { return readNow }}, s, g
}
func leadEvidenceRequest() LeadEvidenceRequest {
	return LeadEvidenceRequest{Actor: recordActor(), Business: "business-1", LeadID: "record-1", Task: contracts.TaskExplain}
}

func TestLeadEvidenceSourceScopeAndHistoricalScores(t *testing.T) {
	r, s, g := leadEvidenceFixture()
	out, err := r.Read(context.Background(), leadEvidenceRequest())
	if err != nil {
		t.Fatal(err)
	}
	if g.calls != 1 || s.getCalls != 1 || s.attrCalls != 1 || s.scoreCalls != 1 {
		t.Fatal("expected one scope-checked read per source")
	}
	if len(out.Attributions) != 1 || len(out.Scores) != 1 || out.HasMoreAttributions || out.HasMoreScores {
		t.Fatalf("unexpected bounded evidence: %+v", out)
	}
	if out.LeadEvidence.Completeness != contracts.EvidencePartial ||
		out.Attributions[0].Value.Channel == nil || *out.Attributions[0].Value.Channel != "whatsapp" ||
		out.Scores[0].Value.Value != "88.00" {
		t.Fatal("evidence lost historical meaning")
	}
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"confidential_campaign", "secret_internal_factor", "internal-model", "qualification_context"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("raw data leaked: %s", secret)
		}
	}
}
func TestLeadEvidenceRejectsMissingPermissionsAndTenant(t *testing.T) {
	for _, which := range []string{"business", "principal", "AI", "sales", "guard"} {
		t.Run(which, func(t *testing.T) {
			r, s, g := leadEvidenceFixture()
			req := leadEvidenceRequest()
			switch which {
			case "business":
				req.Business = "other"
			case "principal":
				req.Actor.PrincipalID = ""
			case "AI":
				req.Actor.Permissions = []string{"sales:read"}
			case "sales":
				req.Actor.Permissions = []string{"merchant_ai:use"}
			case "guard":
				g.err = ports.ErrBusinessAIDisabled
			}
			_, err := r.Read(context.Background(), req)
			if !errors.Is(err, ErrReadDenied) || s.getCalls != 0 || s.attrCalls != 0 || s.scoreCalls != 0 {
				t.Fatalf("unauthorized lead read: %v", err)
			}
		})
	}
}
func TestLeadEvidenceInvalidSourceFailsClosed(t *testing.T) {
	for _, which := range []string{"attr-tenant", "attr-lead", "attr-channel", "attr-time", "score-tenant", "score-lead", "score-range", "score-future", "score-band", "over-limit"} {
		t.Run(which, func(t *testing.T) {
			r, s, _ := leadEvidenceFixture()
			switch which {
			case "attr-tenant":
				s.attrs.Items[0].BusinessID = "other"
			case "attr-lead":
				s.attrs.Items[0].LeadID = "wrong"
			case "attr-channel":
				s.attrs.Items[0].SourceChannel = ptrString("telegram")
			case "attr-time":
				s.attrs.Items[0].CapturedAt = readNow.Add(time.Minute)
			case "score-tenant":
				s.scores.Items[0].BusinessID = "other"
			case "score-lead":
				s.scores.Items[0].LeadID = "other"
			case "score-range":
				s.scores.Items[0].Value = "100.01"
			case "score-future":
				s.scores.Items[0].CalculatedAt = readNow.Add(time.Minute)
			case "score-band":
				s.scores.Items[0].Band = "guaranteed_conversion"
			case "over-limit":
				s.attrs.Items = append(s.attrs.Items, s.attrs.Items[0], s.attrs.Items[0], s.attrs.Items[0], s.attrs.Items[0], s.attrs.Items[0])
			}
			out, err := r.Read(context.Background(), leadEvidenceRequest())
			if !errors.Is(err, ErrInvalidSourceEvidence) || len(out.Attributions) > 0 || len(out.Scores) > 0 {
				t.Fatalf("%s returned data: %v %+v", which, err, out)
			}
		})
	}
}
func TestLeadEvidencePaginationIsExplicit(t *testing.T) {
	r, s, _ := leadEvidenceFixture()
	s.attrs.HasMore = true
	s.attrs.NextCursor = "cursor"
	s.scores.HasMore = true
	s.scores.NextCursor = "cursor2"
	out, err := r.Read(context.Background(), leadEvidenceRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasMoreScores || !out.HasMoreAttributions {
		t.Fatal("partial evidence coverage hidden")
	}
}
