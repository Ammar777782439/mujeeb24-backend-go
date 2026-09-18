package database_test

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestLoadMigrations_SequenceIntegrity(t *testing.T) {
	loaded, err := database.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() failed: %v", err)
	}

	if len(loaded) < 54 {
		t.Fatalf("expected at least 54 migrations, got %d", len(loaded))
	}

	for i, m := range loaded {
		expectedVersion := i + 1
		if m.Version != expectedVersion {
			t.Errorf("migration %d has version %d, expected %d", i, m.Version, expectedVersion)
		}
		if m.Name == "" {
			t.Errorf("migration %d has empty name", m.Version)
		}
		if m.SQL == "" {
			t.Errorf("migration %d (%s) has empty SQL", m.Version, m.Name)
		}
	}

	// Verify last migration is 000054_merchant_ai_sessions
	last := loaded[len(loaded)-1]
	if last.Version != 54 || last.Name != "merchant_ai_sessions" {
		t.Errorf("expected migration 54 to be merchant_ai_sessions, got version=%d name=%s", last.Version, last.Name)
	}
}
