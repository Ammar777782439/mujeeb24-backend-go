package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"golang.org/x/crypto/bcrypt"
	"github.com/google/uuid"
)

const (
	seedConfirmation = "CREATE_PLATFORM_ADMIN_OWNER"
	defaultBusinessID = "00000000-0000-0000-0000-000000000001"
	defaultBusinessName = "متجر مجيب 24 الذكي للإلكترونيات"
	defaultBusinessSlug = "mujeeb-store"
	defaultDisplayName = "مدير المنصة"
)

type seedConfig struct {
	DatabaseURL string
	Email       string
	Password    string
	DisplayName string
	BusinessID  string
	BusinessName string
	BusinessSlug string
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := uuid.Parse(cfg.BusinessID); err != nil {
		log.Fatalf("PLATFORM_ADMIN_OWNER_BUSINESS_ID must be a UUID: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adapter, err := postgres.Open(ctx, cfg.DatabaseURL, postgres.PoolConfig{
		MaxConns:       2,
		MinConns:       0,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer adapter.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(cfg.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	var principalID string
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}

		err = executor.QueryRow(txCtx, `
			INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
			VALUES ($1::uuid, lower($2), $3, $4, 'active', $5, $5)
			ON CONFLICT (lower(email)) DO UPDATE
			SET display_name = EXCLUDED.display_name,
			    password_hash = EXCLUDED.password_hash,
			    status = 'active',
			    updated_at = EXCLUDED.updated_at
			RETURNING id::text
		`, uuid.NewString(), cfg.Email, cfg.DisplayName, string(passwordHash), time.Now().UTC()).Scan(&principalID)
		if err != nil {
			return errors.New("upsert platform admin principal: " + err.Error())
		}

		_, err = executor.Exec(txCtx, `
			INSERT INTO platform_super_admins (principal_id, status, created_at, updated_at, revoked_at)
			VALUES ($1::uuid, 'active', $2, $2, NULL)
			ON CONFLICT (principal_id) DO UPDATE
			SET status = 'active',
			    updated_at = EXCLUDED.updated_at,
			    revoked_at = NULL
		`, principalID, time.Now().UTC())
		if err != nil {
			return errors.New("upsert platform super admin: " + err.Error())
		}

		_, err = executor.Exec(txCtx, `
			INSERT INTO businesses (
				id, name, slug, status, vertical_type, timezone, default_currency, locale,
				created_at, updated_at, resource_version
			)
			VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Riyadh', 'SAR', 'ar-SA', $4, $4, 1)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name,
			    slug = EXCLUDED.slug,
			    status = 'active',
			    vertical_type = EXCLUDED.vertical_type,
			    timezone = EXCLUDED.timezone,
			    default_currency = EXCLUDED.default_currency,
			    locale = EXCLUDED.locale,
			    updated_at = EXCLUDED.updated_at
		`, cfg.BusinessID, cfg.BusinessName, cfg.BusinessSlug, time.Now().UTC())
		if err != nil {
			return errors.New("upsert business: " + err.Error())
		}

		_, err = executor.Exec(txCtx, `
			INSERT INTO business_memberships (
				business_id, principal_id, role, permissions, status, created_at, updated_at
			)
			VALUES ($1::uuid, $2::uuid, 'owner', '["*"]'::jsonb, 'active', $3, $3)
			ON CONFLICT (business_id, principal_id) DO UPDATE
			SET role = 'owner',
			    permissions = '["*"]'::jsonb,
			    status = 'active',
			    updated_at = EXCLUDED.updated_at
		`, cfg.BusinessID, principalID, time.Now().UTC())
		if err != nil {
			return errors.New("upsert business owner membership: " + err.Error())
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("==========================================================")
	log.Println("Platform Admin + Business Owner Seeder completed.")
	log.Printf("Principal: %s", principalID)
	log.Printf("Email: %s", cfg.Email)
	log.Printf("Business: %s (%s)", cfg.BusinessName, cfg.BusinessID)
	log.Println("Scopes: platform_super_admin + business owner")
	log.Println("==========================================================")
}

func loadConfig() (seedConfig, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return seedConfig{}, errors.New("DATABASE_URL is required")
	}

	email := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_OWNER_EMAIL"))
	if email == "" {
		return seedConfig{}, errors.New("PLATFORM_ADMIN_OWNER_EMAIL is required")
	}

	password := os.Getenv("PLATFORM_ADMIN_OWNER_PASSWORD")
	if len(password) < 12 {
		return seedConfig{}, errors.New("PLATFORM_ADMIN_OWNER_PASSWORD must contain at least 12 characters")
	}

	if strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_OWNER_CONFIRM")) != seedConfirmation {
		return seedConfig{}, errors.New("PLATFORM_ADMIN_OWNER_CONFIRM must equal " + seedConfirmation)
	}

	displayName := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_OWNER_DISPLAY_NAME"))
	if displayName == "" {
		displayName = defaultDisplayName
	}

	businessID := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_OWNER_BUSINESS_ID"))
	if businessID == "" {
		businessID = defaultBusinessID
	}

	businessName := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_OWNER_BUSINESS_NAME"))
	if businessName == "" {
		businessName = defaultBusinessName
	}

	businessSlug := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_OWNER_BUSINESS_SLUG"))
	if businessSlug == "" {
		businessSlug = defaultBusinessSlug
	}

	return seedConfig{
		DatabaseURL: databaseURL,
		Email: email,
		Password: password,
		DisplayName: displayName,
		BusinessID: businessID,
		BusinessName: businessName,
		BusinessSlug: businessSlug,
	}, nil
}
