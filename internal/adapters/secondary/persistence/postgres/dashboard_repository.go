package postgres

import (
	"context"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
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
	return record, nil
}

var _ ports.DashboardRepository = (*DashboardRepository)(nil)
