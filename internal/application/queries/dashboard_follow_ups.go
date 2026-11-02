package queries

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"time"
)

type GetDashboardFollowUpsQuery struct{ Meta QueryMeta }
type DashboardFollowUpsView struct {
	Items   []ports.DashboardFollowUpRecord
	HasMore bool
	AsOf    time.Time
}
type GetDashboardFollowUpsHandler = commands.QueryHandler[GetDashboardFollowUpsQuery, DashboardFollowUpsView]
