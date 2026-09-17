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

	if len(loaded) < 53 {
		t.Fatalf("expected at least 53 migrations, got %d", len(loaded))
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

	// Verify last migration is 000053_remove_chatwoot_artifacts
	last := loaded[len(loaded)-1]
	if last.Version != 53 || last.Name != "remove_chatwoot_artifacts" {
		t.Errorf("expected migration 53 to be remove_chatwoot_artifacts, got version=%d name=%s", last.Version, last.Name)
	}
}
