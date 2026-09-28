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

const bootstrapConfirmation = "CREATE_PLATFORM_SUPER_ADMIN"

func main() {
	databaseURL := requiredEnv("DATABASE_URL")
	email := requiredEnv("PLATFORM_BOOTSTRAP_EMAIL")
	password := requiredEnv("PLATFORM_BOOTSTRAP_PASSWORD")
	if requiredEnv("PLATFORM_BOOTSTRAP_CONFIRM") != bootstrapConfirmation {
		log.Fatalf("PLATFORM_BOOTSTRAP_CONFIRM must equal %s", bootstrapConfirmation)
	}
	if len(password) < 12 {
		log.Fatal("PLATFORM_BOOTSTRAP_PASSWORD must contain at least 12 characters")
	}
	name := strings.TrimSpace(os.Getenv("PLATFORM_BOOTSTRAP_DISPLAY_NAME"))
	if name == "" {
		name = "Ammar Ragha"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adapter, err := postgres.Open(ctx, databaseURL, postgres.PoolConfig{MaxConns: 2, MinConns: 0, ConnectTimeout: 10 * time.Second})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer adapter.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}
	repository := postgres.NewPlatformAccessRepository(adapter)
	principalID, err := repository.EnsurePlatformSuperAdmin(ctx, ports.PlatformSuperAdminBootstrap{Principal: commands.PrincipalID(uuid.NewString()), Email: email, Name: name, Hash: string(hash), Now: time.Now().UTC()})
	if err != nil {
		log.Fatalf("bootstrap platform super admin: %v", err)
	}
	log.Printf("platform super admin bootstrap complete: principal_id=%s", principalID)
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}
