package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const seedConfirmation = "CREATE_PLATFORM_SUPER_ADMIN"

type seedConfig struct {
	databaseURL string
	email       string
	password    string
	displayName string
}

func main() {
	cfg, err := loadSeedConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adapter, err := postgres.Open(ctx, cfg.databaseURL, postgres.PoolConfig{
		MaxConns:       2,
		MinConns:       0,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer adapter.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(cfg.password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	repository := postgres.NewPlatformAccessRepository(adapter)
	principalID, err := repository.EnsurePlatformSuperAdmin(ctx, ports.PlatformSuperAdminBootstrap{
		Principal: commands.PrincipalID(uuid.NewString()),
		Email:     cfg.email,
		Name:      cfg.displayName,
		Hash:      string(passwordHash),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		log.Fatalf("seed platform super admin: %v", err)
	}

	log.Println("==========================================================")
	log.Println("Platform Super Admin Seeder completed.")
	log.Printf("Principal ID: %s", principalID)
	log.Printf("Email: %s", cfg.email)
	log.Printf("Display name: %s", cfg.displayName)
	log.Println("Scope: PLATFORM ONLY")
	log.Println("Business membership: NONE")
	log.Println("Business owner role: NONE")
	log.Println("==========================================================")
}

func loadSeedConfig() (seedConfig, error) {
	databaseURL, err := requiredEnv("DATABASE_URL")
	if err != nil {
		return seedConfig{}, err
	}
	email, err := requiredEnv("PLATFORM_SEED_EMAIL")
	if err != nil {
		return seedConfig{}, err
	}

	password := os.Getenv("PLATFORM_SEED_PASSWORD")
	if len(password) < 12 {
		return seedConfig{}, errors.New("PLATFORM_SEED_PASSWORD must contain at least 12 characters")
	}

	if strings.TrimSpace(os.Getenv("PLATFORM_SEED_CONFIRM")) != seedConfirmation {
		return seedConfig{}, errors.New("PLATFORM_SEED_CONFIRM must equal " + seedConfirmation)
	}

	displayName := strings.TrimSpace(os.Getenv("PLATFORM_SEED_DISPLAY_NAME"))
	if displayName == "" {
		displayName = "مدير المنصة"
	}

	return seedConfig{
		databaseURL: databaseURL,
		email:       email,
		password:    password,
		displayName: displayName,
	}, nil
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", errors.New(key + " is required")
	}
	return value, nil
}
