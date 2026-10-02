package handlers

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// ----------------------------------------------------------------------------
// Tests: Platform AI Operations — Kill Switch + Provider Health
// (Contract §73-108)
// ----------------------------------------------------------------------------

// newPlatformServerWithOperations builds a server with the in-memory
// PlatformOperations registry pre-seeded.
func newPlatformServerWithOperations() *Server {
	operations := services.NewInMemoryPlatformOperationsRepository(true, true)
	return newPlatformServer(PlatformDeps{
		Operations:    operations,
		PlatformAudit: &stubPlatformAuditRepository{},
	})
}

func TestPlatformGetAIOverviewReturnsRuntimeAndProvider(t *testing.T) {
	server := newPlatformServerWithOperations()
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformGetAIOverview", nil)
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestPlatformGetAIOverviewReturns501WhenOperationsUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformGetAIOverview", nil)
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformAIDisableFlipsRuntimeToDisabledAndAudits(t *testing.T) {
	operations := services.NewInMemoryPlatformOperationsRepository(true, true)
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Operations: operations, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformAIDisable", &dto.PlatformAIRuntimeDisableInput{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "ai.disabled" {
		t.Errorf("expected action=ai.disabled, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
	// Verify the state actually flipped.
	state, _ := operations.GetRuntimeState(ctx)
	if state.AdminState != ports.ProviderAdminDisabled {
		t.Errorf("expected runtime admin_state=DISABLED, got %s", state.AdminState)
	}
}

func TestPlatformAIEnableFlipsRuntimeToEnabledAndAudits(t *testing.T) {
	operations := services.NewInMemoryPlatformOperationsRepository(true, true)
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Operations: operations, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	// First disable, then enable — verify the audit + state transitions.
	_, _ = server.dispatchPlatformCommand(ctx, "platformAIDisable", &dto.PlatformAIRuntimeDisableInput{})
	auditRepo.appendedDraft = nil // reset
	_, handled := server.dispatchPlatformCommand(ctx, "platformAIEnable", &dto.PlatformAIRuntimeEnableInput{})
	if !handled {
		t.Fatalf("expected handled=true for enable")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append for enable")
	}
	if auditRepo.appendedDraft.Action != "ai.enabled" {
		t.Errorf("expected action=ai.enabled, got %s", auditRepo.appendedDraft.Action)
	}
	state, _ := operations.GetRuntimeState(ctx)
	if state.AdminState != ports.ProviderAdminEnabled {
		t.Errorf("expected runtime admin_state=ENABLED, got %s", state.AdminState)
	}
}

func TestPlatformAIHealthCheckRunsProbeAndAudits(t *testing.T) {
	operations := services.NewInMemoryPlatformOperationsRepository(true, true)
	// Register a stub probe that returns HEALTHY.
	operations.RegisterProbe("google_gemini", &stubHealthCheckProbe{
		health: ports.ProviderHealthHealthy,
	})
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Operations: operations, PlatformAudit: auditRepo})

	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformAIHealthCheck", &dto.PlatformAIHealthCheckInput{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "provider.health_checked" {
		t.Errorf("expected action=provider.health_checked, got %s", auditRepo.appendedDraft.Action)
	}
}

// ----------------------------------------------------------------------------
// Stub probe for health check tests
// ----------------------------------------------------------------------------

type stubHealthCheckProbe struct {
	health       ports.ProviderHealthState
	latencyNanos int64
	failureCode  *string
}

func (p *stubHealthCheckProbe) Probe(_ context.Context) (ports.ProviderHealthState, int64, *string) {
	return p.health, p.latencyNanos, p.failureCode
}
