//go:build integration

// Package postgres_test — external test package. Uses package_test convention
// so this file compiles INDEPENDENTLY of the pre-existing broken file
// ai_audit_repository_integration_test.go (which references an undefined
// FakeContractRuntimeForLegacyTests). This file does NOT modify that
// broken file — it sidesteps the compilation issue via a separate
// Go test package.
package postgres_test

import (
        "context"
        "os"
        "testing"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

// TestSubscriptionRepositoryCreateAgainstPostgres proves the fix for the
// SubscriptionRepository.Create() bug where RETURNING subscriptionSelectColumns
// referenced `s.*` and `p.*` aliases that don't exist in an INSERT context.
//
// Per spec §4 + §5: this test uses REAL PostgreSQL (not a fake repository)
// + goes through the actual SQL path.
//
// Per spec §4:
//   A) Create subscription using business_id 00000000-...002 + plan_id 00000000-...a001
//   B) The plan exists + is ACTIVE (seeded by migration 000061)
//   C) Create() returns: Status=PENDING, PlanCode=basic, PlanVersion=1,
//      BusinessID=correct, PlanID=correct
//   D) The row actually exists in PostgreSQL after Create()
func TestSubscriptionRepositoryCreateAgainstPostgres(t *testing.T) {
        dsn := os.Getenv("POSTGRES_TEST_DSN")
        if dsn == "" {
                t.Skip("POSTGRES_TEST_DSN is not set")
        }
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        // Run migrations (creates businesses, plans, subscriptions tables + seeds
        // the 3 baseline plans including plan_id a001 = basic).
        if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
                t.Fatalf("run migrations: %v", err)
        }

        adapter, err := postgres.Open(ctx, dsn, postgres.DefaultPoolConfig())
        if err != nil {
                t.Fatalf("open adapter: %v", err)
        }
        defer adapter.Close()

        // Seed the business (00000000-...002) — required because subscriptions
        // has FK to businesses. The plan (00000000-...a001 = basic) is already
        // seeded by migration 000061.
        const businessID = "00000000-0000-0000-0000-000000000002"
        const planID = "00000000-0000-0000-0000-00000000a001"
        const subscriptionID = "00000000-0000-0000-0000-0000000000aa"

        pool := adapter.Pool()
        // Cleanup any prior test rows.
        _, _ = pool.Exec(ctx, `DELETE FROM subscription_ai_usage WHERE business_id = $1::uuid`, businessID)
        _, _ = pool.Exec(ctx, `DELETE FROM subscriptions WHERE business_id = $1::uuid`, businessID)
        _, _ = pool.Exec(ctx, `DELETE FROM business_policies WHERE business_id = $1::uuid`, businessID)
        _, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, businessID)

        // Seed the business.
        _, err = pool.Exec(ctx, `
                INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, resource_version, created_at, updated_at)
                VALUES ($1::uuid, 'Test Business', 'test-business-002', 'active', 'retail', 'Asia/Aden', 'YER', 'ar', 1, now(), now())
                ON CONFLICT (id) DO NOTHING
        `, businessID)
        if err != nil {
                t.Fatalf("seed business: %v", err)
        }

        // Verify the plan exists + is ACTIVE (per spec §4 B).
        var planStatus string
        err = pool.QueryRow(ctx, `SELECT status FROM plans WHERE id = $1::uuid`, planID).Scan(&planStatus)
        if err != nil {
                t.Fatalf("plan lookup: %v — the basic plan (a001) should be seeded by migration 000061", err)
        }
        if planStatus != "ACTIVE" {
                t.Fatalf("plan status = %s, want ACTIVE", planStatus)
        }
        t.Logf("plan OK: id=%s status=%s", planID, planStatus)

        // Create the subscription via the repository (this is the code path
        // being tested — it goes through real SQL).
        repo := postgres.NewSubscriptionRepository(adapter)
        now := time.Now().UTC()
        periodStart := now.Truncate(time.Hour)
        periodEnd := periodStart.Add(30 * 24 * time.Hour) // 1 month

        record, err := repo.Create(ctx, ports.SubscriptionCreate{
                ID:                     subscriptionID,
                BusinessID:             businessID,
                PlanID:                 planID,
                PeriodStart:            periodStart,
                PeriodEnd:              periodEnd,
                AIReplyLimit:           500,
                AICatalogLimit:         200,
                ChannelLimit:           1,
                InternalAICostBudgetYER: 1000,
                Now:                    now,
        })
        if err != nil {
                t.Fatalf("repo.Create: %v — this is the bug being fixed (RETURNING with s.*/p.* aliases fails in INSERT context)", err)
        }

        // Per spec §4 C: verify the returned record has correct fields.
        if record.ID != subscriptionID {
                t.Errorf("record.ID = %s, want %s", record.ID, subscriptionID)
        }
        if record.BusinessID != businessID {
                t.Errorf("record.BusinessID = %s, want %s", record.BusinessID, businessID)
        }
        if record.PlanID != planID {
                t.Errorf("record.PlanID = %s, want %s", record.PlanID, planID)
        }
        if record.Status != "PENDING" {
                t.Errorf("record.Status = %s, want PENDING", record.Status)
        }
        if record.PlanCode != "basic" {
                t.Errorf("record.PlanCode = %s, want basic — PlanCode comes from the plans table JOIN, which was broken in the old RETURNING clause", record.PlanCode)
        }
        if record.PlanVersion != 1 {
                t.Errorf("record.PlanVersion = %d, want 1 — PlanVersion comes from the plans table JOIN", record.PlanVersion)
        }
        if record.AIReplyLimit != 500 {
                t.Errorf("record.AIReplyLimit = %d, want 500", record.AIReplyLimit)
        }
        if record.ChannelLimit != 1 {
                t.Errorf("record.ChannelLimit = %d, want 1", record.ChannelLimit)
        }
        t.Logf("Create() returned: id=%s business=%s plan=%s/%s/v%d status=%s ai_reply_limit=%d",
                record.ID, record.BusinessID, record.PlanID, record.PlanCode, record.PlanVersion, record.Status, record.AIReplyLimit)

        // Per spec §4 D: verify the row actually exists in PostgreSQL (direct
        // SELECT, not via the repository — proves the INSERT persisted).
        var dbStatus, dbPlanCode string
        var dbPlanVersion int
        err = pool.QueryRow(ctx, `
                SELECT s.status, p.code AS plan_code, p.version AS plan_version
                FROM subscriptions s
                LEFT JOIN plans p ON p.id = s.plan_id
                WHERE s.id = $1::uuid
        `, subscriptionID).Scan(&dbStatus, &dbPlanCode, &dbPlanVersion)
        if err != nil {
                t.Fatalf("direct DB verification: %v — the INSERT did not persist the row", err)
        }
        if dbStatus != "PENDING" {
                t.Errorf("DB status = %s, want PENDING", dbStatus)
        }
        if dbPlanCode != "basic" {
                t.Errorf("DB plan_code = %s, want basic", dbPlanCode)
        }
        if dbPlanVersion != 1 {
                t.Errorf("DB plan_version = %d, want 1", dbPlanVersion)
        }
        t.Logf("DB verification OK: row exists in PostgreSQL — status=%s plan_code=%s plan_version=%d", dbStatus, dbPlanCode, dbPlanVersion)
}
