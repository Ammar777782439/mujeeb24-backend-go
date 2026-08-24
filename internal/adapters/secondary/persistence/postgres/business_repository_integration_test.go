//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestBusinessRepositoryAgainstPostgres(t *testing.T) {
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
	repo := NewBusinessRepository(adapter)
	const id = "00000000-0000-0000-0000-000000000009"
	_, err = adapter.Pool().Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	if err != nil {
		t.Fatalf("cleanup before test: %v", err)
	}
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, id, "Integration Shop", "integration-shop")
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	defer adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id = $1::uuid`, id)

	record, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get business: %v", err)
	}
	if record.ID != id || record.Name != "Integration Shop" || record.DefaultCurrency != "YER" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if _, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000010"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected not-found repository error, got %v", err)
	}

	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `UPDATE businesses SET name = $1 WHERE id = $2::uuid`, "Committed Shop", id)
		return err
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	record, err = repo.GetByID(ctx, id)
	if err != nil || record.Name != "Committed Shop" {
		t.Fatalf("commit not visible: record=%#v err=%v", record, err)
	}

	rollbackErr := errors.New("force rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err = executor.Exec(txCtx, `UPDATE businesses SET name = $1 WHERE id = $2::uuid`, "Rolled Back Shop", id); err != nil {
			return err
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	record, err = repo.GetByID(ctx, id)
	if err != nil || record.Name != "Committed Shop" {
		t.Fatalf("rollback not preserved: record=%#v err=%v", record, err)
	}
}
