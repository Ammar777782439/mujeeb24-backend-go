package services

import (
	"context"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ownerTxStub struct{}

func (ownerTxStub) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type ownerPrincipalStub struct {
	called      bool
	permissions []string
	err         error
}

func (s *ownerPrincipalStub) CreateOwnerPrincipalAndMembership(_ context.Context, principal ports.PrincipalRecord, _ commands.BusinessID, permissions []string, _ time.Time) (ports.PrincipalRecord, error) {
	s.called = true
	s.permissions = permissions
	if s.err != nil {
		return ports.PrincipalRecord{}, s.err
	}
	principal.Status = "active"
	return principal, nil
}

type ownerBusinessStub struct {
	statusBefore string
	ownerBefore  *string
	activated    bool
}

func (s *ownerBusinessStub) GetByID(_ context.Context, _ string) (ports.PlatformBusinessRecord, error) {
	return ports.PlatformBusinessRecord{ID: "00000000-0000-0000-0000-000000000701", PlatformStatus: s.statusBefore, OwnerIdentitySummary: s.ownerBefore}, nil
}

func (s *ownerBusinessStub) ActivateFromPendingSetup(_ context.Context, _ string, _ time.Time) (ports.PlatformBusinessRecord, error) {
	s.activated = true
	summary := "Owner <owner@example.com>"
	return ports.PlatformBusinessRecord{ID: "00000000-0000-0000-0000-000000000701", PlatformStatus: "active", OwnerIdentitySummary: &summary}, nil
}

func TestAssignBusinessOwnerServiceAssignsAndActivates(t *testing.T) {
	principalRepo := &ownerPrincipalStub{}
	businessRepo := &ownerBusinessStub{statusBefore: "pending_setup"}
	service := AssignBusinessOwnerService{
		Transactions:  ownerTxStub{},
		OwnerAccounts: principalRepo,
		Business:      businessRepo,
		Audit:         &governanceAuditStub{},
	}
	result, err := service.Handle(context.Background(), AssignBusinessOwnerInput{
		Actor:       governanceActor(),
		Permissions: []string{"catalog:read"},
		BusinessID:  "00000000-0000-0000-0000-000000000701",
		Email:       "owner@example.com",
		DisplayName: "Owner",
		Password:    "a-strong-owner-password",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !principalRepo.called || !businessRepo.activated || len(principalRepo.permissions) != 1 || principalRepo.permissions[0] != "catalog:read" {
		t.Fatalf("owner flow not completed: principal=%v activated=%v", principalRepo.called, businessRepo.activated)
	}
	if result.Role != "owner" || result.BusinessStatus != "ACTIVE" || result.Email != "owner@example.com" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAssignBusinessOwnerServiceRejectsActiveBusiness(t *testing.T) {
	service := AssignBusinessOwnerService{
		Transactions:  ownerTxStub{},
		OwnerAccounts: &ownerPrincipalStub{},
		Business:      &ownerBusinessStub{statusBefore: "active"},
		Audit:         &governanceAuditStub{},
	}
	if _, err := service.Handle(context.Background(), AssignBusinessOwnerInput{
		Actor:       governanceActor(),
		Permissions: []string{"catalog:read"},
		BusinessID:  "00000000-0000-0000-0000-000000000701",
		Email:       "owner@example.com",
		DisplayName: "Owner",
		Password:    "a-strong-owner-password",
	}); err == nil {
		t.Fatal("expected active business to reject owner assignment")
	}
}

func TestAssignOwnerReturnsConflictWithoutLeakingPersistenceDetails(t *testing.T) {
	principalRepo := &ownerPrincipalStub{err: governancePersistenceError("conflict")}
	service := AssignBusinessOwnerService{Transactions: ownerTxStub{}, OwnerAccounts: principalRepo, Business: &ownerBusinessStub{statusBefore: "pending_setup"}, Audit: &governanceAuditStub{}}
	_, err := service.Handle(context.Background(), AssignBusinessOwnerInput{Actor: governanceActor(), BusinessID: "00000000-0000-0000-0000-000000000701", Email: "new@example.test", DisplayName: "Owner", Password: "isolated-test-password", Permissions: []string{"catalog:read"}})
	if !hasApplicationCode(err, appErrors.CodeConflict) {
		t.Fatalf("duplicate identity must be a conflict: %v", err)
	}
	if strings.Contains(err.Error(), "internal persistence detail") {
		t.Fatal("persistence details leaked")
	}
}
