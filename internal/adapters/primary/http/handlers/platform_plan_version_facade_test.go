package handlers

import (
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ----------------------------------------------------------------------------
// Tests: Plan Version-on-Edit (Contract §17, AIUsageTokenTelemetry.md §22-23)
// ----------------------------------------------------------------------------

const planIDForTests = "00000000-0000-0000-0000-00000000plan1"

func platformPlanPath() dto.PlatformPlanPath {
	return dto.PlatformPlanPath{PlanID: dto.UUID(planIDForTests)}
}

func TestPlatformCreatePlanVersionReturns501WhenPlansUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlanVersion", &dto.CreatePlanVersionInput{
		PlatformPlanPath: platformPlanPath(),
		Body: dto.CreatePlanVersionRequest{
			DisplayName: "Basic v2", PriceYER: 6000, AIReplyLimit: 600,
			AICatalogLimit: 250, ChannelLimit: 1, InternalAICostBudgetYER: 1300, Reason: "Updated cost budget",
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCreatePlanVersionRejectsMissingReason(t *testing.T) {
	server := newPlatformServer(PlatformDeps{
		Plans:         &stubPlanRepository{},
		PlatformAudit: &stubPlatformAuditRepository{},
	})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlanVersion", &dto.CreatePlanVersionInput{
		PlatformPlanPath: platformPlanPath(),
		Body: dto.CreatePlanVersionRequest{
			DisplayName: "Basic v2", PriceYER: 6000, AIReplyLimit: 600,
			AICatalogLimit: 250, ChannelLimit: 1, InternalAICostBudgetYER: 1300, Reason: "  ",
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCreatePlanVersionFailsIfBasePlanMissing(t *testing.T) {
	planRepo := &stubPlanRepository{getByIDErr: errors.New("not found")}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlanVersion", &dto.CreatePlanVersionInput{
		PlatformPlanPath: platformPlanPath(),
		Body: dto.CreatePlanVersionRequest{
			DisplayName: "Basic v2", PriceYER: 6000, AIReplyLimit: 600,
			AICatalogLimit: 250, ChannelLimit: 1, InternalAICostBudgetYER: 1300, Reason: "Updated cost budget",
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit on base plan lookup failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformCreatePlanVersionFailsIfBasePlanNotActive(t *testing.T) {
	planRepo := &stubPlanRepository{getByID: ports.PlanRecord{ID: planIDForTests, Code: "basic", Version: 1, Status: "RETIRED"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlanVersion", &dto.CreatePlanVersionInput{
		PlatformPlanPath: platformPlanPath(),
		Body: dto.CreatePlanVersionRequest{
			DisplayName: "Basic v2", PriceYER: 6000, AIReplyLimit: 600,
			AICatalogLimit: 250, ChannelLimit: 1, InternalAICostBudgetYER: 1300, Reason: "Updated cost budget",
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit on BASE_PLAN_NOT_ACTIVE")
	}
	if auditRepo.appendedDraft.FailureCode == nil || *auditRepo.appendedDraft.FailureCode != "BASE_PLAN_NOT_ACTIVE" {
		t.Errorf("expected failure_code=BASE_PLAN_NOT_ACTIVE, got %v", auditRepo.appendedDraft.FailureCode)
	}
}

func TestPlatformCreatePlanVersionAuditsSuccessWithFieldDeltas(t *testing.T) {
	basePlan := ports.PlanRecord{
		ID: planIDForTests, Code: "basic", Version: 1, Status: "ACTIVE",
		PriceYER: 5000, AIReplyLimit: 500, InternalAICostBudgetYER: 1000,
		AICatalogLimit: 200, ChannelLimit: 1,
	}
	newPlan := ports.PlanRecord{
		ID: "00000000-0000-0000-0000-00000000plan2", Code: "basic", Version: 2, Status: "ACTIVE",
		PriceYER: 6000, AIReplyLimit: 600, InternalAICostBudgetYER: 1300,
		AICatalogLimit: 250, ChannelLimit: 1,
	}
	planRepo := &stubPlanRepository{getByID: basePlan, newVersion: newPlan}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Plans: planRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreatePlanVersion", &dto.CreatePlanVersionInput{
		PlatformPlanPath: platformPlanPath(),
		Body: dto.CreatePlanVersionRequest{
			DisplayName: "Basic v2", PriceYER: 6000, AIReplyLimit: 600,
			AICatalogLimit: 250, ChannelLimit: 1, InternalAICostBudgetYER: 1300, Reason: "Updated cost budget",
		},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	// Final audit should be the plan.version_created SUCCESS event.
	if auditRepo.appendedDraft.Action != "plan.version_created" {
		t.Errorf("expected final action=plan.version_created, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
	// Metadata must contain base/new plan IDs + versions.
	if auditRepo.appendedDraft.Metadata == nil {
		t.Fatalf("expected non-nil metadata")
	}
}
