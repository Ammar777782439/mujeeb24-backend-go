package bootstrap

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func BuildDependencies(adapter *postgres.Adapter) handlers.Dependencies {
	businessRepository := postgres.NewBusinessRepository(adapter)
	businessManagement := services.BusinessManagementService{Repository: businessRepository, Transactions: adapter}
	dashboardRepository := postgres.NewDashboardRepository(adapter)
	customerRepository := postgres.NewCustomerRepository(adapter)
	conversationRepository := postgres.NewConversationRepository(adapter)
	conversationLabelRepository := postgres.NewConversationLabelRepository(adapter)
	conversationReferenceRepository := postgres.NewConversationReferenceRepository(adapter)
	customerRuntime := services.CustomerRuntimeService{Repository: customerRepository, Transactions: adapter}
	catalogRepository := postgres.NewCatalogRepository(adapter)
	catalogCommands := services.NewCatalogCommandServices(catalogRepository, adapter)

	leadRepository := postgres.NewLeadRepository(adapter)
	transactionRepository := postgres.NewTransactionRepository(adapter)
	salesCommands := services.NewSalesCommandServices(leadRepository, transactionRepository, adapter)

	decisionRepository := postgres.NewAIDecisionRepository(adapter)
	auditRepository := postgres.NewAuditEventRepository(adapter)
	messageRepository := postgres.NewMessageRepository(adapter)
	conversationRuntime := services.ConversationRuntimeService{Repository: conversationRepository, Reader: conversationRepository, Labels: conversationLabelRepository, References: conversationReferenceRepository, Messages: messageRepository, Transactions: adapter}
	manualOutbound := services.ManualOutboundMessageService{References: conversationReferenceRepository, Connections: postgres.NewChannelConnectionRepository(adapter), Outbound: postgres.NewOutboundMessageRepository(adapter), Outbox: postgres.NewPostgresOutboxStore(adapter), Transactions: adapter}
	channelConnectionRepository := postgres.NewChannelConnectionRepository(adapter)
	channelRuntime := services.ChannelRuntimeService{Reader: channelConnectionRepository, Runtime: channelConnectionRepository, Transactions: adapter}
	channelCapabilityRepository := postgres.NewChannelCapabilityRepository(adapter)

	return handlers.Dependencies{
		GetLiveness:               livenessQueryService{},
		GetReadiness:              readinessQueryService{Ping: adapter.Ping},
		GetMetrics:                metricsQueryService{TotalConns: adapter.Pool().Stat().TotalConns, AcquiredConns: adapter.Pool().Stat().AcquiredConns, IdleConns: adapter.Pool().Stat().IdleConns},
		GetBusiness:               services.GetBusinessQueryService{Repository: businessRepository},
		GetBusinessPolicy:         services.BusinessPolicyQueryService{BusinessManagementService: businessManagement},
		UpdateBusinessProfile:     services.UpdateBusinessProfileService{BusinessManagementService: businessManagement},
		UpdateBusinessPolicy:      services.UpdateBusinessPolicyService{BusinessManagementService: businessManagement},
		GetDashboardOverview:      services.DashboardOverviewQueryService{Repository: dashboardRepository},
		GetCustomer:               services.GetCustomerQueryService{Repository: customerRepository},
		ListCustomers:             services.ListCustomersQueryService{CustomerRuntimeService: customerRuntime},
		CreateCustomer:            services.CreateCustomerCommandService{CustomerRuntimeService: customerRuntime},
		UpdateCustomer:            services.UpdateCustomerCommandService{CustomerRuntimeService: customerRuntime},
		MergeCustomer:             services.MergeCustomerCommandService{CustomerRuntimeService: customerRuntime},
		ListConversations:         services.ListConversationsQueryService{ConversationRuntimeService: conversationRuntime},
		ListCustomerConversations: services.ListCustomerConversationsQueryService{ConversationRuntimeService: conversationRuntime},
		UpdateConversation:        services.UpdateConversationCommandService{ConversationRuntimeService: conversationRuntime},
		AssignConversation:        services.AssignConversationCommandService{ConversationRuntimeService: conversationRuntime},
		UpdateConversationLabels:  services.UpdateConversationLabelsCommandService{ConversationRuntimeService: conversationRuntime},
		AddPrivateNote:            services.AddPrivateNoteCommandService{ConversationRuntimeService: conversationRuntime},
		GetConversation:           services.GetConversationQueryService{Repository: conversationRepository, Labels: conversationLabelRepository},

		ListConversationMessages: services.MessageQueryService{Repository: messageRepository},
		CreateOutboundMessage:    manualOutbound,
		GetConnectionCapabilities: services.ConnectionCapabilitiesQueryService{
			Repository: channelCapabilityRepository,
		},
		ListChannelConnections: services.ListChannelConnectionsQueryService{ChannelRuntimeService: channelRuntime},
		GetChannelConnection:   services.GetChannelConnectionQueryService{ChannelRuntimeService: channelRuntime},
		ReconnectChannel:       services.ReconnectChannelCommandService{ChannelRuntimeService: channelRuntime},
		DisconnectChannel:      services.DisconnectChannelCommandService{ChannelRuntimeService: channelRuntime},

		CreateCatalog:                services.CreateCatalogCommandService{CatalogCommandServices: catalogCommands},
		UpdateCatalog:                services.UpdateCatalogCommandService{CatalogCommandServices: catalogCommands},
		CreateAttributeSchemaVersion: services.CreateAttributeSchemaVersionCommandService{CatalogCommandServices: catalogCommands},
		CreateCatalogItem:            services.CreateCatalogItemCommandService{CatalogCommandServices: catalogCommands},
		UpdateCatalogItem:            services.UpdateCatalogItemCommandService{CatalogCommandServices: catalogCommands},
		CreateOffer:                  services.CreateOfferCommandService{CatalogCommandServices: catalogCommands},
		UpdateOffer:                  services.UpdateOfferCommandService{CatalogCommandServices: catalogCommands},
		CreateVariant:                services.CreateVariantCommandService{CatalogCommandServices: catalogCommands},
		UpdateVariant:                services.UpdateVariantCommandService{CatalogCommandServices: catalogCommands},

		ListCatalogs:         services.ListCatalogsQueryService{Repository: catalogRepository},
		GetCatalog:           services.GetCatalogQueryService{Repository: catalogRepository},
		ListCatalogItems:     services.ListCatalogItemsQueryService{Repository: catalogRepository},
		GetCatalogItem:       services.GetCatalogItemQueryService{Repository: catalogRepository},
		ListOffers:           services.ListOffersQueryService{Repository: catalogRepository},
		ListVariants:         services.ListVariantsQueryService{Repository: catalogRepository},
		ListAttributeSchemas: services.ListAttributeSchemasQueryService{Repository: catalogRepository},
		GetAttributeSchema:   services.GetAttributeSchemaQueryService{Repository: catalogRepository},

		CreateLead:               services.CreateLeadCommandService{SalesCommandServices: salesCommands},
		UpdateLead:               services.UpdateLeadCommandService{SalesCommandServices: salesCommands},
		QualifyLead:              services.QualifyLeadCommandService{SalesCommandServices: salesCommands},
		MarkLeadLost:             services.MarkLeadLostCommandService{SalesCommandServices: salesCommands},
		CreateTransactionDraft:   services.CreateTransactionDraftCommandService{SalesCommandServices: salesCommands},
		UpdateTransactionDraft:   services.UpdateTransactionDraftCommandService{SalesCommandServices: salesCommands},
		ConfirmTransaction:       services.ConfirmTransactionCommandService{SalesCommandServices: salesCommands},
		CancelTransaction:        services.CancelTransactionCommandService{SalesCommandServices: salesCommands},
		SubmitTransactionReview:  services.SubmitTransactionReviewCommandService{SalesCommandServices: salesCommands},
		ApproveTransactionReview: services.ApproveTransactionReviewCommandService{SalesCommandServices: salesCommands},
		RejectTransactionReview:  services.RejectTransactionReviewCommandService{SalesCommandServices: salesCommands},

		ListLeads:                services.ListLeadsQueryService{Repository: leadRepository},
		GetLead:                  services.GetLeadQueryService{Repository: leadRepository},
		ListLeadAttributions:     services.ListLeadAttributionsQueryService{Repository: leadRepository},
		ListLeadScores:           services.ListLeadScoresQueryService{Repository: leadRepository},
		ListTransactions:         services.ListTransactionsQueryService{Repository: transactionRepository},
		ListCustomerTransactions: services.ListCustomerTransactionsQueryService{Repository: transactionRepository},
		GetTransaction:           services.GetTransactionQueryService{Repository: transactionRepository},
		GetTransactionReview:     services.GetTransactionReviewQueryService{Repository: transactionRepository},

		RequestHumanReview: services.NewRequestHumanReviewCommandService(decisionRepository, auditRepository, adapter),
		ListAIDecisions:    services.ListAIDecisionsQueryService{Repository: decisionRepository},
		GetAIDecision:      services.GetAIDecisionQueryService{Repository: decisionRepository},
		ListAuditEvents:    services.ListAuditEventsQueryService{Repository: auditRepository},
		GetAuditEvent:      services.GetAuditEventQueryService{Repository: auditRepository},
	}
}
