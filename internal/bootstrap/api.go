package bootstrap

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "net/url"
        "strings"
        "sync"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/auth/ed25519jwt"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
        realtimePostgres "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/realtime/postgres"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
        "github.com/danielgtaylor/huma/v2"
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
        closeOnce           sync.Once
}

type authenticationRuntime struct {
        Verifier   middleware.AccessTokenVerifier
        Repository *postgres.AuthenticationRepository
        Service    services.AuthenticationService
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
        catalogRepository := postgres.NewCatalogRepository(database)
        capabilityRegistry := services.NewCapabilityRegistry()
        catalogCapability := services.NewCatalogDataCapability(
                services.ListCatalogsQueryService{Repository: catalogRepository},
                services.ListCatalogItemsQueryService{Repository: catalogRepository},
                services.GetCatalogItemQueryService{Repository: catalogRepository},
                services.ListOffersQueryService{Repository: catalogRepository},
                services.ListVariantsQueryService{Repository: catalogRepository},
                services.GetAttributeSchemaQueryService{Repository: catalogRepository},
        )
        if err := capabilityRegistry.Register(catalogCapability); err != nil {
                database.Close()
                return nil, err
        }
        catalogCommands := services.NewCatalogCommandServices(catalogRepository, database)
        catalogAuthoringCapability := services.NewCatalogAuthoringCapability(
                services.AuthorCatalogItemCommandService{CatalogCommandServices: catalogCommands},
                services.CreateCatalogCommandService{CatalogCommandServices: catalogCommands},
                services.CreateCatalogItemCommandService{CatalogCommandServices: catalogCommands},
                services.CreateOfferCommandService{CatalogCommandServices: catalogCommands},
                services.CreateVariantCommandService{CatalogCommandServices: catalogCommands},
                services.CreateAttributeSchemaVersionCommandService{CatalogCommandServices: catalogCommands},
        )
        if err := capabilityRegistry.Register(catalogAuthoringCapability); err != nil {
                database.Close()
                return nil, err
        }
        runtime, err := newAPIWithExternalAndAuthentication(database, cfg.HTTPAddr, BuildExternalAdapters(cfg, capabilityRegistry), authentication)
        if err != nil {
                database.Close()
                return nil, err
        }
        return runtime, nil
}

func NewAPI(database *postgres.Adapter, address string) (*APIRuntime, error) {
        return NewAPIWithExternal(database, address, ExternalAdapters{})
}

