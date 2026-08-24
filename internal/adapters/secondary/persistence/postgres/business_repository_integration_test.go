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

func TestCoreRepositoriesRespectBusinessScopeAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	pool := adapter.Pool()
	const businessA = "00000000-0000-0000-0000-000000000011"
	const businessB = "00000000-0000-0000-0000-000000000012"
	const customerA = "00000000-0000-0000-0000-000000000021"
	const customerB = "00000000-0000-0000-0000-000000000022"
	const conversationA = "00000000-0000-0000-0000-000000000031"
	const connectionA = "00000000-0000-0000-0000-000000000041"
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	}
	_, err = pool.Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()), ($4::uuid, $5, $6, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, businessA, "Tenant A", "tenant-a", businessB, "Tenant B", "tenant-b")
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	_, err = pool.Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{"name":"A"}', '[]', 'ar-YE', 'active', now(), now()), ($3::uuid, $4::uuid, '{"name":"B"}', '[]', 'ar-YE', 'active', now(), now())`, customerA, businessA, customerB, businessB)
	if err != nil {
		t.Fatalf("insert customers: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM customers WHERE id IN ($1::uuid, $2::uuid)`, customerA, customerB)
	_, err = pool.Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', now(), now(), now())`, conversationA, businessA, customerA)
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM conversations WHERE id = $1::uuid`, conversationA)
	_, err = pool.Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'facebook', 'connection-a', 'active', 'secret-ref-a', now(), now())`, connectionA, businessA)
	if err != nil {
		t.Fatalf("insert channel connection: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM channel_connections WHERE id = $1::uuid`, connectionA)

	customerRepo := NewCustomerRepository(adapter)
	customer, err := customerRepo.GetByID(ctx, businessA, customerA)
	if err != nil || customer.BusinessID != businessA {
		t.Fatalf("customer read: %#v err=%v", customer, err)
	}
	if _, err := customerRepo.GetByID(ctx, businessB, customerA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("customer crossed tenant boundary: %v", err)
	}
	conversationRepo := NewConversationRepository(adapter)
	conversation, err := conversationRepo.GetByID(ctx, businessA, conversationA)
	if err != nil || conversation.CustomerID != customerA {
		t.Fatalf("conversation read: %#v err=%v", conversation, err)
	}
	if _, err := conversationRepo.GetByID(ctx, businessB, conversationA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("conversation crossed tenant boundary: %v", err)
	}
	connectionRepo := NewChannelConnectionRepository(adapter)
	connection, err := connectionRepo.GetByID(ctx, businessA, connectionA)
	if err != nil || connection.ProviderReference != "socialapi" {
		t.Fatalf("connection read: %#v err=%v", connection, err)
	}
	if _, err := connectionRepo.GetByID(ctx, businessB, connectionA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("connection crossed tenant boundary: %v", err)
	}
}
