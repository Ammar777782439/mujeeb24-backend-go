package services

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const (
	CustomerSalesContextSchemaVersion = 1
	AIEvidenceSchemaVersion           = 1
	CustomerSalesContextFresh         = "fresh"
	CustomerSalesContextPartial       = "partial"
	CustomerSalesContextMissing       = "missing"
	CustomerSalesContextStale         = "stale"
	CustomerSalesContextGrounded      = "grounded"
)

// AutoReplyContextBuilder builds a bounded, tenant-scoped context from Mujeeb
// records. It never calls a provider and never treats model output as evidence.
type AutoReplyContextBuilder struct {
	Sales         ports.SalesWorkflowRepository
	Businesses    ports.BusinessRepository
	Conversations ports.ConversationRepository
	Customers     ports.CustomerRepository
	Catalogs      ports.CatalogRepository
	CatalogAI     ports.CatalogAIReadRepository
	Messages      ports.MessageRepository
	Knowledge     ports.KnowledgeDocumentRepository
	Policies      ports.BusinessPolicyRepository
	Now           func() time.Time
	TTL           time.Duration
	MaxCatalogs   int
	MaxItems      int
	MaxOffers     int
	MaxVariants   int
	MaxMessages   int
	MaxKnowledge  int
	MaxPolicies   int
}

func NewAutoReplyContextBuilder(businesses ports.BusinessRepository, conversations ports.ConversationRepository, customers ports.CustomerRepository, catalogs ports.CatalogRepository, messages ports.MessageRepository) AutoReplyContextBuilder {
	return AutoReplyContextBuilder{
		Businesses:    businesses,
		Conversations: conversations,
		Customers:     customers,
		Catalogs:      catalogs,
		Messages:      messages,
		Now:           func() time.Time { return time.Now().UTC() },
		TTL:           2 * time.Minute,
		MaxCatalogs:   10,
		MaxItems:      5,
		MaxOffers:     5,
		MaxVariants:   5,
		MaxMessages:   4, // Exact recent context retained until the per-provider live comparisons pass.
		MaxKnowledge:  10,
		MaxPolicies:   10,
	}
}

