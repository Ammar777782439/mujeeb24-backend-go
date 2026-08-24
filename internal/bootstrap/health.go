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
	Ping func(context.Context) error
}

func (s readinessQueryService) Handle(ctx context.Context, _ commands.GetReadinessQuery) (commands.HealthView, error) {
	if s.Ping == nil {
		return commands.HealthView{}, appErrors.New(appErrors.CodeExternalDependency, "postgresql readiness check is not configured")
	}
	if err := s.Ping(ctx); err != nil {
		return commands.HealthView{Status: "not_ready", Checks: map[string]string{"postgresql": "unavailable"}}, &appErrors.Error{Code: appErrors.CodeExternalDependency, Message: "postgresql is not ready", Retryable: true, Cause: err}
	}
	return commands.HealthView{Status: "ready", Checks: map[string]string{"postgresql": "ok"}}, nil
}

var _ commands.QueryHandler[commands.GetLivenessQuery, commands.HealthView] = livenessQueryService{}
var _ commands.QueryHandler[commands.GetReadinessQuery, commands.HealthView] = readinessQueryService{}
