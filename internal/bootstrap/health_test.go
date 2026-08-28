package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

func TestReadinessReportsOptionalFeatureConfigurationWithoutBlockingCoreReady(t *testing.T) {
	service := readinessQueryService{Ping: func(context.Context) error { return nil }, FeatureChecks: map[string]string{"auto_reply": "configured", "llm_runtime": "disabled"}}
	view, err := service.Handle(context.Background(), commands.GetReadinessQuery{})
	if err != nil || view.Status != "ready" || view.Checks["postgresql"] != "ok" || view.Checks["auto_reply"] != "configured" || view.Checks["llm_runtime"] != "disabled" {
		t.Fatalf("unexpected readiness view=%#v err=%v", view, err)
	}
}

func TestReadinessReportsDatabaseFailureAlongsideFeatureConfiguration(t *testing.T) {
	service := readinessQueryService{Ping: func(context.Context) error { return errors.New("connection refused") }, FeatureChecks: map[string]string{"socialapi_webhook": "configured"}}
	view, err := service.Handle(context.Background(), commands.GetReadinessQuery{})
	if err == nil || view.Status != "not_ready" || view.Checks["postgresql"] != "unavailable" || view.Checks["socialapi_webhook"] != "configured" {
		t.Fatalf("unexpected readiness view=%#v err=%v", view, err)
	}
}
