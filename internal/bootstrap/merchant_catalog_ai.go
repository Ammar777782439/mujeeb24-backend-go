package bootstrap

import (
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func wireMerchantCatalogAIV2(
	dashboardServer *handlers.Server,
	database *postgres.Adapter,
	geminiClient *gemini.Client,
	configProvider ports.AIConfigurationProvider,
) (*handlers.Server, error) {
	if geminiClient == nil {
		return dashboardServer, nil
	}

	b2bRuntime, err := gemini.NewGeminiMerchantCatalogAuthoringAdapter(geminiClient)
	if err != nil {
		return nil, fmt.Errorf("build merchant catalog AI v2 runtime: %w", err)
	}
	b2bRuntime.SetConfigurationProvider(configProvider)

	catalogRepo := postgres.NewCatalogRepository(database)
	b2bAgent := &merchantcatalogai.Agent{
		Sessions:       postgres.NewMerchantCatalogAISessionStore(database),
		Selector:       merchantcatalogai.DeterministicCatalogSelector{Repository: catalogRepo},
		Business:       postgres.NewBusinessRepository(database),
		EntityContract: merchantcatalogai.CanonicalEntityContractProvider{},
		Authoring:      b2bRuntime,
		CapabilitiesFactory: func(selectedCatalogID string) ports.AICapabilityDispatcher {
			return merchantcatalogai.NewReadOnlyCapabilityRegistry(catalogRepo, selectedCatalogID)
		},
	}

	return dashboardServer.WithMerchantCatalogAIV2(
		handlers.MerchantCatalogAIV2Deps{
			Handler: handlers.NewMerchantCatalogAIV2Handler(b2bAgent),
		},
	), nil
}