func (b AutoReplyContextBuilder) Build(ctx context.Context, input ports.CustomerSalesContextInput) (ports.CustomerSalesContext, error) {
	if strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.ConversationID) == "" || strings.TrimSpace(input.Text) == "" {
		return ports.CustomerSalesContext{}, errors.New("business, conversation, and message text are required to build AI context")
	}
	if b.Businesses == nil || b.Conversations == nil || b.Customers == nil || b.Catalogs == nil || b.Messages == nil {
		return ports.CustomerSalesContext{}, errors.New("AI context builder repositories are not configured")
	}
	now := b.now()
	ttl := b.TTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	business, err := b.Businesses.GetByID(ctx, input.BusinessID)
	if err != nil {
		return ports.CustomerSalesContext{}, err
	}
	if business.ID != input.BusinessID {
		return ports.CustomerSalesContext{}, errors.New("AI context business scope mismatch")
	}
	conversation, err := b.Conversations.GetByID(ctx, input.BusinessID, input.ConversationID)
	if err != nil {
		return ports.CustomerSalesContext{}, err
	}
	if conversation.ID != input.ConversationID || conversation.BusinessID != input.BusinessID {
		return ports.CustomerSalesContext{}, errors.New("AI context conversation scope mismatch")
	}
	customer, err := b.Customers.GetByID(ctx, input.BusinessID, conversation.CustomerID)
	if err != nil {
		return ports.CustomerSalesContext{}, err
	}
	if customer.BusinessID != input.BusinessID {
		return ports.CustomerSalesContext{}, errors.New("AI context customer scope mismatch")
	}

	context := ports.CustomerSalesContext{
		SchemaVersion: CustomerSalesContextSchemaVersion,
		Freshness:     CustomerSalesContextFresh,
		Business: ports.CustomerSalesContextBusiness{
			Reference:       business.ID,
			Name:            business.Name,
			VerticalType:    business.VerticalType,
			Locale:          business.Locale,
			DefaultCurrency: business.DefaultCurrency,
		},
		Conversation: ports.CustomerSalesContextConversation{
			Reference:           conversation.ID,
			CustomerReference:   conversation.CustomerID,
			State:               conversation.State,
			Ownership:           conversation.Ownership,
			Priority:            conversation.Priority,
			AIModeOverride:      stringValue(conversation.AIModeOverride),
			AssignmentReference: stringValue(conversation.AssignmentReference),
			// Per Item 8: carry the conversation's last Gemini
			// interaction ID so AutoReply can pass it as
			// PreviousInteractionID to the next DecideContract
			// call (Gemini Interactions API chaining per §9).
		},
		Customer: ports.CustomerSalesContextCustomer{
			Reference:        customer.ID,
			LocalePreference: stringValue(customer.LocalePreference),
			Status:           customer.Status,
			Profile:          safeJSONDocument(customer.Profile),
			ContactPoints:    safeJSONDocument(customer.ContactPoints),
		},
		PolicyEvidence: ports.CustomerSalesPolicyEvidence{
			Reference:     "application-policy/" + nonEmpty(input.PolicyVersion, "auto-reply-v1"),
			Version:       nonEmpty(input.PolicyVersion, "auto-reply-v1"),
			State:         "application_policy_only",
			MissingReason: "catalog and business policy repository is not configured in this slice",
			RetrievedAt:   now,
			SchemaVersion: AIEvidenceSchemaVersion,
		},
		ConversationState: input.ConversationState,
		// Per ADR-039: pass the conversation summary through to Gemini.
		// The summary is generated by ConversationSummaryService and stored
		// in conversation_state.summary. It contains a compressed view of
		// older conversation turns beyond the sliding window of RecentMessages.
		KnowledgeState: CustomerSalesContextMissing,
		GeneratedAt:    now,
		ExpiresAt:      now.Add(ttl),
	}
	if input.ConversationState != nil {
		context.ConversationSummary = input.ConversationState.Summary
	}

	// A checkpointed summary covers the conversation only up to its message
	// ID. Provide EVERY newer message so the model sees no gap during the
	// four-turn refresh cadence, regardless of provider or customer cadence.
	if input.ConversationState != nil && input.ConversationState.SummaryLastMessageID != "" {
		uncovered, err := listMessagesAfterCheckpoint(ctx, b.Messages, input.BusinessID, input.ConversationID, input.ConversationState.SummaryLastMessageID)
		if err != nil {
			return ports.CustomerSalesContext{}, err
		}
		context.RecentMessages = buildRecentMessageEvidence(uncovered, input.SourceMessageReference, now)
	} else {
		// Legacy summaries have no trusted coverage boundary. Preserve the
		// previous bounded window until the summarizer rebuilds a checkpoint.
		page, err := b.Messages.ListByConversation(ctx, input.BusinessID, input.ConversationID, b.maxMessages(), "")
		if err != nil {
			return ports.CustomerSalesContext{}, err
		}
		context.RecentMessages = buildRecentMessageEvidence(page.Items, input.SourceMessageReference, now)
	}

	// Universal Catalog AI v3: every turn gets a bounded map of the whole
	// active catalog. This is discovery metadata, not executable evidence.
	if b.CatalogAI != nil {
		manifest, manifestErr := b.CatalogAI.GetManifest(ctx, input.BusinessID)
		if manifestErr != nil {
			return ports.CustomerSalesContext{}, manifestErr
		}
		context.CatalogManifest = &manifest
	}

	mode, focus, comparison := resolveRetrievalMode(input.ConversationState)
	switch mode {
	case retrievalScopedOffer, retrievalScopedItem, retrievalScopedCatalog, retrievalScopedVariant:
		scopedItems, scopedOffers, scopedVariants, err := b.retrieveScoped(ctx, input.BusinessID, focus, comparison, now)
		if err != nil {
			// Invalid/stale focus is not validated truth: fall through to broader.
			if !isScopedNotFound(err) {
				return ports.CustomerSalesContext{}, err
			}
		} else {
			context.CatalogEvidence = scopedItems
			context.OfferEvidence = scopedOffers
			context.VariantEvidence = scopedVariants
			return b.finalizeContext(ctx, context, input, now)
		}
	case retrievalScopedComparison:
		scopedItems, scopedOffers, scopedVariants, err := b.retrieveComparison(ctx, input.BusinessID, comparison, now)
		if err != nil {
			if !isScopedNotFound(err) {
				return ports.CustomerSalesContext{}, err
			}
		} else {
			context.CatalogEvidence = scopedItems
			context.OfferEvidence = scopedOffers
			context.VariantEvidence = scopedVariants
			return b.finalizeContext(ctx, context, input, now)
		}
	}
	// Without a validated conversation focus, only the bounded catalog manifest
	// is attached here. Item-level catalog data is handled by the complete
	// catalog evaluation path when the model requests additional data.

	return b.finalizeContext(ctx, context, input, now)
}

