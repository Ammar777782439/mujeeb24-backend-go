// Package merchantassist contains tenant-authorized application read facades.
// It does not expose records to a model or execute commercial mutations.
package merchantassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

const (
	maxPublishedSearchBytes = 160
	maxPublishedRows        = 5
	maxPublishedItemBytes   = 8192
	maxPublishedTotalBytes  = 24576
)

var (
	ErrReadDenied            = errors.New("merchant assist read denied")
	ErrReadUnavailable       = errors.New("merchant assist read unavailable")
	ErrInvalidReadRequest    = errors.New("merchant assist invalid read request")
	ErrInvalidSourceEvidence = errors.New("merchant assist invalid source evidence")
)

type PublishedKind string

const (
	PublishedPolicies  PublishedKind = "policies"
	PublishedKnowledge PublishedKind = "knowledge"
)

// PublishedReadRequest is application input, never a model-provided SQL query.
// Actor is expected to originate from authenticated middleware, not the body.
type PublishedReadRequest struct {
	Actor    commands.ActorContext
	Business string
	Kind     PublishedKind
	Task     contracts.AssistTask
	Search   string
	Limit    int
}

type PublishedPolicyEvidence struct {
	Evidence contracts.EvidenceEnvelopeV1      `json:"evidence"`
	Value    contracts.PublishedPolicyAssistV1 `json:"value"`
}

type PublishedKnowledgeEvidence struct {
	Evidence contracts.EvidenceEnvelopeV1         `json:"evidence"`
	Value    contracts.PublishedKnowledgeAssistV1 `json:"value"`
}

type PublishedReadResult struct {
	Policies  []PublishedPolicyEvidence    `json:"policies,omitempty"`
	Knowledge []PublishedKnowledgeEvidence `json:"knowledge,omitempty"`
}

// PublishedReader reuses the existing business-scoped repository ports.
// The business AI guard must be checked before reading; record provenance
// is re-checked afterwards. Nothing in this service calls an AI provider.
type PublishedReader struct {
	Policies  ports.BusinessPolicyRepository
	Knowledge ports.KnowledgeDocumentRepository
	AIGuard   ports.BusinessAIExecutionGuard
	Now       func() time.Time
}

