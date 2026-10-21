package bootstrap

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func wireMerchantCatalogAuthoringAI(
	dashboardServer *handlers.Server,
	database *postgres.Adapter,
	authoring merchantcatalogai.MerchantCatalogAuthoringPort,
	gatewaySettings ...*ports.NativeAIPlatformConfig,
) (*handlers.Server, error) {
	if authoring == nil {
		return dashboardServer, nil
	}

	// The native adapter records each model round; unknown provider prices are not fabricated.
	// A legacy/fake authoring implementation is not required to implement this.
	if len(gatewaySettings) != 0 && gatewaySettings[0] != nil {
		if reporter, ok := authoring.(interface {
			SetUsageReporter(func(context.Context, string, string, ports.CustomerSalesUsageTelemetry) error)
		}); ok {
			recorder := services.NativeMerchantUsageRecorder{
				Usage:         postgres.NewAIUsageRepository(database),
				Subscriptions: postgres.NewSubscriptionRepository(database),
				AIPricing:     postgres.NewAIProviderPricingRepository(database),
				FXRateYER:     "",
			}
			reporter.SetUsageReporter(recorder.Record)
		}
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
	handler.Execution = &merchantcatalogai.ExecutionService{Catalogs: catalogRepo, Transactions: database, Entitlements: postgres.NewSubscriptionRepository(database), Store: postgres.MerchantAIExecutionStore{Adapter: database}, Sessions: merchantCatalogAuthoringAgent.Sessions}
	return dashboardServer.WithMerchantCatalogAIV2(
		handlers.MerchantCatalogAIV2Deps{
			Handler: handler,
		},
	), nil
}
