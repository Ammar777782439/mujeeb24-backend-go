package bootstrap

import (
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func wireMerchantCatalogAuthoringAI(
	dashboardServer *handlers.Server,
	database *postgres.Adapter,
	geminiHTTPClient *gemini.GeminiHTTPClient,
	configProvider ports.AIConfigurationProvider,
	merchantOverride merchantcatalogai.MerchantCatalogAuthoringPort,
) (*handlers.Server, error) {
	if geminiHTTPClient == nil && merchantOverride == nil {
		return dashboardServer, nil
	}

	var merchantCatalogAuthoring merchantcatalogai.MerchantCatalogAuthoringPort = merchantOverride
	if merchantCatalogAuthoring == nil {
		geminiAuthoring, err := gemini.NewGeminiMerchantCatalogAuthoringAdapter(geminiHTTPClient)
		if err != nil {
			return nil, fmt.Errorf("build Gemini merchant catalog authoring adapter: %w", err)
		}
		geminiAuthoring.SetConfigurationProvider(configProvider)
		merchantCatalogAuthoring = geminiAuthoring
	}

	catalogRepo := postgres.NewCatalogRepository(database)
	merchantCatalogAuthoringAgent := &merchantcatalogai.Agent{
		Sessions:       postgres.NewMerchantCatalogAISessionStore(database),
		Selector:       merchantcatalogai.DeterministicCatalogSelector{Repository: catalogRepo},
		Business:       postgres.NewBusinessRepository(database),
		EntityContract: merchantcatalogai.CanonicalEntityContractProvider{},
		Authoring:      merchantCatalogAuthoring,
		CapabilitiesFactory: func(selectedCatalogID string) merchantcatalogai.MerchantCatalogDiscoveryPort {
			return merchantcatalogai.NewReadOnlyCapabilityRegistry(catalogRepo, selectedCatalogID)
		},
	}

	handler := handlers.NewMerchantCatalogAIV2Handler(merchantCatalogAuthoringAgent)
	handler.Execution = &merchantcatalogai.ExecutionService{Catalogs: catalogRepo, Transactions: database, Store: postgres.MerchantAIExecutionStore{Adapter: database}, Sessions: merchantCatalogAuthoringAgent.Sessions}
	return dashboardServer.WithMerchantCatalogAIV2(
		handlers.MerchantCatalogAIV2Deps{
			Handler: handler,
		},
	), nil
}
