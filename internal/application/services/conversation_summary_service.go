// Package services — ConversationSummaryService implements the Summary +
// Sliding Window hybrid context strategy (ADR-039).
//
// Per research findings (Microsoft Learn, getmaxim.ai, IrisAgent, mem0.ai,
// Oracle blogs), the gold-standard pattern for managing LLM conversation
// context is:
//
//	[Running Summary of older turns] + [Last N turns verbatim]
//
// The summary is regenerated every N turns (default 4) and captures:
//   - Customer's stated intent and what they asked about
//   - Products mentioned (item names, IDs)
//   - Pricing / availability facts already disclosed
//   - Open questions (pending)
//   - Whether the customer asked for human handoff
//
// This dramatically reduces token consumption for long conversations while
// preserving long-term context that would otherwise fall out of the
// sliding window. The summary is stored in conversation_state.summary +
// summary_turn_count.
//
// Trigger logic: After each successful AutoReply completion, the
// ConversationSummaryService.MaybeSummarize() method is called. It counts
// the conversation turns since the last summary; if >= SummaryInterval,
// it calls Gemini to summarize the older turns (everything before the
// sliding window) and persists the result.
package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// repositoryErrorKind is a minimal interface to check repository error kinds
// without importing the postgres package (which would create an import
// cycle: services → postgres → services).
//
// The postgres.RepositoryError type implements this interface via its
// ErrorKind() method (returns the kind as a string like "not_found").
type kindedError interface {
	ErrorKind() string
}

// isRepositoryNotFound returns true if err is a postgres.RepositoryError
// with Kind == RepositoryNotFound. We check via interface assertion to
// avoid importing the postgres package.
func isRepositoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ke kindedError
	if errors.As(err, &ke) {
		return ke.ErrorKind() == "not_found"
	}
	// Fallback: string match for safety (in case the error doesn't
	// implement ErrorKind but the message contains "not found").
	return strings.Contains(err.Error(), "not found")
}

// SummaryInterval is the number of conversation turns between summary
// regenerations. Per Microsoft Learn guidance: "update the summary after
// every 3-5 turns". We use 4 as the midpoint.
const SummaryInterval = 4

// SlidingWindowSize is the number of recent messages kept verbatim
// alongside the summary. The summary covers everything older than this
// window. Per IrisAgent: "switch to summarization if average turns >10";
// our 4-turn summary + 4-turn sliding window = 8 turns covered, which
// keeps us under the 10-turn threshold where summarization alone becomes
// preferable.
const SlidingWindowSize = 4

// ConversationSummaryService generates and maintains the running summary
// of conversation history. It is invoked after each successful AutoReply
// to check if the summary needs to be refreshed.
type ConversationSummaryService struct {
	StateRepository ports.ConversationStateRepository
	Messages        ports.MessageRepository
	// CustomerSalesDecisionPort is the only AI execution path for summaries.
	// It reads the active AI configuration through the provider wired
	// into the GeminiCustomerSalesAdapter, so summaries use the same customer-sales decision port
	// as AutoReply.
	CustomerSalesDecisionPort ports.CustomerSalesDecisionPort
	// AIUsageRepository records per-execution telemetry for the summary
	// call. Per P1-5: summary Gemini calls MUST be metered — they are
	// not free. If nil, telemetry is logged but not persisted.
	AIUsage ports.AIUsageRepository
	// AIPricingRepository looks up the current pricing version for
	// computing provider_cost_yer on the summary call. If nil, the
	// record is marked status="pricing_failed" per P1-6.
	AIPricing ports.AIProviderPricingRepository
	// Subscriptions looks up the business's active subscription ID so
	// the summary usage record is associated with the correct
	// subscription (same as AutoReply.recordAIUsage).
	Subscriptions ports.SubscriptionRepository
	Now           func() time.Time
	NewID         func() string
	// CostProtection (optional) is the shared AI execution gate / Entitlement /
	// Cost-Budget boundary. Per Item 2: when wired, MaybeSummarize
	// checks it BEFORE calling Gemini — so the Platform Admin's
	// platformAIDisable call (Contract §81) blocks this background
	// summarization path too. The summary is best-effort async work;
	// when the runtime is disabled, it MUST NOT silently consume
	// Gemini quota via the summary goroutine.
	CostProtection AICostProtectionChecker
}

// NewConversationSummaryService constructs a ConversationSummaryService.
// CustomerSalesDecisionPort is required; there is no legacy AI fallback.
func NewConversationSummaryService(
	stateRepo ports.ConversationStateRepository,
	messages ports.MessageRepository,
	customerSalesDecision ports.CustomerSalesDecisionPort,
) *ConversationSummaryService {
	return &ConversationSummaryService{
		StateRepository:           stateRepo,
		Messages:                  messages,
		CustomerSalesDecisionPort: customerSalesDecision,
		Now:                       func() time.Time { return time.Now().UTC() },
	}
}

