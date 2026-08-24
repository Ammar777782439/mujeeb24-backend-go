package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	applied, err := database.RunMigrations(ctx, databaseURL, time.Now().UTC())
	if err != nil {
		log.Fatalf("run migrations: %v", err)
	}
	log.Printf("migrations complete: applied=%d", applied)
}
