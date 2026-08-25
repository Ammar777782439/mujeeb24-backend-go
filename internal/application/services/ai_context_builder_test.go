package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type contextBusinessRepository struct{ record ports.BusinessRecord }

func (r contextBusinessRepository) GetByID(context.Context, string) (ports.BusinessRecord, error) {
	return r.record, nil
}

type contextConversationRepository struct{ record ports.ConversationRecord }

func (r contextConversationRepository) GetByID(context.Context, string, string) (ports.ConversationRecord, error) {
	return r.record, nil
}

type contextCustomerRepository struct{ record ports.CustomerRecord }

func (r contextCustomerRepository) GetByID(context.Context, string, string) (ports.CustomerRecord, error) {
	return r.record, nil
}

type contextMessageRepository struct{ page ports.MessagePage }

func (r contextMessageRepository) Record(context.Context, ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord, error) {
	return ports.CommunicationMessageRecord{}, nil
}
func (r contextMessageRepository) ListByConversation(context.Context, string, string, int, string) (ports.MessagePage, error) {
	return r.page, nil
}

type contextCatalogRepository struct {
	catalogs ports.CatalogPage
	items    map[string]ports.CatalogItemPage
	offers   map[string]ports.OfferPage
	variants map[string]ports.VariantPage
}

func (r contextCatalogRepository) ListCatalogs(context.Context, string, string, int, string) (ports.CatalogPage, error) {
	return r.catalogs, nil
}
func (r contextCatalogRepository) GetCatalog(context.Context, string, string) (ports.CatalogRecord, error) {
	return ports.CatalogRecord{}, nil
}
func (r contextCatalogRepository) CreateCatalog(context.Context, ports.CatalogDraft) (ports.CatalogRecord, error) {
	return ports.CatalogRecord{}, nil
}
func (r contextCatalogRepository) UpdateCatalog(context.Context, ports.CatalogPatch) (ports.CatalogRecord, error) {
	return ports.CatalogRecord{}, nil
}
func (r contextCatalogRepository) ListCatalogItems(_ context.Context, _ string, catalogID, _ string, _ string, _ int, _ string) (ports.CatalogItemPage, error) {
	return r.items[catalogID], nil
}
func (r contextCatalogRepository) GetCatalogItem(context.Context, string, string, string) (ports.CatalogItemRecord, error) {
	return ports.CatalogItemRecord{}, nil
}
func (r contextCatalogRepository) CreateCatalogItem(context.Context, ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
	return ports.CatalogItemRecord{}, nil
}
func (r contextCatalogRepository) UpdateCatalogItem(context.Context, ports.CatalogItemPatch) (ports.CatalogItemRecord, error) {
	return ports.CatalogItemRecord{}, nil
}
func (r contextCatalogRepository) ListOffers(_ context.Context, _ string, itemID, _ string, _ int, _ string) (ports.OfferPage, error) {
	return r.offers[itemID], nil
}
func (r contextCatalogRepository) CreateOffer(context.Context, ports.OfferDraft) (ports.OfferRecord, error) {
	return ports.OfferRecord{}, nil
}
func (r contextCatalogRepository) UpdateOffer(context.Context, ports.OfferPatch) (ports.OfferRecord, error) {
	return ports.OfferRecord{}, nil
}
func (r contextCatalogRepository) ListVariants(_ context.Context, _ string, itemID, _ string, _ int, _ string) (ports.VariantPage, error) {
	return r.variants[itemID], nil
}
func (r contextCatalogRepository) CreateVariant(context.Context, ports.VariantDraft) (ports.VariantRecord, error) {
	return ports.VariantRecord{}, nil
}
func (r contextCatalogRepository) UpdateVariant(context.Context, ports.VariantPatch) (ports.VariantRecord, error) {
	return ports.VariantRecord{}, nil
}
func (r contextCatalogRepository) ListAttributeSchemas(context.Context, string, string, *int, int, string) (ports.AttributeSchemaPage, error) {
	return ports.AttributeSchemaPage{}, nil
}
func (r contextCatalogRepository) GetAttributeSchema(context.Context, string, string) (ports.AttributeSchemaRecord, error) {
	return ports.AttributeSchemaRecord{}, nil
}
func (r contextCatalogRepository) NextAttributeSchemaVersion(context.Context, string, string) (int, error) {
	return 1, nil
}
func (r contextCatalogRepository) CreateAttributeSchemaVersion(context.Context, ports.AttributeSchemaDraft) (ports.AttributeSchemaRecord, error) {
	return ports.AttributeSchemaRecord{}, nil
}

