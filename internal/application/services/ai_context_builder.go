package services

import (
        "context"
        "encoding/json"
        "errors"
        "sort"
        "strconv"
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
        AIContextGrounded       = "grounded"
)

// AutoReplyContextBuilder builds a bounded, tenant-scoped context from Mujeeb
// records. It never calls a provider and never treats model output as evidence.
type AutoReplyContextBuilder struct {
        Businesses    ports.BusinessRepository
        Conversations ports.ConversationRepository
        Customers     ports.CustomerRepository
        Catalogs      ports.CatalogRepository
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
                MaxMessages:   6, // Per ADR-038: reduced from 8 to 6 per IrisAgent/Microsoft Learn best-practice research (sliding window threshold).
                MaxKnowledge:  10,
                MaxPolicies:   10,
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
                ConversationState: input.ConversationState,
                KnowledgeState:    AIContextMissing,
                GeneratedAt:       now,
                ExpiresAt:         now.Add(ttl),
        }

        messagePage, err := b.Messages.ListByConversation(ctx, input.BusinessID, input.ConversationID, b.maxMessages(), "")
        if err != nil {
                return ports.AIContext{}, err
        }
        context.RecentMessages = buildRecentMessageEvidence(messagePage.Items, input.SourceMessageReference, now)

        mode, focus, comparison := resolveRetrievalMode(input.ConversationState)
        switch mode {
        case retrievalScopedOffer, retrievalScopedItem, retrievalScopedCatalog, retrievalScopedVariant:
                scopedItems, scopedOffers, scopedVariants, err := b.retrieveScoped(ctx, input.BusinessID, focus, comparison, now)
                if err != nil {
                        // Invalid/stale focus is not validated truth: fall through to broader.
                        if !isScopedNotFound(err) {
                                return ports.AIContext{}, err
                        }
                } else {
                        context.CatalogEvidence = scopedItems
                        context.OfferEvidence = scopedOffers
                        context.VariantEvidence = scopedVariants
                        // Candidates let AI resolve a switch to a new entity. Focused
                        // evidence stays first; validation still rejects mixing.
                        if addItems, addOffers, addVariants, err := b.augmentScopedWithCandidates(ctx, input.BusinessID, input.Text, scopedItems, now); err != nil {
                                return ports.AIContext{}, err
                        } else {
                                context.CatalogEvidence = append(context.CatalogEvidence, addItems...)
                                context.OfferEvidence = append(context.OfferEvidence, addOffers...)
                                context.VariantEvidence = append(context.VariantEvidence, addVariants...)
                        }
                        return b.finalizeContext(ctx, context, input, now)
                }
        case retrievalScopedComparison:
                scopedItems, scopedOffers, scopedVariants, err := b.retrieveComparison(ctx, input.BusinessID, comparison, now)
                if err != nil {
                        if !isScopedNotFound(err) {
                                return ports.AIContext{}, err
                        }
                } else {
                        context.CatalogEvidence = scopedItems
                        context.OfferEvidence = scopedOffers
                        context.VariantEvidence = scopedVariants
                        // Same candidate augmentation as scoped mode, so the AI can leave
                        // the comparison cleanly when the customer asks something new.
                        if addItems, addOffers, addVariants, err := b.augmentScopedWithCandidates(ctx, input.BusinessID, input.Text, scopedItems, now); err != nil {
                                return ports.AIContext{}, err
                        } else {
                                context.CatalogEvidence = append(context.CatalogEvidence, addItems...)
                                context.OfferEvidence = append(context.OfferEvidence, addOffers...)
                                context.VariantEvidence = append(context.VariantEvidence, addVariants...)
                        }
                        return b.finalizeContext(ctx, context, input, now)
                }
        }
        // Broader retrieval: active catalogs, active items, lexical rank.
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
                                Reference:            item.ID,
                                CatalogReference:     item.CatalogID,
                                ItemType:             item.ItemType,
                                Name:                 item.Name,
                                Status:               item.Status,
                                Attributes:           safeJSONObject(item.Attributes),
                                EvidenceState:        AIContextFresh,
                                RetrievedAt:          now,
                                SchemaVersion:        AIEvidenceSchemaVersion,
                                ShortDescription:     item.ShortDescription,
                                LongDescription:      item.LongDescription,
                                PricingMode:          item.PricingMode,
                                AvailabilityMode:     item.AvailabilityMode,
                                FulfillmentMode:      item.FulfillmentMode,
                                RequiresConfirmation: item.RequiresConfirmation,
                        })
                        if len(context.CatalogEvidence) >= b.maxItems() {
                                break
                        }
                }
                if len(context.CatalogEvidence) >= b.maxItems() {
                        break
                }
        }

        // Build the catalog summary: fetch ALL active item names (lightweight)
        // so Gemini knows the full catalog exists even though only MaxItems
        // have detailed evidence. This prevents Gemini from saying "we don't
        // have this product" for products that exist but weren't in the 5-item
        // detailed sample.
        for _, catalog := range catalogPage.Items {
                summaryCursor := ""
                for {
                        summaryItems, summaryErr := b.Catalogs.ListCatalogItems(ctx, input.BusinessID, catalog.ID, "", "active", 500, summaryCursor)
                        if summaryErr != nil {
                                break
                        }
                        for _, item := range summaryItems.Items {
                                context.CatalogSummary = append(context.CatalogSummary, ports.CatalogSummaryEntry{
                                        ID:   item.ID,
                                        Name: item.Name,
                                })
                        }
                        if !summaryItems.HasMore {
                                break
                        }
                        summaryCursor = summaryItems.NextCursor
                }
        }

        if b.Knowledge != nil {
                knowledgeRecords, listErr := b.Knowledge.ListPublished(ctx, input.BusinessID, "", now, b.maxKnowledge()*3)
                if listErr != nil {
                        return ports.AIContext{}, listErr
                }
                for _, record := range rankKnowledgeRecords(knowledgeRecords, input.Text) {
                        if record.BusinessID != input.BusinessID {
                                return ports.AIContext{}, errors.New("AI context knowledge scope mismatch")
                        }
                        context.KnowledgeEvidence = append(context.KnowledgeEvidence, ports.AIKnowledgeEvidence{
                                Reference: record.ID, KnowledgeKey: record.KnowledgeKey, Title: record.Title, Content: record.Content,
                                ContentType: record.ContentType, SourceReference: record.SourceReference, Authority: record.Authority,
                                EvidenceState: evidenceStateForValidity(now, record.ValidFrom, record.ValidUntil), Version: record.Version, ValidFrom: record.ValidFrom, ValidUntil: record.ValidUntil,
                                RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
                        })
                        if len(context.KnowledgeEvidence) >= b.maxKnowledge() {
                                break
                        }
                }
        }
        if b.Policies != nil {
                policyRecords, listErr := b.Policies.ListPublished(ctx, input.BusinessID, "", now, b.maxPolicies()*3)
                if listErr != nil {
                        return ports.AIContext{}, listErr
                }
                for _, record := range rankPolicyRecords(policyRecords, input.Text) {
                        if record.BusinessID != input.BusinessID {
                                return ports.AIContext{}, errors.New("AI context policy scope mismatch")
                        }
                        context.BusinessPolicyEvidence = append(context.BusinessPolicyEvidence, ports.AIBusinessPolicyEvidence{
                                Reference: record.ID, PolicyKey: record.PolicyKey, Category: record.Category, Title: record.Title,
                                Summary: record.Summary, Rules: safeJSONObject(record.Rules), Authority: record.Authority,
                                EvidenceState: evidenceStateForValidity(now, record.ValidFrom, record.ValidUntil), Version: record.Version, ValidFrom: record.ValidFrom, ValidUntil: record.ValidUntil,
                                RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
                        })
                        if len(context.BusinessPolicyEvidence) >= b.maxPolicies() {
                                break
                        }
                }
                if len(context.BusinessPolicyEvidence) > 0 {
                        first := context.BusinessPolicyEvidence[0]
                        context.PolicyEvidence = ports.AIPolicyEvidence{Reference: first.Reference, Version: "policy-v" + formatInt(first.Version), State: "published", RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion}
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
        if len(context.KnowledgeEvidence) > 0 || len(context.BusinessPolicyEvidence) > 0 {
                context.KnowledgeState = AIContextGrounded
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
        for _, evidence := range context.KnowledgeEvidence {
                if evidence.EvidenceState == AIContextStale {
                        context.Freshness = AIContextStale
                        context.KnowledgeState = AIContextPartial
                        break
                }
        }
        for _, evidence := range context.BusinessPolicyEvidence {
                if evidence.EvidenceState == AIContextStale {
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
                searchable := normalizeArabic(strings.ToLower(item.Name + " " + item.ItemType + " " + string(item.Attributes)))
                score := 0
                for _, token := range queryTokens {
                        if strings.Contains(searchable, token) {
                                score++
                        }
                }
                scoredItems = append(scoredItems, scored{item: item, score: score})
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

func normalizeArabic(s string) string {
        var b strings.Builder
        for _, r := range s {
                switch r {
                case 'أ', 'إ', 'آ', 'ٱ':
                        b.WriteRune('ا')
                case 'ة':
                        b.WriteRune('ه')
                case 'ى':
                        b.WriteRune('ي')
                case 'ؤ':
                        b.WriteRune('و')
                case 'ئ':
                        b.WriteRune('ي')
                default:
                        b.WriteRune(r)
                }
        }
        return b.String()
}

func tokenize(text string) []string {
        normalized := normalizeArabic(strings.ToLower(text))
        fields := strings.Fields(normalized)
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

func evidenceStateForValidity(now, validFrom time.Time, validUntil *time.Time) string {
        if now.Before(validFrom) {
                return AIContextMissing
        }
        if validUntil != nil && !now.Before(*validUntil) {
                return AIContextStale
        }
        return AIContextFresh
}

func formatInt(value int) string {
        return strconv.Itoa(value)
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
                return 6 // Per ADR-038: 6 turns sliding window per IrisAgent research.
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
