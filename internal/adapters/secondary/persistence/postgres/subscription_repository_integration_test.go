//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestSubscriptionRepositoryUsesPlanMetadata(t *testing.T) {
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

	const businessID = "00000000-0000-0000-0000-000000000551"
	const subscriptionID = "00000000-0000-0000-0000-000000000552"
	const planID = "00000000-0000-0000-0000-00000000a001"

	_, _ = adapter.Pool().Exec(ctx, "DELETE FROM subscriptions WHERE id = $1::uuid", subscriptionID)
	_, _ = adapter.Pool().Exec(ctx, "DELETE FROM businesses WHERE id = $1::uuid", businessID)
	_, err = adapter.Pool().Exec(ctx,
		`INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
		 VALUES ($1::uuid, 'Subscription Integration', 'subscription-integration', 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`,
		businessID,
	)
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	defer adapter.Pool().Exec(context.Background(), "DELETE FROM subscriptions WHERE id = $1::uuid", subscriptionID)
	defer adapter.Pool().Exec(context.Background(), "DELETE FROM businesses WHERE id = $1::uuid", businessID)

	repo := NewSubscriptionRepository(adapter)
	now := time.Now().UTC().Truncate(time.Microsecond)
	record, err := repo.Create(ctx, ports.SubscriptionCreate{
		ID:                     subscriptionID,
		BusinessID:             businessID,
		PlanID:                 planID,
		PeriodStart:            now,
		PeriodEnd:              now.Add(30 * 24 * time.Hour),
		AIReplyLimit:           500,
		AICatalogLimit:         200,
		ChannelLimit:           1,
		InternalAICostBudgetYER: 1000,
		Now:                    now,
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if record.PlanCode != "basic" || record.PlanVersion != 1 {
		t.Fatalf("plan metadata mismatch: code=%q version=%d", record.PlanCode, record.PlanVersion)
	}

	got, err := repo.GetByID(ctx, subscriptionID)
	if err != nil || got.PlanCode != "basic" || got.PlanVersion != 1 {
		t.Fatalf("get plan metadata mismatch: record=%#v err=%v", got, err)
	}

	page, err := repo.List(ctx, ports.SubscriptionListFilter{BusinessID: businessID, Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list subscriptions: page=%#v err=%v", page, err)
	}
	if page.Items[0].PlanCode != "basic" || page.Items[0].PlanVersion != 1 {
		t.Fatalf("list plan metadata mismatch: %#v", page.Items[0])
	}
}
