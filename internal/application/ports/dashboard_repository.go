package ports

import (
	"context"
	"time"
)

type MerchantSubscriptionOverview struct {
	HasSubscription    bool
	ID                 string
	BusinessID         string
	PlanID             string
	PlanCode           string
	PlanName           string
	PlanVersion        int
	Status             string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	PriceYERSnapshot   int
	AmountDueYER       int
	AIReplyLimit       int
	AIRepliesUsed      int
	AIRepliesRemaining int
	AICatalogLimit     int
	ChannelLimit       int
	ChannelCeiling     int
	SeatLimit          int
	ActiveChannels     int
	ActiveSeats        int
	DaysRemaining      int
	ActivatedAt        *time.Time
	CreatedAt          time.Time
}

type DashboardOverviewRecord struct {
	OpenConversations         int
	WaitingHuman              int
	NewCustomers              int
	NewLeads                  int
	TransactionsNeedingReview int
	Subscription              *MerchantSubscriptionOverview
}

type DashboardRepository interface {
	GetOverview(ctx context.Context, businessID string) (DashboardOverviewRecord, error)
	GetMerchantSubscription(ctx context.Context, businessID string) (MerchantSubscriptionOverview, error)
}
