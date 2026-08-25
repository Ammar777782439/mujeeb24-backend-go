package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const (
	AIContextSchemaVersion  = 1
	AIEvidenceSchemaVersion = 1
	AIContextFresh          = "fresh"
	AIContextPartial        = "partial"
	AIContextMissing        = "missing"
	AIContextStale          = "stale"
)

// AutoReplyContextBuilder builds a bounded, tenant-scoped context from Mujeeb
// records. It never calls a provider and never treats model output as evidence.
type AutoReplyContextBuilder struct {
	Businesses    ports.BusinessRepository
	Conversations ports.ConversationRepository
	Customers     ports.CustomerRepository
	Catalogs      ports.CatalogRepository
	Messages      ports.MessageRepository
	Now           func() time.Time
	TTL           time.Duration
	MaxCatalogs   int
	MaxItems      int
	MaxOffers     int
	MaxVariants   int
	MaxMessages   int
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
		MaxMessages:   8,
	}
}

func (b AutoReplyContextBuilder) Build(ctx context.Context, input ports.ContextBuildInput) (ports.AIContext, error) {
	if strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.ConversationID) == "" || strings.TrimSpace(input.Text) == "" {
		return ports.AIContext{}, errors.New("business, conversation, and message text are required to build AI context")
	}
	if b.Businesses == nil || b.Conversations == nil || b.Customers == nil || b.Catalogs == nil || b.Messages == nil {
		return ports.AIContext{}, errors.New("AI context builder repositories are not configured")
	}
	now := b.now()
	ttl := b.TTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	business, err := b.Businesses.GetByID(ctx, input.BusinessID)
	if err != nil {
		return ports.AIContext{}, err
	}
	if business.ID != input.BusinessID {
		return ports.AIContext{}, errors.New("AI context business scope mismatch")
	}
	conversation, err := b.Conversations.GetByID(ctx, input.BusinessID, input.ConversationID)
	if err != nil {
		return ports.AIContext{}, err
	}
	if conversation.ID != input.ConversationID || conversation.BusinessID != input.BusinessID {
		return ports.AIContext{}, errors.New("AI context conversation scope mismatch")
	}
	customer, err := b.Customers.GetByID(ctx, input.BusinessID, conversation.CustomerID)
	if err != nil {
		return ports.AIContext{}, err
	}
	if customer.BusinessID != input.BusinessID {
		return ports.AIContext{}, errors.New("AI context customer scope mismatch")
	}

	context := ports.AIContext{
		SchemaVersion: AIContextSchemaVersion,
		Freshness:     AIContextFresh,
		Business: ports.AIContextBusiness{
			Reference:       business.ID,
			Name:            business.Name,
			VerticalType:    business.VerticalType,
			Locale:          business.Locale,
			DefaultCurrency: business.DefaultCurrency,
		},
		Conversation: ports.AIContextConversation{
			Reference:           conversation.ID,
			CustomerReference:   conversation.CustomerID,
			State:               conversation.State,
			Ownership:           conversation.Ownership,
			Priority:            conversation.Priority,
			AIModeOverride:      stringValue(conversation.AIModeOverride),
			AssignmentReference: stringValue(conversation.AssignmentReference),
		},
		Customer: ports.AIContextCustomer{
			Reference:        customer.ID,
			LocalePreference: stringValue(customer.LocalePreference),
			Status:           customer.Status,
			Profile:          safeJSONDocument(customer.Profile),
			ContactPoints:    safeJSONDocument(customer.ContactPoints),
		},
		PolicyEvidence: ports.AIPolicyEvidence{
			Reference:     "application-policy/" + nonEmpty(input.PolicyVersion, "auto-reply-v1"),
			Version:       nonEmpty(input.PolicyVersion, "auto-reply-v1"),
			State:         "application_policy_only",
			MissingReason: "catalog and business policy repository is not configured in this slice",
			RetrievedAt:   now,
			SchemaVersion: AIEvidenceSchemaVersion,
		},
		KnowledgeState: AIContextMissing,
		GeneratedAt:    now,
		ExpiresAt:      now.Add(ttl),
	}

	messagePage, err := b.Messages.ListByConversation(ctx, input.BusinessID, input.ConversationID, b.maxMessages(), "")
	if err != nil {
		return ports.AIContext{}, err
	}
	context.RecentMessages = buildRecentMessageEvidence(messagePage.Items, input.SourceMessageReference, now)

	catalogPage, err := b.Catalogs.ListCatalogs(ctx, input.BusinessID, "active", b.maxCatalogs(), "")
	if err != nil {
		return ports.AIContext{}, err
	}
	for _, catalog := range catalogPage.Items {
		if catalog.BusinessID != input.BusinessID {
			return ports.AIContext{}, errors.New("AI context catalog scope mismatch")
		}
		items, listErr := b.Catalogs.ListCatalogItems(ctx, input.BusinessID, catalog.ID, "", "active", b.maxItems()*3, "")
		if listErr != nil {
			return ports.AIContext{}, listErr
		}
		for _, item := range rankCatalogItems(items.Items, input.Text) {
			if item.BusinessID != input.BusinessID || item.CatalogID != catalog.ID {
				return ports.AIContext{}, errors.New("AI context catalog item scope mismatch")
			}
			context.CatalogEvidence = append(context.CatalogEvidence, ports.AICatalogEvidence{
				Reference:        item.ID,
				CatalogReference: item.CatalogID,
				ItemType:         item.ItemType,
				Name:             item.Name,
				Status:           item.Status,
				Attributes:       safeJSONObject(item.Attributes),
				EvidenceState:    AIContextFresh,
				RetrievedAt:      now,
				SchemaVersion:    AIEvidenceSchemaVersion,
			})
			if len(context.CatalogEvidence) >= b.maxItems() {
				break
			}
		}
		if len(context.CatalogEvidence) >= b.maxItems() {
			break
		}
	}

	for _, item := range context.CatalogEvidence {
		offers, listErr := b.Catalogs.ListOffers(ctx, input.BusinessID, item.Reference, "active", b.maxOffers(), "")
		if listErr != nil {
			return ports.AIContext{}, listErr
		}
		for _, offer := range offers.Items {
			if offer.BusinessID != input.BusinessID || offer.CatalogItemID != item.Reference {
				return ports.AIContext{}, errors.New("AI context offer scope mismatch")
			}
			evidenceState := AIContextFresh
			if strings.EqualFold(strings.TrimSpace(offer.AvailabilityStatus), "unknown") || strings.EqualFold(strings.TrimSpace(offer.AvailabilityStatus), "stale") {
				evidenceState = AIContextStale
			}
			context.OfferEvidence = append(context.OfferEvidence, ports.AIOfferEvidence{
				Reference:            offer.ID,
				CatalogItemReference: offer.CatalogItemID,
				VariantReference:     stringValue(offer.VariantID),
				Name:                 offer.Name,
				PricingMode:          offer.PricingMode,
				Amount:               stringValue(offer.Amount),
				Currency:             stringValue(offer.Currency),
				AvailabilityState:    offer.AvailabilityStatus,
				Status:               offer.Status,
				EvidenceState:        evidenceState,
				RetrievedAt:          now,

				SchemaVersion: AIEvidenceSchemaVersion,
			})
		}
		variants, listErr := b.Catalogs.ListVariants(ctx, input.BusinessID, item.Reference, "active", b.maxVariants(), "")
		if listErr != nil {
			return ports.AIContext{}, listErr
		}
		for _, variant := range variants.Items {
			if variant.BusinessID != input.BusinessID || variant.CatalogItemID != item.Reference {
				return ports.AIContext{}, errors.New("AI context variant scope mismatch")
			}
			context.VariantEvidence = append(context.VariantEvidence, ports.AIVariantEvidence{
				Reference:            variant.ID,
				CatalogItemReference: variant.CatalogItemID,
				Name:                 variant.Name,
				Status:               variant.Status,
				Attributes:           safeJSONObject(variant.Attributes),
				EvidenceState:        AIContextFresh,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			})
		}
	}
	if len(context.CatalogEvidence) > 0 || len(context.OfferEvidence) > 0 || len(context.VariantEvidence) > 0 {
		context.KnowledgeState = AIContextPartial
	}
	if len(context.CatalogEvidence) == 0 {
		context.Freshness = AIContextPartial
	}
	for _, offer := range context.OfferEvidence {
		if offer.EvidenceState == AIContextStale {
			context.Freshness = AIContextStale
			context.KnowledgeState = AIContextPartial
			break
		}
	}
	return context, nil
}

