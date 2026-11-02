package services

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"testing"
	"time"
)

type followUpStub struct {
	called bool
	access ports.ConversationAccess
}

func (s *followUpStub) ListFollowUps(_ context.Context, b string, a ports.ConversationAccess, now time.Time, limit int) ([]ports.DashboardFollowUpRecord, error) {
	s.called = true
	s.access = a
	if b != "business" || limit != 6 || now.IsZero() {
		panic("invalid scoped read")
	}
	return make([]ports.DashboardFollowUpRecord, 6), nil
}
func TestDashboardFollowUpsRequireEveryReadPermission(t *testing.T) {
	for _, permissions := range [][]string{{}, {"dashboard:read"}, {"dashboard:read", "sales:read"}, {"dashboard:read", "conversations:read"}} {
		repo := &followUpStub{}
		s := DashboardFollowUpsQueryService{Repository: repo}
		_, err := s.Handle(context.Background(), queries.GetDashboardFollowUpsQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: "business", PrincipalID: "agent", Role: "agent", Permissions: permissions}}})
		if err == nil || repo.called {
			t.Fatal("missing permission must reject before reading records")
		}
	}
}
func TestDashboardFollowUpsBoundAndScopeRead(t *testing.T) {
	for _, role := range []string{"owner", "admin", "manager", "agent", "viewer"} {
		repo := &followUpStub{}
		s := DashboardFollowUpsQueryService{Repository: repo}
		v, err := s.Handle(context.Background(), queries.GetDashboardFollowUpsQuery{Meta: queries.QueryMeta{Actor: commands.ActorContext{BusinessID: "business", PrincipalID: "agent", Role: role, Permissions: []string{"dashboard:read", "sales:read", "conversations:read"}}}})
		if err != nil || len(v.Items) != 5 || !v.HasMore || repo.access.PrincipalID != "agent" {
			t.Fatalf("bounded view=%#v error=%v", v, err)
		}
		if repo.access.Supervisor != (role == "owner" || role == "admin" || role == "manager") {
			t.Fatal("role visibility does not match inbox")
		}
	}
}
