package main

import (
	"context"
	"encoding/json"
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

func main() {
	databaseURL := requiredEnv("DATABASE_URL")
	email := requiredEnv("BOOTSTRAP_EMAIL")
	password := requiredEnv("BOOTSTRAP_PASSWORD")
	displayName := requiredEnv("BOOTSTRAP_DISPLAY_NAME")
	businessID := commands.BusinessID(requiredEnv("BOOTSTRAP_BUSINESS_ID"))
	role := requiredEnv("BOOTSTRAP_ROLE")
	permissionsRaw := requiredEnv("BOOTSTRAP_PERMISSIONS_JSON")
	if len(password) < 12 {
		log.Fatal("BOOTSTRAP_PASSWORD must contain at least 12 characters")
	}
	if _, err := uuid.Parse(string(businessID)); err != nil {
		log.Fatal("BOOTSTRAP_BUSINESS_ID must be a UUID")
	}
	permissions := []string(nil)
	if err := json.Unmarshal([]byte(permissionsRaw), &permissions); err != nil || len(permissions) == 0 {
		log.Fatal("BOOTSTRAP_PERMISSIONS_JSON must be a non-empty JSON string array")
	}
	for _, permission := range permissions {
		if strings.TrimSpace(permission) == "" {
			log.Fatal("BOOTSTRAP_PERMISSIONS_JSON must not contain blank permissions")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adapter, err := postgres.Open(ctx, databaseURL, postgres.PoolConfig{MaxConns: 2, MinConns: 0, ConnectTimeout: 10 * time.Second})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer adapter.Close()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash bootstrap password: %v", err)
	}
	repository := postgres.NewAuthenticationRepository(adapter)
	principal, err := repository.EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{Email: email, DisplayName: displayName, PasswordHash: string(passwordHash), Status: "active"}, businessID, role, permissions, time.Now().UTC())
	if err != nil {
		log.Fatalf("ensure principal and membership: %v", err)
	}
	log.Printf("bootstrap complete: principal_id=%s business_id=%s email=%s", principal.ID, businessID, principal.Email)
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}