func NewAPIWithExternal(database *postgres.Adapter, address string, external ExternalAdapters) (*APIRuntime, error) {
        return newAPIWithExternalAndAuthentication(database, address, external, nil)
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
        if authentication != nil {
                dependencies.Scope = handlers.PostgresScopeProvider{Memberships: authentication.Repository}
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
                } else if external.ChannelProvisioningSocial == nil {
                        dependencies.BeginChannelConnection = services.ChannelProvisioningUnavailableService{Cause: errors.New("channel provisioning adapters are not configured")}
                } else {
                        service := services.ChannelProvisioningService{
                                Sessions:    postgres.NewChannelProvisioningStore(database),
                                Social:      external.ChannelProvisioningSocial,
                                Connections: postgres.NewChannelConnectionRepository(database),
                                RedirectURI: external.ChannelProvisioningRedirectURI,
                        }
                        provisioningService = &service
                        dependencies.BeginChannelConnection = &services.BeginChannelConnectionHandler{Provisioning: service}
                }
        }
        eventStore := postgres.NewInboundEventStore(database)
        outboxStore := postgres.NewPostgresOutboxStore(database)
        var autoReply commands.AutoReplyHandler
        if external.AutoReplyEnabled {
                if external.AIRuntime == nil {
                        return nil, errors.New("AutoReply requires a configured LLM runtime")
                }
                // Per contract ④ §8, wrap the legacy Gemini Client with the
                // contract-aligned ContractClient (implements ports.ContractRuntime).
                // The legacy Client.Decide method is no longer used for AutoReply.
                var contractRuntime ports.ContractRuntime
                if geminiClient, ok := external.AIRuntime.(*gemini.Client); ok {
                        cc, err := gemini.NewContractClient(geminiClient)
                        if err != nil {
                                return nil, fmt.Errorf("build contract client: %w", err)
                        }
                        contractRuntime = cc
                } else {
                        // Fallback for openaicompatible.Client or other AIRuntime
                        // implementations: they do NOT yet implement ContractRuntime.
                        // Production should use gemini.NewContractClient. Until the
                        // OpenAI-compatible adapter implements ContractRuntime, AutoReply
                        // is unavailable for that provider.
                        return nil, errors.New("AutoReply requires a gemini.Client (contract-aligned); OpenAI-compatible provider not yet supported")
                }
                referenceRepository := postgres.NewConversationReferenceRepository(database)
                service := services.NewAutoReplyService(
                        contractRuntime,
                        postgres.NewAIDecisionRepository(database),
                        referenceRepository,
                        postgres.NewOutboundMessageRepository(database),
                        outboxStore,
                        database,
                )
                // Per contract ⑧ §5, wire the AI Run trace repository.
                service.RunRepository = postgres.NewAIRunTraceRepository(database)
                // Per contract ⑤ §7, build the Catalog Entity Contract payload
                // once and reuse for every call.
                entityContract := services.BuildCatalogEntityContractPayload()
                if payloadBytes, err := json.Marshal(entityContract); err == nil {
                        service.EntityContractPayload = payloadBytes
                }
                contextBuilder := services.NewAutoReplyContextBuilder(
                        postgres.NewBusinessRepository(database),
                        postgres.NewConversationRepository(database),
                        postgres.NewCustomerRepository(database),
                        postgres.NewCatalogRepository(database),
                        postgres.NewMessageRepository(database),
                )
                contextBuilder.Knowledge = postgres.NewKnowledgeDocumentRepository(database)
                contextBuilder.Policies = postgres.NewBusinessPolicyRepository(database)
                service.ContextBuilder = contextBuilder
                // Per contract ⑥ §2, wire the validation pipeline. The pipeline
                // uses nil ReferenceValidator and TenantValidator for now (they
                // will be wired once postgres adapters implement them); the
                // pipeline treats nil as "always passes" with a logged warning
                // for incremental rollout per contract ⑥.
                service.Validation = services.NewValidationPipeline(nil, nil, nil, nil)
                service.StateRepository = postgres.NewConversationStateRepository(database)
                service.MessageRepository = postgres.NewMessageRepository(database)
                service.Conversations = postgres.NewConversationRepository(database)
                service.Realtime = realtimeBroker
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
        if external.SocialWebhook != nil {
                dependencies.IngestSocialAPIWebhook = services.SocialAPIWebhookService{
                        Receiver:         external.SocialWebhook,
                        RawPayloads:      postgres.NewRawPayloadStore(database),
                        Connections:      postgres.NewChannelConnectionRepository(database),
                        Events:           eventStore,
                        Inbound:          postgres.NewProviderInboundStore(database),
                        DeliveryStatuses: postgres.NewDeliveryStatusStore(database),
                        Automation:       inboundAutomation,
                        AutoReply:        autoReply,
                        Realtime:         realtimeBroker,
                }
        }
        var apiMiddleware []func(ctx huma.Context, next func(huma.Context))
        if authentication != nil {
                apiMiddleware = append(apiMiddleware, middleware.RequireAccessTokenHuma(authentication.Verifier))
        }
        _, mux := contract.BuildAPIWithHandlersAndMiddleware(handlers.NewServer(dependencies), apiMiddleware)

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
                                var redirectURL string
                                if err != nil {
                                        redirectURL = fmt.Sprintf("%s/channels?status=failed&error=%s", external.FrontendURL, url.QueryEscape(err.Error()))
                                } else {
                                        redirectURL = fmt.Sprintf("%s/channels?status=connected&channel=%s&provisioning_id=%s", external.FrontendURL, url.QueryEscape(session.Channel), url.QueryEscape(session.ID))
                                }
                                http.Redirect(writer, request, redirectURL, http.StatusFound)
                                return
                        }
                        writer.Header().Set("Content-Type", "application/json")
                        if err != nil {
                                writer.WriteHeader(http.StatusBadRequest)
                                _, _ = fmt.Fprintf(writer, `{"status":"failed","error":%q}`, err.Error())
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
        server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
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
        return &authenticationRuntime{Verifier: issuer, Repository: repository, Service: service}, nil
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
