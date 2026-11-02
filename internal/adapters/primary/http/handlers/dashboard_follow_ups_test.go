package handlers

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type dashboardFollowUpHTTPStub struct{ called bool }

func (s *dashboardFollowUpHTTPStub) ListFollowUps(_ context.Context, _ string, _ ports.ConversationAccess, _ time.Time, _ int) ([]ports.DashboardFollowUpRecord, error) {
	s.called = true
	return []ports.DashboardFollowUpRecord{{LeadID: "22222222-2222-4222-8222-222222222222", CustomerID: "33333333-3333-4333-8333-333333333333", CustomerName: "أحمد", LeadStatus: "new", Attention: "new", UpdatedAt: time.Now()}}, nil
}
func TestDashboardFollowUpsHTTPPermissionAndProjection(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		permissions := []string{"dashboard:read"}
		if allowed {
			permissions = append(permissions, "sales:read", "conversations:read")
		}
		repo := &dashboardFollowUpHTTPStub{}
		server := NewServer(Dependencies{Scope: fakeScope{actor: commands.ActorContext{BusinessID: "11111111-1111-4111-8111-111111111111", PrincipalID: "44444444-4444-4444-8444-444444444444", Role: "owner", Permissions: permissions}}, GetDashboardFollowUps: services.DashboardFollowUpsQueryService{Repository: repo}})
		_, mux := contract.BuildAPIWithHandlers(server)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/businesses/11111111-1111-4111-8111-111111111111/dashboard/follow-ups", nil))
		if !allowed {
			if rec.Code != 403 || repo.called {
				t.Fatalf("restricted owner bypassed permission: %d %s", rec.Code, rec.Body.String())
			}
			continue
		}
		if rec.Code != 200 || !repo.called || !strings.Contains(rec.Body.String(), `"customer_name":"أحمد"`) {
			t.Fatalf("scoped projection: %d %s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "current_score") {
			t.Fatal("opaque score leaked into actionable dashboard")
		}
	}
}