// SummaryTriggerResult describes what happened during a MaybeSummarize call.
type SummaryTriggerResult struct {
	Summarized        bool   // true if a new summary was generated
	PreviousTurnCount int    // turn count before this call
	NewTurnCount      int    // turn count after this call
	OldSummary        string // summary before this call (for logging)
	NewSummary        string // summary after this call (for logging)
	SkippedReason     string // non-empty if summarization was skipped
}

// MaybeSummarize checks if the conversation needs a summary refresh and
// generates one if so. It is safe to call after every AutoReply; it will
// be a no-op when the turn count hasn't reached the threshold.
//
// Turn counting: 1 turn = 1 inbound customer message. We count inbound
// messages as the turn counter (outbound messages are responses, not
// turns initiated by the customer).
func (s *ConversationSummaryService) MaybeSummarize(
	ctx context.Context,
	businessID, conversationID string,
) (SummaryTriggerResult, error) {
	result := SummaryTriggerResult{}

	// Per Item 2: check the SHARED AICostProtectionChecker BEFORE any
	// other check. If the kill switch is DISABLED, there's no point
	// checking whether the service is configured or whether the LLM
	// is wired — Gemini won't be called regardless. This makes the
	// kill switch the FIRST gate, proving it fires even when the
	// service isn't fully wired.
	if s != nil && s.CostProtection != nil {
		allowed, reason := s.CostProtection.IsAIExecutionAllowed(ctx, businessID)
		if !allowed {
			result.SkippedReason = "ai_runtime_disabled: " + reason
			log.Printf("[SummaryService] SKIPPED business=%s conversation=%s reason=%s", businessID, conversationID, reason)
			return result, nil
		}
	}

	if s == nil || s.StateRepository == nil || s.Messages == nil {
		result.SkippedReason = "service_not_configured"
		return result, nil
	}
	if s.CustomerSalesDecisionPort == nil {
		result.SkippedReason = "customer_sales_decision_not_configured"
		return result, nil
	}

	// Load current state
	state, err := s.StateRepository.Get(ctx, businessID, conversationID)
	if err != nil {
		if isRepositoryNotFound(err) {
			// State doesn't exist yet — no conversation to summarize.
			result.SkippedReason = "state_not_found"
			return result, nil
		}
		result.SkippedReason = fmt.Sprintf("state_load_failed: %v", err)
		return result, err
	}

	result.OldSummary = state.Summary
	result.PreviousTurnCount = state.SummaryTurnCount

	// Count inbound messages (turns) since the last summary
	page, err := s.Messages.ListByConversation(ctx, businessID, conversationID, 1000, "")
	if err != nil {
		result.SkippedReason = fmt.Sprintf("message_list_failed: %v", err)
		return result, err
	}
	totalInbound := 0
	for _, m := range page.Items {
		if strings.EqualFold(m.Direction, "inbound") {
			totalInbound++
		}
	}
	result.NewTurnCount = totalInbound

	// Decide if we need to regenerate
	turnsSinceLastSummary := totalInbound - state.SummaryTurnCount
	if turnsSinceLastSummary < SummaryInterval {
		result.SkippedReason = fmt.Sprintf("below_threshold: %d < %d", turnsSinceLastSummary, SummaryInterval)
		return result, nil
	}

	// We need to summarize. We already have all messages in page.Items
	// (fetched above with limit=1000). Slice off the older turns.
	allMessages := page.Items
	cutoffIdx := len(allMessages) - (SlidingWindowSize * 2)
	if cutoffIdx < 0 {
		cutoffIdx = 0
	}
	olderMessages := allMessages[:cutoffIdx]
	if len(olderMessages) == 0 {
		result.SkippedReason = "no_older_messages"
		return result, nil
	}

	// Build the transcript text
	transcript := buildTranscript(olderMessages)
	// Per ADR-051: cap transcript to 4000 chars to avoid exceeding
	// the 12000 char limit in Gemini client. Also ensure chronological
	// order (oldest first) — ListByConversation returns ASC after
	// reversal, but olderMessages[:cutoffIdx] takes the first (oldest)
	// entries which is correct. The cap prevents overflow.
	if len(transcript) > 4000 {
		transcript = transcript[:4000]
	}

	// Generate summary via Gemini (per P1-5: use CustomerSalesDecisionPort when
	// wired so the dynamic AI config applies).
	summaryText, err := s.generateSummary(ctx, businessID, conversationID, transcript, state.Summary)
	if err != nil {
		result.SkippedReason = fmt.Sprintf("summary_generation_failed: %v", err)
		log.Printf("[SummaryService] GENERATION_FAILED business=%s conversation=%s err=%v", businessID, conversationID, err)
		return result, err
	}
	if strings.TrimSpace(summaryText) == "" {
		result.SkippedReason = "empty_summary_returned"
		return result, nil
	}

	// Persist the new summary + turn count
	state.Summary = summaryText
	state.SummaryTurnCount = totalInbound - SlidingWindowSize
	if _, err := s.StateRepository.UpsertValidated(ctx, state); err != nil {
		result.SkippedReason = fmt.Sprintf("state_persist_failed: %v", err)
		return result, err
	}

	result.Summarized = true
	result.NewSummary = summaryText
	log.Printf("[SummaryService] SUMMARY_UPDATED business=%s conversation=%s turns=%d summary_len=%d",
		businessID, conversationID, totalInbound, len(summaryText))
	return result, nil
}

