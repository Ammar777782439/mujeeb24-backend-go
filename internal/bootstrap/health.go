package bootstrap

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

type livenessQueryService struct{}

func (livenessQueryService) Handle(context.Context, commands.GetLivenessQuery) (commands.HealthView, error) {
	return commands.HealthView{Status: "ok", Checks: map[string]string{"process": "ok"}}, nil
}

type readinessQueryService struct {
	Ping          func(context.Context) error
	FeatureChecks map[string]string
}

func (s readinessQueryService) Handle(ctx context.Context, _ commands.GetReadinessQuery) (commands.HealthView, error) {
	if s.Ping == nil {
		return commands.HealthView{}, appErrors.New(appErrors.CodeExternalDependency, "postgresql readiness check is not configured")
	}
	checks := cloneChecks(s.FeatureChecks)
	if s.Ping == nil {
		return commands.HealthView{}, appErrors.New(appErrors.CodeExternalDependency, "postgresql readiness check is not configured")
	}
	if err := s.Ping(ctx); err != nil {
		checks["postgresql"] = "unavailable"
		return commands.HealthView{Status: "not_ready", Checks: checks}, &appErrors.Error{Code: appErrors.CodeExternalDependency, Message: "postgresql is not ready", Retryable: true, Cause: err}
	}
	checks["postgresql"] = "ok"
	return commands.HealthView{Status: "ready", Checks: checks}, nil
}

func cloneChecks(source map[string]string) map[string]string {
	checks := make(map[string]string, len(source)+1)
	for key, value := range source {
		checks[key] = value
	}
	return checks
}

var _ commands.QueryHandler[commands.GetLivenessQuery, commands.HealthView] = livenessQueryService{}
var _ commands.QueryHandler[commands.GetReadinessQuery, commands.HealthView] = readinessQueryService{}
