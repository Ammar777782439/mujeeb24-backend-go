package merchantassist

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

const leadEvidencePageLimit = 5

type LeadEvidenceRepository interface {
	Get(context.Context, string, string) (ports.LeadRecord, error)
	ListAttributions(context.Context, string, string, int, string) (ports.LeadAttributionPage, error)
	ListScores(context.Context, string, string, int, string) (ports.LeadScorePage, error)
}

type LeadEvidenceRequest struct {
	Actor    commands.ActorContext
	Business string
	LeadID   string
	Task     contracts.AssistTask
}

type LeadAttributionEvidence struct {
	Evidence contracts.EvidenceEnvelopeV1      `json:"evidence"`
	Value    contracts.LeadAttributionAssistV1 `json:"value"`
}
type LeadScoreEvidence struct {
	Evidence contracts.EvidenceEnvelopeV1        `json:"evidence"`
	Value    contracts.LeadScoreSnapshotAssistV1 `json:"value"`
}
type LeadEvidenceResult struct {
	Lead                contracts.LeadAssistV1       `json:"lead"`
	LeadEvidence        contracts.EvidenceEnvelopeV1 `json:"lead_evidence"`
	Attributions        []LeadAttributionEvidence    `json:"attributions"`
	HasMoreAttributions bool                         `json:"has_more_attributions"`
	Scores              []LeadScoreEvidence          `json:"scores"`
	HasMoreScores       bool                         `json:"has_more_scores"`
}

type LeadEvidenceReader struct {
	Leads   LeadEvidenceRepository
	AIGuard ports.BusinessAIExecutionGuard
	Now     func() time.Time
}

func (r *LeadEvidenceReader) Read(ctx context.Context, req LeadEvidenceRequest) (LeadEvidenceResult, error) {
	var empty LeadEvidenceResult
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if r == nil || r.Leads == nil || r.AIGuard == nil || r.Now == nil {
		return empty, ErrReadUnavailable
	}
	if req.Business == "" || string(req.Actor.BusinessID) != req.Business ||
		strings.TrimSpace(string(req.Actor.PrincipalID)) == "" ||
		!authorization.Has(req.Actor, authorization.PermissionMerchantAIUse) ||
		!authorization.Has(req.Actor, authorization.PermissionSalesRead) {
		return empty, ErrReadDenied
	}
	if req.Task != contracts.TaskExplain && req.Task != contracts.TaskSummarize ||
		req.LeadID == "" || len(req.LeadID) > 128 || strings.TrimSpace(req.LeadID) != req.LeadID ||
		strings.ContainsAny(req.LeadID, " \t\n\r") {
		return empty, ErrInvalidReadRequest
	}
	if err := r.AIGuard.Check(ctx, req.Business); err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadDenied, err)
	}
	now := r.Now().UTC()
	if now.IsZero() {
		return empty, ErrReadUnavailable
	}
	record, err := r.Leads.Get(ctx, req.Business, req.LeadID)
	if err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
	}
	if err := validateScopedRecord(RecordReadRequest{Business: req.Business, RecordID: req.LeadID}, record.ID, record.BusinessID, record.ResourceVersion); err != nil {
		return empty, err
	}
	lead := contracts.LeadAssistV1{
		LeadRef: record.ID, CustomerRef: record.CustomerID, Status: record.Status,
		QualificationState: record.QualificationState, CurrentScoreBand: record.CurrentScoreBand,
		CurrentScoreValue: record.CurrentScoreValue, NextActionAt: record.NextActionAt,
	}
	if err := lead.Validate(); err != nil {
		return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
	}
	attrPage, err := r.Leads.ListAttributions(ctx, req.Business, req.LeadID, leadEvidencePageLimit, "")
	if err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
	}
	scorePage, err := r.Leads.ListScores(ctx, req.Business, req.LeadID, leadEvidencePageLimit, "")
	if err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
	}
	if len(attrPage.Items) > leadEvidencePageLimit || len(scorePage.Items) > leadEvidencePageLimit ||
		(attrPage.HasMore && attrPage.NextCursor == "") || (scorePage.HasMore && scorePage.NextCursor == "") {
		return empty, ErrInvalidSourceEvidence
	}
	out := LeadEvidenceResult{
		Lead:         lead,
		LeadEvidence: historyEvidence(now, contracts.SourceLead, "lead:"+record.ID, contracts.AuthoritySystemRecord, versionPointer(record.ResourceVersion)),
		Attributions: []LeadAttributionEvidence{}, Scores: []LeadScoreEvidence{},
		HasMoreAttributions: attrPage.HasMore, HasMoreScores: scorePage.HasMore,
	}
	seen := map[string]bool{}
	for _, a := range attrPage.Items {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if a.ID == "" || a.BusinessID != req.Business || a.LeadID != req.LeadID || seen[a.ID] {
			return empty, ErrInvalidSourceEvidence
		}
		seen[a.ID] = true
		v := contracts.LeadAttributionAssistV1{
			AttributionRef: a.ID, LeadRef: a.LeadID, Channel: a.SourceChannel,
			ConversationRef: a.SourceConversationID, CatalogItemRef: a.CatalogItemID,
			OfferRef: a.OfferID, CapturedAt: a.CapturedAt,
		}
		if err := v.Validate(now); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		out.Attributions = append(out.Attributions, LeadAttributionEvidence{
			Evidence: historyEvidence(now, contracts.SourceLead, "lead_attribution:"+a.ID, contracts.AuthoritySystemRecord, nil),
			Value:    v,
		})
	}
	seen = map[string]bool{}
	for _, s := range scorePage.Items {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if s.ID == "" || s.BusinessID != req.Business || s.LeadID != req.LeadID || seen[s.ID] {
			return empty, ErrInvalidSourceEvidence
		}
		seen[s.ID] = true
		v := contracts.LeadScoreSnapshotAssistV1{
			ScoreRef: s.ID, LeadRef: s.LeadID, Value: s.Value, Band: s.Band, CalculatedAt: s.CalculatedAt,
		}
		if err := v.Validate(now); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		out.Scores = append(out.Scores, LeadScoreEvidence{
			Evidence: historyEvidence(now, contracts.SourceLead, "lead_score:"+s.ID, contracts.AuthoritySystemRecord, nil),
			Value:    v,
		})
	}
	// Ensure no accidental unbounded JSONB/factors or metadata were copied.
	size := 0
	if err := accountSize(out, &size); err != nil {
		return empty, err
	}
	if out.LeadEvidence.ResourceVersion == nil ||
		*out.LeadEvidence.ResourceVersion != strconv.FormatInt(record.ResourceVersion, 10) {
		return empty, ErrInvalidSourceEvidence
	}
	return out, nil
}
