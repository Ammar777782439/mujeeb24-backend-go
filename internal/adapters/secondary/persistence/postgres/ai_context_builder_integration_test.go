//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestAutoReplyContextBuilderGroundsCatalogAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()

	businessID := uuid.NewString()
	otherBusinessID := uuid.NewString()
	customerID := uuid.NewString()
	conversationID := uuid.NewString()
	catalogID := uuid.NewString()
	itemID := uuid.NewString()
	variantID := uuid.NewString()
	offerID := uuid.NewString()
	knowledgeID := uuid.NewString()
	policyID := uuid.NewString()
	connectionID := uuid.NewString()
	referenceID := uuid.NewString()
	base := time.Date(2026, 8, 25, 20, 0, 0, 0, time.UTC)
	defer func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM offers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM business_policy_versions WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM knowledge_documents WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM variants WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM catalog_items WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM catalogs WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
	}()

	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Grounded Shop', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Other Tenant', $2, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessID, otherBusinessID, base); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{"name":"Customer"}'::jsonb, '[]'::jsonb, 'ar-YE', 'active', $3, $3)`, customerID, businessID, base); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationID, businessID, customerID, base); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'Electronics', 'phones', 'active', $3, $3)`, catalogID, businessID, base); err != nil {
		t.Fatalf("insert catalog: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO catalog_items (id, business_id, catalog_id, item_type, name, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'physical_good', 'iPhone 15', 'active', 'fixed', 'stock', 'delivery', false, '{"color":"black","storage":"256GB"}'::jsonb, $4, $4)`, itemID, businessID, catalogID, base); err != nil {
		t.Fatalf("insert catalog item: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO variants (id, business_id, catalog_item_id, name, attributes, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'Black 256GB', '{"color":"black","storage":"256GB"}'::jsonb, 'active', $4, $4)`, variantID, businessID, itemID, base); err != nil {
		t.Fatalf("insert variant: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO offers (id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency, availability_mode, availability_status, fulfillment_mode, price_verification_status, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 'iPhone 15 Black 256GB', 'fixed', 250000, 'YER', 'stock', 'available', 'delivery', 'verified', 'active', $5, $5)`, offerID, businessID, itemID, variantID, base); err != nil {
		t.Fatalf("insert offer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO knowledge_documents (id, business_id, knowledge_key, title, content, content_type, source_reference, authority, status, version, valid_from, valid_until, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'iphone-availability', 'iPhone availability', 'The current iPhone availability is published here', 'faq', 'merchant-knowledge-1', 'merchant', 'published', 1, $3, NULL, $3, $3)`, knowledgeID, businessID, base); err != nil {
		t.Fatalf("insert knowledge document: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO business_policy_versions (id, business_id, policy_key, category, title, summary, rules, authority, status, version, valid_from, valid_until, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'iphone-availability', 'availability', 'Availability policy', 'Merchant availability evidence is required', '{"requires_fresh_offer":true}'::jsonb, 'merchant', 'published', 1, $3, NULL, $3, $3)`, policyID, businessID, base); err != nil {
		t.Fatalf("insert policy version: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'account-' || $1::text, 'connection-' || $1::text, 'active', 'local-secret-ref', $3, $3)`, connectionID, businessID, base); err != nil {
		t.Fatalf("insert channel connection: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'provider-conversation', $4::uuid, 'dm', true, 'active', $5, $5)`, referenceID, businessID, conversationID, connectionID, base); err != nil {
		t.Fatalf("insert conversation reference: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO communication_messages (id, business_id, conversation_reference_id, direction, origin, transport, provider_message_id, content_type, text_content, content_reference, occurred_at, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'inbound', 'customer', 'provider', 'history-1', 'text', 'أريد هاتفًا', 'content://test/history-1', $4, $4)`, uuid.NewString(), businessID, referenceID, base.Add(-time.Minute)); err != nil {
		t.Fatalf("insert history message: %v", err)
	}

	builder := services.NewAutoReplyContextBuilder(
		NewBusinessRepository(adapter),
		NewConversationRepository(adapter),
		NewCustomerRepository(adapter),
		NewCatalogRepository(adapter),
		NewMessageRepository(adapter),
	)
	builder.CatalogAI = NewCatalogAIReadRepository(adapter)
	builder.Knowledge = NewKnowledgeDocumentRepository(adapter)
	builder.Policies = NewBusinessPolicyRepository(adapter)
	builder.Now = func() time.Time { return base }
	contextValue, err := builder.Build(ctx, ports.CustomerSalesContextInput{BusinessID: businessID, ConversationID: conversationID, SourceMessageReference: "incoming-current", Text: "هل iPhone 15 الأسود 256GB متوفر؟", Channel: "whatsapp", PolicyVersion: "auto-reply-v1",
		// Item-level evidence must be driven by a validated conversation focus,
		// not implicit text search across the merchant's catalog.
		ConversationState: &ports.ConversationStateRecord{BusinessID: businessID, ConversationID: conversationID,
			Focus: &ports.ConversationFocus{Type: "item", ID: itemID, CatalogID: &catalogID}}})
	if err != nil {
		t.Fatalf("build grounded context: %v", err)
	}
	if contextValue.Business.Reference != businessID || contextValue.Conversation.Reference != conversationID || contextValue.Customer.Reference != customerID {
		t.Fatalf("wrong tenant context: %#v", contextValue)
	}
	if len(contextValue.CatalogEvidence) != 1 || contextValue.CatalogEvidence[0].Reference != itemID || len(contextValue.OfferEvidence) != 1 || contextValue.OfferEvidence[0].Reference != offerID || contextValue.OfferEvidence[0].AvailabilityStatus != "available" || contextValue.OfferEvidence[0].Amount != "250000.0000" || len(contextValue.VariantEvidence) != 1 || contextValue.VariantEvidence[0].Reference != variantID {
		t.Fatalf("grounding mismatch: %#v", contextValue)
	}
	// Real PostgreSQL pagination test: exceed the 200-record cursor limit.
	// All 206 options and 206 offers must reach the scoped AI context.
	extraOfferIDs := make([]string, 0, 205)
	for n := 0; n < 205; n++ {
		extraVariantID := uuid.NewString()
		extraOfferID := uuid.NewString()
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO variants
			(id, business_id, catalog_item_id, name, attributes, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, '{}'::jsonb, 'active', $5, $5)`,
			extraVariantID, businessID, itemID, fmt.Sprintf("Variant %d", n), base); err != nil {
			t.Fatalf("insert additional variant %d: %v", n, err)
		}
		extraOfferIDs = append(extraOfferIDs, extraOfferID)
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO offers
			(id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency,
			 availability_mode, availability_status, fulfillment_mode, price_verification_status,
			 status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'fixed', 100, 'YER',
			 'stock', 'available', 'delivery', 'verified', 'active', $6, $6)`,
			extraOfferID, businessID, itemID, extraVariantID, fmt.Sprintf("Offer %d", n), base); err != nil {
			t.Fatalf("insert additional offer %d: %v", n, err)
		}
	}
	complete, err := builder.Build(ctx, ports.CustomerSalesContextInput{
		BusinessID: businessID, ConversationID: conversationID, SourceMessageReference: "incoming-current",
		Text: "قارن جميع خيارات الهاتف", Channel: "whatsapp",
		ConversationState: &ports.ConversationStateRecord{BusinessID: businessID, ConversationID: conversationID,
			Focus: &ports.ConversationFocus{Type: "item", ID: itemID, CatalogID: &catalogID}},
	})
	if err != nil {
		t.Fatalf("real PostgreSQL complete catalog entity retrieval: %v", err)
	}
	if len(complete.CatalogEvidence) != 1 || len(complete.VariantEvidence) != 206 || len(complete.OfferEvidence) != 206 {
		t.Fatalf("truncated complete entity: items=%d variants=%d offers=%d",
			len(complete.CatalogEvidence), len(complete.VariantEvidence), len(complete.OfferEvidence))
	}

	// Locate a target offer that is definitively OUTSIDE the original
	// first five database rows, then verify focused retrieval uses the exact
	// record while still hydrating its full 206-offer commercial parent.
	firstPage, err := NewCatalogRepository(adapter).ListOffers(ctx, businessID, itemID, "active", 5, "")
	if err != nil {
		t.Fatalf("load first real offer page: %v", err)
	}
	firstFive := map[string]bool{}
	for _, record := range firstPage.Items {
		firstFive[record.ID] = true
	}
	targetOfferID := ""
	for _, id := range extraOfferIDs {
		if !firstFive[id] {
			targetOfferID = id
			break
		}
	}
	if targetOfferID == "" {
		t.Fatal("test did not find an offer beyond first five rows")
	}
	focused, err := builder.Build(ctx, ports.CustomerSalesContextInput{
		BusinessID: businessID, ConversationID: conversationID, Text: "تفاصيل هذا العرض",
		ConversationState: &ports.ConversationStateRecord{
			BusinessID: businessID, ConversationID: conversationID,
			Focus: &ports.ConversationFocus{Type: "offer", ID: targetOfferID, ItemID: &itemID, CatalogID: &catalogID},
		},
	})
	if err != nil {
		t.Fatalf("focused offer beyond first page was not resolved: %v", err)
	}
	if len(focused.CatalogEvidence) != 1 || len(focused.OfferEvidence) != 206 || len(focused.VariantEvidence) != 206 {
		t.Fatalf("focused offer lost siblings: items=%d offers=%d variants=%d",
			len(focused.CatalogEvidence), len(focused.OfferEvidence), len(focused.VariantEvidence))
	}
	found := false
	for _, evidence := range focused.OfferEvidence {
		if evidence.Reference == targetOfferID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("focused commercial offer was missing despite complete context")
	}
	wrongItemID := uuid.NewString()
	_, mismatchErr := builder.Build(ctx, ports.CustomerSalesContextInput{
		BusinessID: businessID, ConversationID: conversationID, Text: "تفاصيل العرض",
		ConversationState: &ports.ConversationStateRecord{
			BusinessID: businessID, ConversationID: conversationID,
			Focus: &ports.ConversationFocus{Type: "offer", ID: targetOfferID, ItemID: &wrongItemID},
		},
	})
	if mismatchErr == nil {
		t.Fatal("mismatched item ID was permitted to own a focused offer")
	}

	// A same-count, unchanged-timestamp commercial edit must still change the
	// content revision. PostgreSQL is real, not a stubbed repository.
	catalogReader := NewCatalogAIReadRepository(adapter)
	originalRevision, err := catalogReader.GetRevision(ctx, businessID)
	if err != nil {
		t.Fatalf("revision before offer change: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE offers SET amount=250001 WHERE id=$1::uuid`, offerID); err != nil {
		t.Fatalf("edit price without timestamp update: %v", err)
	}
	changedRevision, err := catalogReader.GetRevision(ctx, businessID)
	if err != nil {
		t.Fatalf("revision after offer change: %v", err)
	}
	if changedRevision == originalRevision {
		t.Fatal("commercial price changed but catalog revision did not")
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE offers SET amount=250000 WHERE id=$1::uuid`, offerID); err != nil {
		t.Fatalf("restore test offer price: %v", err)
	}

	if len(contextValue.RecentMessages) != 1 || contextValue.RecentMessages[0].Text != "أريد هاتفًا" {
		t.Fatalf("history mismatch: %#v", contextValue.RecentMessages)
	}
	if contextValue.KnowledgeState != services.CustomerSalesContextGrounded || contextValue.Freshness != services.CustomerSalesContextFresh || len(contextValue.KnowledgeEvidence) != 1 || contextValue.KnowledgeEvidence[0].Reference != knowledgeID || len(contextValue.BusinessPolicyEvidence) != 1 || contextValue.BusinessPolicyEvidence[0].Reference != policyID || contextValue.PolicyEvidence.State != "published" {
		t.Fatalf("unexpected knowledge state: %#v", contextValue)
	}

	if contextValue.CatalogManifest == nil || contextValue.CatalogManifest.TotalCatalogs != 1 ||
		contextValue.CatalogManifest.TotalActiveItems != 1 {
		t.Fatalf("missing merchant-scoped catalog manifest: %#v", contextValue.CatalogManifest)
	}
	// A new conversation without a focus must not use keyword search or
	// silently attach a product based on the incoming message alone.
	unfocused, unfocusedErr := builder.Build(ctx, ports.CustomerSalesContextInput{
		BusinessID: businessID, ConversationID: conversationID, Text: "iPhone 15", Channel: "whatsapp",
	})
	if unfocusedErr != nil || len(unfocused.CatalogEvidence) != 0 || len(unfocused.OfferEvidence) != 0 ||
		unfocused.CatalogManifest == nil || unfocused.CatalogManifest.TotalActiveItems != 1 {
		t.Fatalf("unfocused retrieval leaked item-level evidence: %#v err=%v", unfocused, unfocusedErr)
	}
	otherContext, err := builder.Build(ctx, ports.CustomerSalesContextInput{BusinessID: otherBusinessID, ConversationID: conversationID, Text: "iPhone"})
	if err == nil || otherContext.Business.Reference != "" {
		t.Fatalf("cross-tenant conversation was accepted: context=%#v err=%v", otherContext, err)
	}
}
