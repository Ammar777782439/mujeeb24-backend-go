package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/auth/ed25519jwt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
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
	runtime, err := newAPIWithExternalAndAuthentication(database, cfg.HTTPAddr, BuildExternalAdapters(cfg), authentication)
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
	dependencies := BuildDependencies(database)
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
		referenceRepository := postgres.NewConversationReferenceRepository(database)
		service := services.NewAutoReplyService(
			external.AIRuntime,
			postgres.NewAIDecisionRepository(database),
			referenceRepository,
			postgres.NewOutboundMessageRepository(database),
			outboxStore,
			database,
		)
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
		service.PolicyEvaluator = services.GroundedPolicyEngine{}
		autoReply = service
	}
	if external.SocialWebhook != nil {
		dependencies.IngestSocialAPIWebhook = services.SocialAPIWebhookService{
			Receiver:         external.SocialWebhook,
			RawPayloads:      postgres.NewRawPayloadStore(database),
			Connections:      postgres.NewChannelConnectionRepository(database),
			Events:           eventStore,
			Inbound:          postgres.NewProviderInboundStore(database),
			DeliveryStatuses: postgres.NewDeliveryStatusStore(database),
			AutoReply:        autoReply,
		}
	}
	var apiMiddleware []func(ctx huma.Context, next func(huma.Context))
	if authentication != nil {
		apiMiddleware = append(apiMiddleware, middleware.RequireAccessTokenHuma(authentication.Verifier))
	}
	_, mux := contract.BuildAPIWithHandlersAndMiddleware(handlers.NewServer(dependencies), apiMiddleware)
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
	return &APIRuntime{HTTP: server, Database: database, Dependencies: dependencies, EventStore: eventStore, Outbox: outboxStore, SocialAPI: external.SocialAPI, SocialWebhook: external.SocialWebhook, ChannelProvisioning: provisioningService}, nil
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
		if r.HTTP != nil {
			shutdownErr = r.HTTP.Shutdown(ctx)
		}
		if r.Database != nil {
			r.Database.Close()
		}
	})
	return shutdownErr
}