// generateSummary calls Gemini with the older-turn transcript + the
// previous running summary, and returns a fresh compressed summary.
//
// The prompt is intentionally minimal: we ask Gemini to compress the
// conversation into key facts that will help future responses. We use a
// plain text response (not structured outputs) because we're storing
// the result as TEXT in the database.
func (s *ConversationSummaryService) generateSummary(
	ctx context.Context,
	businessID, conversationID, transcript, previousSummary string,
) (string, error) {
	systemPrompt := `You are a conversation summarizer for the Mujeeb 24 customer service AI.

Your job: read the conversation transcript and produce a CONCISE running summary in Arabic that captures the key facts needed for future AI responses.

Capture these elements (skip any that don't apply):
1. نية العميل (what the customer wants)
2. المنتجات المذكورة (product names + IDs if available)
3. الأسعار والتوفر المُعلنة (any prices/availability disclosed)
4. الأسئلة المعلقة (open questions not yet answered)
5. السياسات المذكورة (any business policies referenced)
6. حالة التسليم البشري (whether customer requested human handoff)

Rules:
- Maximum 200 words.
- Use plain Arabic prose, not bullet points.
- Do NOT include greetings or pleasantries — only facts.
- Do NOT include prices or availability that the AI couldn't confirm (only confirmed facts from the transcript).
- If the previous summary is provided, integrate it — don't repeat what's there, add new info.
- Output ONLY the summary text — no JSON, no markdown, no explanation.`

	userPrompt := fmt.Sprintf("Previous summary:\n%s\n\nConversation transcript to summarize:\n%s", previousSummary, transcript)

	// CustomerSalesDecisionPort is the only AI execution path.
	fullPrompt := systemPrompt + "\n\n" + userPrompt
	startedAt := s.now()
	out, err := s.CustomerSalesDecisionPort.Decide(ctx, ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{
			BusinessID:     businessID,
			ConversationID: conversationID,
			Channel:        "internal",
			Text:           fullPrompt,
			PolicyVersion:  "summary-v1",
		},
	})
	if err != nil {
		return "", fmt.Errorf("customer sales decision: %w", err)
	}
	summary := strings.TrimSpace(out.Proposal.ResponseText)
	usageTelemetry := out.Usage
	latencyMs := out.LatencyMs

	// Per P1-5: record usage telemetry for the summary call. This is
	// a real Gemini invocation that consumes tokens — it MUST be
	// metered like any other AI execution. The same P1-6 logic
	// applies: if pricing lookup fails, the record is marked
	// status="pricing_failed" and does NOT count as a successful
	// AI Reply (final_ai_replies=0).
	s.recordSummaryUsage(ctx, businessID, usageTelemetry, latencyMs, startedAt)

	// The summary is the response text — strip any metadata
	if summary == "" {
		return "", errors.New("empty summary from LLM")
	}

	// Sanity check: limit to 5000 chars to prevent runaway summaries
	if len(summary) > 5000 {
		summary = summary[:5000]
	}

	return summary, nil
}

