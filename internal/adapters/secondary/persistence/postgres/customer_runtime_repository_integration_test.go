package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestCustomerRuntimeRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer adapter.Close()
	businessA := uuid.NewString()
	businessB := uuid.NewString()
	firstID := uuid.NewString()
	targetID := uuid.NewString()
	otherID := uuid.NewString()
	pool := adapter.Pool()
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1::uuid`, id)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Customer Runtime A','customer-runtime-a','active','retail','Asia/Aden','YER','ar-YE',now(),now()),($2::uuid,'Customer Runtime B','customer-runtime-b','active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessA, businessB); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`, businessA, businessB)
	repo := NewCustomerRepository(adapter)
	first, err := repo.Create(ctx, ports.CustomerCreate{ID: firstID, BusinessID: businessA, Profile: []byte(`{"display_name":"First Customer"}`), ContactPoints: []byte(`[{"kind":"email","value_normalized":"first@example.test","verification_status":"unverified"}]`)})
	if err != nil || first.ResourceVersion != 1 {
		t.Fatalf("create first: record=%#v err=%v", first, err)
	}
	if _, err := repo.Create(ctx, ports.CustomerCreate{ID: targetID, BusinessID: businessA, Profile: []byte(`{"display_name":"Target Customer"}`), ContactPoints: []byte(`[]`)}); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if _, err := repo.Create(ctx, ports.CustomerCreate{ID: otherID, BusinessID: businessB, Profile: []byte(`{"display_name":"Other Tenant"}`), ContactPoints: []byte(`[]`)}); err != nil {
		t.Fatalf("create other: %v", err)
	}
	page, err := repo.List(ctx, businessA, "First", "active", 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != firstID {
		t.Fatalf("list/search: page=%#v err=%v", page, err)
	}
	updated, err := repo.Update(ctx, ports.CustomerUpdate{BusinessID: businessA, CustomerID: firstID, ExpectedVersion: first.ResourceVersion, Profile: []byte(`{"display_name":"First Updated"}`)})
	if err != nil || updated.ResourceVersion != 2 {
		t.Fatalf("update: record=%#v err=%v", updated, err)
	}
	if _, err := repo.Update(ctx, ports.CustomerUpdate{BusinessID: businessA, CustomerID: firstID, ExpectedVersion: first.ResourceVersion, Profile: []byte(`{"display_name":"stale"}`)}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("expected stale, got %v", err)
	}
	if _, err := repo.GetByID(ctx, businessB, firstID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("tenant isolation get: %v", err)
	}
	merge, err := repo.Merge(ctx, ports.CustomerMerge{BusinessID: businessA, CustomerID: firstID, TargetCustomerID: targetID, ExpectedVersion: updated.ResourceVersion, Reason: "duplicate identity"})
	if err != nil || merge.Status != "merged" || merge.MergedIntoCustomer == nil || *merge.MergedIntoCustomer != targetID {
		t.Fatalf("merge: record=%#v err=%v", merge, err)
	}
	rollbackErr := errors.New("force rollback")
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		_, updateErr := repo.Update(txCtx, ports.CustomerUpdate{BusinessID: businessA, CustomerID: targetID, ExpectedVersion: 1, Profile: []byte(`{"display_name":"must rollback"}`)})
		if updateErr != nil {
			return updateErr
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback: %v", err)
	}
	target, err := repo.GetByID(ctx, businessA, targetID)
	if err != nil || target.ResourceVersion != 1 {
		t.Fatalf("rollback persisted: record=%#v err=%v", target, err)
	}
}