// listMessagesAfterCheckpoint collects a chronological, complete delta since
// the summary's last persisted message. The repository pages DESC. Never
// silently truncate uncovered history or assume a missing cursor is valid.
func listMessagesAfterCheckpoint(ctx context.Context, repo ports.MessageRepository, businessID, conversationID, checkpointID string) ([]ports.CommunicationMessageRecord, error) {
	const pageSize = 64
	const maxUncovered = 256
	cursor := ""
	collected := make([]ports.CommunicationMessageRecord, 0, pageSize)
	for {
		page, err := repo.ListByConversation(ctx, businessID, conversationID, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		found := false
		for _, m := range page.Items {
			if m.ID == checkpointID {
				found = true
				break
			}
			collected = append(collected, m)
			if len(collected) > maxUncovered {
				return nil, errors.New("conversation has too many messages not covered by committed summary")
			}
		}
		if found {
			slices.Reverse(collected)
			return collected, nil
		}
		if !page.HasMore {
			return nil, errors.New("conversation summary checkpoint cannot be found in current history")
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return nil, errors.New("conversation checkpoint paging cursor did not advance")
		}
		cursor = page.NextCursor
	}
}

func buildRecentMessageEvidence(records []ports.CommunicationMessageRecord, sourceReference string, now time.Time) []ports.CustomerSalesRecentMessageEvidence {
	items := make([]ports.CustomerSalesRecentMessageEvidence, 0, len(records))
	for _, record := range records {
		if record.ProviderMessageID != nil && strings.TrimSpace(*record.ProviderMessageID) == strings.TrimSpace(sourceReference) {
			continue
		}
		text := ""
		if record.TextContent != nil {
			text = strings.TrimSpace(*record.TextContent)
		}
		if text == "" {
			continue
		}
		items = append(items, ports.CustomerSalesRecentMessageEvidence{
			Reference:     record.ID,
			Direction:     record.Direction,
			Origin:        record.Origin,
			Text:          text,
			OccurredAt:    record.OccurredAt,
			EvidenceState: CustomerSalesContextFresh,
			SchemaVersion: AIEvidenceSchemaVersion,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].OccurredAt.Equal(items[j].OccurredAt) {
			return items[i].OccurredAt.Before(items[j].OccurredAt)
		}
		// Per ADR-051: tiebreaker — Reference (message ID), to fix time inversion
		return items[i].Reference < items[j].Reference
	})
	return items
}

func rankKnowledgeRecords(records []ports.KnowledgeDocumentRecord, text string) []ports.KnowledgeDocumentRecord {
	tokens := tokenize(text)
	type scored struct {
		record ports.KnowledgeDocumentRecord
		score  int
	}
	items := make([]scored, 0, len(records))
	for _, record := range records {
		searchable := normalizeArabic(strings.ToLower(record.KnowledgeKey + " " + record.Title + " " + record.Content))
		score := 0
		for _, token := range tokens {
			if strings.Contains(searchable, token) {
				score++
			}
		}
		items = append(items, scored{record: record, score: score})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].record.ID < items[j].record.ID
	})
	result := make([]ports.KnowledgeDocumentRecord, 0, len(items))
	for _, item := range items {
		result = append(result, item.record)
	}
	return result
}

func rankPolicyRecords(records []ports.BusinessPolicyRecord, text string) []ports.BusinessPolicyRecord {
	tokens := tokenize(text)
	type scored struct {
		record ports.BusinessPolicyRecord
		score  int
	}
	items := make([]scored, 0, len(records))
	for _, record := range records {
		searchable := normalizeArabic(strings.ToLower(record.PolicyKey + " " + record.Category + " " + record.Title + " " + record.Summary + " " + string(record.Rules)))
		score := 0
		for _, token := range tokens {
			if strings.Contains(searchable, token) {
				score++
			}
		}
		items = append(items, scored{record: record, score: score})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].record.ID < items[j].record.ID
	})
	result := make([]ports.BusinessPolicyRecord, 0, len(items))
	for _, item := range items {
		result = append(result, item.record)
	}
	return result
}

func (b AutoReplyContextBuilder) now() time.Time {
	if b.Now == nil {
		return time.Now().UTC()
	}
	return b.Now().UTC()
}

func (b AutoReplyContextBuilder) maxCatalogs() int {
	if b.MaxCatalogs <= 0 {
		return 10
	}
	return b.MaxCatalogs
}

func (b AutoReplyContextBuilder) maxItems() int {
	if b.MaxItems <= 0 {
		return 5
	}
	return b.MaxItems
}

func (b AutoReplyContextBuilder) maxOffers() int {
	if b.MaxOffers <= 0 {
		return 5
	}
	return b.MaxOffers
}

func (b AutoReplyContextBuilder) maxVariants() int {
	if b.MaxVariants <= 0 {
		return 5
	}
	return b.MaxVariants
}

func (b AutoReplyContextBuilder) maxKnowledge() int {
	if b.MaxKnowledge <= 0 {
		return 10
	}
	return b.MaxKnowledge
}

func (b AutoReplyContextBuilder) maxPolicies() int {
	if b.MaxPolicies <= 0 {
		return 10
	}
	return b.MaxPolicies
}

func (b AutoReplyContextBuilder) maxMessages() int {
	if b.MaxMessages <= 0 {
		return 4 // Per ADR-038: 6 turns sliding window per IrisAgent research.
	}
	return b.MaxMessages
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

var _ ports.CustomerSalesContextBuilder = AutoReplyContextBuilder{}
