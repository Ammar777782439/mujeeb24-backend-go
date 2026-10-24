package services

import (
	"context"
	"time"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type DashboardOverviewQueryService struct{ Repository ports.DashboardRepository }

func (s DashboardOverviewQueryService) Handle(ctx context.Context, query queries.GetDashboardOverviewQuery) (queries.DashboardOverviewView, error) {
	if s.Repository == nil {
		return queries.DashboardOverviewView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.GetOverview(ctx, string(query.Meta.Actor.BusinessID))
	if err != nil {
		return queries.DashboardOverviewView{}, err
	}
	return queries.DashboardOverviewView{
		OpenConversations:         record.OpenConversations,
		WaitingHuman:              record.WaitingHuman,
		NewCustomers:              record.NewCustomers,
		NewLeads:                  record.NewLeads,
		TransactionsNeedingReview: record.TransactionsNeedingReview,
		Subscription:              mapMerchantSubscriptionView(record.Subscription),
	}, nil
}

type BusinessSubscriptionQueryService struct{ Repository ports.DashboardRepository }

func (s BusinessSubscriptionQueryService) Handle(ctx context.Context, query queries.GetBusinessSubscriptionQuery) (queries.MerchantSubscriptionView, error) {
	if s.Repository == nil {
		return queries.MerchantSubscriptionView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.GetMerchantSubscription(ctx, string(query.Meta.Actor.BusinessID))
	if err != nil {
		return queries.MerchantSubscriptionView{}, err
	}
	return mapMerchantSubscriptionDirect(record), nil
}

func mapMerchantSubscriptionView(sub *ports.MerchantSubscriptionOverview) *queries.MerchantSubscriptionView {
	if sub == nil || !sub.HasSubscription {
		return nil
	}
	v := mapMerchantSubscriptionDirect(*sub)
	return &v
}

func mapMerchantSubscriptionDirect(sub ports.MerchantSubscriptionOverview) queries.MerchantSubscriptionView {
	view := queries.MerchantSubscriptionView{
		HasSubscription: sub.HasSubscription,
		BusinessID:      sub.BusinessID,
		ActiveChannels:  sub.ActiveChannels,
		ActiveSeats:     sub.ActiveSeats,
	}
	if !sub.HasSubscription {
		return view
	}
	view.ID = &sub.ID
	view.PlanID = &sub.PlanID
	view.PlanCode = &sub.PlanCode
	view.PlanName = &sub.PlanName
	view.PlanVersion = &sub.PlanVersion
	view.Status = &sub.Status
	startStr := sub.PeriodStart.Format(time.RFC3339)
	endStr := sub.PeriodEnd.Format(time.RFC3339)
	view.PeriodStart = &startStr
	view.PeriodEnd = &endStr
	view.PriceYERSnapshot = &sub.PriceYERSnapshot
	view.AmountDueYER = &sub.AmountDueYER
	view.AIReplyLimit = &sub.AIReplyLimit
	view.AIRepliesUsed = &sub.AIRepliesUsed
	view.AIRepliesRemaining = &sub.AIRepliesRemaining
	view.AICatalogLimit = &sub.AICatalogLimit
	view.ChannelLimit = &sub.ChannelLimit
	view.ChannelCeiling = &sub.ChannelCeiling
	view.SeatLimit = &sub.SeatLimit
	view.DaysRemaining = &sub.DaysRemaining
	if sub.ActivatedAt != nil {
		actStr := sub.ActivatedAt.Format(time.RFC3339)
		view.ActivatedAt = &actStr
	}
	createdStr := sub.CreatedAt.Format(time.RFC3339)
	view.CreatedAt = &createdStr
	return view
}

var _ queries.GetDashboardOverviewHandler = DashboardOverviewQueryService{}
var _ queries.GetBusinessSubscriptionHandler = BusinessSubscriptionQueryService{}
