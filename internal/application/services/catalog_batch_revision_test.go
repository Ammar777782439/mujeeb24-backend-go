package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Embed the read interface to keep the fixture scoped to methods actually
// used by the evaluation controller. GetManifest is intentionally unused.
type catalogRevisionProbe struct {
	ports.CatalogAIReadRepository
	revisions []string
	reads     int
}

func (p *catalogRevisionProbe) GetRevision(_ context.Context, _ string) (string, error) {
	if p.reads >= len(p.revisions) {
		return "", errors.New("unexpected extra revision read")
	}
	rev := p.revisions[p.reads]
	p.reads++
	if rev == "ERROR" {
		return "", errors.New("revision storage unavailable")
	}
	return rev, nil
}

func (p *catalogRevisionProbe) ListProjectionPage(_ context.Context, _ ports.CatalogAIProjectionRequest) (ports.CatalogAIProjectionPage, error) {
	return ports.CatalogAIProjectionPage{}, nil
}

func TestCatalogEvaluationChecksRevisionAfterFinalModelDecision(t *testing.T) {
	cases := []struct {
		name      string
		revisions []string
		wantError string
	}{
		{name: "unchanged after final", revisions: []string{"v1", "v1", "v1"}},
		{name: "changed while final model running", revisions: []string{"v1", "v1", "v2"}, wantError: "catalog changed during final evaluation"},
		{name: "changed while paginating", revisions: []string{"v1", "v2"}, wantError: "catalog changed during full evaluation"},
		{name: "revision read fails after final", revisions: []string{"v1", "v1", "ERROR"}, wantError: "read catalog revision after final evaluation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := &catalogRevisionProbe{revisions: tc.revisions}
			client := &exactTokenBatchGeminiStub{}
			controller := &CatalogBatchController{CatalogAI: probe, Evaluator: client}
			out, err := controller.runPagedCatalogEvaluation(context.Background(), CatalogEvaluationInput{
				AIRunID: "run-1", BusinessID: "business-1", CustomerMessage: "What is available?",
			})
			if tc.wantError == "" {
				if err != nil || out.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction {
					t.Fatalf("unchanged revision rejected: %+v, %v", out, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected %q, got %v", tc.wantError, err)
			}
			if probe.reads != len(tc.revisions) {
				t.Fatalf("revision checks=%d, want %d", probe.reads, len(tc.revisions))
			}
		})
	}
}
