package postgres

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type DashboardRepository struct{ adapter *Adapter }

func NewDashboardRepository(adapter *Adapter) *DashboardRepository {
	return &DashboardRepository{adapter: adapter}
}

func (r *DashboardRepository) GetOverview(ctx context.Context, businessID string) (ports.DashboardOverviewRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.DashboardOverviewRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.DashboardOverviewRecord{}, invalidRepositoryInput("dashboard.get_overview", "business id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.DashboardOverviewRecord{}, err
	}
	const query = `SELECT
  (SELECT count(*) FROM conversations WHERE business_id = $1::uuid AND state <> 'closed'),
  (SELECT count(*) FROM conversations WHERE business_id = $1::uuid AND state = 'waiting_human'),
  (SELECT count(*) FROM customers WHERE business_id = $1::uuid AND created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')),
  (SELECT count(*) FROM leads WHERE business_id = $1::uuid AND created_at >= date_trunc('day', now() AT TIME ZONE 'UTC')),
  (SELECT count(*) FROM transaction_reviews WHERE business_id = $1::uuid AND required = true AND status = 'pending')`
	var record ports.DashboardOverviewRecord
	if err := executor.QueryRow(ctx, query, businessID).Scan(&record.OpenConversations, &record.WaitingHuman, &record.NewCustomers, &record.NewLeads, &record.TransactionsNeedingReview); err != nil {
		return ports.DashboardOverviewRecord{}, &RepositoryError{Operation: "dashboard.get_overview", Kind: RepositoryInvalid, Err: err}
	}
	sub, subErr := r.GetMerchantSubscription(ctx, businessID)
	if subErr == nil && sub.HasSubscription {
		record.Subscription = &sub
	}
	return record, nil
}

func (r *DashboardRepository) GetMerchantSubscription(ctx context.Context, businessID string) (ports.MerchantSubscriptionOverview, error) {
	if r == nil || r.adapter == nil {
		return ports.MerchantSubscriptionOverview{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.MerchantSubscriptionOverview{}, invalidRepositoryInput("dashboard.get_subscription", "business id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.MerchantSubscriptionOverview{}, err
	}
	const query = `SELECT
  s.id::text,
  s.business_id::text,
  s.plan_id::text,
  COALESCE(p.code, ''),
  COALESCE(p.display_name, ''),
  COALESCE(p.version, 1),
  s.status,
  s.period_start,
  s.period_end,
  s.price_yer_snapshot,
  s.amount_due_yer,
  s.ai_reply_limit + COALESCE((SELECT sum(ai_reply_delta) FROM subscription_addons a WHERE a.subscription_id=s.id AND a.status='ACTIVE'), 0),
  s.ai_catalog_limit + COALESCE((SELECT sum(catalog_delta) FROM subscription_addons a WHERE a.subscription_id=s.id AND a.status='ACTIVE'), 0),
  s.included_channel_limit + COALESCE((SELECT sum(channel_delta) FROM subscription_addons a WHERE a.subscription_id=s.id AND a.status='ACTIVE'), 0),
  s.channel_ceiling,
  s.seat_limit + COALESCE((SELECT sum(seat_delta) FROM subscription_addons a WHERE a.subscription_id=s.id AND a.status='ACTIVE'), 0),
  s.activated_at,
  s.created_at,
  COALESCE(u.ai_replies_used, 0),
  (SELECT count(*) FROM channel_connections WHERE business_id = s.business_id AND status = 'active'),
  (SELECT count(*) FROM business_memberships WHERE business_id = s.business_id AND status = 'active')
FROM subscriptions s
LEFT JOIN plans p ON p.id = s.plan_id
LEFT JOIN subscription_ai_usage u ON u.subscription_id = s.id
WHERE s.business_id = $1::uuid
ORDER BY
  CASE s.status WHEN 'ACTIVE' THEN 1 WHEN 'PENDING' THEN 2 ELSE 3 END,
  s.created_at DESC
LIMIT 1`

	var sub ports.MerchantSubscriptionOverview
	err = executor.QueryRow(ctx, query, businessID).Scan(
		&sub.ID,
		&sub.BusinessID,
		&sub.PlanID,
		&sub.PlanCode,
		&sub.PlanName,
		&sub.PlanVersion,
		&sub.Status,
		&sub.PeriodStart,
		&sub.PeriodEnd,
		&sub.PriceYERSnapshot,
		&sub.AmountDueYER,
		&sub.AIReplyLimit,
		&sub.AICatalogLimit,
		&sub.ChannelLimit,
		&sub.ChannelCeiling,
		&sub.SeatLimit,
		&sub.ActivatedAt,
		&sub.CreatedAt,
		&sub.AIRepliesUsed,
		&sub.ActiveChannels,
		&sub.ActiveSeats,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.MerchantSubscriptionOverview{HasSubscription: false, BusinessID: businessID}, nil
		}
		return ports.MerchantSubscriptionOverview{}, &RepositoryError{Operation: "dashboard.get_subscription", Kind: RepositoryInvalid, Err: err}
	}
	sub.HasSubscription = true
	rem := sub.AIReplyLimit - sub.AIRepliesUsed
	if rem < 0 {
		rem = 0
	}
	sub.AIRepliesRemaining = rem
	days := int(math.Ceil(time.Until(sub.PeriodEnd).Hours() / 24))
	if days < 0 {
		days = 0
	}
	sub.DaysRemaining = days
	return sub, nil
}

var _ ports.DashboardRepository = (*DashboardRepository)(nil)
