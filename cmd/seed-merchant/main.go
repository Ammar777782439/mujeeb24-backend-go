package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Fixed demo merchant requested for end-to-end testing.
//
// Business: نظام مجيب مقدم خدمات
// Email:    merchant@mujeeb24.com
// Password: Mujeeb24@2026
//
// The password is never stored in plaintext; the database receives only
// a bcrypt hash. Subscription activation follows the real contract:
// PENDING -> recorded payment -> ACTIVE.

const (
	merchantEmail    = "merchant@mujeeb24.com"
	merchantPassword = "Mujeeb24@2026"
	merchantName     = "نظام مجيب مقدم خدمات"
	merchantSlug     = "mujeeb-service-provider"

	merchantPrincipalID    = "00000000-0000-0000-0000-000000000004"
	merchantBusinessID     = "00000000-0000-0000-0000-000000000004"
	merchantSubscriptionID = "00000000-0000-0000-0000-000000000005"

	basicPlanID       = "00000000-0000-0000-0000-00000000a001"
	basicPlanPriceYER = 5000
)

func main() {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	adapter, err := postgres.Open(ctx, dbURL, postgres.PoolConfig{
		MaxConns:       4,
		MinConns:       1,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer adapter.Close()

	tx, err := adapter.Pool().Begin(ctx)
	if err != nil {
		log.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()

	log.Println("1. Upserting merchant principal...")
	hash, err := bcrypt.GenerateFromPassword([]byte(merchantPassword), 12)
	if err != nil {
		log.Fatalf("hash merchant password: %v", err)
	}

	var principalID string
	err = tx.QueryRow(ctx, `
		INSERT INTO principals (
			id, email, display_name, password_hash, status, created_at, updated_at
		)
		VALUES ($1::uuid, lower($2), $3, $4, 'active', $5, $5)
		ON CONFLICT (lower(email)) DO UPDATE
		SET display_name = EXCLUDED.display_name,
			password_hash = EXCLUDED.password_hash,
			status = 'active',
			updated_at = EXCLUDED.updated_at
		RETURNING id::text
	`, merchantPrincipalID, merchantEmail, merchantName, string(hash), now).Scan(&principalID)
	if err != nil {
		log.Fatalf("upsert merchant principal: %v", err)
	}

	log.Println("2. Upserting merchant business...")
	_, err = tx.Exec(ctx, `
		INSERT INTO businesses (
			id, name, slug, status, vertical_type, timezone,
			default_currency, locale, created_at, updated_at, resource_version
		)
		VALUES (
			$1::uuid, $2, $3, 'active', 'services', 'Asia/Aden',
			'YER', 'ar', $4, $4, 1
		)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			status = 'active',
			vertical_type = EXCLUDED.vertical_type,
			timezone = EXCLUDED.timezone,
			default_currency = EXCLUDED.default_currency,
			locale = EXCLUDED.locale,
			updated_at = EXCLUDED.updated_at
	`, merchantBusinessID, merchantName, merchantSlug, now)
	if err != nil {
		log.Fatalf("upsert merchant business: %v", err)
	}

	log.Println("3. Upserting merchant policy...")
	_, err = tx.Exec(ctx, `
		INSERT INTO business_policies (
			business_id, ai_mode, default_human_review,
			allow_auto_reply, allow_auto_lead_creation,
			allow_auto_transaction_draft, allow_auto_confirmation,
			created_at, updated_at
		)
		VALUES (
			$1::uuid, 'restricted_auto', false,
			true, true, true, true,
			$2, $2
		)
		ON CONFLICT (business_id) DO UPDATE
		SET ai_mode = EXCLUDED.ai_mode,
			allow_auto_reply = EXCLUDED.allow_auto_reply,
			allow_auto_lead_creation = EXCLUDED.allow_auto_lead_creation,
			allow_auto_transaction_draft = EXCLUDED.allow_auto_transaction_draft,
			allow_auto_confirmation = EXCLUDED.allow_auto_confirmation,
			updated_at = EXCLUDED.updated_at
	`, merchantBusinessID, now)
	if err != nil {
		log.Fatalf("upsert merchant policy: %v", err)
	}

	log.Println("4. Upserting owner membership...")
	_, err = tx.Exec(ctx, `
		INSERT INTO business_memberships (
			business_id, principal_id, role, permissions, status, created_at, updated_at
		)
		VALUES ($1::uuid, $2::uuid, 'owner', '["*"]'::jsonb, 'active', $3, $3)
		ON CONFLICT (business_id, principal_id) DO UPDATE
		SET role = 'owner',
			permissions = '["*"]'::jsonb,
			status = 'active',
			updated_at = EXCLUDED.updated_at
	`, merchantBusinessID, principalID, now)
	if err != nil {
		log.Fatalf("upsert merchant membership: %v", err)
	}

	log.Println("5. Ensuring Basic plan (5000 YER/month)...")
	_, err = tx.Exec(ctx, `
		INSERT INTO plans (
			id, code, version, display_name, price_yer, billing_interval,
			ai_reply_limit, ai_catalog_limit, channel_limit,
			internal_ai_cost_budget_yer, status, created_at, updated_at
		)
		VALUES (
			$1::uuid, 'basic', 1, 'Basic', 5000, 'MONTH',
			500, 200, 1, 1000, 'ACTIVE', $2, $2
		)
		ON CONFLICT (code, version) DO NOTHING
	`, basicPlanID, now)
	if err != nil {
		log.Fatalf("ensure basic plan: %v", err)
	}

	var planID string
	var planPrice int
	var planStatus string
	var aiReplyLimit, aiCatalogLimit, channelLimit, costBudget int
	err = tx.QueryRow(ctx, `
		SELECT id::text, price_yer, status,
		       ai_reply_limit, ai_catalog_limit,
		       channel_limit, internal_ai_cost_budget_yer
		FROM plans
		WHERE code = 'basic' AND version = 1
	`).Scan(
		&planID, &planPrice, &planStatus,
		&aiReplyLimit, &aiCatalogLimit, &channelLimit, &costBudget,
	)
	if err != nil {
		log.Fatalf("load basic plan: %v", err)
	}
	if planPrice != basicPlanPriceYER || planStatus != "ACTIVE" {
		log.Fatalf("basic plan contract mismatch: price=%d status=%s", planPrice, planStatus)
	}

	log.Println("6. Creating/validating subscription...")
	var subscriptionStatus string
	var existingBusinessID, existingPlanID string
	err = tx.QueryRow(ctx, `
		SELECT business_id::text, plan_id::text, status
		FROM subscriptions
		WHERE id = $1::uuid
	`, merchantSubscriptionID).Scan(&existingBusinessID, &existingPlanID, &subscriptionStatus)

	if errors.Is(err, pgx.ErrNoRows) {
		periodStart := now
		periodEnd := now.AddDate(0, 1, 0)

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
		`, merchantSubscriptionID, merchantBusinessID, planID, periodStart, periodEnd,
			aiReplyLimit, aiCatalogLimit, channelLimit, costBudget)
		if err != nil {
			log.Fatalf("create subscription: %v", err)
		}
		subscriptionStatus = "PENDING"
	} else if err != nil {
		log.Fatalf("load subscription: %v", err)
	} else {
		if existingBusinessID != merchantBusinessID || existingPlanID != planID {
			log.Fatalf("fixed subscription id already belongs to another business or plan")
		}
	}

	log.Println("7. Recording payment and activating subscription...")
	var paymentExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM subscription_payments
			WHERE subscription_id = $1::uuid
			  AND reference = 'SEED-MUJEEB-BASIC-5000'
		)
	`, merchantSubscriptionID).Scan(&paymentExists)
	if err != nil {
		log.Fatalf("check seed payment: %v", err)
	}

	if !paymentExists {
		var recordedBy string
		if err := tx.QueryRow(ctx, `
			SELECT principal_id::text
			FROM platform_super_admins
			WHERE status = 'active'
			ORDER BY created_at ASC
			LIMIT 1
		`).Scan(&recordedBy); err != nil {
			log.Fatalf("no active platform super admin available to record payment: %v", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO subscription_payments (
				id, subscription_id, business_id, amount_yer,
				method, reference, paid_at, recorded_by, created_at
			)
			VALUES (
				$1::uuid, $2::uuid, $3::uuid, $4,
				'CASH', $5, $6, $7::uuid, $6
			)
		`, uuid.NewString(), merchantSubscriptionID, merchantBusinessID,
			basicPlanPriceYER, "SEED-MUJEEB-BASIC-5000", now, recordedBy)
		if err != nil {
			log.Fatalf("record seed payment: %v", err)
		}
	}

	if subscriptionStatus == "PENDING" {
		_, err = tx.Exec(ctx, `
			UPDATE subscriptions
			SET status = 'ACTIVE', updated_at = $2
			WHERE id = $1::uuid AND status = 'PENDING'
		`, merchantSubscriptionID, now)
		if err != nil {
			log.Fatalf("activate subscription: %v", err)
		}
	} else if subscriptionStatus != "ACTIVE" {
		log.Fatalf("fixed subscription is in terminal/non-active state: %s", subscriptionStatus)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("commit merchant seed: %v", err)
	}

	log.Println("==========================================================")
	log.Println("Mujeeb merchant seed completed successfully.")
	log.Printf("Business:  %s\n", merchantName)
	log.Printf("Email:     %s\n", merchantEmail)
	log.Printf("Password:  %s\n", merchantPassword)
	log.Printf("Plan:      Basic (%d YER/month)\n", basicPlanPriceYER)
	log.Println("Subscription: ACTIVE")
	log.Printf("Business ID: %s\n", merchantBusinessID)
	log.Println("==========================================================")
}
