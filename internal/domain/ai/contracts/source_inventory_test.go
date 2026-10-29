package contracts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This audit verifies CREATE/DROP TABLE migration declarations, not the
// final field-level live schema. ALTER TABLE and runtime schema queries
// require separate source-by-source review before approving projections.
func TestMigrationTableInventoryFailClosed(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "migrations")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fileName := regexp.MustCompile(`^([0-9]{6})_.*\.up\.sql$`)
	create := regexp.MustCompile(`(?im)^\s*CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:public\.)?([a-z_][a-z_0-9]*)\s*\(`)
	drop := regexp.MustCompile(`(?im)^\s*DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:public\.)?([a-z_][a-z_0-9]*)\b`)
	created := map[string]int{}
	removed := map[string]int{}
	migrationIDs := map[int]bool{}
	for _, file := range files {
		m := fileName.FindStringSubmatch(file.Name())
		if m == nil {
			continue
		}
		id, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatal(err)
		}
		if migrationIDs[id] {
			t.Fatalf("duplicate migration number %d", id)
		}
		migrationIDs[id] = true
		bytes, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		// Leading -- comments cannot match either anchored statement regex.
		source := string(bytes)
		for _, found := range create.FindAllStringSubmatch(source, -1) {
			table := strings.ToLower(found[1])
			if earlier, exists := created[table]; exists {
				t.Fatalf("table %s created twice by %06d and %06d", table, earlier, id)
			}
			created[table] = id
		}
		for _, found := range drop.FindAllStringSubmatch(source, -1) {
			table := strings.ToLower(found[1])
			if earlier, exists := removed[table]; exists {
				t.Fatalf("table %s dropped twice in %06d and %06d", table, earlier, id)
			}
			removed[table] = id
		}
	}
	if len(migrationIDs) == 0 {
		t.Fatal("no migration files found")
	}
	for i := 1; i <= len(migrationIDs); i++ {
		if !migrationIDs[i] {
			t.Fatalf("migration %06d missing; this audit expects contiguous history", i)
		}
	}
	inventory := map[string]TableSourceRecord{}
	for _, entry := range SourceInventory() {
		if entry.Table == "" || entry.Domain == "" {
			t.Fatalf("empty table/domain classification for %+v", entry)
		}
		switch entry.Disposition {
		case SourceProjectionCandidate, SourceInternalOnly, SourceProhibited, SourceRetired:
		default:
			t.Fatalf("table %s has unknown disposition %s", entry.Table, entry.Disposition)
		}
		if _, found := inventory[entry.Table]; found {
			t.Fatalf("duplicate inventory entry %s", entry.Table)
		}
		inventory[entry.Table] = entry
	}
	for table, createdAt := range created {
		entry, ok := inventory[table]
		if !ok {
			t.Fatalf("migration created UNREVIEWED table %s in %06d; update source classification before merging", table, createdAt)
		}
		if entry.IntroducedMigration != createdAt {
			t.Fatalf("%s: source migration is %06d, registered %06d", table, createdAt, entry.IntroducedMigration)
		}
		removedAt := removed[table]
		if entry.RemovedMigration != removedAt {
			t.Fatalf("%s: removal migration is %06d, registered %06d", table, removedAt, entry.RemovedMigration)
		}
		if (removedAt != 0) != (entry.Disposition == SourceRetired) {
			t.Fatalf("%s: retired classification does not match migration history", table)
		}
	}
	for name := range inventory {
		if _, ok := created[name]; !ok {
			t.Fatal(fmt.Sprintf("table %s registered without CREATE TABLE evidence", name))
		}
	}
	for table, removedAt := range removed {
		if _, ok := created[table]; !ok {
			t.Fatalf("DROP TABLE for unknown %s in migration %06d", table, removedAt)
		}
	}
	if len(created) != len(inventory) {
		t.Fatalf("migration-created tables %d, inventoried tables %d", len(created), len(inventory))
	}
}

func TestSourceInventoryNeverGrantsAIFieldAccess(t *testing.T) {
	entries := SourceInventory()
	if len(entries) == 0 {
		t.Fatal("source audit is empty")
	}
	entries[0].Table = "injected"
	if _, found := LookupTableSource("injected"); found {
		t.Fatal("source records must not expose mutable global inventory")
	}
	if _, ok := LookupTableSource("made_up_table"); ok {
		t.Fatal("unknown sources must fail closed")
	}
	for _, name := range []string{"chatwoot_workspace_bindings", "chatwoot_contact_links", "chatwoot_mirror_jobs"} {
		entry, ok := LookupTableSource(name)
		if !ok || entry.Disposition != SourceRetired || entry.RemovedMigration != 53 {
			t.Fatalf("legacy %s must be retired by 000053", name)
		}
	}
	if r, err := NewRegistry(); err != nil || r == nil {
		t.Fatal("inventory does not populate or authorize runtime projection registry")
	}
}
