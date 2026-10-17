package handlers

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"testing"
)

func TestNativePlatformConfigurationShowsRealProvidersWithoutSecrets(t *testing.T) {
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{NativeAISettings: &ports.NativeAIPlatformConfig{
		GeminiModel: "gemini-2.5-flash", DeepSeekModel: "deepseek-flash",
		CustomerModel: "mujeeb-sales", CatalogModel: "mujeeb-catalog", MerchantModel: "mujeeb-merchant",
		GeminiConfigured: true, DeepSeekConfigured: true, MaxOutputTokens: 4096, MaxInputCharacters: 48000,
	}})
	ctx := context.Background()
	result, ok := s.platformGetAIConfiguration(ctx)
	if !ok {
		t.Fatal("platform configuration unhandled")
	}
	cfg := result.(*contract.Single[dto.AIConfigurationView]).Body.Data
	if cfg.Provider != "native" || cfg.Model != "gemini-2.5-flash" || cfg.CatalogModel != "mujeeb-catalog" || cfg.MerchantModel != "mujeeb-merchant" {
		t.Fatalf("native provider and aliases mismatch: %+v", cfg)
	}
	raw, ok := s.platformListAIProvidersConfig(ctx)
	if !ok {
		t.Fatal("provider list unhandled")
	}
	providers := raw.(*contract.List[dto.AICredentialView]).Body.Data
	if len(providers) != 2 || providers[0].KeyHint != "" || providers[1].KeyHint != "" {
		t.Fatalf("provider secrets exposed or backup omitted: %+v", providers)
	}
	for _, provider := range []string{"google_gemini", "deepseek"} {
		models, ok := s.platformDiscoverAIModels(ctx, &dto.DiscoverModelsInput{Provider: provider})
		if !ok || len(models.(*contract.List[dto.AIModelView]).Body.Data) != 1 {
			t.Fatalf("deployment model %s not reported", provider)
		}
		probe, ok := s.platformTestAIConnection(ctx, &dto.TestConnectionInput{Provider: provider})
		if !ok || probe == nil {
			t.Fatalf("missing honest unavailable probe for %s", provider)
		}
	}
	if result, ok := s.platformAddAICredential(ctx, &dto.AddCredentialInput{}); !ok || result == nil {
		t.Fatal("provider credential upload did not fail closed")
	}
	if result, ok := s.platformUpdateAIConfiguration(ctx, &dto.UpdateConfigurationInput{}); !ok || result == nil {
		t.Fatal("provider model mutation did not fail closed")
	}
}

func TestNativePlatformConfigurationGeminiOnlyIsActive(t *testing.T) {
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{NativeAISettings: &ports.NativeAIPlatformConfig{
		GeminiModel: "gemini-2.5-flash", DeepSeekModel: "deepseek-chat",
		GeminiConfigured: true, DeepSeekConfigured: false,
		CustomerModel: "mujeeb-sales", CatalogModel: "mujeeb-catalog",
		MerchantModel: "mujeeb-merchant",
	}})
	ctx := context.Background()
	out, ok := s.platformGetAIConfiguration(ctx)
	if !ok { t.Fatal("Gemini-only configuration was not handled") }
	cfg := out.(*contract.Single[dto.AIConfigurationView]).Body.Data
	if cfg.Status != "ACTIVE" || cfg.CredentialStatus != "ACTIVE" {
		t.Fatalf("Gemini-only runtime incorrectly shown as inactive: %+v", cfg)
	}
	list, ok := s.platformListAIProvidersConfig(ctx)
	if !ok { t.Fatal("Gemini-only provider list was not handled") }
	providers := list.(*contract.List[dto.AICredentialView]).Body.Data
	if len(providers) != 1 || providers[0].Provider != "google_gemini" {
		t.Fatalf("unconfigured DeepSeek must not appear as active credential: %+v", providers)
	}
	models, ok := s.platformDiscoverAIModels(ctx, &dto.DiscoverModelsInput{})
	if !ok || len(models.(*contract.List[dto.AIModelView]).Body.Data) != 1 {
		t.Fatal("unconfigured standby model advertised as configured")
	}
}