func buildRecentMessageEvidence(records []ports.CommunicationMessageRecord, sourceReference string, now time.Time) []ports.AIRecentMessageEvidence {
	items := make([]ports.AIRecentMessageEvidence, 0, len(records))
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
		items = append(items, ports.AIRecentMessageEvidence{
			Reference:     record.ID,
			Direction:     record.Direction,
			Origin:        record.Origin,
			Text:          text,
			OccurredAt:    record.OccurredAt,
			EvidenceState: AIContextFresh,
			SchemaVersion: AIEvidenceSchemaVersion,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].OccurredAt.Before(items[j].OccurredAt) })
	return items
}

func rankCatalogItems(items []ports.CatalogItemRecord, text string) []ports.CatalogItemRecord {
	queryTokens := tokenize(text)
	type scored struct {
		item  ports.CatalogItemRecord
		score int
	}
	scoredItems := make([]scored, 0, len(items))
	for _, item := range items {
		searchable := strings.ToLower(item.Name + " " + string(item.Attributes))
		score := 0
		for _, token := range queryTokens {
			if strings.Contains(searchable, token) {
				score++
			}
		}
		if score > 0 {
			scoredItems = append(scoredItems, scored{item: item, score: score})
		}
	}
	sort.SliceStable(scoredItems, func(i, j int) bool {
		if scoredItems[i].score != scoredItems[j].score {
			return scoredItems[i].score > scoredItems[j].score
		}
		return scoredItems[i].item.ID < scoredItems[j].item.ID
	})
	result := make([]ports.CatalogItemRecord, 0, len(scoredItems))
	for _, value := range scoredItems {
		result = append(result, value.item)
	}
	return result
}

func tokenize(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,!?؟:;()[]{}\"'")
		if len([]rune(field)) >= 2 {
			result = append(result, field)
		}
	}
	return result
}

func safeJSONObject(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil {
		return []byte(`{}`)
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return []byte(`{}`)
	}
	return encoded
}

func safeJSONDocument(value []byte) []byte {
	if len(value) == 0 || !json.Valid(value) {
		return []byte(`{}`)
	}
	return append([]byte(nil), value...)
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

func (b AutoReplyContextBuilder) maxMessages() int {
	if b.MaxMessages <= 0 {
		return 8
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

var _ ports.AIContextBuilder = AutoReplyContextBuilder{}
