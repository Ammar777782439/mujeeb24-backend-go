package services

import (
	"context"

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
	return queries.DashboardOverviewView{OpenConversations: record.OpenConversations, WaitingHuman: record.WaitingHuman, NewCustomers: record.NewCustomers, NewLeads: record.NewLeads, TransactionsNeedingReview: record.TransactionsNeedingReview}, nil
}

var _ queries.GetDashboardOverviewHandler = DashboardOverviewQueryService{}
