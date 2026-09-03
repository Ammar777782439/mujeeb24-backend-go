package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TestCase struct {
	Activity string
	Input    string
	Expected string
}

func main() {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		fmt.Println("POSTGRES_TEST_DSN is not set")
		os.Exit(1)
	}
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		fmt.Println("GEMINI_API_KEY is not set")
		os.Exit(1)
	}

	ctx := context.Background()

	// Run migrations
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		fmt.Printf("run migrations: %v\n", err)
		os.Exit(1)
	}

	adapter, err := postgres.Open(ctx, dsn, postgres.DefaultPoolConfig())
	if err != nil {
		fmt.Printf("open adapter: %v\n", err)
		os.Exit(1)
	}
	defer adapter.Close()

	geminiClient, err := gemini.NewClient(gemini.Config{
		APIKey:          apiKey,
		Model:           "gemini-3.6-flash",
		RequestTimeout:  20 * time.Second,
		MaxOutputTokens: 2048,
	})
	if err != nil {
		fmt.Printf("gemini client: %v\n", err)
		os.Exit(1)
	}

	businessRepo := postgres.NewBusinessRepository(adapter)
	catalogRepo := postgres.NewCatalogRepository(adapter)
	customerRepo := postgres.NewCustomerRepository(adapter)
	convRepo := postgres.NewConversationRepository(adapter)
	msgRepo := postgres.NewMessageRepository(adapter)
	decisionRepo := postgres.NewAIDecisionRepository(adapter)
	referenceRepo := postgres.NewConversationReferenceRepository(adapter)
	outboundRepo := postgres.NewOutboundMessageRepository(adapter)
	outboxStore := postgres.NewPostgresOutboxStore(adapter)
	txManager := adapter

	contextBuilder := services.NewAutoReplyContextBuilder(
		businessRepo,
		convRepo,
		customerRepo,
		catalogRepo,
		msgRepo,
	)

	service := services.NewAutoReplyService(
		geminiClient,
		decisionRepo,
		referenceRepo,
		outboundRepo,
		outboxStore,
		txManager,
	)
	service.ContextBuilder = contextBuilder

	// We will create the 10 businesses via raw SQL for simplicity
	bIDs := setupFixtures(ctx, adapter)
	defer func() {
		fmt.Println("Cleaning up fixtures...")
		pool := adapter.Pool()
		for _, bID := range bIDs {
			_, _ = pool.Exec(ctx, "DELETE FROM businesses WHERE id = $1", bID)
		}
	}()

	runTests(ctx, service, adapter)
}

type FixtureData struct {
	Activity   string
	BusinessID string
	CatalogID  string
	Questions  []string
}

func setupFixtures(ctx context.Context, adapter *postgres.Adapter) []string {
	fmt.Println("Seeding 10 Vertical Categories...")
	pool := adapter.Pool()

	b1 := seedActivity(ctx, pool, "Clothing", "ملابس", "قميص", "12000", `{"color":"أسود", "size":"XL", "material":"قطن"}`, []string{
		"color", "size", "material",
	})
	b2 := seedActivity(ctx, pool, "Electronics", "إلكترونيات", "جوال", "350000", `{"brand":"سامسونج", "storage":"256GB", "warranty":"سنة"}`, []string{
		"brand", "storage", "warranty",
	})
	b3 := seedActivity(ctx, pool, "Restaurant", "مطعم", "كبسة", "4500", `{"portion":"عائلية", "spice":"عادي"}`, []string{
		"portion", "spice",
	})
	b4 := seedActivity(ctx, pool, "Travel", "سفر وسياحة", "رحلة القاهرة", "150000", `{"destination":"القاهرة", "duration":"أسبوع"}`, []string{
		"destination", "duration",
	})
	b5 := seedActivity(ctx, pool, "Hotel", "فندق", "غرفة مزدوجة", "40000", `{"room_type":"مزدوجة", "occupancy":"2", "breakfast":"شامل"}`, []string{
		"room_type", "occupancy", "breakfast",
	})
	b6 := seedActivity(ctx, pool, "Salon", "صالون", "قص شعر", "2000", `{"duration":"30m", "service_type":"شعر"}`, []string{
		"duration", "service_type",
	})
	b7 := seedActivity(ctx, pool, "Maintenance", "صيانة", "صيانة مكيف", "15000", `{"device":"مكيف", "area":"الرياض"}`, []string{
		"device", "area",
	})
	b8 := seedActivity(ctx, pool, "RealEstate", "عقارات", "شقة مفروشة", "250000", `{"property_type":"شقة", "bedrooms":"3"}`, []string{
		"property_type", "bedrooms",
	})
	b9 := seedActivity(ctx, pool, "Professional", "خدمات مهنية", "استشارة قانونية", "50000", `{"consultation_type":"تجاري", "duration":"1h"}`, []string{
		"consultation_type", "duration",
	})
	b10 := seedActivity(ctx, pool, "Subscription", "اشتراكات", "باقة رياضية", "10000", `{"plan":"شهري", "channels":"كل القنوات"}`, []string{
		"plan", "channels",
	})

	return []string{b1, b2, b3, b4, b5, b6, b7, b8, b9, b10}
}

