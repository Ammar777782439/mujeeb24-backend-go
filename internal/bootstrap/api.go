package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	merchantgateway "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gateway"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/auth/ed25519jwt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	realtimePostgres "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/realtime/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type APIRuntime struct {
	HTTP                *http.Server
	Database            *postgres.Adapter
	Dependencies        handlers.Dependencies
	EventStore          ports.EventStore
	Outbox              ports.OutboxStore
	SocialAPI           ports.ChannelProvider
	SocialWebhook       ports.WebhookReceiver
	ChannelProvisioning *services.ChannelProvisioningService
	RealtimeBroker      *realtimePostgres.Broker
	RealtimeHub         *realtimePostgres.LocalHub
	// AutoReplyWorkerPool (optional) — per Item 5: when wired, the
	// webhook service uses it to bound AutoReply goroutine concurrency.
	// Shutdown() calls pool.Stop() to drain queued + in-flight tasks
	// before the HTTP server + database close. Without this, in-flight
	// AutoReply work would be dropped silently on shutdown.
	AutoReplyWorkerPool *services.AutoReplyWorkerPool
	closeOnce           sync.Once
}

type authenticationRuntime struct {
	Verifier   middleware.AccessTokenVerifier
	Repository *postgres.AuthenticationRepository
	Service    services.AuthenticationService
	// PlatformChecker verifies whether a principal is an active platform super
	// admin. Wired from the same authentication repository — the platform_super_admins
	// table is checked on every /api/v1/platform/* request via RequirePlatformAdminHuma.
	PlatformChecker middleware.PlatformSuperAdminChecker
}

func BuildAPI(ctx context.Context, cfg config.ProcessConfig) (*APIRuntime, error) {
	database, err := postgres.Open(ctx, cfg.DatabaseURL, postgres.PoolConfig{MaxConns: cfg.DBMaxConns, MinConns: cfg.DBMinConns, MaxConnLifetime: cfg.DBMaxConnLifetime, MaxConnIdleTime: cfg.DBMaxConnIdleTime, HealthCheckPeriod: cfg.DBHealthCheckPeriod, ConnectTimeout: cfg.DBConnectTimeout})
	if err != nil {
		return nil, err
	}
	authentication, err := buildAuthenticationRuntime(cfg, database)
	if err != nil {
		database.Close()
		return nil, err
	}
	runtime, err := newAPIWithExternalAndAuthentication(database, cfg.HTTPAddr, BuildExternalAdapters(cfg), authentication)
	if err != nil {
		database.Close()
		return nil, err
	}
	return runtime, nil
}

