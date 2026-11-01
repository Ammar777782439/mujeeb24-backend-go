//go:build integration

package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestBusinessPolicyBackfillPreservesExistingSettings(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(context.Background())
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	// Shadow only these tables in this session. Replaying the repair cannot
	// change fixtures owned by other packages or any public business policies.
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE businesses (LIKE public.businesses INCLUDING ALL) ON COMMIT DROP; CREATE TEMP TABLE business_policies (LIKE public.business_policies INCLUDING ALL) ON COMMIT DROP`); err != nil {
		t.Fatal(err)
	}
	missingA, missingB, existing := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) SELECT id::uuid,'Policy repair fixture',id,status,'retail','Asia/Aden','YER','ar',now(),now() FROM (VALUES ($1::text,'active'),($2::text,'pending_setup'),($3::text,'active')) fixtures(id,status)`, missingA, missingB, existing); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO business_policies (business_id,ai_mode,default_human_review,allow_auto_reply,allow_auto_lead_creation,allow_auto_transaction_draft,allow_auto_confirmation,sales_workflow_enabled,allow_sales_interactions,business_hours_reference,escalation_policy_reference,resource_version,created_at,updated_at) VALUES ($1::uuid,'restricted_auto',true,true,true,true,true,true,true,'custom hours','custom escalation',7,'2026-01-01T00:00:00Z','2026-01-02T00:00:00Z')`, existing); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := tx.QueryRow(ctx, `SELECT row_to_json(p)::text FROM business_policies p WHERE business_id=$1::uuid`, existing).Scan(&before); err != nil {
		t.Fatal(err)
	}
	migrations, err := database.LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var repair string
	for _, migration := range migrations {
		if migration.Version == 85 {
			repair = migration.SQL
		}
	}
	if repair == "" {
		t.Fatal("missing policy repair migration")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := tx.Exec(ctx, repair); err != nil {
			t.Fatal(err)
		}
		var after string
		var count int
		if err := tx.QueryRow(ctx, `SELECT row_to_json(p)::text FROM business_policies p WHERE business_id=$1::uuid`, existing).Scan(&after); err != nil || after != before {
			t.Fatal("repair changed existing merchant settings, references, version or timestamps")
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM business_policies WHERE business_id=ANY($1::uuid[]) AND ai_mode='assist' AND NOT default_human_review AND NOT allow_auto_reply AND NOT allow_auto_lead_creation AND NOT allow_auto_transaction_draft AND NOT allow_auto_confirmation AND NOT sales_workflow_enabled AND NOT allow_sales_interactions AND resource_version=1`, []string{missingA, missingB}).Scan(&count); err != nil || count != 2 {
			t.Fatal("missing policies were not initialized without automation")
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM business_policies`).Scan(&count); err != nil || count != 3 {
			t.Fatal("repair created duplicate policies")
		}
	}
}
