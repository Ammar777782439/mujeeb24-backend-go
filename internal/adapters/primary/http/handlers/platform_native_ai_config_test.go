package handlers

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestNativePlatformShowsOnlyActiveDeploymentProviderWithoutSecrets(t *testing.T) {
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{NativeAISettings: &ports.NativeAIPlatformConfig{
		Provider: "deepseek", Model: "deepseek-flash", Configured: true,
		CustomerModel: "mujeeb-sales", CatalogModel: "mujeeb-catalog", MerchantModel: "mujeeb-merchant",
		MaxOutputTokens: 4096, MaxInputCharacters: 48000,
	}})
	ctx := context.Background()
	result, ok := s.platformGetAIConfiguration(ctx)
	if !ok {
		t.Fatal("configuration unhandled")
	}
	cfg := result.(*contract.Single[dto.AIConfigurationView]).Body.Data
	if cfg.Status != "ACTIVE" || cfg.Model != "deepseek-flash" ||
		cfg.CatalogModel != "mujeeb-catalog" || cfg.MerchantModel != "mujeeb-merchant" {
		t.Fatalf("active provider not reflected: %+v", cfg)
	}
	list, ok := s.platformListAIProvidersConfig(ctx)
	if !ok {
		t.Fatal("provider list unhandled")
	}
	providers := list.(*contract.List[dto.AICredentialView]).Body.Data
	if len(providers) != 1 || providers[0].Provider != "deepseek" || providers[0].KeyHint != "" {
		t.Fatalf("provider secrets exposed or provider missing: %+v", providers)
	}
	models, ok := s.platformDiscoverAIModels(ctx, &dto.DiscoverModelsInput{Provider: "deepseek"})
	if !ok || len(models.(*contract.List[dto.AIModelView]).Body.Data) != 1 {
		t.Fatal("selected provider model not reported")
	}
	if _, ok := s.platformTestAIConnection(ctx, &dto.TestConnectionInput{Provider: "deepseek"}); !ok {
		t.Fatal("missing honest unavailable probe")
	}
	if _, ok := s.platformAddAICredential(ctx, &dto.AddCredentialInput{}); !ok {
		t.Fatal("credential mutation must fail closed")
	}
	if _, ok := s.platformUpdateAIConfiguration(ctx, &dto.UpdateConfigurationInput{}); !ok {
		t.Fatal("model mutation must fail closed")
	}
}

func TestNativePlatformDisabledProviderIsInactive(t *testing.T) {
	s := &Server{}
	s.WithPlatformDeps(PlatformDeps{NativeAISettings: &ports.NativeAIPlatformConfig{
		Provider: "deepseek", Model: "deepseek-flash", Configured: false,
	}})
	ctx := context.Background()
	result, _ := s.platformGetAIConfiguration(ctx)
	cfg := result.(*contract.Single[dto.AIConfigurationView]).Body.Data
	if cfg.Status != "INACTIVE" || cfg.CredentialStatus != "INACTIVE" {
		t.Fatalf("unconfigured inference runtime reported active: %+v", cfg)
	}
	list, _ := s.platformListAIProvidersConfig(ctx)
	if len(list.(*contract.List[dto.AICredentialView]).Body.Data) != 0 {
		t.Fatal("unconfigured provider advertised as active")
	}
}