func newAPIWithExternalAndAuthentication(database *postgres.Adapter, address string, external ExternalAdapters, authentication *authenticationRuntime) (*APIRuntime, error) {
	if database == nil {
		return nil, errors.New("postgres adapter is required")
	}
	if external.LLMConfigError != nil {
		return nil, external.LLMConfigError
	}
	if address == "" {
		return nil, errors.New("http address is required")
	}
	realtimeHub := realtimePostgres.NewLocalHub()
	realtimeBroker := realtimePostgres.NewBroker(database.Pool(), realtimeHub)
	realtimeBroker.StartListener(context.Background())

	dependencies := BuildDependenciesWithRealtime(database, realtimeBroker)
	dependencies.GetReadiness = readinessQueryService{Ping: database.Ping, FeatureChecks: external.ReadinessChecks()}
	dependencies.BeginChannelConnection = services.ChannelProvisioningDisabledService{}
	dependencies.IngestSocialAPIWebhook = services.WebhookReceiverDisabledService{Receiver: "SocialAPI"}
	if disconnector, ok := external.SocialAPI.(ports.ChannelAccountDisconnector); ok {
		channelConnectionRepository := postgres.NewChannelConnectionRepository(database)
		channelRuntime := services.ChannelRuntimeService{
			Reader:       channelConnectionRepository,
			Runtime:      channelConnectionRepository,
			Transactions: database,
			Disconnector: disconnector,
			Provisioning: postgres.NewChannelProvisioningStore(database),
		}
		if external.ChannelProvisioningBrand != nil {
			channelRuntime.BrandCleanup = postgres.NewProviderBrandRepository(database)
			channelRuntime.BrandProvider = external.ChannelProvisioningBrand
		}
		channelRuntime.HistoryStore = postgres.NewChannelHistoryRepository(database)
		dependencies.ReconnectChannel = services.ReconnectChannelCommandService{ChannelRuntimeService: channelRuntime}
		dependencies.DisconnectChannel = services.DisconnectChannelCommandService{ChannelRuntimeService: channelRuntime}
		dependencies.RemoveChannel = services.RemoveChannelCommandService{ChannelRuntimeService: channelRuntime}
	}
	if authentication != nil {
		dependencies.Scope = handlers.PostgresScopeProvider{Memberships: authentication.Repository}
		dependencies.PlatformAccess = authentication.PlatformChecker
		dependencies.AuthenticatePrincipal = authentication.Service
		dependencies.RotateRefreshSession = services.RefreshSessionRotationService{Authentication: authentication.Service}
		dependencies.RevokeRefreshSession = services.RefreshSessionRevocationService{Authentication: authentication.Service}
		principalQueries := services.PrincipalQueryService{Principals: authentication.Repository}
		dependencies.GetCurrentPrincipal = principalQueries
		dependencies.ListAccessibleBusinesses = services.MembershipQueryService{PrincipalQueryService: principalQueries}
	}
	var provisioningService *services.ChannelProvisioningService
	if external.ChannelProvisioningEnabled {
		if external.ChannelProvisioningError != nil {
			dependencies.BeginChannelConnection = services.ChannelProvisioningUnavailableService{Cause: external.ChannelProvisioningError}
		} else if external.ChannelProvisioningSocial == nil || external.ChannelProvisioningBrand == nil {
			dependencies.BeginChannelConnection = services.ChannelProvisioningUnavailableService{Cause: errors.New("channel provisioning adapters are not configured")}
		} else {
			service := services.ChannelProvisioningService{
				Sessions:       postgres.NewChannelProvisioningStore(database),
				Social:         external.ChannelProvisioningSocial,
				SocialBrands:   external.ChannelProvisioningBrand,
				ProviderBrands: postgres.NewProviderBrandRepository(database),
				Businesses:     postgres.NewBusinessRepository(database),
				Connections:    postgres.NewChannelConnectionRepository(database),
				RedirectURI:    external.ChannelProvisioningRedirectURI,
			}
			provisioningService = &service
			dependencies.BeginChannelConnection = &services.BeginChannelConnectionHandler{Provisioning: service}
		}
	}
	eventStore := postgres.NewInboundEventStore(database)
	outboxStore := postgres.NewPostgresOutboxStore(database)
	salesAdminWorkflow := buildSalesWorkflowService(database, outboxStore)
	dependencies.SalesAdmin = services.SalesWorkflowAdminService{Repository: postgres.NewSalesWorkflowRepository(database), Workflow: salesAdminWorkflow}
	var autoReply commands.AutoReplyHandler
	// PostgreSQL is the authority for each merchant's AI opt-out.
	businessAIGuard := &services.BusinessAIGuard{Policies: postgres.NewBusinessRepository(database)}
	// Per Item 1 + 2: hoist platformOperations to the top of the
	// function scope so it's available to ALL branches that need
	// the shared kill switch state (AutoReply webhook branch +
	// Summary service branch + Merchant AI agent branch). Before
	// this hoist, the variable was declared at line ~380 (after the
	// AutoReply-enabled branch) — so the Summary service wiring
	// (line ~280) and the Merchant AI wiring (line ~575) couldn't
	// reference it.
	//
	// The flags (aiConfigured/channelConfigured) are computed below
	// at the point of use; the InMemoryPlatformOperationsRepository
	// is stateless except for its runtime flag, so creating it early
	// is safe.
	platformOperations := services.NewInMemoryPlatformOperationsRepository(
		external.NativeCustomerDecision != nil && external.LLMConfigError == nil,
		external.SocialAPI != nil,
	)
	if external.NativeAISettings != nil {
		platformOperations.SetAIModel(external.NativeAISettings.Model)
	}
	if external.SocialAPIHealthProbe != nil {
		platformOperations.RegisterProbe("socialapi", external.SocialAPIHealthProbe)
	}

	// Customer Sales deliberately has no catalog tool registry. The catalog has
	// one authoritative AI path: bounded manifest on the initial turn, then
	// complete catalog paging/batching only when the model requests it.
	// This prevents competing catalog-read paths and keeps evidence deterministic.

	if external.AutoReplyEnabled || external.CustomerSalesEnabled {
		// One customer-sales decision interface owns B2C and summary requests.
		// Native provider selection is isolated from Customer Sales business decisions.
		customerSalesDecision := external.NativeCustomerDecision
		if customerSalesDecision == nil {
			return nil, errors.New("AutoReply requires a configured native customer-sales decision provider")
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
		service.AIGuard = businessAIGuard
		// Both AI routes use the same canonical run trace lifecycle.
		service.RunRepository = postgres.NewAIRunTraceRepository(database)
		// Per AIUsageTokenTelemetry.md §6: wire the telemetry pipeline so
		// every model request's tokens + cost are recorded to ai_usage_records
		// and flow through to the Platform Admin AI Usage view.
		service.AIUsage = postgres.NewAIUsageRepository(database)
		service.AIPricing = postgres.NewAIProviderPricingRepository(database)

		service.Subscriptions = postgres.NewSubscriptionRepository(database)
		// Per contract ⑤ §7, build the Catalog Entity Contract payload
		// once and reuse for every call.
		entityContract := services.BuildCatalogEntityContractPayload()
		payloadBytes, err := json.Marshal(entityContract)
		if err != nil {
			return nil, fmt.Errorf("marshal catalog entity contract: %w", err)
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
		service.SummaryService.AIPricing = service.AIPricing
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
			BusinessAIGuard:    businessAIGuard,
			Subscriptions:      postgres.NewSubscriptionRepository(database),
			AIUsage:            postgres.NewAIUsageRepository(database),
			PlatformOperations: platformOperations,
		}
		// Per contract ⑥ §2, wire the validation pipeline with the
		// concrete Postgres validators. Per contract ⑥ §6-7 the
		// ReferenceValidator checks that every selected ID (item/variant/
		// offer) was actually sent to the model as evidence AND exists as
		// a real row in catalog_items/variants/offers. Per contract ⑥ §8-9
		// the TenantValidator checks that every selected reference
		// belongs to the current authenticated business (cross-tenant
		// protection).
		//
		// Per contract ⑥ §12-13, the PolicyEvaluator is the
		// Postgres-backed evaluator that reads business_policies per
		// migration 000002 and decides requires_approval — NOT the model.
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
			return nil, errors.New("AutoReply requires a configured native catalog model")
		}
		catalogBudget, err := external.NativeCatalogClient.BatchTokenBudget()
		if err != nil {
			return nil, fmt.Errorf("configure native catalog token budget: %w", err)
		}
		// Guard individual catalog model calls, including each batch and final.
		if guarded, ok := any(external.NativeCatalogClient).(interface {
			SetAIExecutionGuard(ports.BusinessAIExecutionGuard)
		}); ok {
			guarded.SetAIExecutionGuard(businessAIGuard)
		}
		service.CatalogBatch = &services.CatalogBatchController{
			CatalogAI:   postgres.NewCatalogAIReadRepository(database),
			Evaluator:   external.NativeCatalogClient,
			RunRepo:     postgres.NewAIRunTraceRepository(database),
			TokenBudget: catalogBudget,
			Now:         func() time.Time { return time.Now().UTC() },
			NewID:       uuid.NewString,
			AIUsage:     postgres.NewAIUsageRepository(database),
			AIPricing:   service.AIPricing,

			Subscriptions: postgres.NewSubscriptionRepository(database),
		}

		if os.Getenv("CATALOG_MEDIA_ENABLED") == "true" {
			service.CatalogMedia = postgres.NewCatalogMediaRepository(database)
			if selector, ok := customerSalesDecision.(ports.CustomerMediaSelectionPort); ok {
				service.MediaSelection = selector
			}
		}
		service.CatalogBatch.GatewayFXRateYER = service.GatewayFXRateYER
		autoReply = service
	}
	inboundAutomation := services.InboundAutomationService{
		Rules:         postgres.NewAutomationRuleRepository(database),
		Executions:    postgres.NewAutomationExecutionRepository(database),
		Conversations: postgres.NewConversationRepository(database),
		Reader:        postgres.NewConversationRepository(database),
		Labels:        postgres.NewConversationLabelRepository(database),
		Assignees:     postgres.NewTeamRepository(database),
		Transactions:  database,
	}
	// Per P0-2 + Item 1 + Item 2: platformOperations is declared at
	// the top of this function (line 168) — hoisted so it's available
	// to ALL branches (AutoReply webhook + Summary + Merchant AI +
	// PlatformDeps wire-up). The previous local declaration here has
	// been removed. The flags below remain here for the
	// aiConfigured/channelConfigured dashboard view.
	aiConfigured := external.NativeCustomerDecision != nil && external.LLMConfigError == nil
	channelConfigured := external.SocialAPI != nil
	_ = aiConfigured
	_ = channelConfigured

	// Per Item 5: capture the worker pool reference so APIRuntime.Shutdown
	// can drain it gracefully. The variable is set inside the
	// SocialWebhook branch below; remains nil when the webhook is
	// not wired (the pool is only needed for the AutoReply webhook path).
	var autoReplyPool *services.AutoReplyWorkerPool

	if external.SocialWebhook != nil {
		// Per-merchant customer enrichment: SocialAPI's DM webhook payload
		// delivers only `author.id` (WhatsApp wa_id / FB PSID / IG IGSID);
		// the customer's display_name and picture must be fetched via the
		// REST endpoint GET /v1/inbox/conversations/{id} AFTER Materialize.
		// The Enricher interface is implemented by *socialapi.Client
		// (verified at compile time via var _ ports.ConversationEnricher).
		// The Customers repo is the SAME one used by the customer CRUD
		// endpoints (ports.CustomerRuntimeRepository) — no duplicate repo.
		webhookService := services.SocialAPIWebhookService{
			Receiver:         external.SocialWebhook,
			RawPayloads:      postgres.NewRawPayloadStore(database),
			History:          postgres.NewChannelHistoryRepository(database),
			Connections:      postgres.NewChannelConnectionRepository(database),
			Events:           eventStore,
			Inbound:          postgres.NewProviderInboundStore(database),
			DeliveryStatuses: postgres.NewDeliveryStatusStore(database),
			Automation:       inboundAutomation,
			AutoReply:        autoReply,
			Realtime:         realtimeBroker,
			Customers:        postgres.NewCustomerRepository(database),
		}
		// Wire the enricher ONLY when the SocialAPI client is configured.
		// The client implements both ports.ChannelProvider and
		// ports.ConversationEnricher; we type-assert to expose the
		// enrichment capability without breaking the existing webhook
		// receiver contract.
		if external.SocialAPI != nil {
			if enricher, ok := external.SocialAPI.(ports.ConversationEnricher); ok {
				webhookService.Enricher = enricher
			}
		}
		// Per P0-2 + P0-3 + AIUsageTokenTelemetry.md §19: wire the
		// AI Cost Protection checker with the shared Platform Operations
		// registry. This connects the platformAIDisable/platformAIEnable
		// kill switch (Contract §81) to the actual AutoReply execution
		// path. It also enforces the merchant AI Reply entitlement
		// (Contract §33) before the gateway model is called.
		webhookService.AICostProtectionChecker = &services.AICostProtectionService{
			BusinessAIGuard:    businessAIGuard,
			Subscriptions:      postgres.NewSubscriptionRepository(database),
			AIUsage:            postgres.NewAIUsageRepository(database),
			PlatformOperations: platformOperations,
		}
		// Per P1-10: wire the bounded AutoReplyWorkerPool so a viral
		// DM burst doesn't spawn unbounded goroutines (each consuming
		// gateway quota + DB conns). 4 concurrent workers + 64-deep
		// queue absorbs a burst; larger values can be tuned via env
		// when gateway capacity allows.
		webhookService.AutoReplyWorkerPool = services.NewAutoReplyWorkerPool(4, 64)
		autoReplyPool = webhookService.AutoReplyWorkerPool
		dependencies.IngestSocialAPIWebhook = webhookService
	}
	var assignBusinessOwner *services.AssignBusinessOwnerService
	if authentication != nil {
		assignBusinessOwner = &services.AssignBusinessOwnerService{
			Transactions:       database,
			PrincipalBootstrap: authentication.Repository,
			Business:           postgres.NewPlatformBusinessRepository(database),
		}
	}
	dashboardServer := handlers.NewServer(dependencies)
	// Independent merchant AI Assist: reviewed read-only tools share the
	// canonical tenant-scoped repositories, native inference router and guard.
	// If native inference is disabled, the API fails closed as unavailable.
	if external.NativeAssistCompletion != nil {
		registry, registryErr := contracts.NewMerchantAssistRegistry()
		if registryErr != nil {
			return nil, fmt.Errorf("merchant AI Assist registry: %w", registryErr)
		}
		model := &merchantgateway.MerchantAssistCompletion{
			Client: external.NativeAssistCompletion, Model: "mujeeb-merchant",
		}
		sanitizer := merchantassist.ConservativeSanitizer{}
		assistant := &merchantassist.Assistant{
			Registry: registry, Generator: model, Native: &merchantgateway.MerchantAssistNativeTools{Client: external.NativeAssistCompletion, Model: "mujeeb-merchant"}, Sanitizer: sanitizer,
			Discovery: &merchantassist.DiscoveryReader{
				Guard:                 businessAIGuard,
				Overview:              dependencies.GetDashboardOverview,
				Customers:             dependencies.ListCustomers,
				GetCustomer:           dependencies.GetCustomer,
				GetTransaction:        dependencies.GetTransaction,
				CustomerConversations: dependencies.ListCustomerConversations,
				CustomerTransactions:  dependencies.ListCustomerTransactions,
				Conversations:         dependencies.ListConversations,
				Leads:                 dependencies.ListLeads,
				Transactions:          dependencies.ListTransactions,
				Catalogs:              dependencies.ListCatalogs,
				Channels:              dependencies.ListChannelConnections,
				Subscription:          dependencies.GetBusinessSubscription,
			},
			Published: &merchantassist.PublishedReader{
				Policies:  postgres.NewBusinessPolicyRepository(database),
				Knowledge: postgres.NewKnowledgeDocumentRepository(database),
				AIGuard:   businessAIGuard, Now: time.Now,
			},
			Records: &merchantassist.RecordReader{
				Customers:     postgres.NewCustomerRepository(database),
				Conversations: postgres.NewConversationRepository(database),
				Leads:         postgres.NewLeadRepository(database),
				Transactions:  postgres.NewTransactionRepository(database),
				AIGuard:       businessAIGuard, Now: time.Now,
			},
			History: &merchantassist.ConversationHistoryReader{
				Conversations: postgres.NewConversationRepository(database),
				State:         postgres.NewConversationStateRepository(database),
				Messages:      postgres.NewMessageRepository(database),
				AIGuard:       businessAIGuard, Sanitizer: sanitizer, Now: time.Now,
			},
			Leads: &merchantassist.LeadEvidenceReader{
				Leads:   postgres.NewLeadRepository(database),
				AIGuard: businessAIGuard, Now: time.Now,
			},
		}
		dashboardServer.WithMerchantAssist(assistant)
	}
	dashboardServer = dashboardServer.WithPlatformDeps(handlers.PlatformDeps{
		Plans:            postgres.NewPlanRepository(database),
		PlatformBusiness: postgres.NewPlatformBusinessRepository(database),
		PlatformAudit:    postgres.NewPlatformAuditRepository(database),
		Subscriptions:    postgres.NewSubscriptionRepository(database),
		Payments:         postgres.NewPaymentRepository(database),
		Addons:           postgres.NewSubscriptionAddonRepository(database),
		Support:          postgres.NewSupportRepository(database),
		AIUsage:          postgres.NewAIUsageRepository(database),
		Transactions:     database,
		Operations:       platformOperations,
		ChannelReader:    postgres.NewPlatformChannelReadRepository(database),

		NativeAISettings:    external.NativeAISettings,
		AssignBusinessOwner: assignBusinessOwner,
	})

	var merchantCatalogErr error
	dashboardServer, merchantCatalogErr = wireMerchantCatalogAuthoringAI(dashboardServer, database, external.NativeMerchantAuthoring, external.NativeAISettings)
	if merchantCatalogErr != nil {
		return nil, merchantCatalogErr
	}

	var apiMiddleware []func(ctx huma.Context, next func(huma.Context))
	if authentication != nil {
		apiMiddleware = append(apiMiddleware, middleware.RequireAccessTokenHuma(authentication.Verifier))
		// Per Platform Administration Contract §4, §55, §63-64: Platform
		// scope is a SEPARATE authorization layer from merchant membership.
		// RequirePlatformAdminHuma is intentionally a separate middleware
		// (not part of RequireAccessTokenHuma) so the /api/v1/platform/*
		// routes can be selectively protected while /api/v1/businesses/*
		// routes remain on the merchant requireScope boundary.
		//
		// The check internally verifies the principal is an active
		// platform_super_admin via the platform_access_repository. A
		// merchant owner token used against /platform/* returns 403.
		if authentication.PlatformChecker != nil {
			apiMiddleware = append(apiMiddleware, middleware.RequirePlatformAdminHuma(authentication.PlatformChecker))
		}
	}

	_, mux := contract.BuildAPIWithHandlersAndMiddleware(dashboardServer, apiMiddleware)

	realtimeSSEHandler := handlers.NewRealtimeSSEHandler(realtimeHub, dependencies.Scope)
	var sseHTTPHandler http.Handler = realtimeSSEHandler
	if authentication != nil {
		sseHTTPHandler = middleware.RequireAccessToken(authentication.Verifier, realtimeSSEHandler)
	}
	mux.Handle("GET /api/v1/businesses/{business_id}/realtime/events", sseHTTPHandler)

	if provisioningService != nil {
		mux.HandleFunc("/oauth/socialapi/callback", func(writer http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodGet {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			query := request.URL.Query()
			callback := ports.SocialAuthorizationCallback{State: query.Get("state"), Status: query.Get("status"), Platform: query.Get("platform"), AccountID: query.Get("account_id"), ConnectionID: query.Get("connection_id"), PlatformAccountID: query.Get("platform_account_id"), PageIDs: append([]string(nil), query["page_id"]...)}
			if len(callback.PageIDs) == 0 && query.Get("page_ids") != "" {
				for _, pageID := range strings.Split(query.Get("page_ids"), ",") {
					if value := strings.TrimSpace(pageID); value != "" {
						callback.PageIDs = append(callback.PageIDs, value)
					}
				}
			}
			session, err := provisioningService.CompleteOAuthCallback(request.Context(), callback)
			if external.FrontendURL != "" {
				// FRONTEND_URL may include a UI route such as /landing. The
				// merchant dashboard lives at /channels, so appending /channels
				// to FRONTEND_URL directly would produce /landing/channels.
				// Redirect to the configured frontend origin instead.
				frontendRedirectBase := external.FrontendURL
				if parsed, parseErr := url.Parse(external.FrontendURL); parseErr == nil && parsed.Scheme != "" && parsed.Host != "" {
					frontendRedirectBase = parsed.Scheme + "://" + parsed.Host
				}
				var redirectURL string
				if err != nil {
					log.Printf("[ChannelProvisioning] OAuth callback failed: %v", err)
					redirectURL = fmt.Sprintf("%s/channels?status=failed&error=%s", strings.TrimRight(frontendRedirectBase, "/"), url.QueryEscape("تعذر إكمال ربط القناة. يرجى المحاولة مرة أخرى."))
				} else {
					redirectURL = fmt.Sprintf("%s/channels?status=connected&channel=%s&provisioning_id=%s", strings.TrimRight(frontendRedirectBase, "/"), url.QueryEscape(session.Channel), url.QueryEscape(session.ID))
				}
				http.Redirect(writer, request, redirectURL, http.StatusFound)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			if err != nil {
				// The callback is a public browser endpoint. Never expose a raw
				// database/provider error here: it can include implementation
				// details such as SQL constraints and is not actionable for a
				// merchant. The full cause remains in the server log.
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = writer.Write([]byte(`{"status":"failed","error":{"code":"channel_connection_failed","message":"Unable to complete the channel connection. Return to the dashboard and try again."}}`))
				return
			}
			_, _ = fmt.Fprintf(writer, `{"status":%q,"provisioning_id":%q,"provider":%q,"channel":%q}`, session.Status, session.ID, session.ProviderRef, session.Channel)
		})
	}
	mux.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok","service":"mujeeb24-api"}`))
	})
	// SECURITY audit H2: enforce a hard body-size limit on the public
	// webhook endpoint. Without this, an attacker could POST a multi-
	// hundred-MB body that Huma buffers into memory AND that the raw
	// payload store then persists as BYTEA — unbounded memory + DB
	// growth. The OpenAPI already advertises 413 as a possible error
	// response; this middleware makes it real.
	//
	// We wrap the mux in a small middleware that applies
	// http.MaxBytesReader to the webhook route specifically. Other
	// routes (auth, dashboard, SSE) are unaffected — their Huma
	// bodies are small JSON envelopes.
	wrappedMux := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/v1/businesses/") && strings.HasSuffix(request.URL.Path, "/media/uploads") {
			// JSON/base64 overhead for a maximum 10 MiB image. Enforce this
			// before Huma allocates the JSON body, including chunked requests.
			request.Body = http.MaxBytesReader(writer, request.Body, 15*1024*1024)
		}
		if strings.HasPrefix(request.URL.Path, "/api/v1/webhooks/") {
			// 256 KB — SocialAPI webhook payloads are typically
			// 1-5 KB. 256 KB is generous headroom and still
			// blocks the multi-MB adversarial case.
			const maxWebhookBodyBytes = 256 * 1024
			request.Body = http.MaxBytesReader(writer, request.Body, maxWebhookBodyBytes)
		}
		mux.ServeHTTP(writer, request)
	})
	// Cross-origin browser access is required because the production frontend is hosted
	// separately from the Render API. CORS is centralized in newCORSMiddleware so the
	// allowed request headers are covered by a regression-tested contract.
	corsMux := newCORSMiddleware(wrappedMux, external.FrontendURL)
	server := &http.Server{Addr: address, Handler: corsMux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: 60 * time.Second}
	return &APIRuntime{
		HTTP:                server,
		Database:            database,
		Dependencies:        dependencies,
		EventStore:          eventStore,
		Outbox:              outboxStore,
		SocialAPI:           external.SocialAPI,
		SocialWebhook:       external.SocialWebhook,
		ChannelProvisioning: provisioningService,
		RealtimeBroker:      realtimeBroker,
		RealtimeHub:         realtimeHub,
		AutoReplyWorkerPool: autoReplyPool,
	}, nil
}

