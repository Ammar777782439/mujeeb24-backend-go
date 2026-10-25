//go:build integration

package database_test

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "reflect"
    "sort"
    "strings"
    "testing"
    "time"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
    "github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
    "github.com/jackc/pgx/v5"
)

// The schema snapshot is derived from a real PostgreSQL 16 database after
// RunMigrations has applied the *entire* embedded migration sequence.
//
// The snapshot is schema METADATA. It does not approve reading or sending
// any merchant rows or fields to a model.
type merchantAssistColumn struct {
    Table       string `json:"table"`
    Name        string `json:"name"`
    Type        string `json:"type"`
    NotNull     bool   `json:"not_null"`
    Default     string `json:"default,omitempty"`
    Identity    string `json:"identity,omitempty"`
    Generated   string `json:"generated,omitempty"`
}

type merchantAssistConstraint struct {
    Table      string `json:"table"`
    Name       string `json:"name"`
    Kind       string `json:"kind"`
    Definition string `json:"definition"`
}

type merchantAssistSchemaSnapshot struct {
    MigrationCount int                        `json:"migration_count"`
    Columns        []merchantAssistColumn     `json:"columns"`
    Constraints    []merchantAssistConstraint `json:"constraints"`
}

func TestMerchantAssistSchemaAfterAllMigrations(t *testing.T) {
    dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
    if dsn == "" {
        t.Skip("requires an isolated PostgreSQL test DSN")
    }
    ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
    defer cancel()

    migrations, err := database.LoadMigrations()
    if err != nil { t.Fatal(err) }
    if len(migrations) != 81 {
        t.Fatalf("unexpected migration sequence length %d (review migration inventory first)", len(migrations))
    }
    applied, err := database.RunMigrations(ctx, dsn, time.Now().UTC())
    if err != nil {
        t.Fatalf("cannot audit schema without applying migrations: %v", err)
    }
    t.Logf("verified all %d migrations; newly applied %d", len(migrations), applied)
    db, err := pgx.Connect(ctx, dsn)
    if err != nil { t.Fatal(err) }
    defer db.Close(context.Background())

    var count, maxVersion int
    if err := db.QueryRow(ctx, "SELECT count(*), coalesce(max(version),0) FROM schema_migrations").Scan(&count, &maxVersion); err != nil {
        t.Fatal(err)
    }
    if count != len(migrations) || maxVersion != migrations[len(migrations)-1].Version {
        t.Fatalf("migration state incomplete: count %d, last %d", count, maxVersion)
    }

    snapshot := merchantAssistSchemaSnapshot{MigrationCount:count, Columns: []merchantAssistColumn{}, Constraints: []merchantAssistConstraint{}}
    columnRows, err := db.Query(ctx, `
SELECT c.relname::text, a.attname::text,
       pg_catalog.format_type(a.atttypid, a.atttypmod)::text,
       a.attnotnull,
       coalesce(pg_get_expr(d.adbin, d.adrelid), '')::text,
       a.attidentity::text,
       a.attgenerated::text
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid
LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
WHERE n.nspname = 'public' AND c.relkind IN ('r','p')
  AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY c.relname, a.attnum`)
    if err != nil { t.Fatal(err) }
    for columnRows.Next() {
        var c merchantAssistColumn
        if err := columnRows.Scan(&c.Table,&c.Name,&c.Type,&c.NotNull,&c.Default,&c.Identity,&c.Generated); err != nil {
            columnRows.Close()
            t.Fatal(err)
        }
        snapshot.Columns = append(snapshot.Columns,c)
    }
    if err := columnRows.Err(); err != nil { t.Fatal(err) }
    columnRows.Close()

    constraintRows, err := db.Query(ctx, `
SELECT c.relname::text, k.conname::text, k.contype::text,
       pg_catalog.pg_get_constraintdef(k.oid,true)::text
FROM pg_catalog.pg_constraint k
JOIN pg_catalog.pg_class c ON c.oid=k.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relkind IN ('r','p')
ORDER BY c.relname, k.conname`)
    if err != nil { t.Fatal(err) }
    for constraintRows.Next() {
        var c merchantAssistConstraint
        if err := constraintRows.Scan(&c.Table,&c.Name,&c.Kind,&c.Definition); err != nil {
            constraintRows.Close()
            t.Fatal(err)
        }
        snapshot.Constraints = append(snapshot.Constraints,c)
    }
    if err := constraintRows.Err(); err != nil { t.Fatal(err) }
    constraintRows.Close()

    active := make(map[string]contracts.TableSourceRecord)
    for _, src := range contracts.SourceInventory() {
        if src.RemovedMigration == 0 {
            active[src.Table] = src
        }
    }
    observed := map[string]int{}
    for _, col := range snapshot.Columns { observed[col.Table]++ }
    for table := range active {
        if observed[table] == 0 { t.Errorf("missing live table or columns %s",table) }
    }
    for table := range observed {
        if table == "schema_migrations" { continue }
        if _, known := active[table]; !known {
            t.Errorf("live table is not reviewed in AI source inventory: %s",table)
        }
    }
    if len(observed) != len(active)+1 {
        t.Errorf("table census mismatch: actual %d, expected %d including schema_migrations",len(observed),len(active)+1)
    }

    // A snapshot is the FINAL migrated schema, not a parse of CREATE TABLE
    // statements; it also captures ALTER TABLE, DROP COLUMN and constraints.
    for _, col := range snapshot.Columns {
        if col.Table=="attribute_definitions" && col.Name=="is_searchable" {
            t.Error("000068 removed is_searchable; it must not reappear")
        }
        if col.Type=="" { t.Errorf("column %s.%s has an empty type",col.Table,col.Name) }
    }
    for _, retired := range []string{"chatwoot_workspace_bindings","chatwoot_contact_links","chatwoot_mirror_jobs"} {
        if observed[retired]>0 { t.Errorf("retired table unexpectedly exists: %s", retired) }
    }

    data,err:=json.MarshalIndent(snapshot,"","  ")
    if err!=nil {t.Fatal(err)}
    if os.Getenv("MERCHANT_ASSIST_SCHEMA_RECORD")=="1" {
        // Printed once for an explicit review/commit; NEVER a production
        // startup side effect or a runtime schema download to the LLM.
        fmt.Printf("ASSIST_SCHEMA_SNAPSHOT_START\n%s\nASSIST_SCHEMA_SNAPSHOT_END\n", data)
        return
    }
    expected,err:=os.ReadFile("testdata/merchant_assist_schema_pg16.json")
    if err!=nil {
        t.Fatalf("missing reviewed PostgreSQL schema baseline: %v",err)
    }
    var baseline merchantAssistSchemaSnapshot
    if err:=json.Unmarshal(expected,&baseline);err!=nil {t.Fatalf("invalid reviewed schema baseline: %v",err)}
    if !reflect.DeepEqual(baseline,snapshot) {
        // Do not log sensitive DB rows: metadata diffs only.
        t.Fatalf("migrated PostgreSQL column/type/default/constraint snapshot drift (baseline: %d cols, %d constraints; live: %d cols, %d constraints); update only after source review",len(baseline.Columns),len(baseline.Constraints),len(snapshot.Columns),len(snapshot.Constraints))
    }
    t.Logf("schema verified: %d active tables + migration ledger, %d columns, %d constraints; candidate projections remain denied",len(active),len(snapshot.Columns),len(snapshot.Constraints))
}
