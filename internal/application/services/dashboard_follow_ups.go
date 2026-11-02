package services

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"time"
)

type DashboardFollowUpsQueryService struct {
	Repository ports.DashboardFollowUpRepository
	Now        func() time.Time
}

func (s DashboardFollowUpsQueryService) Handle(ctx context.Context, q queries.GetDashboardFollowUpsQuery) (queries.DashboardFollowUpsView, error) {
	actor := q.Meta.Actor
	if actor.BusinessID == "" || actor.PrincipalID == "" || !authorization.Has(actor, authorization.PermissionDashboardRead) || !authorization.Has(actor, authorization.PermissionSalesRead) || !authorization.Has(actor, authorization.PermissionConversationsRead) {
		return queries.DashboardFollowUpsView{}, appErrors.New(appErrors.CodeForbidden, "dashboard, sales and conversation read permissions are required")
	}
	if s.Repository == nil {
		return queries.DashboardFollowUpsView{}, appErrors.NotImplemented()
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	items, err := s.Repository.ListFollowUps(ctx, string(actor.BusinessID), authorization.ConversationAccess(actor), now, 6)
	if err != nil {
		return queries.DashboardFollowUpsView{}, err
	}
	out := queries.DashboardFollowUpsView{Items: items, HasMore: len(items) > 5, AsOf: now}
	if out.HasMore {
		out.Items = items[:5]
	}
	if out.Items == nil {
		out.Items = []ports.DashboardFollowUpRecord{}
	}
	return out, nil
}
