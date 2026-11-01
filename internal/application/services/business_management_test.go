package services

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type stubBusinessManagementRepo struct {
	lastUpdate  ports.BusinessRuntimePolicyUpdate
	getError    error
	updateError error
	record      ports.BusinessRuntimePolicyRecord
}

func (s *stubBusinessManagementRepo) UpdateProfile(_ context.Context, _ ports.BusinessProfileUpdate) (ports.BusinessRecord, error) {
	return ports.BusinessRecord{}, nil
}

func (s *stubBusinessManagementRepo) GetRuntimePolicy(_ context.Context, _ string) (ports.BusinessRuntimePolicyRecord, error) {
	return s.record, s.getError
}

func (s *stubBusinessManagementRepo) UpdateRuntimePolicy(_ context.Context, update ports.BusinessRuntimePolicyUpdate) (ports.BusinessRuntimePolicyRecord, error) {
	s.lastUpdate = update
	if s.updateError != nil {
		return ports.BusinessRuntimePolicyRecord{}, s.updateError
	}
	rec := s.record
	if update.SalesWorkflowEnabled != nil {
		rec.SalesWorkflowEnabled = *update.SalesWorkflowEnabled
	}
	if update.AllowSalesInteractions != nil {
		rec.AllowSalesInteractions = *update.AllowSalesInteractions
	}
	rec.ResourceVersion = update.ExpectedVersion + 1
	return rec, nil
}

type stubTx struct{}

func (stubTx) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestUpdateBusinessPolicyForwardsSalesFlags(t *testing.T) {
	repo := &stubBusinessManagementRepo{
		record: ports.BusinessRuntimePolicyRecord{
			BusinessID:      "b-1",
			ResourceVersion: 1,
		},
	}
	service := UpdateBusinessPolicyService{
		BusinessManagementService: BusinessManagementService{
			Repository:   repo,
			Transactions: stubTx{},
		},
	}

	trueVal := true
	version := commands.ResourceVersion("1")
	result, err := service.Handle(context.Background(), commands.UpdateBusinessPolicyCommand{
		Meta: commands.CommandMeta{
			Actor:           commands.ActorContext{BusinessID: "b-1"},
			ExpectedVersion: &version,
		},
		SalesWorkflowEnabled:   &trueVal,
		AllowSalesInteractions: &trueVal,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastUpdate.SalesWorkflowEnabled == nil || !*repo.lastUpdate.SalesWorkflowEnabled {
		t.Fatalf("expected SalesWorkflowEnabled true, got %#v", repo.lastUpdate.SalesWorkflowEnabled)
	}
	if repo.lastUpdate.AllowSalesInteractions == nil || !*repo.lastUpdate.AllowSalesInteractions {
		t.Fatalf("expected AllowSalesInteractions true, got %#v", repo.lastUpdate.AllowSalesInteractions)
	}
	if !result.Policy.SalesWorkflowEnabled || !result.Policy.AllowSalesInteractions {
		t.Fatalf("expected result policy to have sales flags true, got %+v", result.Policy)
	}
	if result.Policy.ResourceVersion != "2" {
		t.Fatalf("expected resource version 2, got %s", result.Policy.ResourceVersion)
	}
}

func TestBusinessPolicyMissingAndStaleAreTypedErrors(t *testing.T) {
	repo := &stubBusinessManagementRepo{getError: governancePersistenceError("not_found"), updateError: governancePersistenceError("stale")}
	service := BusinessManagementService{Repository: repo, Transactions: stubTx{}}
	_, err := service.GetPolicy(context.Background(), queries.GetBusinessPolicyQuery{})
	if !hasApplicationCode(err, appErrors.CodeNotFound) {
		t.Fatalf("missing policy must not become an internal error: %v", err)
	}
	version := commands.ResourceVersion("1")
	flag := true
	_, err = service.UpdatePolicy(context.Background(), commands.UpdateBusinessPolicyCommand{Meta: commands.CommandMeta{ExpectedVersion: &version}, DefaultHumanReview: &flag})
	if !hasApplicationCode(err, appErrors.CodeStaleResource) {
		t.Fatalf("stale policy must not become an internal error: %v", err)
	}
	unexpected := errors.New("unexpected storage failure")
	if mapBusinessPolicyRepositoryError(unexpected) != unexpected {
		t.Fatal("unexpected storage failures must stay internal")
	}
}
