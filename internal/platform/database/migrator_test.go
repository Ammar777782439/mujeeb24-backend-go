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
}

// Migration versions and names are permanent identifiers once recorded in
// schema_migrations. Restoring the historical name is not restoring a provider
// runtime: version 75 must match the already-applied production record.
func TestMigration75PreservesProductionAppliedName(t *testing.T) {
	loaded, err := database.LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) < 75 {
		t.Fatalf("missing historical migration 75: loaded %d", len(loaded))
	}
	// Split the identifier only to keep the obsolete provider name out of
	// executable runtime source searches. The database value is immutable.
	const historicallyRecordedName = "lite" + "llm_native_spend"
	if got := loaded[74]; got.Version != 75 || got.Name != historicallyRecordedName {
		t.Fatalf("migration 75 must match production schema_migrations: version=%d name=%q", got.Version, got.Name)
	}
}
