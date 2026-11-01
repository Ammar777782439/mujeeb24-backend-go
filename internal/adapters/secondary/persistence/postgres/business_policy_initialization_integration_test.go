package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestPlatformBusinessCreationInitializesPolicyAtomically(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	id := uuid.NewString()
	rollbackID := uuid.NewString()
	defer func() {
		if _, err := adapter.Pool().Exec(context.Background(), `DELETE FROM business_policies WHERE business_id=ANY($1::uuid[])`, []string{id, rollbackID}); err != nil {
			t.Error(err)
		}
		if _, err := adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id=ANY($1::uuid[])`, []string{id, rollbackID}); err != nil {
			t.Error(err)
		}
	}()
	create := ports.PlatformBusinessCreate{ID: id, Name: "Policy Initialization", Slug: "policy-" + id, VerticalType: "retail", Timezone: "Asia/Aden", DefaultCurrency: "YER", Locale: "ar", Now: time.Now().UTC()}
	repository := NewPlatformBusinessRepository(adapter)
	if _, err := repository.Create(ctx, create); err != nil {
		t.Fatal(err)
	}
	policy, err := NewBusinessRepository(adapter).GetRuntimePolicy(ctx, id)
	if err != nil || policy.ResourceVersion != 1 || policy.AIMode != "assist" || policy.DefaultHumanReview || policy.AllowAutoReply || policy.AllowAutoLeadCreation || policy.AllowAutoTransactionDraft || policy.AllowAutoConfirmation || policy.SalesWorkflowEnabled || policy.AllowSalesInteractions {
		t.Fatalf("missing or incorrect initial policy: %#v err=%v", policy, err)
	}
	// Force the policy insert to fail using transaction-local shadow tables. The
	// business insert must roll back as part of the SAME statement, even without
	// the outer owner-provisioning transaction.
	tx, err := adapter.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE businesses (LIKE public.businesses INCLUDING ALL) ON COMMIT DROP; CREATE TEMP TABLE business_policies (LIKE public.business_policies INCLUDING ALL) ON COMMIT DROP; ALTER TABLE business_policies ADD CONSTRAINT test_reject_policy CHECK (false); SAVEPOINT failed_create`); err != nil {
		t.Fatal(err)
	}
	create.ID = rollbackID
	create.Slug = "policy-fail-" + rollbackID
	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	if _, err := repository.Create(txCtx, create); err == nil {
		t.Fatal("failed policy insert reported successful business creation")
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT failed_create`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM businesses WHERE id=$1::uuid`, rollbackID).Scan(&count); err != nil || count != 0 {
		t.Fatal("policy failure left a partial business")
	}
}