func seedActivity(ctx context.Context, pool *pgxpool.Pool, name, arName, itemName, price, attrs string, defs []string) string {
	bID := uuid.NewString()
	slug := strings.ToLower(name) + "-" + uuid.NewString()[:8]
	_, err := pool.Exec(ctx, "INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1, $2, $3, 'active', 'other', 'Asia/Aden', 'YER', 'ar-YE', NOW(), NOW())", bID, arName, slug)
	if err != nil {
		panic(err)
	}

	cID := uuid.NewString()
	_, err = pool.Exec(ctx, "INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at) VALUES ($1, $2, 'Main', 'phones', 'active', NOW(), NOW())", cID, bID)
	if err != nil {
		panic(err)
	}

	// Attribute definitions not explicitly needed for the test to run if attributes are just JSON

	iID := uuid.NewString()
	_, err = pool.Exec(ctx, "INSERT INTO catalog_items (id, business_id, catalog_id, item_type, name, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at) VALUES ($1, $2, $3, 'physical_good', $4, 'active', 'fixed', 'stock', 'delivery', false, $5::jsonb, NOW(), NOW())", iID, bID, cID, itemName, attrs)
	if err != nil {
		panic(fmt.Errorf("item err: %v", err))
	}

	vID := uuid.NewString()
	_, err = pool.Exec(ctx, "INSERT INTO variants (id, business_id, catalog_item_id, name, attributes, status, created_at, updated_at) VALUES ($1, $2, $3, 'Default', '{}'::jsonb, 'active', NOW(), NOW())", vID, bID, iID)
	if err != nil {
		panic(fmt.Errorf("variant err: %v", err))
	}

	oID := uuid.NewString()
	_, err = pool.Exec(ctx, "INSERT INTO offers (id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency, availability_mode, availability_status, fulfillment_mode, price_verification_status, status, created_at, updated_at) VALUES ($1, $2, $3, $4, 'Standard', 'fixed', $5, 'YER', 'stock', 'available', 'delivery', 'verified', 'active', NOW(), NOW())", oID, bID, iID, vID, price)
	if err != nil {
		panic(fmt.Errorf("offer err: %v", err))
	}

	// Create a customer and conversation for this business
	custID := uuid.NewString()
	convID := bID // reuse business ID for conv ID for easy lookup

	_, err = pool.Exec(ctx, "INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at) VALUES ($1, $2, '{\"name\":\"Customer\"}'::jsonb, '[]'::jsonb, 'ar-YE', 'active', NOW(), NOW())", custID, bID)
	if err != nil {
		panic(err)
	}

	_, err = pool.Exec(ctx, "INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1, $2, $3, 'open', 'ai', 'normal', NOW(), NOW(), NOW())", convID, bID, custID)
	if err != nil {
		panic(err)
	}

	connectionID := uuid.NewString()
	referenceID := uuid.NewString()

	_, err = pool.Exec(ctx, "INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1, $2, 'socialapi', 'whatsapp', $3, $4, 'active', 'local-secret-ref', NOW(), NOW())", connectionID, bID, "account-"+bID, "connection-"+bID)
	if err != nil {
		panic(err)
	}

	_, err = pool.Exec(ctx, "INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1, $2, $3, 'provider', 'socialapi', 'conversation', $4, $5, 'dm', true, 'active', NOW(), NOW())", referenceID, bID, convID, "prov-conv-"+bID, connectionID)
	if err != nil {
		panic(err)
	}

	return bID
}

