package main

import (
	"context"
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

func main() {
	databaseURL := requiredEnv("DATABASE_URL")
	email := requiredEnv("PLATFORM_SEED_EMAIL")
	password := requiredEnv("PLATFORM_SEED_PASSWORD")
	if strings.TrimSpace(os.Getenv("PLATFORM_SEED_CONFIRM")) != seedConfirmation {
		log.Fatalf("PLATFORM_SEED_CONFIRM must equal %s", seedConfirmation)
	}
	if len(password) < 12 {
		log.Fatal("PLATFORM_SEED_PASSWORD must contain at least 12 characters")
	}

	displayName := strings.TrimSpace(os.Getenv("PLATFORM_SEED_DISPLAY_NAME"))
	if displayName == "" {
		displayName = "مدير المنصة"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adapter, err := postgres.Open(ctx, databaseURL, postgres.PoolConfig{
		MaxConns:       2,
		MinConns:       0,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer adapter.Close()

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	repository := postgres.NewPlatformAccessRepository(adapter)
	principalID, err := repository.EnsurePlatformSuperAdmin(ctx, ports.PlatformSuperAdminBootstrap{
		Principal: commands.PrincipalID(uuid.NewString()),
		Email:     email,
		Name:      displayName,
		Hash:      string(passwordHash),
		Now:       time.Now().UTC(),
	})
	if err != nil {
		log.Fatalf("seed platform super admin: %v", err)
	}

	log.Println("==========================================================")
	log.Println("Platform Super Admin Seeder completed.")
	log.Printf("Principal ID: %s", principalID)
	log.Printf("Email: %s", email)
	log.Printf("Display name: %s", displayName)
	log.Println("Scope: PLATFORM ONLY")
	log.Println("Business membership: NONE")
	log.Println("Business owner role: NONE")
	log.Println("==========================================================")
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}
