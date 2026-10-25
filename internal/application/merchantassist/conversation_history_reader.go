package merchantassist

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

const (
	historyPageSize       = 25
	historyMaxScanned     = 200
	historyMaxPublicTexts = 16
	historyMaxTextBytes   = 1200
	historyMaxTotalBytes  = 24576
)

type ContentKind string

const (
	ContentMessage ContentKind = "public_message"
	ContentSummary ContentKind = "conversation_summary"
)

// ContentSanitizer is mandatory for model-adjacent text. It must be bound to
// a real, separately audited redaction/classification implementation before
// this read facade is wired to an API or a model. No implicit pass-through.
type ContentSanitizer interface {
	Sanitize(context.Context, ContentKind, string) (string, error)
}

type ConversationHistoryRequest struct {
	Actor          commands.ActorContext
	Business       string
	ConversationID string
	Task           contracts.AssistTask
}

type MessageEvidence struct {
	Evidence contracts.EvidenceEnvelopeV1        `json:"evidence"`
	Value    contracts.PublicTextMessageAssistV1 `json:"value"`
}

type SummaryEvidence struct {
	Evidence contracts.EvidenceEnvelopeV1          `json:"evidence"`
	Value    contracts.ConversationSummaryAssistV1 `json:"value"`
}

// The coverage field is explicit: no incomplete or uncheckpointed transcript
// is silently labelled a complete post-summary history.
type ConversationHistoryResult struct {
	ConversationRef string            `json:"conversation_ref"`
	Coverage        string            `json:"coverage"`
	Summary         *SummaryEvidence  `json:"summary,omitempty"`
	Messages        []MessageEvidence `json:"messages"`
}

type ConversationStateGetter interface {
	Get(context.Context, string, string) (ports.ConversationStateRecord, error)
}
type ConversationMessageLister interface {
	ListByConversation(context.Context, string, string, int, string) (ports.MessagePage, error)
}

type ConversationHistoryReader struct {
	Conversations ConversationGetter
	State         ConversationStateGetter
	Messages      ConversationMessageLister
	AIGuard       ports.BusinessAIExecutionGuard
	Sanitizer     ContentSanitizer
	Now           func() time.Time
}