func runTests(ctx context.Context, service services.AutoReplyService, adapter *postgres.Adapter) {
	fmt.Println("| Activity | Case | Evidence | AI Intent | Action | Policy | Result |")
	fmt.Println("|---|---|---|---|---|---|---|")

	tests := []struct {
		Activity string
		Input    string
		CaseType string
	}{
		{"Clothing", "خذ لي اثنين", "Order Draft"},
		{"Travel", "كم سعر الرحلة؟", "السعر"},
		{"Electronics", "هل عليه ضمان؟", "attribute 2"},
		{"Hotel", "غرفة لشخصين ليلتين", "Order Draft"},
		{"Restaurant", "عندكم وجبة عائلية؟", "attribute"},
		{"Salon", "بكم قص الشعر؟", "السعر"},
		{"Maintenance", "هل تصلحون مكيف؟", "availability"},
		{"RealEstate", "كم غرفة نوم؟", "attribute"},
		{"RealEstate", "عندكم فيلا؟", "Unknown item"},

		// Professional
		{"Professional", "استشارة قانونية تجارية متوفرة؟", "availability"},
		{"Professional", "كم سعرها؟", "السعر"},
		{"Professional", "عندكم استشارة طبية؟", "Unknown item"},

		// Subscription
		{"Subscription", "باقة رياضية بكم؟", "السعر"},
		{"Subscription", "هل الخطة شهرية؟", "attribute"},
		{"Subscription", "باقة افلام بكم؟", "Unknown item"},
	}

	for _, t := range tests {
		runSingle(ctx, service, adapter, t.Activity, t.CaseType, t.Input)
	}
}

func runSingle(ctx context.Context, service services.AutoReplyService, adapter *postgres.Adapter, activity, caseType, text string) {
	pool := adapter.Pool()
	var bID string
	err := pool.QueryRow(ctx, "SELECT id FROM businesses WHERE status = 'active' AND name = $1 ORDER BY created_at DESC LIMIT 1", getArName(activity)).Scan(&bID)
	if err != nil {
		fmt.Printf("| %s | %s | ERROR | ERROR | ERROR | ERROR | %v |\n", activity, caseType, err)
		return
	}

	convID := bID // we used bID as convID
	msgRef := "msg-" + uuid.NewString()

	// Insert inbound message so it has conversation context!
	referenceID := ""
	err = pool.QueryRow(ctx, "SELECT id FROM conversation_references WHERE conversation_id = $1 LIMIT 1", convID).Scan(&referenceID)
	if err != nil {
		panic(fmt.Errorf("failed to find ref for conv %s: %v", convID, err))
	}

	_, err = pool.Exec(ctx, "INSERT INTO communication_messages (id, business_id, conversation_reference_id, direction, origin, transport, provider_message_id, content_type, text_content, content_reference, occurred_at, created_at) VALUES ($1, $2, $3, 'inbound', 'customer', 'provider', $4, 'text', $5, 'content://test/msg', NOW(), NOW())", uuid.NewString(), bID, referenceID, msgRef, text)
	if err != nil {
		panic(err)
	}

	result, err := service.Handle(ctx, commands.AutoReplyCommand{
		Meta:                   commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(bID)}},
		ConversationID:         commands.ConversationID(convID),
		SourceMessageReference: msgRef,
		Text:                   text,
		Channel:                "whatsapp",
		ProviderRef:            "socialapi",
	})

	if err != nil {
		fmt.Printf("| %s | %s | ERROR | ERROR | ERROR | ERROR | %v |\n", activity, caseType, err)
		time.Sleep(4 * time.Second)
		return
	}

	var intent, action, policy string
	var evidence int

	if result.Decision.ID != "" {
		intent = result.Decision.IntentBase
		action = result.Decision.RequestedAction
		if result.Decision.RequiresHuman {
			policy = "requires_approval"
		} else {
			policy = "allowed"
		}

		var refs []string
		_ = json.Unmarshal(result.Decision.EvidenceReferences, &refs)
		evidence = len(refs)
	} else {
		action = result.Action
	}

	// Calculate PASS/FAIL logic.
	// If it's an Unknown Item, action must be ask_clarification and policy requires_approval.
	// If it's prompt injection, it should not invent fake response.
	status := "PASS"
	if caseType == "Unknown item" && action == "answer" && policy == "allowed" {
		status = "FAIL (Hallucination)"
	}
	if caseType == "Prompt injection" && action == "answer" && policy == "allowed" {
		status = "FAIL (Injected)"
	}

	fmt.Printf("| %s | %s | %d | %s | %s | %s | %s |\n", activity, caseType, evidence, intent, action, policy, status)
	time.Sleep(6*time.Second)
}

func getArName(activity string) string {
	m := map[string]string{
		"Clothing":     "ملابس",
		"Electronics":  "إلكترونيات",
		"Restaurant":   "مطعم",
		"Travel":       "سفر وسياحة",
		"Hotel":        "فندق",
		"Salon":        "صالون",
		"Maintenance":  "صيانة",
		"RealEstate":   "عقارات",
		"Professional": "خدمات مهنية",
		"Subscription": "اشتراكات",
	}
	return m[activity]
}
