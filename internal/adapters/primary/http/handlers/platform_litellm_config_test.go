package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestGatewayPlatformAISettingsUseRuntimeAliasesNotLegacyGeminiCredentials(t *testing.T) {
	modelsCalls, probeCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer runtime-virtual-key" || r.Header.Get("x-goog-api-key") != "" {
			t.Errorf("gateway secret not sent with bearer authentication")
		}
		switch r.URL.Path {
		case "/v1/models":
			modelsCalls++
			_, _ = fmt.Fprint(w, `{"data":[{"id":"mujeeb-customer"},{"id":"mujeeb-catalog"},{"id":"mujeeb-merchant"}]}`)
		case "/v1beta/interactions":
			probeCalls++
			_, _ = fmt.Fprint(w, `{"status":"completed"}`)
		default:
			t.Errorf("unexpected LiteLLM settings request %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{
		ModelDiscovery: litellm.NewModelsClient(),
		LiteLLMSettings: &ports.LiteLLMPlatformConfig{
			BaseURL: server.URL, APIKey: "runtime-virtual-key",
			CustomerModel: "mujeeb-customer", CatalogModel: "mujeeb-catalog", MerchantModel: "mujeeb-merchant",
			MaxOutputTokens: 4096, MaxInputCharacters: 48000,
		},
	})
	ctx := context.Background()
	rawConfig, ok := s.platformGetAIConfiguration(ctx)
	if !ok {
		t.Fatal("get configuration not handled")
	}
	conf := rawConfig.(*contract.Single[dto.AIConfigurationView]).Body.Data
	if conf.Provider != "litellm" || conf.CustomerModel != "mujeeb-customer" || conf.CatalogModel != "mujeeb-catalog" ||
		conf.MerchantModel != "mujeeb-merchant" || conf.ConfigurationSource != "environment" ||
		conf.Model != "mujeeb-customer" || conf.CredentialID != "" {
		t.Fatalf("platform reported a legacy provider or fake credential: %+v", conf)
	}
	rawProviders, ok := s.platformListAIProvidersConfig(ctx)
	if !ok {
		t.Fatal("providers not handled")
	}
	providers := rawProviders.(*contract.List[dto.AICredentialView]).Body.Data
	if len(providers) != 1 || providers[0].Provider != "litellm" || providers[0].KeyHint != "" {
		t.Fatalf("legacy credential leaked: %+v", providers)
	}
	rawModels, ok := s.platformDiscoverAIModels(ctx, &dto.DiscoverModelsInput{Provider: "litellm"})
	if !ok {
		t.Fatal("discovery not handled")
	}
	models := rawModels.(*contract.List[dto.AIModelView]).Body.Data
	if len(models) != 3 || modelsCalls != 1 {
		t.Fatalf("gateway alias discovery not working: calls=%d models=%+v", modelsCalls, models)
	}
	rawConnection, ok := s.platformTestAIConnection(ctx, &dto.TestConnectionInput{Provider: "litellm"})
	if !ok {
		t.Fatal("connection test not handled")
	}
	conn := rawConnection.(*contract.Single[dto.TestConnectionResult]).Body.Data
	if !conn.Success || conn.Provider != "litellm" || conn.Model != "mujeeb-customer" || probeCalls != 1 {
		t.Fatalf("platform did not probe gateway runtime: %+v", conn)
	}
	// Admin form writes must not claim to change environment-owned routes.
	if raw, ok := s.platformAddAICredential(ctx, &dto.AddCredentialInput{}); !ok || raw == nil {
		t.Fatal("legacy credential write must be explicitly rejected")
	}
	if raw, ok := s.platformUpdateAIConfiguration(ctx, &dto.UpdateConfigurationInput{}); !ok || raw == nil {
		t.Fatal("legacy DB-only activation must be explicitly rejected")
	}
	if modelsCalls != 1 || probeCalls != 1 {
		t.Fatal("credential or model mutation unexpectedly called the upstream provider")
	}
}

func TestGatewayPlatformProviderRejectsGeminiAlias(t *testing.T) {
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{
		ModelDiscovery: litellm.NewModelsClient(),
		LiteLLMSettings: &ports.LiteLLMPlatformConfig{BaseURL: "http://localhost:4000", APIKey: "test", CustomerModel: "mujeeb-customer"},
	})
	for _, provider := range []string{"google_gemini", "openai"} {
		m, ok := s.platformDiscoverAIModels(context.Background(), &dto.DiscoverModelsInput{Provider: provider})
		if !ok || m == nil {
			t.Fatalf("legacy discovery provider %s not rejected", provider)
		}
		r, ok := s.platformTestAIConnection(context.Background(), &dto.TestConnectionInput{Provider: provider})
		if !ok || r == nil {
			t.Fatalf("legacy probe provider %s not rejected", provider)
		}
	}
}

func TestGatewayDiscoveryFailureDoesNotReturnFakeEmptyModelList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "PRIVATE_GATEWAY_ERROR")
	}))
	defer srv.Close()
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{ModelDiscovery: litellm.NewModelsClient(),
		LiteLLMSettings: &ports.LiteLLMPlatformConfig{BaseURL: srv.URL, APIKey: "a-secret", CustomerModel: "mujeeb-customer"},
	})
	raw, ok := s.platformDiscoverAIModels(context.Background(), &dto.DiscoverModelsInput{Provider: "litellm"})
	if !ok || raw == nil {
		t.Fatal("gateway discovery error not handled")
	}
	if strings.Contains(fmt.Sprint(raw), "PRIVATE_GATEWAY_ERROR") || strings.Contains(fmt.Sprint(raw), "a-secret") {
		t.Fatal("gateway discovery leaked private upstream content")
	}
}
