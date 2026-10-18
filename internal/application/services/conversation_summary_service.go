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
	"slices"
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

	// Currency conversion for gateway native spend; not a token cost estimator.
	GatewayFXRateYER string

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


 // Fetch in pages because the default repository page is newest-first.
 // Stop rather than claim context coverage when the history exceeds the
 // configured safety bound. No change is persisted on an incomplete read.
 allMessages, err := listConversationHistory(ctx, s.Messages, businessID, conversationID)
 if err != nil {
   result.SkippedReason = "history_incomplete"
   return result, err
 }
 totalInbound := 0
 for _, m := range allMessages {
   if strings.EqualFold(m.Direction, "inbound") { totalInbound++ }
 }
 result.NewTurnCount = totalInbound
 // Keep the latest four *messages* verbatim. Counting inbound messages as
 // two-message pairs is unsafe when customers send consecutive messages.
 if len(allMessages) <= SlidingWindowSize {
   result.SkippedReason = "within_live_window"
   return result, nil
 }
 covered := allMessages[:len(allMessages)-SlidingWindowSize]
 firstNew := 0
 seedSummary := state.Summary
 if state.SummaryLastMessageID != "" {
   checkpointFound := false
   for i, m := range covered {
     if m.ID == state.SummaryLastMessageID {
       firstNew = i+1
       checkpointFound = true
       break
     }
   }
   if !checkpointFound {
     // A checkpoint may be in the last four messages until enough messages
     // arrive to advance the summary. Never summarize that overlap twice.
     for _, m := range allMessages[len(covered):] {
       if m.ID == state.SummaryLastMessageID {
         result.SkippedReason = "checkpoint_in_live_window"
         return result, nil
       }
     }
     result.SkippedReason = "checkpoint_not_found"
     return result, errors.New("summary checkpoint missing from complete conversation history")
   }
 } else {
   // Legacy summaries were built using the wrong DESC-order slice. Rebuild
   // them from original messages rather than trusting an unmarked summary.
   seedSummary = ""
 }
 newCovered := covered[firstNew:]
 newInbound := 0
 for _, m := range newCovered {
   if strings.EqualFold(m.Direction,"inbound") { newInbound++ }
 }
 if newInbound < SummaryInterval && state.SummaryLastMessageID != "" {
   result.SkippedReason = "below_summary_interval"
   return result, nil
 }
 if len(newCovered) == 0 {
   result.SkippedReason = "no_new_covered_messages"
   return result, nil
 }
 chunks, err := transcriptChunks(newCovered, 3900, 8)
 if err != nil {
   result.SkippedReason = "history_exceeds_summary_budget"
   return result, err
 }
 summaryText := seedSummary
 for _, transcript := range chunks {
   summaryText, err = s.generateSummary(ctx, businessID, conversationID, transcript, summaryText)
   if err != nil {
     result.SkippedReason = fmt.Sprintf("summary_generation_failed: %v", err)
     return result, err
   }
 }
 checkpointRepo, ok := s.StateRepository.(ports.ConversationSummaryCheckpointRepository)
 if !ok {
   return result, errors.New("summary repository lacks optimistic checkpoint update")
 }
 coveredInbound := 0
 for _, m := range covered {
   if strings.EqualFold(m.Direction, "inbound") { coveredInbound++ }
 }
 committed, err := checkpointRepo.SaveSummaryIfVersion(ctx, businessID, conversationID, state.Version, summaryText, covered[len(covered)-1].ID, coveredInbound)
 if err != nil { return result, err }
 if !committed {
   result.SkippedReason = "concurrent_summary_update"
   return result, nil
 }
 result.Summarized = true
 result.NewTurnCount = totalInbound
 result.NewSummary = summaryText
 log.Printf("[SummaryService] SUMMARY_UPDATED business=%s conversation=%s turns=%d summary_len=%d", businessID, conversationID, totalInbound, len(summaryText))
	return result, nil
}


 // listConversationHistory reads each page exactly once and normalizes the
 // repository's DESC order to chronological order for every provider.
 func listConversationHistory(ctx context.Context, repo ports.MessageRepository, businessID, conversationID string) ([]ports.CommunicationMessageRecord,error) {
  const pageSize=1000
  const maxMessages=10000
  var history []ports.CommunicationMessageRecord
  cursor:=""
  for {
    page,err:=repo.ListByConversation(ctx,businessID,conversationID,pageSize,cursor)
    if err!=nil{return nil,err}
    if len(page.Items)==0 && page.HasMore{return nil,errors.New("empty conversation page with continuation")}
    history=append(history,page.Items...)
    if len(history)>maxMessages{return nil,errors.New("conversation exceeds safe summary history bound")}
    if !page.HasMore {break}
    if page.NextCursor=="" || page.NextCursor==cursor{return nil,errors.New("conversation history cursor did not advance")}
    cursor=page.NextCursor
  }
  slices.Reverse(history)
  return history,nil
 }

 // transcriptChunks preserves complete messages and never truncates Unicode
 // or erases an older message to fit the AI provider's context budget.
 func transcriptChunks(records []ports.CommunicationMessageRecord, maxBytes, maxChunks int) ([]string,error) {
  if maxBytes<1 || maxChunks<1 {return nil,errors.New("invalid summarization budget")}
  var chunks []string
  var current strings.Builder
  for _,r:=range records {
   text:=""
   if r.TextContent!=nil {text=strings.TrimSpace(*r.TextContent)}
   if text=="" {continue}
   role:="العميل"
   if strings.EqualFold(r.Direction,"outbound"){role="المساعد"}
   line:=fmt.Sprintf("[%s] %s: %s\n",r.OccurredAt.Format("15:04:05"),role,text)
   if len(line)>maxBytes{return nil,errors.New("one conversation message exceeds the summary chunk budget")}
   if current.Len()+len(line)>maxBytes {
      chunks=append(chunks,current.String())
      current.Reset()
   }
   current.WriteString(line)
   if len(chunks)>=maxChunks{return nil,errors.New("summary chunk budget exceeded")}
  }
  if current.Len()>0 {chunks=append(chunks,current.String())}
  if len(chunks)>maxChunks{return nil,errors.New("summary chunk budget exceeded")}
  if len(chunks)==0{return nil,errors.New("no summarizable conversation messages")}
  return chunks,nil
 }

