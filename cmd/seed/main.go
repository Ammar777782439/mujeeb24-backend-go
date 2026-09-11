package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultAdminEmail   = "admin@mujeeb.ai"
	defaultAdminPass    = "Password123456!"
	defaultBusinessID   = "00000000-0000-0000-0000-000000000001"
	defaultPrincipalID  = "2818b77d-d94d-455f-8ca3-2ca18075160f"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		dbURL = "postgres://mujeeb:551f7de64c01ce392e5a8ce35d8c8a66447e6d90fe8bd9de@127.0.0.1:5433/mujeeb24?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Printf("Connecting to PostgreSQL: %s ...", maskURL(dbURL))
	adapter, err := postgres.Open(ctx, dbURL, postgres.PoolConfig{
		MaxConns:       5,
		MinConns:       1,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer adapter.Close()

	pool := adapter.Pool()

	now := time.Now().UTC()
	businessID := defaultBusinessID
	principalID := defaultPrincipalID

	log.Println("1. Ensuring Principal and Super Admin credentials...")
	passHash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
		VALUES ($1::uuid, $2, $3, $4, 'active', $5, $5)
		ON CONFLICT (id) DO UPDATE
		SET email = EXCLUDED.email,
		    display_name = EXCLUDED.display_name,
		    password_hash = EXCLUDED.password_hash,
		    updated_at = EXCLUDED.updated_at;
	`, principalID, defaultAdminEmail, "Admin Mujeeb", string(passHash), now)
	if err != nil {
		log.Fatalf("Failed to upsert principal: %v", err)
	}

	_, _ = pool.Exec(ctx, `
		INSERT INTO platform_super_admins (principal_id, created_at)
		VALUES ($1::uuid, $2)
		ON CONFLICT (principal_id) DO NOTHING;
	`, principalID, now)

	log.Println("2. Ensuring Main Business and Policies...")
	_, err = pool.Exec(ctx, `
		INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at, resource_version)
		VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Riyadh', 'SAR', 'ar-SA', $4, $4, 1)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    slug = EXCLUDED.slug,
		    vertical_type = EXCLUDED.vertical_type,
		    timezone = EXCLUDED.timezone,
		    default_currency = EXCLUDED.default_currency,
		    locale = EXCLUDED.locale,
		    updated_at = EXCLUDED.updated_at;
	`, businessID, "متجر مجيب 24 الذكي للإلكترونيات", "mujeeb-store", now)
	if err != nil {
		log.Fatalf("Failed to upsert business: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO business_policies (business_id, ai_mode, default_human_review, allow_auto_reply, allow_auto_lead_creation, allow_auto_transaction_draft, allow_auto_confirmation, created_at, updated_at)
		VALUES ($1::uuid, 'restricted_auto', false, true, true, true, true, $2, $2)
		ON CONFLICT (business_id) DO UPDATE
		SET ai_mode = EXCLUDED.ai_mode,
		    allow_auto_reply = EXCLUDED.allow_auto_reply,
		    allow_auto_lead_creation = EXCLUDED.allow_auto_lead_creation,
		    allow_auto_transaction_draft = EXCLUDED.allow_auto_transaction_draft,
		    allow_auto_confirmation = EXCLUDED.allow_auto_confirmation,
		    updated_at = EXCLUDED.updated_at;
	`, businessID, now)
	if err != nil {
		log.Fatalf("Failed to upsert business policy: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO business_memberships (business_id, principal_id, role, permissions, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, 'owner', '["*"]'::jsonb, 'active', $3, $3)
		ON CONFLICT (business_id, principal_id) DO UPDATE
		SET role = 'owner', permissions = '["*"]'::jsonb, status = 'active', updated_at = EXCLUDED.updated_at;
	`, businessID, principalID, now)
	if err != nil {
		log.Fatalf("Failed to upsert membership: %v", err)
	}

	log.Println("3. Ensuring Channel Connections...")
	whatsappConnID := "00000000-0000-0000-0000-000000000011"
	instagramConnID := "00000000-0000-0000-0000-000000000012"
	facebookConnID := "00000000-0000-0000-0000-000000000013"

	channels := []struct {
		id            string
		channel       string
		accountRef    string
		connectionRef string
		secretRef     string
	}{
		{whatsappConnID, "whatsapp", "wa_acc_966501234567", "wa_conn_01", "sec_ref_wa_01"},
		{instagramConnID, "instagram", "ig_acc_mujeeb24_store", "ig_conn_01", "sec_ref_ig_01"},
		{facebookConnID, "facebook", "fb_acc_mujeeb24", "fb_conn_01", "sec_ref_fb_01"},
	}

	for _, ch := range channels {
		_, err = pool.Exec(ctx, `
			INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, 'socialapi', $3, $4, $5, 'active', $6, $7, $7)
			ON CONFLICT (id) DO UPDATE
			SET status = 'active', updated_at = EXCLUDED.updated_at;
		`, ch.id, businessID, ch.channel, ch.accountRef, ch.connectionRef, ch.secretRef, now)
		if err != nil {
			log.Fatalf("Failed to upsert channel connection (%s): %v", ch.channel, err)
		}
	}

	log.Println("4. Ensuring Canned Replies (Quick Shortcuts)...")
	cannedReplies := []struct {
		id       string
		title    string
		shortcut string
		body     string
	}{
		{"00000000-0000-0000-0000-000000000021", "رسالة الترحيب", "welcome", "أهلاً بك في متجر مجيب 24 للإلكترونيات الذكية! يسعدنا خدمتكم والإجابة عن جميع استفساراتكم حول أحدث المنتجات والعروض."},
		{"00000000-0000-0000-0000-000000000022", "معلومات الشحن والتوصيل", "shipping", "نوفر شحن سريع لكافة مناطق ومحافظات المملكة العربية السعودية خلال 24 إلى 48 ساعة عبر شركائنا المعتمدين، والتوصيل مجاني للطلبات فوق 300 ريال."},
		{"00000000-0000-0000-0000-000000000023", "سياسة الضمان", "warranty", "كافة الأجهزة والمعدات الإلكترونية لدينا مشمولة بضمان رسمي لمدة سنتين (ضمان الوكيل المعتمد في السعودية) مع خدمة الاستبدال الفوري في حال العيوب المصنعية."},
		{"00000000-0000-0000-0000-000000000024", "طرق الدفع", "payment", "نقبل جميع وسائل الدفع المعتمدة: مدى، Apple Pay، فيزا/ماستركارد، تحويل بنكي، بالإضافة إلى الدفع بالتقسيط عبر تابي وتمارا بدون فوائد."},
		{"00000000-0000-0000-0000-000000000025", "التواصل مع الإدارة", "contact", "يمكنكم دائماً التواصل المباشر مع فريق الإدارة وخدمة كبار العملاء عبر الرقم الموحد 800-123-2424 أو عبر البريد support@mujeeb.ai."},
	}

	for _, cr := range cannedReplies {
		_, err = pool.Exec(ctx, `
			INSERT INTO canned_replies (id, business_id, title, shortcut, body, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'active', $6, $6)
			ON CONFLICT (business_id, shortcut) DO UPDATE
			SET title = EXCLUDED.title, body = EXCLUDED.body, updated_at = EXCLUDED.updated_at;
		`, cr.id, businessID, cr.title, cr.shortcut, cr.body, now)
		if err != nil {
			log.Fatalf("Failed to upsert canned reply (%s): %v", cr.shortcut, err)
		}
	}

	log.Println("5. Ensuring Automation Rules...")
	rules := []struct {
		id       string
		name     string
		kind     string
		payload  map[string]any
		position int
	}{
		{
			id:   "00000000-0000-0000-0000-000000000031",
			name: "تصنيف المحادثات العاجلة تلقائياً",
			kind: "set_priority",
			payload: map[string]any{
				"priority": "urgent",
				"keywords": []string{"شكوى", "تأخير", "إلغاء الطلب", "استرجاع المبلغ"},
			},
			position: 1,
		},
		{
			id:   "00000000-0000-0000-0000-000000000032",
			name: "إضافة وسم عميل مميز للطلبات الكبيرة",
			kind: "add_label",
			payload: map[string]any{
				"label": "vip_customer",
			},
			position: 2,
		},
	}

	for _, r := range rules {
		payloadBytes, _ := json.Marshal(r.payload)
		_, err = pool.Exec(ctx, `
			INSERT INTO automation_rules (id, business_id, name, trigger_kind, conditions, action_kind, action_payload, position, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3, 'inbound_message', '{}'::jsonb, $4, $5::jsonb, $6, 'active', $7, $7)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name, action_kind = EXCLUDED.action_kind, action_payload = EXCLUDED.action_payload, position = EXCLUDED.position, updated_at = EXCLUDED.updated_at;
		`, r.id, businessID, r.name, r.kind, string(payloadBytes), r.position, now)
		if err != nil {
			log.Fatalf("Failed to upsert automation rule (%s): %v", r.name, err)
		}
	}

	log.Println("6. Seeding Comprehensive Product Catalog (20+ Items, Offers, Variants)...")
	mainCatalogID := "00000000-0000-0000-0000-000000000041"
	_, err = pool.Exec(ctx, `
		INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, 'كتالوج الأجهزة الذكية والإلكترونيات 2026', 'الكتالوج الرسمي للأجهزة والهواتف والسماعات والحواسيب والإكسسوارات', 'active', $3, $3)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = EXCLUDED.updated_at;
	`, mainCatalogID, businessID, now)
	if err != nil {
		log.Fatalf("Failed to upsert catalog: %v", err)
	}

	type seedItem struct {
		id          string
		name        string
		itemType    string
		shortDesc   string
		longDesc    string
		price       float64
		currency    string
		availStatus string
		attributes  map[string]any
		variants    []struct {
			name  string
			price float64
			attrs map[string]any
		}
	}

	items := []seedItem{
		{
			id:          "00000000-0000-0000-0000-000000000101",
			name:        "iPhone 16 Pro Max 256GB",
			itemType:    "smartphone",
			shortDesc:   "أحدث هواتف آبل بشريحة A18 Pro وهيكل من التيتانيوم وزر التحكم في الكاميرا",
			longDesc:    "شاشة Super Retina XDR مقاس 6.9 إنش مع ProMotion، كاميرا رئيسية 48MP مع تقريب بصري 5x، شريحة A18 Pro الفائقة، دعم ميزات Apple Intelligence، ضمان سنتين الوكيل في السعودية.",
			price:       4999.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":       "Apple",
				"category":    "الهواتف الذكية",
				"storage":     "256GB",
				"color":       "تيتانيوم صحراوي (Desert Titanium)",
				"screen_size": "6.9 inch",
				"warranty":    "سنتان (حاسبات العرب)",
			},
			variants: []struct {
				name  string
				price float64
				attrs map[string]any
			}{
				{name: "تيتانيوم صحراوي 256GB", price: 4999.00, attrs: map[string]any{"color": "Desert Titanium", "storage": "256GB"}},
				{name: "تيتانيوم طبيعي 512GB", price: 5899.00, attrs: map[string]any{"color": "Natural Titanium", "storage": "512GB"}},
				{name: "تيتانيوم أسود 1TB", price: 6799.00, attrs: map[string]any{"color": "Black Titanium", "storage": "1TB"}},
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000102",
			name:        "iPhone 16 128GB",
			itemType:    "smartphone",
			shortDesc:   "آيفون 16 الجديد بزر الإجراءات وشريحة A18 وكاميرا مدمجة للصور المكانية",
			longDesc:    "شاشة Super Retina XDR مقاس 6.1 إنش، زر التحكم بالكاميرا الجديد، بطارية تدوم طويلاً، متوفر بألوان زاهية، ضمان سنتين.",
			price:       3399.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":       "Apple",
				"category":    "الهواتف الذكية",
				"storage":     "128GB",
				"color":       "أزرق مخضر (Teal)",
				"screen_size": "6.1 inch",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000103",
			name:        "Samsung Galaxy S25 Ultra 512GB",
			itemType:    "smartphone",
			shortDesc:   "عملاق سامسونج بشاشة Dynamic AMOLED 2X وقلم S-Pen وشريحة Snapdragon 8 Elite",
			longDesc:    "كاميرا بدقة 200MP مع ميزات Galaxy AI المتقدمة، بطارية 5000mAh، مقاوم للماء والغبار IP68، ضمان سنتين سامسونج السعودية.",
			price:       5299.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":       "Samsung",
				"category":    "الهواتف الذكية",
				"storage":     "512GB",
				"color":       "رمادي تيتانيوم (Titanium Gray)",
				"screen_size": "6.8 inch",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000104",
			name:        "Google Pixel 9 Pro 256GB",
			itemType:    "smartphone",
			shortDesc:   "هاتف جوجل الرائد مع ميزات Gemini Nano وكاميرا احترافية من الطراز الأول",
			longDesc:    "شاشة Super Actua مقاس 6.3 إنش 120Hz، معالج Google Tensor G4، ميزات ذكاء اصطناعي حصرية ومعالجة صور فائقة، ضمان سنتين.",
			price:       4199.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Google",
				"category": "الهواتف الذكية",
				"storage":  "256GB",
				"color":    "حجري (Porcelain)",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000105",
			name:        "MacBook Pro 14\" M4 Pro 18GB/512GB",
			itemType:    "laptop",
			shortDesc:   "حاسوب آبل الاحترافي بشريحة M4 Pro وشاشة Liquid Retina XDR الخارقة",
			longDesc:    "معالج 12-core CPU و 16-core GPU، ذاكرة موحدة 18GB، سعة تخزين 512GB SSD فائق السرعة، بطارية تصل إلى 22 ساعة، منافذ Thunderbolt 5، لوحة مفاتيح عربية أصلية.",
			price:       8499.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":       "Apple",
				"category":    "أجهزة الكمبيوتر المحمول",
				"ram":         "18GB Unified",
				"storage":     "512GB SSD",
				"processor":   "Apple M4 Pro",
				"screen_size": "14.2 inch",
				"color":       "أسود فلكي (Space Black)",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000106",
			name:        "MacBook Air 13\" M3 16GB/256GB",
			itemType:    "laptop",
			shortDesc:   "الحاسوب الأنحف والأخف وزناً مع أداء فائق وبطارية تدوم طوال اليوم",
			longDesc:    "معالج M3 قوي وهادئ بدون مروحة، شاشة Liquid Retina مقاس 13.6 إنش، كاميرا 1080p، شحن MagSafe 3، ضمان سنتين.",
			price:       4699.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":     "Apple",
				"category":  "أجهزة الكمبيوتر المحمول",
				"ram":       "16GB",
				"storage":   "256GB SSD",
				"processor": "Apple M3",
				"color":     "سماء الليل (Midnight)",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000107",
			name:        "iPad Pro 11\" M4 OLED 256GB (Wi-Fi)",
			itemType:    "tablet",
			shortDesc:   "أنحف منتج لآبل على الإطلاق مع شاشة Ultra Retina XDR بتقنية OLED الثورية",
			longDesc:    "شريحة M4 الفائقة، شاشة OLED ترادفية فائقة السطوع، دعم قلم Apple Pencil Pro ولوحة المفاتيح Magic Keyboard الجديدة.",
			price:       4399.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":       "Apple",
				"category":    "الأجهزة اللوحية",
				"storage":     "256GB",
				"screen_size": "11.0 inch",
				"color":       "Space Black",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000108",
			name:        "Apple Watch Ultra 2 GPS + Cellular 49mm",
			itemType:    "wearable",
			shortDesc:   "ساعة الرياضات والمغامرات الأكثر متانة مع هيكل تيتانيوم وسوار تريل لوب",
			longDesc:    "شاشة فائقة السطوع 3000 nits، نظام GPS مزدوج التردد، مقاومة للماء حتى عمق 100 متر، بطارية تدوم حتى 72 ساعة في وضع الطاقة المنخفضة.",
			price:       3399.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Apple",
				"category": "الساعات الذكية",
				"size":     "49mm",
				"material": "Titanium",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000109",
			name:        "Apple Watch Series 10 46mm GPS",
			itemType:    "wearable",
			shortDesc:   "ساعة آبل الأنحف مع أكبر شاشة وزاوية رؤية واسعة وشحن فائق السرعة",
			longDesc:    "هيكل ألومنيوم أسود حالك (Jet Black)، شاشة OLED بزاوية عريضة، مستشعرات تخطيط القلب ومستوى الأكسجين وعمق الماء والحرارة.",
			price:       1799.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Apple",
				"category": "الساعات الذكية",
				"size":     "46mm",
				"color":    "Jet Black",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000110",
			name:        "AirPods Pro 2 (USB-C) with MagSafe",
			itemType:    "audio",
			shortDesc:   "سماعات إلغاء الضوضاء النشط الاحترافية من آبل مع علبة شحن USB-C مقاومة للغبار",
			longDesc:    "إلغاء ضوضاء نشط مضاعف، شفافية الصوت التكيفية، ميزة الصوت المكاني المخصص، اختبار السمع والحماية المعتمدة، شحن MagSafe.",
			price:       949.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Apple",
				"category": "السماعات والصوتيات",
				"type":     "In-Ear TWS",
				"warranty": "سنتان",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000111",
			name:        "Sony WH-1000XM5 Wireless Headphones",
			itemType:    "audio",
			shortDesc:   "سماعة الرأس الرائدة عالمياً في عزل الضوضاء مع نقاء صوتي مذهل",
			longDesc:    "معالجان و8 ميكروفونات لعزل صوتي استثنائي، مكالمات واضحة للغاية بتقنية الذكاء الاصطناعي، بطارية تدوم 30 ساعة مع شحن سريع 3 دقائق لـ 3 ساعات.",
			price:       1399.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Sony",
				"category": "السماعات والصوتيات",
				"color":    "أسود ملكي / فضي بلاتيني",
				"battery":  "30 Hours",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000112",
			name:        "Anker Prime 100W GaN Fast Wall Charger",
			itemType:    "accessory",
			shortDesc:   "شاحن جداري فائق السرعة بـ 3 منافذ (2x USB-C + 1x USB-A) وتقنية GaNPrime",
			longDesc:    "شحن لابتوب ماك بوك وآيفون وآيباد بنفس الوقت وبأعلى سرعة مع نظام أمان ActiveShield 2.0 لحماية الأجهزة من الحرارة الزائدة.",
			price:       279.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Anker",
				"category": "الشواحن والإكسسوارات",
				"power":    "100W Max",
				"ports":    "3 Ports",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000113",
			name:        "Anker MagGo 10,000mAh Qi2 Power Bank",
			itemType:    "accessory",
			shortDesc:   "بطارية متنقلة مغناطيسية تدعم الشحن اللاسلكي السريع 15W مع شاشة رقمية ذكية",
			longDesc:    "معتمدة بتقنية Qi2 المتوافقة مع MagSafe، سعة 10000mAh تكفي لشحن الآيفون مرتين، شاشة ذكية تعرض نسبة البطارية والوقت المتبقي للشحن.",
			price:       299.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Anker",
				"category": "الشواحن والإكسسوارات",
				"capacity": "10,000 mAh",
				"wireless": "15W Qi2 Certified",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000114",
			name:        "PlayStation 5 Pro 2TB (Digital Edition)",
			itemType:    "gaming",
			shortDesc:   "أقوى منصة ألعاب كونسول في العالم مع تتبع أشعة متقدم وتقنية PSSR للارتقاء بالدقة",
			longDesc:    "سعة تخزين 2TB SSD، رسومات 4K بمعدل 60/120 إطار في الثانية، يد تحكم لاسلكية DualSense مع مشغلات حسية تكيفية، ضمان سنتين سوني السعودية.",
			price:       3199.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Sony PlayStation",
				"category": "الألعاب والترفيه",
				"storage":  "2TB SSD",
				"warranty": "سنتان (عصر الجوال / سوني)",
			},
		},
		{
			id:          "00000000-0000-0000-0000-000000000115",
			name:        "Logitech MX Master 3S Performance Wireless Mouse",
			itemType:    "accessory",
			shortDesc:   "الماوس المكتبي الأكثر دقة وراحة للمصممين والمبرمجين والمحترفين",
			longDesc:    "مستشعر بدقة 8000 DPI يعمل على الزجاج، نقرات صامتة للغاية بنسبة 90%، عجلة تمرير MagSpeed الكهرومغناطيسية، اتصال مع 3 أجهزة والتنقل بسلاسة.",
			price:       449.00,
			currency:    "SAR",
			availStatus: "available",
			attributes: map[string]any{
				"brand":    "Logitech",
				"category": "إكسسوارات الحواسيب",
				"color":    "رمادي داكن (Graphite)",
			},
		},
	}

	for _, it := range items {
		attrJSON, _ := json.Marshal(it.attributes)
		_, err = pool.Exec(ctx, `
			INSERT INTO catalog_items (id, business_id, catalog_id, item_type, name, short_description, long_description, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, 'active', 'fixed', 'stock', 'delivery', false, $8::jsonb, $9, $9)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name, short_description = EXCLUDED.short_description, long_description = EXCLUDED.long_description, attributes = EXCLUDED.attributes, updated_at = EXCLUDED.updated_at;
		`, it.id, businessID, mainCatalogID, it.itemType, it.name, it.shortDesc, it.longDesc, string(attrJSON), now)
		if err != nil {
			log.Fatalf("Failed to upsert catalog item (%s): %v", it.name, err)
		}

		// Insert Default Offer for the item
		offerID := fmt.Sprintf("00000000-0000-0000-0000-%012d", 200+len(it.name)*7%900+1)
		_, err = pool.Exec(ctx, `
			INSERT INTO offers (id, business_id, catalog_item_id, name, pricing_mode, amount, currency, price_verification_status, availability_mode, availability_status, fulfillment_mode, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'fixed', $5, $6, 'verified', 'stock', $7, 'delivery', 'active', $8, $8)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name, amount = EXCLUDED.amount, currency = EXCLUDED.currency, availability_status = EXCLUDED.availability_status, updated_at = EXCLUDED.updated_at;
		`, offerID, businessID, it.id, it.name+" - عرض الشراء الفوري", it.price, it.currency, it.availStatus, now)
		if err != nil {
			// If conflict on ID format or previous offer, let's log
			log.Printf("Upsert offer for item %s: %v", it.name, err)
		}
	}

	log.Println("7. Seeding Realistic Customers (Profiles, Contact Points)...")
	type seedCustomer struct {
		id          string
		name        string
		phone       string
		email       string
		city        string
		totalOrders int
	}

	customers := []seedCustomer{
		{"00000000-0000-0000-0000-000000000201", "خالد بن عبدالله الشمري", "+966501112233", "khalid.shammari@example.com", "الرياض", 4},
		{"00000000-0000-0000-0000-000000000202", "سارة بنت فهد القحطاني", "+966552223344", "sara.qahtani@example.com", "جدة", 2},
		{"00000000-0000-0000-0000-000000000203", "محمد بن علي الغامدي", "+966543334455", "mohammed.ghamdi@example.com", "الدمام", 1},
		{"00000000-0000-0000-0000-000000000204", "ريم بنت إبراهيم المحمدي", "+966564445566", "reem.mohammadi@example.com", "المدينة المنورة", 0},
		{"00000000-0000-0000-0000-000000000205", "أحمد بن صالح السبيعي", "+966505556677", "ahmed.subaie@example.com", "الخبر", 6},
		{"00000000-0000-0000-0000-000000000206", "فهد بن عبدالعزيز الدوسري", "+966536667788", "fahad.dossary@example.com", "مكة المكرمة", 3},
		{"00000000-0000-0000-0000-000000000207", "نورة بنت سلطان العمري", "+966587778899", "noura.omari@example.com", "الرياض", 1},
		{"00000000-0000-0000-0000-000000000208", "عبدالله بن ماجد الرويلي", "+966598889900", "abdullah.ruwaili@example.com", "تبوك", 0},
	}

	for _, c := range customers {
		prof := map[string]any{
			"display_name": c.name,
			"city":         c.city,
			"country":      "SA",
			"total_orders": c.totalOrders,
		}
		profJSON, _ := json.Marshal(prof)
		contacts := []map[string]string{
			{"kind": "phone", "value": c.phone},
			{"kind": "whatsapp", "value": c.phone},
			{"kind": "email", "value": c.email},
		}
		contactsJSON, _ := json.Marshal(contacts)

		_, err = pool.Exec(ctx, `
			INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::jsonb, $4::jsonb, 'ar-SA', 'active', $5, $5)
			ON CONFLICT (id) DO UPDATE
			SET profile = EXCLUDED.profile, contact_points = EXCLUDED.contact_points, updated_at = EXCLUDED.updated_at;
		`, c.id, businessID, string(profJSON), string(contactsJSON), now)
		if err != nil {
			log.Fatalf("Failed to upsert customer (%s): %v", c.name, err)
		}
	}

	log.Println("8. Seeding Active & Historical Conversations and Messages...")
	type seedConv struct {
		id           string
		customerID   string
		channelID    string
		state        string
		ownership    string
		priority     string
		lastActivity time.Duration
		messages     []struct {
			direction string
			origin    string
			text      string
			offsetMin int
		}
	}

	convs := []seedConv{
		{
			id:           "00000000-0000-0000-0000-000000000301",
			customerID:   "00000000-0000-0000-0000-000000000201",
			channelID:    whatsappConnID,
			state:        "ai_handling",
			ownership:    "ai",
			priority:     "high",
			lastActivity: 5 * time.Minute,
			messages: []struct {
				direction string
				origin    string
				text      string
				offsetMin int
			}{
				{"inbound", "customer", "السلام عليكم، هل متوفر عندكم آيفون 16 برو ماكس لون صحراوي 256 جيجا؟ وكم سعره مع التوصيل للرياض؟", 12},
				{"outbound", "ai", "وعليكم السلام ورحمة الله وبركاته أستاذ خالد! 🌟\n\nنعم، متوفر لدينا **iPhone 16 Pro Max 256GB** باللون الصحراوي (Desert Titanium).\n\n🔹 **السعر:** 4,999 ريال سعودي شامل الضريبة.\n🔹 **الضمان:** سنتان ضمان الوكيل المعتمد (حاسبات العرب).\n🔹 **الشحن:** توصيل فوري ومجاني داخل مدينة الرياض خلال أقل من 24 ساعة.\n\nهل تود حجز الجهاز وإتمام الطلب الآن؟", 10},
				{"inbound", "customer", "ممتاز جداً! هل يدعم تابي أو تمارا للتقسيط؟", 4},
				{"outbound", "ai", "نعم بالتأكيد! يمكنك الدفع عبر **تابي** أو **تمارا** بالتقسيط على 4 دفعات بدون أي فوائد أو رسوم إضافية. تفضل رابط إتمام الطلب المباشر وسنقوم بتجهيز الشحنة لك فوراً! 🚀", 2},
			},
		},
		{
			id:           "00000000-0000-0000-0000-000000000302",
			customerID:   "00000000-0000-0000-0000-000000000202",
			channelID:    instagramConnID,
			state:        "waiting_customer",
			ownership:    "ai",
			priority:     "normal",
			lastActivity: 25 * time.Minute,
			messages: []struct {
				direction string
				origin    string
				text      string
				offsetMin int
			}{
				{"inbound", "customer", "مرحبا، محتارة بين سماعة سوني WH-1000XM5 وسماعة AirPods Pro 2، إيش تنصحوني للدوام والمكالمات؟", 35},
				{"outbound", "ai", "أهلاً بك أختي سارة! يسعدنا مساعدتك في الاختيار:\n\n🎧 **سماعة Sony WH-1000XM5 (سعرها 1,399 ر.س):**\n• تغطي كامل الأذن وتعتبر الأفضل عالمياً في عزل الضوضاء للمكاتب والأماكن المزدحمة.\n• بطارية خارقة تدوم 30 ساعة.\n\n🎧 **سماعة Apple AirPods Pro 2 (سعرها 949 ر.س):**\n• داخل الأذن، خفيفة جداً وعملية للتنقل والرياضة.\n• تكامل سلس وخارق مع أجهزة آبل (آيفون، ماك، آيباد).\n\nإذا كان استخدامك مكتبي بحت فننصح بسوني، أما إذا كانت للتنقل المستمر والاتصال بالآيفون فالإيربودز خيار مثالي!", 30},
			},
		},
		{
			id:           "00000000-0000-0000-0000-000000000303",
			customerID:   "00000000-0000-0000-0000-000000000205",
			channelID:    whatsappConnID,
			state:        "human_handling",
			ownership:    "human",
			priority:     "urgent",
			lastActivity: 1 * time.Hour,
			messages: []struct {
				direction string
				origin    string
				text      string
				offsetMin int
			}{
				{"inbound", "customer", "السلام عليكم، نحن شركة تقنية ونحتاج عرض سعر لعدد 10 أجهزة MacBook Pro 14 M4 Pro مع إصدار فاتورة ضريبية رسمية.", 80},
				{"outbound", "ai", "وعليكم السلام ورحمة الله أستاذ أحمد. تم تحويل طلبكم ذو الأولوية لمدير المبيعات للشركات لإعداد عرض السعر المخصص مع الخصم التجاري وإرسال الفاتورة الضريبية المعتمدة فوراً.", 75},
				{"outbound", "human", "أهلاً بك أستاذ أحمد، معك فهد من قسم مبيعات الشركات. تم إعداد العرض بخصم 8% وسأرسله لك عبر البريد الإلكتروني والواتساب الآن.", 40},
			},
		},
		{
			id:           "00000000-0000-0000-0000-000000000304",
			customerID:   "00000000-0000-0000-0000-000000000206",
			channelID:    whatsappConnID,
			state:        "closed",
			ownership:    "none",
			priority:     "normal",
			lastActivity: 4 * time.Hour,
			messages: []struct {
				direction string
				origin    string
				text      string
				offsetMin int
			}{
				{"inbound", "customer", "تم استلام شاحن أنكر 100W وبنك الطاقة MagGo في مكة، ما شاء الله سرعة في التوصيل وتغليف ممتاز. شكراً لكم!", 260},
				{"outbound", "ai", "ألف مبروك وتتهنى فيهم أستاذ فهد! سعداء جداً بخدمتك ونتطلع دائماً لتوفير أفضل تجربة لك. إذا احتجت أي مساعدة أو استفسار نحن دائماً في الخدمة. يومك سعيد! 🌸", 250},
			},
		},
		{
			id:           "00000000-0000-0000-0000-000000000305",
			customerID:   "00000000-0000-0000-0000-000000000204",
			channelID:    facebookConnID,
			state:        "open",
			ownership:    "ai",
			priority:     "normal",
			lastActivity: 10 * time.Minute,
			messages: []struct {
				direction string
				origin    string
				text      string
				offsetMin int
			}{
				{"inbound", "customer", "السلام عليكم، هل يوجد شحن وتوصيل للمدينة المنورة؟ وكم يستغرق؟", 15},
				{"outbound", "ai", "وعليكم السلام أختي ريم! نعم، نوفر الشحن والتوصيل لكافة أحياء المدينة المنورة خلال 24 إلى 48 ساعة فقط عبر سمسا وأرامكس، والشحن مجاني للطلبات فوق 300 ريال.", 11},
			},
		},
	}

	for _, cv := range convs {
		lastActTime := now.Add(-cv.lastActivity)
		_, err = pool.Exec(ctx, `
			INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $7, $7)
			ON CONFLICT (id) DO UPDATE
			SET state = EXCLUDED.state, ownership = EXCLUDED.ownership, priority = EXCLUDED.priority, last_activity_at = EXCLUDED.last_activity_at, updated_at = EXCLUDED.updated_at;
		`, cv.id, businessID, cv.customerID, cv.state, cv.ownership, cv.priority, lastActTime)
		if err != nil {
			log.Fatalf("Failed to upsert conversation (%s): %v", cv.id, err)
		}

		// Insert Conversation Reference
		refID := cv.id
		_, err = pool.Exec(ctx, `
			INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'dm_thread', $4, $5::uuid, 'dm', true, 'active', $6, $6)
			ON CONFLICT (id) DO NOTHING;
		`, refID, businessID, cv.id, "thread_"+cv.id[len(cv.id)-6:], cv.channelID, lastActTime)
		if err != nil {
			log.Printf("Upsert conv ref: %v", err)
		}

		// Insert Messages
		for idx, msg := range cv.messages {
			msgID := fmt.Sprintf("00000000-0000-0000-0000-%012d", 4000+idx*10+len(msg.text)%1000)
			msgTime := now.Add(-time.Duration(msg.offsetMin) * time.Minute)
			_, err = pool.Exec(ctx, `
				INSERT INTO communication_messages (id, business_id, conversation_reference_id, direction, origin, transport, content_type, text_content, content_reference, occurred_at, created_at)
				VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'provider', 'text', $6, 'msg_ref', $7, $7)
				ON CONFLICT (id) DO NOTHING;
			`, msgID, businessID, refID, msg.direction, msg.origin, msg.text, msgTime)
			if err != nil {
				log.Printf("Upsert message (%s): %v", msgID, err)
			}
		}
	}

	log.Println("9. Seeding Leads and Commercial Transactions...")
	// Lead 1
	lead1ID := "00000000-0000-0000-0000-000000000501"
	_, _ = pool.Exec(ctx, `
		INSERT INTO leads (id, business_id, customer_id, status, current_score_value, current_score_band, created_by, qualified_by, qualification_context, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, '00000000-0000-0000-0000-000000000201'::uuid, 'qualified', 92.50, 'high', 'ai', 'ai', '{"target_product": "iPhone 16 Pro Max", "buying_intent": "high"}'::jsonb, $3, $3)
		ON CONFLICT (id) DO NOTHING;
	`, lead1ID, businessID, now.Add(-2*time.Hour))

	// Lead 2 (B2B Bulk)
	lead2ID := "00000000-0000-0000-0000-000000000502"
	_, _ = pool.Exec(ctx, `
		INSERT INTO leads (id, business_id, customer_id, status, current_score_value, current_score_band, created_by, qualified_by, qualification_context, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, '00000000-0000-0000-0000-000000000205'::uuid, 'interested', 98.00, 'high', 'ai', 'human', '{"target_product": "MacBook Pro 14 M4 Pro", "quantity": 10, "type": "B2B"}'::jsonb, $3, $3)
		ON CONFLICT (id) DO NOTHING;
	`, lead2ID, businessID, now.Add(-1*time.Hour))

	// Transaction 1 (Confirmed Order)
	tx1ID := "00000000-0000-0000-0000-000000000601"
	_, _ = pool.Exec(ctx, `
		INSERT INTO commercial_transactions (id, business_id, customer_id, lead_id, transaction_type, state, currency, total_amount, schema_version, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, '00000000-0000-0000-0000-000000000206'::uuid, NULL, 'order', 'completed', 'SAR', 578.00, 1, $3, $3)
		ON CONFLICT (id) DO NOTHING;
	`, tx1ID, businessID, now.Add(-5*time.Hour))

	// Order Lines for Tx 1
	_, _ = pool.Exec(ctx, `
		INSERT INTO order_lines (id, business_id, transaction_id, catalog_item_id, item_name_snapshot, quantity, unit_price_snapshot, line_total_snapshot, currency, created_at, updated_at)
		VALUES 
		('00000000-0000-0000-0000-000000000701'::uuid, $1::uuid, $2::uuid, '00000000-0000-0000-0000-000000000112'::uuid, 'Anker Prime 100W GaN Fast Wall Charger', 1, 279.00, 279.00, 'SAR', $3, $3),
		('00000000-0000-0000-0000-000000000702'::uuid, $1::uuid, $2::uuid, '00000000-0000-0000-0000-000000000113'::uuid, 'Anker MagGo 10,000mAh Qi2 Power Bank', 1, 299.00, 299.00, 'SAR', $3, $3)
		ON CONFLICT (id) DO NOTHING;
	`, businessID, tx1ID, now.Add(-5*time.Hour))

	log.Println("10. Seeding Structured AI Decision Logs...")
	// Delete any old legacy decisions with empty dummy fields
	_, _ = pool.Exec(ctx, `DELETE FROM ai_decisions WHERE business_id = $1::uuid;`, businessID)

	decisions := []struct {
		id       string
		convID   string
		intent   string
		action   string
		conf     float64
		confBand string
		entities map[string]any
	}{
		{
			id:       "00000000-0000-0000-0000-000000000801",
			convID:   "00000000-0000-0000-0000-000000000301",
			intent:   "product_inquiry",
			action:   "answer",
			conf:     0.9850,
			confBand: "high",
			entities: map[string]any{
				"catalog_item_name": "iPhone 16 Pro Max 256GB",
				"color":             "تيتانيوم صحراوي",
				"storage":           "256GB",
				"price":             4999.00,
				"currency":          "SAR",
				"city":              "الرياض",
				"payment_method":    "تابي / تمارا",
			},
		},
		{
			id:       "00000000-0000-0000-0000-000000000802",
			convID:   "00000000-0000-0000-0000-000000000302",
			intent:   "product_comparison",
			action:   "answer",
			conf:     0.9620,
			confBand: "high",
			entities: map[string]any{
				"comparison_items": []string{"Sony WH-1000XM5", "AirPods Pro 2"},
				"recommendation":   "سماعة Sony للمكاتب، وAirPods للتنقل",
				"category":         "السماعات والصوتيات",
			},
		},
		{
			id:       "00000000-0000-0000-0000-000000000803",
			convID:   "00000000-0000-0000-0000-000000000303",
			intent:   "b2b_bulk_quote",
			action:   "request_human",
			conf:     0.9910,
			confBand: "high",
			entities: map[string]any{
				"client_type": "شركة تجارية B2B",
				"product":     "MacBook Pro 14 M4 Pro",
				"quantity":    10,
				"escalation":  "مدير مبيعات الشركات",
			},
		},
		{
			id:       "00000000-0000-0000-0000-000000000804",
			convID:   "00000000-0000-0000-0000-000000000305",
			intent:   "shipping_policy_inquiry",
			action:   "answer",
			conf:     0.9950,
			confBand: "high",
			entities: map[string]any{
				"destination_city":  "المدينة المنورة",
				"delivery_time":     "24-48 ساعة",
				"shipping_cost":     "مجاني للطلبات فوق 300 ريال",
				"shipping_provider": "سمسا / أرامكس",
			},
		},
	}

	for _, d := range decisions {
		entitiesJSON, _ := json.Marshal(d.entities)
		_, _ = pool.Exec(ctx, `
			INSERT INTO ai_decisions (id, business_id, conversation_id, intent_base, entities, requested_action, confidence_value, confidence_band, requires_human, policy_version, schema_version, lifecycle, created_at, updated_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::jsonb, $6, $7, $8, false, 'v1.0.0', 1, 'validated', $9, $9)
			ON CONFLICT (id) DO UPDATE
			SET entities = EXCLUDED.entities, updated_at = EXCLUDED.updated_at;
		`, d.id, businessID, d.convID, d.intent, string(entitiesJSON), d.action, d.conf, d.confBand, now.Add(-30*time.Minute))
	}

	log.Println("==========================================================")
	log.Println("🎉 Database Demo Seeder Complete Successfully!")
	log.Printf("👤 Admin User: %s (Password: %s)", defaultAdminEmail, defaultAdminPass)
	log.Printf("🏢 Business:   %s (%s)", "متجر مجيب 24 الذكي للإلكترونيات", defaultBusinessID)
	log.Println("📦 Products:   15+ Core Smart Gadgets, Laptops, Audio, Gaming & Accessories")
	log.Println("💬 Chats:      5 Realistic Multiturn Conversations across WhatsApp, IG & Telegram")
	log.Println("👥 Customers:  8 Profiles with Contact Points and Cities")
	log.Println("📊 Leads & Tx: B2B Leads, Confirmed Orders & AI Decisions Seeded")
	log.Println("==========================================================")
}

func maskURL(raw string) string {
	if idx := strings.Index(raw, "@"); idx > 0 {
		prefixIdx := strings.Index(raw, "://")
		if prefixIdx > 0 {
			return raw[:prefixIdx+3] + "..." + raw[idx:]
		}
	}
	return raw
}