func buildAuthenticationRuntime(cfg config.ProcessConfig, database *postgres.Adapter) (*authenticationRuntime, error) {
	if !cfg.AuthEnabled {
		return nil, nil
	}
	issuer, err := ed25519jwt.New(ed25519jwt.Config{PrivateKeyBase64: cfg.JWTEd25519PrivateKey, PublicKeyBase64: cfg.JWTEd25519PublicKey, Issuer: cfg.JWTIssuer, AccessTTL: cfg.JWTAccessTTL})
	if err != nil {
		return nil, err
	}
	repository := postgres.NewAuthenticationRepository(database)
	service := services.AuthenticationService{Principals: repository, Sessions: repository, Tokens: issuer, RefreshTTL: cfg.RefreshSessionTTL}
	// Per Platform Administration Contract §4: Platform Scope is enforced
	// separately from Merchant Scope. The PlatformAccessRepository (which
	// implements both PlatformSuperAdminBootstrapRepository and the IsActiveSuperAdmin
	// checker) is wired here so RequirePlatformAdminHuma can verify every
	// /api/v1/platform/* request.
	platformAccess := postgres.NewPlatformAccessRepository(database)
	return &authenticationRuntime{
		Verifier:        issuer,
		Repository:      repository,
		Service:         service,
		PlatformChecker: platformAccess,
	}, nil
}

func (r *APIRuntime) Serve() error {
	if r == nil || r.HTTP == nil {
		return errors.New("api runtime is not configured")
	}
	err := r.HTTP.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (r *APIRuntime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	var shutdownErr error
	r.closeOnce.Do(func() {
		// Per Item 5: drain the AutoReply worker pool BEFORE closing
		// the HTTP server + database. The pool's Stop() closes the
		// tasks channel + waits for all workers to finish their
		// in-flight + queued tasks. Without this, in-flight AutoReply
		// work (which invokes Native AI + writes to Postgres) would be
		// dropped silently when the database closes.
		if r.AutoReplyWorkerPool != nil {
			r.AutoReplyWorkerPool.Stop()
		}
		if r.RealtimeBroker != nil {
			r.RealtimeBroker.Close()
		}
		if r.HTTP != nil {
			shutdownErr = r.HTTP.Shutdown(ctx)
		}
		if r.Database != nil {
			r.Database.Close()
		}
	})
	return shutdownErr
}
