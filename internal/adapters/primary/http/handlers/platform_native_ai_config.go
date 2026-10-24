package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// These Platform Admin endpoints report the actual native deployment.
// Keys are owned by environment/secrets infrastructure, never by HTTP forms.

func (s *Server) platformGatewayListAIProvidersConfig(_ context.Context) (any, bool) {
	cfg := s.platformDeps.NativeAISettings
	views := []dto.AICredentialView{}
	if cfg != nil && cfg.Configured {
		views = append(views, dto.AICredentialView{
			ID: cfg.Provider + "-environment", Provider: cfg.Provider,
			DisplayName: "Active inference provider (deployment-managed)", Status: "CONFIGURED",
		})
	}
	out := &contract.List[dto.AICredentialView]{}
	out.Body.Data = views
	return out, true
}

func (s *Server) platformGatewayAddAICredential(_ context.Context, _ *dto.AddCredentialInput) (any, bool) {
	return mapApplicationError(appErrors.New(appErrors.CodeConflict,
		"Native provider API credentials are deployment-owned; this endpoint does not accept keys")), true
}

func (s *Server) platformGatewayTestAIConnection(ctx context.Context, in *dto.TestConnectionInput) (any, bool) {
	cfg := s.platformDeps.NativeAISettings
	if cfg == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "native AI is not configured")), true
	}
	provider := strings.TrimSpace(in.Provider)
	if provider != cfg.Provider {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "provider must match the configured runtime provider")), true
	}
	// An unconfigured health probe must never report false success.
	s.appendPlatformAudit(ctx, "ai.connection.tested", "ai_runtime", nil, nil, "FAILURE", "PROBE_NOT_CONFIGURED", map[string]any{"provider": provider})
	return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented,
		"Direct provider connectivity probe is not configured; no live model check was performed")), true
}

func (s *Server) platformGatewayDiscoverAIModels(ctx context.Context, in *dto.DiscoverModelsInput) (any, bool) {
	cfg := s.platformDeps.NativeAISettings
	if cfg == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "native AI is not configured")), true
	}
	if in.Provider != "" && in.Provider != cfg.Provider {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "unsupported provider")), true
	}
	description := "Configured deployment identifier only; not a live provider model discovery"
	result := make([]dto.AIModelView, 0, 1)
	if cfg.Configured && cfg.Model != "" && (in.Provider == "" || in.Provider == cfg.Provider) {
		result = append(result, aiModelProjection(ports.AIProviderModel{
			ID: cfg.Model, Provider: cfg.Provider, ModelName: cfg.Model,
			Description: &description, DiscoveredAt: time.Time{},
		}))
	}
	s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "SUCCESS", "",
		map[string]any{"source": "deployment_configuration", "model_count": len(result)})
	out := &contract.List[dto.AIModelView]{}
	out.Body.Data = result
	return out, true
}

func (s *Server) platformGatewayGetAIConfiguration(_ context.Context) (any, bool) {
	cfg := s.platformDeps.NativeAISettings
	if cfg == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "native AI settings are unavailable")), true
	}
	status := "INACTIVE"
	if cfg.Configured {
		status = "ACTIVE"
	}
	out := &contract.Single[dto.AIConfigurationView]{}
	out.Body.Data = dto.AIConfigurationView{
		Provider: "native", Model: cfg.Model,
		CustomerModel: cfg.CustomerModel, CatalogModel: cfg.CatalogModel, MerchantModel: cfg.MerchantModel,
		ConfigurationSource: "environment", CredentialID: "", CredentialStatus: status,
		MujeebMaxOutputTokens: cfg.MaxOutputTokens, EffectiveMaxOutput: cfg.MaxOutputTokens,
		MujeebMaxInputChars: cfg.MaxInputCharacters, Status: status,
	}
	return out, true
}

func (s *Server) platformGatewayUpdateAIConfiguration(_ context.Context, _ *dto.UpdateConfigurationInput) (any, bool) {
	return mapApplicationError(appErrors.New(appErrors.CodeConflict,
		"Inference provider model selection is deployment-owned; database model switching is disabled")), true
}
