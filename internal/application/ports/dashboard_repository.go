package ports

import "context"

type DashboardOverviewRecord struct {
	OpenConversations         int
	WaitingHuman              int
	NewCustomers              int
	NewLeads                  int
	TransactionsNeedingReview int
}

type DashboardRepository interface {
	GetOverview(ctx context.Context, businessID string) (DashboardOverviewRecord, error)
}
