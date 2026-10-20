package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/google/uuid"
)

// One canonical AutoReplyService configuration for both API and durable worker.
// The worker never opens an AI transaction around the external provider call.
func buildCustomerAutoReplyService(database *postgres.Adapter, outboxStore ports.OutboxStore,
	external ExternalAdapters, platformOperations *services.InMemoryPlatformOperationsRepository,
	realtimeBroker ports.RealtimePublisher) (services.AutoReplyService,error) {
		// One customer-sales decision interface owns B2C and summary requests.
		// Native provider selection is isolated from Customer Sales business decisions.
		customerSalesDecision := external.NativeCustomerDecision
		if customerSalesDecision == nil {
			return services.AutoReplyService{}, errors.New("AutoReply requires a configured native customer-sales decision provider")
		}
		referenceRepository := postgres.NewConversationReferenceRepository(database)
		service := services.NewAutoReplyService(
			customerSalesDecision,
			postgres.NewAIDecisionRepository(database),
			referenceRepository,
			postgres.NewOutboundMessageRepository(database),
			outboxStore,
			database,
		)
		// Both AI routes use the same canonical run trace lifecycle.
		service.RunRepository = postgres.NewAIRunTraceRepository(database)
		// Per AIUsageTokenTelemetry.md §6: wire the telemetry pipeline so
		// every model request's tokens + cost are recorded to ai_usage_records
		// and flow through to the Platform Admin AI Usage view.
		service.AIUsage = postgres.NewAIUsageRepository(database)

		service.Subscriptions = postgres.NewSubscriptionRepository(database)
		// Per contract ⑤ §7, build the Catalog Entity Contract payload
		// once and reuse for every call.
		entityContract := services.BuildCatalogEntityContractPayload()
		payloadBytes, err := json.Marshal(entityContract)
		if err != nil {
			return services.AutoReplyService{}, fmt.Errorf("marshal catalog entity contract: %w", err)
		}
		service.EntityContractPayload = payloadBytes
		contextBuilder := services.NewAutoReplyContextBuilder(
			postgres.NewBusinessRepository(database),
			postgres.NewConversationRepository(database),
			postgres.NewCustomerRepository(database),
			postgres.NewCatalogRepository(database),
			postgres.NewMessageRepository(database),
		)
		// Universal Catalog AI v3: shared read-only catalog boundary.
		// It supplies the bounded manifest and complete bulk projection pages.
		contextBuilder.CatalogAI = postgres.NewCatalogAIReadRepository(database)
		contextBuilder.Knowledge = postgres.NewKnowledgeDocumentRepository(database)
		contextBuilder.Policies = postgres.NewBusinessPolicyRepository(database)
		service.CustomerSalesContextBuilder = contextBuilder
		// Summaries use the same configured provider router as AutoReply,
		// but a generation-only port, never a customer-sales tool decision.
		// Fallback belongs to native transport, not this application.
		service.SummaryService = services.NewConversationSummaryService(
			postgres.NewConversationStateRepository(database),
			postgres.NewMessageRepository(database),
			customerSalesDecision,
		)
		service.SummaryService.AIUsage = postgres.NewAIUsageRepository(database)
		service.SummaryService.GatewayFXRateYER = service.GatewayFXRateYER
		service.SummaryService.Subscriptions = postgres.NewSubscriptionRepository(database)
		service.SummaryService.NewID = uuid.NewString
		// Per Item 2 (Kill Switch applies to Summary): wire the
		// shared AICostProtectionService so MaybeSummarize
		// respects platformAIDisable. Same shared
		// platformOperations instance — one kill switch state,
		// three enforcement points (webhook AutoReply + Merchant
		// AI + Summary).
		service.SummaryService.CostProtection = &services.AICostProtectionService{
			Subscriptions:      postgres.NewSubscriptionRepository(database),
			AIUsage:            postgres.NewAIUsageRepository(database),
			PlatformOperations: platformOperations,
		}
		// Per contract ⑥ §2, wire the validation pipeline with the
		// concrete Postgres validators. Per contract ⑥ §6-7 the
		// ReferenceValidator checks that every selected ID (item/variant/
		// offer) was actually sent to Gemini as evidence AND exists as
		// a real row in catalog_items/variants/offers. Per contract ⑥ §8-9
		// the TenantValidator checks that every selected reference
		// belongs to the current authenticated business (cross-tenant
		// protection).
		//
		// Per contract ⑥ §12-13, the PolicyEvaluator is the
		// Postgres-backed evaluator that reads business_policies per
		// migration 000002 and decides requires_approval — NOT Gemini.
		// Per contract ④ §5, requires_approval is decided by Mujeeb only.
		//
		// The AuthorizationService is left nil; per the pipeline logic,
		// nil Authorization means the PolicyDecision is treated as final
		// (allowed → proceed, requires_approval → wait for human,
		// denied → no execution).
		service.Validation = services.NewValidationPipeline(
			postgres.NewPostgresReferenceValidator(database),
			postgres.NewPostgresTenantValidator(database),
			postgres.NewPostgresCustomerSalesPolicyEvaluator(postgres.NewBusinessRepository(database)),
			nil, // AuthorizationService: nil means PolicyDecision is final
		)
		sales := buildSalesWorkflowService(database, outboxStore)
		if external.CustomerSalesEnabled {
			service.Sales = &sales
			contextBuilder.Sales = sales.Repository
		}
		service.DisableInformationalReplies = !external.AutoReplyEnabled
		service.CustomerSalesContextBuilder = contextBuilder
		service.StateRepository = postgres.NewConversationStateRepository(database)
		service.MessageRepository = postgres.NewMessageRepository(database)
		service.Conversations = postgres.NewConversationRepository(database)
		service.Realtime = realtimeBroker

		// Per contract ② §9, wire the CatalogBatchController into the
		// AutoReply flow. Full catalog evaluation is invoked when the
		// initial proposal needs more data, returns not_found before
		// complete coverage, or selects references outside current evidence:
		// page catalog → exact-token batches → evaluate → aggregate → final.
		//
		// Per contract ② §2, batching retains full commercial evidence.
		// Native transport uses a conservative minimum context to budget catalog batches.
		if external.NativeCatalogClient == nil {
			return services.AutoReplyService{}, errors.New("AutoReply requires a configured native catalog model")
		}
		catalogBudget, err := external.NativeCatalogClient.BatchTokenBudget()
		if err != nil {
			return services.AutoReplyService{}, fmt.Errorf("configure native catalog token budget: %w", err)
		}
		service.CatalogBatch = &services.CatalogBatchController{
			CatalogAI:   postgres.NewCatalogAIReadRepository(database),
			Evaluator:   external.NativeCatalogClient,
			RunRepo:     postgres.NewAIRunTraceRepository(database),
			TokenBudget: catalogBudget,
			Now:         func() time.Time { return time.Now().UTC() },
			NewID:       uuid.NewString,
			AIUsage:     postgres.NewAIUsageRepository(database),

			Subscriptions: postgres.NewSubscriptionRepository(database),
		}

		service.CatalogBatch.GatewayFXRateYER = service.GatewayFXRateYER
		return service,nil
}