func (r *ConversationHistoryReader) Read(ctx context.Context, req ConversationHistoryRequest) (ConversationHistoryResult, error) {
	var empty ConversationHistoryResult
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if r == nil || r.Conversations == nil || r.State == nil || r.Messages == nil ||
		r.AIGuard == nil || r.Sanitizer == nil || r.Now == nil {
		return empty, ErrReadUnavailable
	}
	if req.Business == "" || string(req.Actor.BusinessID) != req.Business ||
		strings.TrimSpace(string(req.Actor.PrincipalID)) == "" ||
		!authorization.Has(req.Actor, authorization.PermissionMerchantAIUse) ||
		!authorization.Has(req.Actor, authorization.PermissionConversationsRead) {
		return empty, ErrReadDenied
	}
	if req.Task != contracts.TaskExplain && req.Task != contracts.TaskSummarize ||
		req.ConversationID == "" || len(req.ConversationID) > 128 ||
		strings.TrimSpace(req.ConversationID) != req.ConversationID ||
		strings.ContainsAny(req.ConversationID, " \t\r\n") {
		return empty, ErrInvalidReadRequest
	}
	if err := r.AIGuard.Check(ctx, req.Business); err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadDenied, err)
	}
	now := r.Now().UTC()
	if now.IsZero() {
		return empty, ErrReadUnavailable
	}
	conversation, err := r.Conversations.GetByID(ctx, req.Business, req.ConversationID)
	if err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
	}
	if err := validateScopedRecord(RecordReadRequest{Business: req.Business, RecordID: req.ConversationID},
		conversation.ID, conversation.BusinessID, conversation.ResourceVersion); err != nil {
		return empty, err
	}
	state, err := r.State.Get(ctx, req.Business, req.ConversationID)
	if err != nil {
		return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
	}
	if state.BusinessID != req.Business || state.ConversationID != req.ConversationID || state.Version <= 0 ||
		(state.Summary != "" && state.SummaryLastMessageID == "") ||
		(state.Summary == "" && state.SummaryLastMessageID != "") {
		return empty, ErrInvalidSourceEvidence
	}
	// The repository implements a DESC keyset cursor. Every page is checked
	// and no checkpoint is guessed from timestamps or source provider IDs.
	cursor := ""
	seenCursor := map[string]bool{}
	seenIDs := map[string]bool{}
	newer := make([]ports.CommunicationMessageRecord, 0, historyMaxPublicTexts)
	foundCheckpoint := false
	scanned := 0
	for {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		page, err := r.Messages.ListByConversation(ctx, req.Business, req.ConversationID, historyPageSize, cursor)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrReadUnavailable, err)
		}
		if len(page.Items) > historyPageSize || len(page.Items) == 0 && page.HasMore {
			return empty, ErrInvalidSourceEvidence
		}
		for _, m := range page.Items {
			scanned++
			if scanned > historyMaxScanned || m.BusinessID != req.Business ||
				m.ConversationID != req.ConversationID || strings.TrimSpace(m.ID) == "" ||
				seenIDs[m.ID] {
				return empty, ErrInvalidSourceEvidence
			}
			seenIDs[m.ID] = true
			if state.SummaryLastMessageID != "" && m.ID == state.SummaryLastMessageID {
				foundCheckpoint = true
				break
			}
			newer = append(newer, m)
			if len(newer) > historyMaxPublicTexts {
				// No truncation: caller must first refresh its summary or use
				// an explicitly different, partial-coverage contract.
				return empty, ErrInvalidSourceEvidence
			}
		}
		if foundCheckpoint || !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor || seenCursor[page.NextCursor] {
			return empty, ErrInvalidSourceEvidence
		}
		seenCursor[page.NextCursor] = true
		cursor = page.NextCursor
	}
	if state.SummaryLastMessageID != "" && !foundCheckpoint {
		return empty, ErrInvalidSourceEvidence
	}
	result := ConversationHistoryResult{ConversationRef: req.ConversationID, Messages: []MessageEvidence{}}
	if foundCheckpoint {
		result.Coverage = "complete_after_checkpoint"
	} else {
		result.Coverage = "complete_history"
	}
	if state.Summary != "" {
		safe, err := sanitizeText(ctx, r.Sanitizer, ContentSummary, state.Summary)
		if err != nil {
			return empty, err
		}
		value := contracts.ConversationSummaryAssistV1{
			Summary: safe, LastCoveredMessageRef: state.SummaryLastMessageID,
			CoveredTurnCount: state.SummaryTurnCount,
		}
		if err := value.Validate(); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		version := strconv.FormatInt(state.Version, 10)
		evidence := historyEvidence(now, contracts.SourceConversation,
			"conversation:"+req.ConversationID, contracts.AuthorityDerived, &version)
		result.Summary = &SummaryEvidence{Evidence: evidence, Value: value}
	}
	// Reverse DESC pages to chronological order, preserving exact ID order
	// for equal timestamps as established by the repository.
	for i := len(newer) - 1; i >= 0; i-- {
		m := newer[i]
		if m.Visibility != "public" || m.ContentType != "text" || m.TextContent == nil ||
			strings.TrimSpace(*m.TextContent) == "" {
			continue
		}
		safe, err := sanitizeText(ctx, r.Sanitizer, ContentMessage, *m.TextContent)
		if err != nil {
			return empty, err
		}
		value := contracts.PublicTextMessageAssistV1{
			MessageRef: m.ID, Direction: m.Direction, Origin: m.Origin,
			Visibility: m.Visibility, ContentType: m.ContentType,
			Text: safe, OccurredAt: m.OccurredAt,
		}
		if err := value.Validate(); err != nil {
			return empty, fmt.Errorf("%w: %w", ErrInvalidSourceEvidence, err)
		}
		authority := contracts.AuthoritySystemRecord
		if m.Direction == "inbound" && m.Origin == "customer" {
			authority = contracts.AuthorityCustomerStatement
		}
		evidence := historyEvidence(now, contracts.SourceMessage,
			"message:"+m.ID, authority, nil)
		result.Messages = append(result.Messages, MessageEvidence{Evidence: evidence, Value: value})
	}
	// A concurrent summary writer may advance the checkpoint while pages
	// are being scanned. Never combine mismatched versions of history.
	latest, err := r.State.Get(ctx, req.Business, req.ConversationID)
	if err != nil {
		return empty, fmt.Errorf("%w: state revalidation failed", ErrReadUnavailable)
	}
	if latest.BusinessID != req.Business || latest.ConversationID != req.ConversationID ||
		latest.Version != state.Version || latest.SummaryLastMessageID != state.SummaryLastMessageID ||
		latest.Summary != state.Summary || latest.SummaryTurnCount != state.SummaryTurnCount {
		return empty, ErrInvalidSourceEvidence
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return empty, fmt.Errorf("%w: result encoding failed", ErrInvalidSourceEvidence)
	}
	if len(payload) > historyMaxTotalBytes {
		return empty, ErrInvalidSourceEvidence
	}
	return result, nil
}

func historyEvidence(now time.Time, source contracts.EvidenceSourceType, ref string,
	authority contracts.EvidenceAuthority, version *string) contracts.EvidenceEnvelopeV1 {
	until := now.Add(2 * time.Minute)
	return contracts.EvidenceEnvelopeV1{
		SchemaVersion: contracts.EvidenceSchemaVersion,
		ResourceRef:   ref, SourceType: source, ResourceVersion: version,
		Authority: authority, Completeness: contracts.EvidencePartial,
		RetrievedAt: now, ValidUntil: &until,
	}
}

func sanitizeText(ctx context.Context, sanitizer ContentSanitizer, kind ContentKind, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	safe, err := sanitizer.Sanitize(ctx, kind, input)
	if err != nil {
		return "", fmt.Errorf("%w: content sanitization was rejected", ErrInvalidSourceEvidence)
	}
	if strings.TrimSpace(safe) == "" || len(safe) > historyMaxTextBytes || !utf8.ValidString(safe) {
		return "", ErrInvalidSourceEvidence
	}
	return safe, nil
}
