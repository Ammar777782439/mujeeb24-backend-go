package handlers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type governanceCountingBusiness struct {
	stubPlatformBusinessRepository
	calls atomic.Int32
}

func (s *governanceCountingBusiness) Create(context.Context, ports.PlatformBusinessCreate) (ports.PlatformBusinessRecord, error) {
	s.calls.Add(1)
	return ports.PlatformBusinessRecord{ID: "business-1", Name: "Test", PlatformStatus: "pending_setup"}, nil
}

func TestGovernanceIdempotencyBindsActorAndBodyAndSerializesRetries(t *testing.T) {
	repo := &governanceCountingBusiness{}
	server := newPlatformServer(PlatformDeps{PlatformBusiness: repo})
	ctx := middleware.WithPlatformAdmin(context.Background(), "00000000-0000-0000-0000-000000000001")
	input := &dto.CreateBusinessInput{Body: dto.CreateBusinessRequest{Name: "Test", Slug: "test", VerticalType: "retail", DefaultCurrency: "YER", Locale: "ar"}}
	input.IdempotencyKey = "same-test-key"
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _ := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", input)
			if err, ok := result.(error); ok {
				t.Errorf("retry: %v", err)
			}
		}()
	}
	wg.Wait()
	if repo.calls.Load() != 1 {
		t.Fatalf("duplicate provisioning calls: %d", repo.calls.Load())
	}
	changed := *input
	changed.Body.Name = "Changed"
	if result, _ := server.dispatchPlatformCommand(ctx, "platformCreateBusiness", &changed); result == nil {
		t.Fatal("missing conflict")
	} else if _, ok := result.(error); !ok {
		t.Fatal("same key with changed body must conflict")
	}
	otherCtx := middleware.WithPlatformAdmin(context.Background(), "00000000-0000-0000-0000-000000000002")
	server.dispatchPlatformCommand(otherCtx, "platformCreateBusiness", input)
	if repo.calls.Load() != 2 {
		t.Fatal("another admin received the first admin's cached result")
	}
}

func TestGovernanceRoutesRejectMissingPlatformScope(t *testing.T) {
	server := NewServer(Dependencies{})
	for _, op := range []string{"platformCreateBusiness", "platformAssignBusinessOwner", "platformListTeamMembers", "platformListTeamInvitations", "platformUpdateTeamMemberPermissions"} {
		result, handled := server.dispatchPlatformCommand(context.Background(), op, nil)
		if !handled {
			t.Fatalf("route not guarded: %s", op)
		}
		if _, ok := result.(error); !ok {
			t.Fatalf("missing platform scope accepted for %s", op)
		}
	}
}
