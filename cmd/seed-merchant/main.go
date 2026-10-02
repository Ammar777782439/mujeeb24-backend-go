package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	businessName   = "نظام مجيب مقدم خدمات"
	businessSlug   = "mujeeb-services"
	verticalType   = "services"
	timezone       = "Asia/Aden"
	defaultCurrency = "YER"
	locale         = "ar-YE"
	planCode       = "basic"
	planVersion    = 1
	expectedPlanYER = 5000
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	email := strings.ToLower(strings.TrimSpace(os.Getenv("MERCHANT_SEED_EMAIL")))
	password := os.Getenv("MERCHANT_SEED_PASSWORD")
	displayName := strings.TrimSpace(os.Getenv("MERCHANT_SEED_DISPLAY_NAME"))
	if email == "" {
		log.Fatal("MERCHANT_SEED_EMAIL is required")
	}
	if password == "" {
		log.Fatal("MERCHANT_SEED_PASSWORD is required")
	}
	if len(password) < 12 {
		log.Fatal("MERCHANT_SEED_PASSWORD must be at least 12 characters")
	}
	if displayName == "" {
		displayName = businessName
	}

	adapter, err := postgres.Open(ctx, dbURL, postgres.PoolConfig{
		MaxConns:       4,
		MinConns:       1,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer adapter.Close()

	pool := adapter.Pool()

	var (
		planID                string
		planPrice              int
		billingInterval        string
		aiReplyLimit           int
		aiCatalogLimit         int
		channelLimit           int
		internalCostBudgetYER  int
	)
	err = pool.QueryRow(ctx, `
		SELECT id, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit,
		       channel_limit, internal_ai_cost_budget_yer
		FROM plans
		WHERE code = $1 AND version = $2 AND status = 'ACTIVE'
	`, planCode, planVersion).Scan(
		&planID,
		&planPrice,
		&billingInterval,
		&aiReplyLimit,
		&aiCatalogLimit,
		&channelLimit,
		&internalCostBudgetYER,
	)
	if err != nil {
		log.Fatalf("active plan %s v%d not found: %v", planCode, planVersion, err)
	}
	if planPrice != expectedPlanYER || billingInterval != "MONTH" {
		log.Fatalf("plan %s v%d mismatch: expected %d YER/MONTH, got %d YER/%s",
			planCode, planVersion, expectedPlanYER, planPrice, billingInterval)
	}

	passHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("password hashing failed: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Fatalf("begin transaction failed: %v", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	var principalID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM principals
		WHERE lower(email) = lower($1)
		FOR UPDATE
	`, email).Scan(&principalID)

	if errors.Is(err, nil) {
		_, err = tx.Exec(ctx, `
			UPDATE principals
			SET display_name = $2, password_hash = $3, status = 'active', updated_at = $4
			WHERE id = $1::uuid
		`, principalID, displayName, string(passHash), now)
		if err != nil {
			log.Fatalf("update merchant principal failed: %v", err)
		}
	} else if err != nil {
		if !strings.Contains(err.Error(), "no rows") {
			log.Fatalf("lookup merchant principal failed: %v", err)
		}
		principalID = uuid.New().String()
		_, err = tx.Exec(ctx, `
			INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
			VALUES ($1::uuid, $2, $3, $4, 'active', $5, $5)
		`, principalID, email, displayName, string(passHash), now)
		if err != nil {
			log.Fatalf("create merchant principal failed: %v", err)
		}
	}

	var businessID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM businesses
		WHERE slug = $1
		FOR UPDATE
	`, businessSlug).Scan(&businessID)

	if errors.Is(err, nil) {
		_, err = tx.Exec(ctx, `
			UPDATE businesses
			SET name = $2,
			    status = CASE WHEN status = 'archived' THEN 'active' ELSE status END,
			    vertical_type = $3,
			    timezone = $4,
			    default_currency = $5,
			    locale = $6,
			    updated_at = $7
			WHERE id = $1::uuid
		`, businessID, businessName, verticalType, timezone, defaultCurrency, locale, now)
		if err != nil {
			log.Fatalf("update merchant business failed: %v", err)
		}
	} else if err != nil {
		if !strings.Contains(err.Error(), "no rows") {
			log.Fatalf("lookup merchant business failed: %v", err)
		}
		businessID = uuid.New().String()
		_, err = tx.Exec(ctx, `
			INSERT INTO businesses (
				id, name, slug, status, vertical_type, timezone, default_currency,
				locale, created_at, updated_at, resource_version
			)
			VALUES ($1::uuid, $2, $3, 'active', $4, $5, $6, $7, $8, $8, 1)
		`, businessID, businessName, businessSlug, verticalType, timezone, defaultCurrency, locale, now)
		if err != nil {
			log.Fatalf("create merchant business failed: %v", err)
		}
	}

	var existingOwner string
	err = tx.QueryRow(ctx, `
		SELECT principal_id::text
		FROM business_memberships
		WHERE business_id = $1::uuid AND role = 'owner' AND status = 'active'
		LIMIT 1
	`, businessID).Scan(&existingOwner)
	if err == nil && existingOwner != principalID {
		log.Fatalf("business %s already has a different active owner", businessSlug)
	}
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		log.Fatalf("owner lookup failed: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO business_memberships (
			business_id, principal_id, role, permissions, status, created_at, updated_at
		)
		VALUES ($1::uuid, $2::uuid, 'owner', '["*"]'::jsonb, 'active', $3, $3)
		ON CONFLICT (business_id, principal_id)
		DO UPDATE SET role = 'owner', permissions = '["*"]'::jsonb,
		              status = 'active', updated_at = EXCLUDED.updated_at
	`, businessID, principalID, now)
	if err != nil {
		log.Fatalf("merchant membership failed: %v", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO business_policies (
			business_id, ai_mode, default_human_review, allow_auto_reply,
			allow_auto_lead_creation, allow_auto_transaction_draft,
			allow_auto_confirmation, created_at, updated_at
		)
		VALUES ($1::uuid, 'restricted_auto', false, true, true, true, true, $2, $2)
		ON CONFLICT (business_id) DO UPDATE
		SET updated_at = EXCLUDED.updated_at
	`, businessID, now)
	if err != nil {
		log.Fatalf("merchant business policy failed: %v", err)
	}

	var subscriptionID string
	err = tx.QueryRow(ctx, `
		SELECT id::text
		FROM subscriptions
		WHERE business_id = $1::uuid
		  AND status IN ('PENDING', 'ACTIVE')
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, businessID).Scan(&subscriptionID)

	if err == nil {
		var existingPlanID string
		err = tx.QueryRow(ctx, `
			SELECT plan_id::text FROM subscriptions WHERE id = $1::uuid
		`, subscriptionID).Scan(&existingPlanID)
		if err != nil {
			log.Fatalf("subscription plan lookup failed: %v", err)
		}
		if existingPlanID != planID {
			log.Fatalf("business already has a PENDING/ACTIVE subscription on another plan")
		}
	} else if !strings.Contains(err.Error(), "no rows") {
		log.Fatalf("subscription lookup failed: %v", err)
	} else {
		subscriptionID = uuid.New().String()
		_, err = tx.Exec(ctx, `
			INSERT INTO subscriptions (
				id, business_id, plan_id, period_start, period_end, status,
				ai_reply_limit, ai_catalog_limit, channel_limit,
				internal_ai_cost_budget_yer, created_at, updated_at
			)
			VALUES (
				$1::uuid, $2::uuid, $3::uuid, $4, $5, 'PENDING',
				$6, $7, $8, $9, $4, $4
			)
		`, subscriptionID, businessID, planID, now, now.AddDate(0, 1, 0),
			aiReplyLimit, aiCatalogLimit, channelLimit, internalCostBudgetYER)
		if err != nil {
			log.Fatalf("create merchant subscription failed: %v", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		log.Fatalf("commit failed: %v", err)
	}

	log.Println("Merchant seed completed.")
	log.Printf("Business: %s (%s)", businessName, businessSlug)
	log.Printf("Owner email: %s", email)
	log.Printf("Subscription: %s / %d YER per MONTH / %s", planCode, planPrice, subscriptionStatus(pool, ctx, subscriptionID))
	log.Printf("Business ID: %s", businessID)
	log.Printf("Subscription ID: %s", subscriptionID)
	log.Println("No payment was fabricated; subscription remains PENDING until a real payment is recorded.")
}

func subscriptionStatus(pool interface {
	QueryRow(context.Context, string, ...any) interface {
		Scan(...any) error
	}
}, ctx context.Context, subscriptionID string) string {
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM subscriptions WHERE id = $1::uuid", subscriptionID).Scan(&status); err != nil {
		return "UNKNOWN"
	}
	return status
}

func init() {
	if os.Getenv("BCRYPT_COST") != "" {
		_ = fmt.Sprint("")
	}
}