func (r *PublishedReader) Read(ctx context.Context, req PublishedReadRequest) (PublishedReadResult, error) {
	var empty PublishedReadResult
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if r == nil || r.AIGuard == nil || r.Now == nil {
		return empty, ErrReadUnavailable
	}
	// A forged or unrelated business route must fail before repository access.
	if req.Business == "" || string(req.Actor.BusinessID) != req.Business ||
		string(req.Actor.PrincipalID) == "" ||
		!authorization.Has(req.Actor, authorization.PermissionMerchantAIUse) ||
		!authorization.Has(req.Actor, authorization.PermissionBusinessRead) {
		return empty, ErrReadDenied
	}
	if req.Task != contracts.TaskExplain && req.Task != contracts.TaskSummarize {
		return empty, ErrInvalidReadRequest
	}
	term := strings.TrimSpace(req.Search)
	if term == "" || len(term) > maxPublishedSearchBytes || req.Limit < 1 || req.Limit > maxPublishedRows ||
		(req.Kind != PublishedPolicies && req.Kind != PublishedKnowledge) {
		return empty, ErrInvalidReadRequest
	}
	if err := r.AIGuard.Check(ctx, req.Business); err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadDenied, err)
	}
	now := r.Now().UTC()
	if now.IsZero() {
		return empty, ErrReadUnavailable
	}
	switch req.Kind {
	case PublishedPolicies:
		if r.Policies == nil {
			return empty, ErrReadUnavailable
		}
		records, err := r.Policies.ListPublished(ctx, req.Business, term, now, req.Limit)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if len(records) > req.Limit {
			return empty, ErrInvalidSourceEvidence
		}
		var size int
		result := make([]PublishedPolicyEvidence, 0, len(records))
		for _, record := range records {
			if err := ctx.Err(); err != nil {
				return empty, err
			}
			if record.BusinessID != req.Business || strings.TrimSpace(record.ID) == "" {
				return empty, ErrInvalidSourceEvidence
			}
			value := contracts.PublishedPolicyAssistV1{
				PolicyKey: record.PolicyKey, Category: record.Category, Title: record.Title,
				Summary: record.Summary, Authority: record.Authority, Status: record.Status,
				Version: record.Version, ValidFrom: record.ValidFrom, ValidUntil: record.ValidUntil,
			}
			if err := value.Validate(now); err != nil {
				return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
			}
			version := strconv.Itoa(record.Version)
			evidence := contracts.EvidenceEnvelopeV1{
				SchemaVersion:   contracts.EvidenceSchemaVersion,
				ResourceRef:     "policy:" + record.ID,
				SourceType:      contracts.SourceBusinessPolicy,
				ResourceVersion: &version,
				Authority:       contracts.AuthorityPublishedPolicy,
				Completeness:    contracts.EvidenceComplete,
				RetrievedAt:     now,
				ValidUntil:      record.ValidUntil,
			}
			if err := evidence.Validate(now); err != nil {
				return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
			}
			item := PublishedPolicyEvidence{Evidence: evidence, Value: value}
			if err := accountSize(item, &size); err != nil {
				return empty, err
			}
			result = append(result, item)
		}
		return PublishedReadResult{Policies: result}, nil
	case PublishedKnowledge:
		if r.Knowledge == nil {
			return empty, ErrReadUnavailable
		}
		records, err := r.Knowledge.ListPublished(ctx, req.Business, term, now, req.Limit)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if len(records) > req.Limit {
			return empty, ErrInvalidSourceEvidence
		}
		var size int
		result := make([]PublishedKnowledgeEvidence, 0, len(records))
		for _, record := range records {
			if err := ctx.Err(); err != nil {
				return empty, err
			}
			if record.BusinessID != req.Business || strings.TrimSpace(record.ID) == "" {
				return empty, ErrInvalidSourceEvidence
			}
			value := contracts.PublishedKnowledgeAssistV1{
				KnowledgeKey: record.KnowledgeKey, Title: record.Title, Content: record.Content,
				ContentType: record.ContentType, Authority: record.Authority, Status: record.Status,
				Version: record.Version, ValidFrom: record.ValidFrom, ValidUntil: record.ValidUntil,
			}
			if err := value.Validate(now); err != nil {
				return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
			}
			version := strconv.Itoa(record.Version)
			authority := contracts.AuthorityMerchantRecord
			if record.Authority == "system" {
				authority = contracts.AuthoritySystemRecord
			}
			evidence := contracts.EvidenceEnvelopeV1{
				SchemaVersion:   contracts.EvidenceSchemaVersion,
				ResourceRef:     "knowledge:" + record.ID,
				SourceType:      contracts.SourceKnowledge,
				ResourceVersion: &version,
				Authority:       authority,
				Completeness:    contracts.EvidenceComplete,
				RetrievedAt:     now,
				ValidUntil:      record.ValidUntil,
			}
			if err := evidence.Validate(now); err != nil {
				return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
			}
			item := PublishedKnowledgeEvidence{Evidence: evidence, Value: value}
			if err := accountSize(item, &size); err != nil {
				return empty, err
			}
			result = append(result, item)
		}
		return PublishedReadResult{Knowledge: result}, nil
	}
	return empty, ErrInvalidReadRequest
}

// accountSize enforces a strict serialization limit. A large result is
// rejected whole, not silently truncated mid-UTF8 or misrepresented as
// complete evidence. PII redaction is a separate mandatory model-bound gate.
func accountSize(value any, total *int) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
	}
	if len(raw) > maxPublishedItemBytes || *total+len(raw) > maxPublishedTotalBytes {
		return ErrInvalidSourceEvidence
	}
	*total += len(raw)
	return nil
}