func TestAutoReplyContextBuilderBuildsBoundedGroundedContext(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	builder := NewAutoReplyContextBuilder(
		contextBusinessRepository{record: ports.BusinessRecord{ID: "business-1", Name: "متجر صنعاء", VerticalType: "retail", Locale: "ar-YE", DefaultCurrency: "YER"}},
		contextConversationRepository{record: ports.ConversationRecord{ID: "conversation-1", BusinessID: "business-1", CustomerID: "customer-1", State: "open", Ownership: "team", Priority: "normal"}},
		contextCustomerRepository{record: ports.CustomerRecord{ID: "customer-1", BusinessID: "business-1", Status: "active", Profile: []byte(`{"name":"عميل"}`), ContactPoints: []byte(`{"phone":"redacted-in-test"}`)}},
		contextCatalogRepository{
			catalogs: ports.CatalogPage{Items: []ports.CatalogRecord{{ID: "catalog-1", BusinessID: "business-1", Name: "إلكترونيات", Status: "active"}}},
			items: map[string]ports.CatalogItemPage{"catalog-1": {Items: []ports.CatalogItemRecord{
				{ID: "item-irrelevant", BusinessID: "business-1", CatalogID: "catalog-1", Name: "سماعة", Status: "active", Attributes: []byte(`{}`)},
				{ID: "item-iphone", BusinessID: "business-1", CatalogID: "catalog-1", ItemType: "device", Name: "iPhone 15", Status: "active", Attributes: []byte(`{"color":"black","storage":"256GB"}`)},
			}}},
			offers:   map[string]ports.OfferPage{"item-iphone": {Items: []ports.OfferRecord{{ID: "offer-iphone", BusinessID: "business-1", CatalogItemID: "item-iphone", Name: "iPhone 15 Black 256GB", PricingMode: "fixed", Amount: contextStringPtr("250000"), Currency: contextStringPtr("YER"), AvailabilityStatus: "available", Status: "active"}}}},
			variants: map[string]ports.VariantPage{"item-iphone": {Items: []ports.VariantRecord{{ID: "variant-black", BusinessID: "business-1", CatalogItemID: "item-iphone", Name: "Black 256GB", Status: "active", Attributes: []byte(`{"color":"black","storage":"256GB"}`)}}}},
		},
		contextMessageRepository{page: ports.MessagePage{Items: []ports.CommunicationMessageRecord{
			{ID: "message-current", ProviderMessageID: contextStringPtr("source-message"), Direction: "inbound", Origin: "customer", TextContent: contextStringPtr("هل الآيفون 15 الأسود 256 متوفر؟"), OccurredAt: now},
			{ID: "message-newer", Direction: "outbound", Origin: "agent", TextContent: contextStringPtr("أهلًا"), OccurredAt: now.Add(-time.Minute)},
			{ID: "message-older", Direction: "inbound", Origin: "customer", TextContent: contextStringPtr("أريد هاتفًا"), OccurredAt: now.Add(-2 * time.Minute)},
		}}},
	)
	builder.Now = func() time.Time { return now }
	builder.MaxItems = 1
	builder.MaxMessages = 2
	contextValue, err := builder.Build(context.Background(), ports.ContextBuildInput{BusinessID: "business-1", ConversationID: "conversation-1", SourceMessageReference: "source-message", Text: "هل الآيفون 15 الأسود 256 متوفر؟", PolicyVersion: "auto-reply-v1"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if contextValue.SchemaVersion != AIContextSchemaVersion || contextValue.Business.Reference != "business-1" || contextValue.Conversation.CustomerReference != "customer-1" || contextValue.Customer.Reference != "customer-1" {
		t.Fatalf("unexpected base context: %#v", contextValue)
	}
	if len(contextValue.CatalogEvidence) != 1 || contextValue.CatalogEvidence[0].Reference != "item-iphone" || len(contextValue.OfferEvidence) != 1 || len(contextValue.VariantEvidence) != 1 {
		t.Fatalf("unexpected grounded evidence: %#v", contextValue)
	}
	if contextValue.OfferEvidence[0].AvailabilityState != "available" || contextValue.OfferEvidence[0].Amount != "250000" || contextValue.OfferEvidence[0].Currency != "YER" {
		t.Fatalf("offer evidence lost commercial fields: %#v", contextValue.OfferEvidence[0])
	}
	if len(contextValue.RecentMessages) != 2 || contextValue.RecentMessages[0].Reference != "message-older" || contextValue.RecentMessages[1].Reference != "message-newer" {
		t.Fatalf("unexpected ordered history: %#v", contextValue.RecentMessages)
	}
	if contextValue.KnowledgeState != AIContextPartial || contextValue.Freshness != AIContextFresh || !contextValue.ExpiresAt.After(now) || contextValue.PolicyEvidence.State != "application_policy_only" {
		t.Fatalf("unexpected freshness/policy state: %#v", contextValue)
	}
	if string(contextValue.Customer.Profile) != `{"name":"عميل"}` || string(contextValue.Customer.ContactPoints) != `{"phone":"redacted-in-test"}` {
		t.Fatalf("customer context was not preserved: %#v", contextValue.Customer)
	}
}

func TestAutoReplyContextBuilderRejectsTenantMismatch(t *testing.T) {
	builder := NewAutoReplyContextBuilder(
		contextBusinessRepository{record: ports.BusinessRecord{ID: "other-business"}},
		contextConversationRepository{}, contextCustomerRepository{}, contextCatalogRepository{}, contextMessageRepository{},
	)
	_, err := builder.Build(context.Background(), ports.ContextBuildInput{BusinessID: "business-1", ConversationID: "conversation-1", Text: "hello"})
	if err == nil {
		t.Fatal("Build accepted business tenant mismatch")
	}
}

func contextStringPtr(value string) *string { return &value }

func TestAutoReplyContextBuilderMarksUnknownAvailabilityStale(t *testing.T) {
	builder := NewAutoReplyContextBuilder(
		contextBusinessRepository{record: ports.BusinessRecord{ID: "business-1"}},
		contextConversationRepository{record: ports.ConversationRecord{ID: "conversation-1", BusinessID: "business-1", CustomerID: "customer-1"}},
		contextCustomerRepository{record: ports.CustomerRecord{ID: "customer-1", BusinessID: "business-1"}},
		contextCatalogRepository{
			catalogs: ports.CatalogPage{Items: []ports.CatalogRecord{{ID: "catalog-1", BusinessID: "business-1"}}},
			items:    map[string]ports.CatalogItemPage{"catalog-1": {Items: []ports.CatalogItemRecord{{ID: "item-1", BusinessID: "business-1", CatalogID: "catalog-1", Name: "خدمة"}}}},
			offers:   map[string]ports.OfferPage{"item-1": {Items: []ports.OfferRecord{{ID: "offer-1", BusinessID: "business-1", CatalogItemID: "item-1", AvailabilityStatus: "unknown", Status: "active"}}}},
			variants: map[string]ports.VariantPage{},
		},
		contextMessageRepository{},
	)
	contextValue, err := builder.Build(context.Background(), ports.ContextBuildInput{BusinessID: "business-1", ConversationID: "conversation-1", Text: "خدمة متوفرة؟"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(contextValue.OfferEvidence) != 1 || contextValue.OfferEvidence[0].EvidenceState != AIContextStale || contextValue.Freshness != AIContextStale || contextValue.KnowledgeState != AIContextPartial {
		t.Fatalf("unknown availability was not marked stale: %#v", contextValue)
	}
}
