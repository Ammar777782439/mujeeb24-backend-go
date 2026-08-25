package services

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// SafeAutoReplyRuntime is the first vertical-slice runtime. It intentionally
// produces only a non-factual acknowledgement. A future LLM adapter must keep
// the same structured proposal contract and pass the same application gates.
type SafeAutoReplyRuntime struct {
	ResponseText string
}

func (r SafeAutoReplyRuntime) Decide(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
	if err := ctx.Err(); err != nil {
		return ports.AIDecisionProposal{}, err
	}
	if strings.TrimSpace(input.Text) == "" {
		return ports.AIDecisionProposal{}, errors.New("AI input text is required")
	}
	response := r.ResponseText
	if strings.TrimSpace(response) == "" {
		response = "شكرًا لتواصلك معنا. استلمنا رسالتك وسيرد عليك فريقنا قريبًا."
	}
	return ports.AIDecisionProposal{
		IntentBase:         "information_request",
		DomainContext:      "safe_acknowledgement",
		Entities:           []byte(`{}`),
		EvidenceReferences: []byte(`[]`),
		RequestedAction:    AutoReplyActionAnswer,
		ResponseText:       response,
		ConfidenceBand:     "medium",
		RequiresHuman:      false,
		MissingInformation: []byte(`[]`),
		ReasonCodes:        []byte(`["safe_acknowledgement"]`),
		PolicyDecision:     "allowed",
		PolicyVersion:      input.PolicyVersion,
		KnowledgeVersion:   "none",
		ModelReference:     "rule-based-auto-reply-v1",
		SchemaVersion:      1,
	}, nil
}

var _ ports.AIRuntime = SafeAutoReplyRuntime{}
