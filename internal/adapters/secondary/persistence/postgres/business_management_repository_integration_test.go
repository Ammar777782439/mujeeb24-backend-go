package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestBusinessManagementRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	const id = "00000000-0000-0000-0000-000000000120"
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM business_policies WHERE business_id = $1::uuid`, id)
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Management Shop', 'management-shop', 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, id); err != nil {
		t.Fatalf("insert business: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO business_policies (business_id, ai_mode, default_human_review, allow_auto_reply, allow_auto_lead_creation, allow_auto_transaction_draft, allow_auto_confirmation, created_at, updated_at) VALUES ($1::uuid, 'assist', false, false, false, false, false, now(), now())`, id); err != nil {
		t.Fatalf("insert policy: %v", err)
	}
	defer adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id = $1::uuid`, id)

	repo := NewBusinessRepository(adapter)
	record, err := repo.GetByID(ctx, id)
	if err != nil || record.ResourceVersion != 1 {
		t.Fatalf("initial business version: record=%#v err=%v", record, err)
	}
	name := "Management Shop Updated"
	updated, err := repo.UpdateProfile(ctx, ports.BusinessProfileUpdate{BusinessID: id, ExpectedVersion: record.ResourceVersion, Name: &name})
	if err != nil || updated.Name != name || updated.ResourceVersion != 2 {
		t.Fatalf("update profile: record=%#v err=%v", updated, err)
	}
	if _, err := repo.UpdateProfile(ctx, ports.BusinessProfileUpdate{BusinessID: id, ExpectedVersion: 1, Name: &name}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("expected stale profile update, got %v", err)
	}
	policy, err := repo.GetRuntimePolicy(ctx, id)
	if err != nil || policy.ResourceVersion != 1 || policy.AIMode != "assist" {
		t.Fatalf("get policy: record=%#v err=%v", policy, err)
	}
	allow := true
	updatedPolicy, err := repo.UpdateRuntimePolicy(ctx, ports.BusinessRuntimePolicyUpdate{BusinessID: id, ExpectedVersion: policy.ResourceVersion, AllowAutoReply: &allow})
	if err != nil || !updatedPolicy.AllowAutoReply || updatedPolicy.ResourceVersion != 2 {
		t.Fatalf("update policy: record=%#v err=%v", updatedPolicy, err)
	}
	rollbackErr := errors.New("force business management rollback")
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		rollbackName := "must not commit"
		if _, updateErr := repo.UpdateProfile(txCtx, ports.BusinessProfileUpdate{BusinessID: id, ExpectedVersion: updated.ResourceVersion, Name: &rollbackName}); updateErr != nil {
			return updateErr
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	rolledBack, err := repo.GetByID(ctx, id)
	if err != nil || rolledBack.Name != name || rolledBack.ResourceVersion != updated.ResourceVersion {
		t.Fatalf("rollback profile update: record=%#v err=%v", rolledBack, err)
	}
}
