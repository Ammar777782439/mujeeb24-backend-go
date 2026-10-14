package bootstrap

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func wireMerchantCatalogAuthoringAI(
	dashboardServer *handlers.Server,
	database *postgres.Adapter,
	authoring merchantcatalogai.MerchantCatalogAuthoringPort,
) (*handlers.Server, error) {
	if authoring == nil {
		return dashboardServer, nil
	}

	catalogRepo := postgres.NewCatalogRepository(database)
	merchantCatalogAuthoringAgent := &merchantcatalogai.Agent{
		Sessions:       postgres.NewMerchantCatalogAISessionStore(database),
		Selector:       merchantcatalogai.DeterministicCatalogSelector{Repository: catalogRepo},
		Business:       postgres.NewBusinessRepository(database),
		EntityContract: merchantcatalogai.CanonicalEntityContractProvider{},
		Authoring:      authoring,
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
