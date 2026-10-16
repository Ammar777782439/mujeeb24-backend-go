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

func TestSubscriptionAddonsAreDurableAndPeriodBound(t *testing.T) {
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

	const (
		businessID     = "00000000-0000-0000-0000-000000000761"
		subscriptionID = "00000000-0000-0000-0000-000000000762"
		planID         = "00000000-0000-0000-0000-00000000a102"
		actorID        = "00000000-0000-0000-0000-000000000763"
	)
	cleanupSubscriptionAddonIntegration(ctx, adapter, subscriptionID, businessID)
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Add-on Integration','addon-integration','active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessID)
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	defer cleanupSubscriptionAddonIntegration(context.Background(), adapter, subscriptionID, businessID)

	now := time.Now().UTC().Truncate(time.Microsecond)
	subscriptions := NewSubscriptionRepository(adapter)
	if _, err = subscriptions.Create(ctx, ports.SubscriptionCreate{ID: subscriptionID, BusinessID: businessID, PlanID: planID, PriceYERSnapshot: 5000, AmountDueYER: 5000, PeriodStart: now.Add(-time.Hour), PeriodEnd: now.Add(time.Hour), AIReplyLimit: 500, AICatalogLimit: 200, ChannelLimit: 1, ChannelCeiling: 1, SeatLimit: 1, InternalAICostBudgetYER: 1000, Now: now}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if _, err = subscriptions.Activate(ctx, subscriptionID, now); err != nil {
		t.Fatalf("activate subscription: %v", err)
	}

	addons := NewSubscriptionAddonRepository(adapter)
	created, err := addons.CreatePending(ctx, ports.SubscriptionAddonCreate{SubscriptionID: subscriptionID, BusinessID: businessID, Kind: "AI_REPLIES_100", CreatedBy: actorID, IdempotencyKey: "create-1", Now: now})
	if err != nil {
		t.Fatalf("create addon: %v", err)
	}
	again, err := addons.CreatePending(ctx, ports.SubscriptionAddonCreate{SubscriptionID: subscriptionID, BusinessID: businessID, Kind: "AI_REPLIES_100", CreatedBy: actorID, IdempotencyKey: "create-1", Now: now})
	if err != nil || again.ID != created.ID {
		t.Fatalf("creation retry = %#v, %v; want existing addon %s", again, err, created.ID)
	}

	payment := ports.SubscriptionAddonPayment{AddonID: created.ID, RecordedBy: actorID, IdempotencyKey: "payment-1", AmountYER: 1500, Method: "CASH", Reference: "receipt-1", PaidAt: now, Now: now}
	activated, err := addons.RecordPaymentAndActivate(ctx, payment)
	if err != nil || activated.Status != "ACTIVE" {
		t.Fatalf("activate addon = %#v, %v", activated, err)
	}
	retried, err := addons.RecordPaymentAndActivate(ctx, payment)
	if err != nil || retried.ID != created.ID || retried.Status != "ACTIVE" {
		t.Fatalf("payment retry = %#v, %v", retried, err)
	}
	if _, err = addons.RecordPaymentAndActivate(ctx, ports.SubscriptionAddonPayment{AddonID: created.ID, RecordedBy: actorID, IdempotencyKey: "payment-2", AmountYER: 1500, Method: "CASH", Reference: "receipt-2", PaidAt: now, Now: now}); err == nil {
		t.Fatal("a second payment identity must not be accepted for an active addon")
	}

	cancelled, err := addons.Cancel(ctx, ports.SubscriptionAddonCancel{AddonID: created.ID, CancelledBy: actorID, Reason: "test cancellation", Now: now})
	if err != nil || cancelled.Status != "CANCELLED" || cancelled.CancelledAt == nil {
		t.Fatalf("cancel addon = %#v, %v", cancelled, err)
	}
}

func cleanupSubscriptionAddonIntegration(ctx context.Context, adapter *Adapter, subscriptionID, businessID string) {
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM subscription_addon_payments WHERE addon_id IN (SELECT id FROM subscription_addons WHERE subscription_id=$1::uuid)`, subscriptionID)
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM subscription_channel_pass_capacity_approvals WHERE subscription_id=$1::uuid`, subscriptionID)
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM subscription_addons WHERE subscription_id=$1::uuid`, subscriptionID)
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM subscriptions WHERE id=$1::uuid`, subscriptionID)
	_, _ = adapter.Pool().Exec(ctx, `DELETE FROM businesses WHERE id=$1::uuid`, businessID)
}