// recordSummaryUsage persists per-execution telemetry for the summary
// Gemini call. Mirrors AutoReply.recordAIUsage but inline (no shared
// helper) to keep the change minimal. Per P1-6: when pricing lookup
// fails, the record is marked status="pricing_failed" — never silently
// stored as "success" with cost=0.
func (s *ConversationSummaryService) recordSummaryUsage(ctx context.Context, businessID string, usage ports.ContractUsageTelemetry, latencyMs int64, startedAt time.Time) {
	if s.AIUsage == nil {
		log.Printf("[SummaryService] AI_USAGE_SKIP business=%s reason=AIUsage_repository_not_wired", businessID)
		return
	}
	if usage.Model == "" {
		// No model reported by the runtime — nothing meaningful to record.
		log.Printf("[SummaryService] AI_USAGE_SKIP business=%s reason=no_model_in_telemetry", businessID)
		return
	}
	now := s.now()
	// Find the business's active subscription ID for association.
	var subscriptionID string
	if s.Subscriptions != nil {
		page, err := s.Subscriptions.List(ctx, ports.SubscriptionListFilter{
			BusinessID: businessID, Status: "ACTIVE", Limit: 1,
		})
		if err == nil && len(page.Items) > 0 {
			subscriptionID = page.Items[0].ID
		}
	}
	if subscriptionID == "" {
		log.Printf("[SummaryService] AI_USAGE_SKIP business=%s reason=no_active_subscription", businessID)
		return
	}
	// Compute provider_cost from the pricing table (per P1-6: never
	// silently store cost=0 as "success").
	providerCostYER := 0
	pricingVersion := "unknown"
	pricingFailed := false
	totalInput := usage.InputTokens
	cachedInput := usage.CachedTokens
	nonCachedInput := totalInput - cachedInput
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	if s.AIPricing != nil {
		pricing, err := s.AIPricing.GetCurrentForProvider(ctx, "google_gemini", usage.Model)
		if err == nil {
			pricingVersion = pricing.PricingVersion
			inputCost := int64(nonCachedInput) * int64(pricing.InputPerMillionYER) / 1_000_000
			cachedCost := int64(cachedInput) * int64(pricing.CachedInputPerMillionYER) / 1_000_000
			outputCost := int64(usage.OutputTokens) * int64(pricing.OutputPerMillionYER) / 1_000_000
			providerCostYER = int(inputCost + cachedCost + outputCost)
		} else {
			log.Printf("[SummaryService] AI_USAGE_PRICING_LOOKUP_FAILED business=%s model=%s err=%v", businessID, usage.Model, err)
			pricingFailed = true
		}
	} else {
		log.Printf("[SummaryService] AI_USAGE_PRICING_REPO_NOT_WIRED business=%s model=%s", businessID, usage.Model)
		pricingFailed = true
	}
	// Per §3: summary calls do NOT count as final AI Replies —
	// they're internal background work, not customer-facing responses.
	// The customer-facing entitlement is incremented only by the
	// AutoReply.recordAIUsage path. The summary record is persisted
	// with final_ai_replies=0 so the platform admin sees the tokens
	// spent on summarization but the merchant's entitlement is not
	// consumed by background work.
	finalAIReplies := 0
	recordStatus := "success"
	if pricingFailed {
		recordStatus = "pricing_failed"
	}
	recordID := s.newID()
	_, err := s.AIUsage.AppendRecord(ctx, ports.AIUsageAppend{
		ID:                recordID,
		BusinessID:        businessID,
		SubscriptionID:    subscriptionID,
		Provider:          "google_gemini",
		Model:             usage.Model,
		InputTokens:       int64(nonCachedInput),
		CachedInputTokens: int64(cachedInput),
		OutputTokens:      int64(usage.OutputTokens),
		ModelRequests:     1,
		ToolCalls:         0,
		FinalAIReplies:    finalAIReplies,
		ProviderCostYER:   providerCostYER,
		PricingVersion:    pricingVersion,
		Status:            recordStatus,
		CorrelationID:     summaryStrPtr("summary-" + businessID),
		StartedAt:         startedAt,
		CompletedAt:       now,
		Now:               now,
	})
	if err != nil {
		log.Printf("[SummaryService] AI_USAGE_RECORD_FAILED business=%s subscription=%s err=%v", businessID, subscriptionID, err)
		return
	}
	log.Printf("[SummaryService] AI_USAGE_RECORDED business=%s subscription=%s tokens_in=%d tokens_cached=%d tokens_out=%d cost_yer=%d pricing=%s status=%s",
		businessID, subscriptionID, usage.InputTokens, usage.CachedTokens, usage.OutputTokens, providerCostYER, pricingVersion, recordStatus)
}

func (s *ConversationSummaryService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *ConversationSummaryService) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return uuid.NewString()
}

func summaryStrPtr(s string) *string { return &s }

// buildTranscript converts message records into a readable transcript for
// the summarizer. It marks each line with who said it (customer vs agent)
// and includes the timestamp for chronological context.
func buildTranscript(records []ports.CommunicationMessageRecord) string {
	var b strings.Builder
	for _, r := range records {
		text := ""
		if r.TextContent != nil {
			text = strings.TrimSpace(*r.TextContent)
		}
		if text == "" {
			continue
		}
		role := "العميل"
		if strings.EqualFold(r.Direction, "outbound") {
			role = "المساعد"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", r.OccurredAt.Format("15:04:05"), role, text)
	}
	return b.String()
}