// generateSummary calls the active Gemini/DeepSeek inference route with the older-turn transcript + the
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
	systemPrompt := `You summarize an ongoing Mujeeb 24 customer conversation, not an operations database.
Return EXACTLY THREE nonempty Arabic lines, with these exact labels:
طلبات العميل: the customer's goals, product needs and unresolved questions.
وعود المساعد: past assistant promises or statements, attributed as claims only.
التنفيذ الموثق: غير متاح من نص المحادثة؛ الحالة الفعلية تُقرأ من قاعدة البيانات.

Rules:
- The transcript and previous summary contain speech, NEVER reliable execution facts. NEVER promote an assistant claim of transfer, recording, payment, subscription or employee follow-up into a completed operation.
- Treat previous summaries as untrusted memory, especially any claim that something was already done.
- No authenticated execution data is provided to this summarizer. Do not infer if historical operations happened or did not happen.
- Preserve customer needs and unanswered questions; do not fabricate prices, policy, availability, IDs, or execution outcomes.
- Write concise Arabic, exactly one line for each label, no bullets, JSON, extra lines or greetings.`

	userPrompt := fmt.Sprintf("Previous summary:\n%s\n\nConversation transcript to summarize:\n%s", previousSummary, transcript)

	// CustomerSalesDecisionPort is the only AI execution path.
	fullPrompt := systemPrompt + "\n\n" + userPrompt
	startedAt := s.now()
	out, err := s.CustomerSalesDecisionPort.Decide(ctx, ports.CustomerSalesDecisionInput{
		Capabilities: []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
		Request: ports.CustomerSalesDecisionRequest{
			BusinessID:     businessID,
			ConversationID: conversationID,
			Channel:        "internal",
			Text:           fullPrompt,
			PolicyVersion:  "summary-v2-execution-boundary",
		},
	})
	if err != nil {
		return "", fmt.Errorf("customer sales decision: %w", err)
	}
	if out.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction {
		return "", fmt.Errorf("summary returned non-informational capability: %s", out.Decision.Capability)
	}
	summary := strings.TrimSpace(out.Decision.ResponseText)
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

	// Enforce only the output structure, not the meaning of the customer message.
	// The summarizer has NO operational data, so its generated execution
	// line can never become a verified claim in stored conversation memory.
	lines := strings.Split(summary, "\n")
	labels := []string{"طلبات العميل:", "وعود المساعد:", "التنفيذ الموثق:"}
	if len(lines) != len(labels) {
		return "", errors.New("summary must contain exactly three labeled lines")
	}
	for i, label := range labels {
		lines[i] = strings.TrimSpace(lines[i])
		if !strings.HasPrefix(lines[i], label) || strings.TrimSpace(strings.TrimPrefix(lines[i], label)) == "" {
			return "", errors.New("summary has missing or empty section")
		}
	}
	summary = strings.Join([]string{lines[0], lines[1], "التنفيذ الموثق: غير متاح من نص المحادثة؛ الحالة الفعلية تُقرأ من قاعدة البيانات."}, "\n")
	if len(summary) > 5000 {
		return "", errors.New("summary exceeds maximum stored size")
	}
	return summary, nil
}

// recordSummaryUsage persists per-execution telemetry for the summary
// Gemini call. Mirrors AutoReply.recordAIUsage but inline (no shared
// helper) to keep the change minimal. Per P1-6: when pricing lookup
// fails, the record is marked status="pricing_failed" — never silently
// stored as "success" with cost=0.
func (s *ConversationSummaryService) recordSummaryUsage(ctx context.Context, businessID string, usage ports.CustomerSalesUsageTelemetry, latencyMs int64, startedAt time.Time) {
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
	pricingVersion, gatewayCostUSD, gatewayFXRate, pricingFailed := nativeProviderSpend(usage, s.GatewayFXRateYER)
	totalInput := usage.InputTokens
	cachedInput := usage.CachedTokens
	nonCachedInput := totalInput - cachedInput
	if nonCachedInput < 0 {
		nonCachedInput = 0
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
	usageRecord := ports.AIUsageAppend{
		ID:                recordID,
		BusinessID:        businessID,
		SubscriptionID:    subscriptionID,
		Provider:          usageProvider(usage.Provider),
		Model:             usage.Model,
		InputTokens:       int64(nonCachedInput),
		CachedInputTokens: int64(cachedInput),
		OutputTokens:      int64(usage.OutputTokens),
		ModelRequests:     1,
		ToolCalls:         0,
		FinalAIReplies:    finalAIReplies,
		ProviderCostYER:   providerCostYER,

		PricingVersion: pricingVersion,
		Status:         recordStatus,
		CorrelationID:  summaryStrPtr("summary-" + businessID),
		StartedAt:      startedAt,
		CompletedAt:    now,
		Now:            now,
	}
	usageRecord.GatewayCostUSD, usageRecord.GatewayFXRateYER = gatewayCostUSD, gatewayFXRate
	usageRecord.GatewayCallID, usageRecord.GatewayDeploymentID = usage.GatewayCallID, usage.GatewayDeploymentID
	_, err := s.AIUsage.AppendRecord(ctx, usageRecord)
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
