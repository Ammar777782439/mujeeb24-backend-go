// Package services — ConversationSummaryService implements the Summary +
// Sliding Window hybrid context strategy (ADR-039).
//
// Per research findings (Microsoft Learn, getmaxim.ai, IrisAgent, mem0.ai,
// Oracle blogs), the gold-standard pattern for managing LLM conversation
// context is:
//
//      [Running Summary of older turns] + [Last N turns verbatim]
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
        LLM             ports.AIRuntime // Gemini client for summary generation
        GeminiModel     string
        Now             func() time.Time
}

// NewConversationSummaryService constructs a ConversationSummaryService.
// The LLM is optional — if nil, summarization is skipped (useful for
// environments without Gemini configured).
func NewConversationSummaryService(
        stateRepo ports.ConversationStateRepository,
        messages ports.MessageRepository,
        llm ports.AIRuntime,
        geminiModel string,
) *ConversationSummaryService {
        return &ConversationSummaryService{
                StateRepository: stateRepo,
                Messages:        messages,
                LLM:             llm,
                GeminiModel:     geminiModel,
                Now:             func() time.Time { return time.Now().UTC() },
        }
}

// SummaryTriggerResult describes what happened during a MaybeSummarize call.
type SummaryTriggerResult struct {
        Summarized         bool   // true if a new summary was generated
        PreviousTurnCount  int    // turn count before this call
        NewTurnCount       int    // turn count after this call
        OldSummary         string // summary before this call (for logging)
        NewSummary         string // summary after this call (for logging)
        SkippedReason      string // non-empty if summarization was skipped
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

        if s == nil || s.StateRepository == nil || s.Messages == nil {
                result.SkippedReason = "service_not_configured"
                return result, nil
        }
        if s.LLM == nil {
                result.SkippedReason = "llm_not_configured"
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

        // Generate summary via Gemini
        summaryText, err := s.generateSummary(ctx, transcript, state.Summary)
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
        transcript, previousSummary string,
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

        // Use AIRuntime.Decide to generate the summary. The runtime wraps
        // the ContractClient which calls Gemini.
        // Note: the system prompt for the runtime is CustomerSalesSystemPrompt,
        // not the summary-specific prompt above. We work around this by
        // prepending our summary instructions to the user text. This is a
        // pragmatic compromise; a future ADR should add a dedicated summary
        // endpoint with system_prompt parameter.
        fullPrompt := systemPrompt + "\n\n" + userPrompt
        input := ports.AIDecisionInput{
                BusinessID:     "summary-service",
                ConversationID: "summary-service",
                Channel:        "internal",
                Text:           fullPrompt,
                PolicyVersion:  "summary-v1",
        }

        proposal, err := s.LLM.Decide(ctx, input)
        if err != nil {
                return "", fmt.Errorf("llm decide: %w", err)
        }

        // The summary is the response text — strip any metadata
        summary := strings.TrimSpace(proposal.ResponseText)
        if summary == "" {
                return "", errors.New("empty summary from LLM")
        }

        // Sanity check: limit to 5000 chars to prevent runaway summaries
        if len(summary) > 5000 {
                summary = summary[:5000]
        }

        return summary, nil
}

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
