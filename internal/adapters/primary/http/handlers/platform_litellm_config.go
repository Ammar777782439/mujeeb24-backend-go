package handlers

import (
	"context"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

// Platform AI configuration is currently deployment-owned in LiteLLM gateway
// mode. The actual three runtime aliases are configured via LITELLM_* env,
// not the legacy google_gemini configuration table. The admin APIs MUST NOT
// claim to rotate a key or activate a model that the running adapters ignore.

func (s *Server) platformGatewayListAIProvidersConfig(_ context.Context) (any, bool) {
	cfg := s.platformDeps.LiteLLMSettings
	credentials := []dto.AICredentialView{}
	if cfg != nil && strings.TrimSpace(cfg.APIKey) != "" {
		credentials = append(credentials, dto.AICredentialView{
			ID: "litellm-environment", Provider: "litellm",
			DisplayName: "LiteLLM gateway (environment-managed)",
			KeyHint: "", Status: "CONFIGURED",
			CreatedAt: "",
		})
	}
	out := &contract.List[dto.AICredentialView]{}
	out.Body.Data = credentials
	return out, true
}

func (s *Server) platformGatewayAddAICredential(_ context.Context, _ *dto.AddCredentialInput) (any, bool) {
	// Accepting and storing a key here would be misleading: the runtime and
	// gateway use LITELLM_API_KEY, not credentials in the Gemini config DB.
	return mapApplicationError(appErrors.New(appErrors.CodeConflict,
		"LiteLLM virtual keys are managed in the gateway and LITELLM_API_KEY deployment setting; credential rotation through this endpoint is disabled")), true
}

func (s *Server) platformGatewayTestAIConnection(ctx context.Context, in *dto.TestConnectionInput) (any, bool) {
	cfg := s.platformDeps.LiteLLMSettings
	if cfg == nil || s.platformDeps.ModelDiscovery == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "LiteLLM discovery not configured")), true
	}
	if in.Provider != "" && in.Provider != "litellm" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "provider must be litellm")), true
	}
	// The current admin contract tests the primary customer route.
	// Merchant and catalog aliases are separately visible in configuration
	// and can be verified by their real workloads. This probe proves the
	// gateway route works, not that all deployed aliases share capabilities.
	success, latency, code := s.platformDeps.ModelDiscovery.TestConnection(ctx, cfg.APIKey, cfg.CustomerModel, cfg.BaseURL)
	status := "SUCCESS"
	if !success {
		status = "FAILURE"
	}
	s.appendPlatformAudit(ctx, "ai.connection.tested", "ai_runtime", nil, nil, status, code,
		map[string]any{"provider": "litellm", "model": cfg.CustomerModel, "latency_ms": latency})
	out := &contract.Single[dto.TestConnectionResult]{}
	out.Body.Data = dto.TestConnectionResult{
		Success: success, Provider: "litellm", Model: cfg.CustomerModel,
		LatencyMS: latency, ErrorCode: code,
	}
	return out, true
}

func (s *Server) platformGatewayDiscoverAIModels(ctx context.Context, in *dto.DiscoverModelsInput) (any, bool) {
	cfg := s.platformDeps.LiteLLMSettings
	if cfg == nil || s.platformDeps.ModelDiscovery == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "LiteLLM discovery not configured")), true
	}
	if in.Provider != "" && in.Provider != "litellm" {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "provider must be litellm")), true
	}
	models, err := s.platformDeps.ModelDiscovery.DiscoverModels(ctx, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		// Never log a credential or raw upstream error body.
		s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "FAILURE", "LITELLM_DISCOVERY_FAILED",
			map[string]any{"provider": "litellm"})
		return mapApplicationError(appErrors.New(appErrors.CodeConflict,
			"LiteLLM model discovery failed; check gateway URL, virtual-key permissions and connectivity")), true
	}
	// Read the gateway's current deployed aliases on every request. Do NOT
	// write to the legacy Gemini activation table, which is not authoritative.
	views := make([]dto.AIModelView, 0, len(models))
	for _, model := range models {
		views = append(views, aiModelProjection(model))
	}
	s.appendPlatformAudit(ctx, "ai.models.discovered", "ai_runtime", nil, nil, "SUCCESS", "",
		map[string]any{"provider": "litellm", "model_count": len(models)})
	out := &contract.List[dto.AIModelView]{}
	out.Body.Data = views
	return out, true
}

func (s *Server) platformGatewayGetAIConfiguration(_ context.Context) (any, bool) {
	cfg := s.platformDeps.LiteLLMSettings
	if cfg == nil {
		return mapApplicationError(appErrors.New(appErrors.CodeNotImplemented, "LiteLLM settings unavailable")), true
	}
	out := &contract.Single[dto.AIConfigurationView]{}
	out.Body.Data = dto.AIConfigurationView{
		Provider: "litellm", Model: cfg.CustomerModel,
		CustomerModel: cfg.CustomerModel, CatalogModel: cfg.CatalogModel,
		MerchantModel: cfg.MerchantModel, ConfigurationSource: "environment",
		CredentialID: "", CredentialStatus: "CONFIGURED",
		MujeebMaxOutputTokens: cfg.MaxOutputTokens,
		EffectiveMaxOutput: cfg.MaxOutputTokens,
		MujeebMaxInputChars: cfg.MaxInputCharacters, Status: "ACTIVE",
	}
	return out, true
}

func (s *Server) platformGatewayUpdateAIConfiguration(_ context.Context, _ *dto.UpdateConfigurationInput) (any, bool) {
	// Never write a legacy google_gemini version that cannot change the
	// running LiteLLM adapters. Real route changes require deployment /
	// gateway configuration and deliberate runtime reload support.
	return mapApplicationError(appErrors.New(appErrors.CodeConflict,
		"LiteLLM model routes are deployment-managed. Update the LITELLM_CUSTOMER_MODEL, LITELLM_CATALOG_MODEL and LITELLM_MERCHANT_MODEL settings and restart the API; database-only activation cannot change live routes")), true
}

