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
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type APIRuntime struct {
	HTTP                *http.Server
	Database            *postgres.Adapter
	Dependencies        handlers.Dependencies
	EventStore          ports.EventStore
	Outbox              ports.OutboxStore
	SocialAPI           ports.ChannelProvider
	Chatwoot            ports.CommunicationWorkspace
	SocialWebhook       ports.WebhookReceiver
	ChatwootWebhook     ports.WebhookReceiver
	ChannelProvisioning *services.ChannelProvisioningService
	closeOnce           sync.Once
}

func BuildAPI(ctx context.Context, cfg config.ProcessConfig) (*APIRuntime, error) {
	database, err := postgres.Open(ctx, cfg.DatabaseURL, postgres.PoolConfig{MaxConns: cfg.DBMaxConns, MinConns: cfg.DBMinConns, MaxConnLifetime: cfg.DBMaxConnLifetime, MaxConnIdleTime: cfg.DBMaxConnIdleTime, HealthCheckPeriod: cfg.DBHealthCheckPeriod, ConnectTimeout: cfg.DBConnectTimeout})
	if err != nil {
		return nil, err
	}
	runtime, err := NewAPIWithExternal(database, cfg.HTTPAddr, BuildExternalAdapters(cfg))
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
	var provisioningService *services.ChannelProvisioningService
	if external.ChannelProvisioningEnabled {
		if external.ChannelProvisioningError != nil {
			return nil, external.ChannelProvisioningError
		}
		if external.ChannelProvisioningSocial == nil || external.ChannelProvisioningWorkspace == nil {
			return nil, errors.New("channel provisioning adapters are not configured")
		}
		service := services.ChannelProvisioningService{
			Sessions:    postgres.NewChannelProvisioningStore(database),
			Social:      external.ChannelProvisioningSocial,
			Workspace:   external.ChannelProvisioningWorkspace,
			Bindings:    postgres.NewChatwootWorkspaceBindingRepository(database),
			Connections: postgres.NewChannelConnectionRepository(database),
			RedirectURI: external.ChannelProvisioningRedirectURI,
			WebhookURL:  external.ChannelProvisioningWebhookURL,
		}
		provisioningService = &service
		dependencies.BeginChannelConnection = &services.BeginChannelConnectionHandler{Provisioning: service}
	}
	eventStore := postgres.NewInboundEventStore(database)
	outboxStore := postgres.NewPostgresOutboxStore(database)
	if external.SocialWebhook != nil {
		dependencies.IngestSocialAPIWebhook = services.SocialAPIWebhookService{
			Receiver:         external.SocialWebhook,
			RawPayloads:      postgres.NewRawPayloadStore(database),
			Connections:      postgres.NewChannelConnectionRepository(database),
			Events:           eventStore,
			Inbound:          postgres.NewProviderInboundStoreWithMirror(database, external.ChatwootMirrorEnabled && external.Chatwoot != nil),
			DeliveryStatuses: postgres.NewDeliveryStatusStore(database),
		}
	}
	if external.ChatwootWebhook != nil {
		chatwootService := services.ChatwootWebhookService{Receiver: external.ChatwootWebhook, Inbound: postgres.NewChatwootInboundStore(database)}
		if external.ChatwootAutoReplyEnabled {
			if external.AIRuntime == nil {
				return nil, errors.New("Chatwoot AutoReply requires a configured LLM runtime")
			}
			referenceRepository := postgres.NewConversationReferenceRepository(database)
			autoReply := services.NewAutoReplyService(
				external.AIRuntime,
				postgres.NewAIDecisionRepository(database),
				referenceRepository,
				postgres.NewOutboundMessageRepository(database),
				outboxStore,
				database,
			)
			autoReplyContextBuilder := services.NewAutoReplyContextBuilder(
				postgres.NewBusinessRepository(database),
				postgres.NewConversationRepository(database),
				postgres.NewCustomerRepository(database),
				postgres.NewCatalogRepository(database),
				postgres.NewMessageRepository(database),
			)
			autoReplyContextBuilder.Knowledge = postgres.NewKnowledgeDocumentRepository(database)
			autoReplyContextBuilder.Policies = postgres.NewBusinessPolicyRepository(database)
			autoReply.ContextBuilder = autoReplyContextBuilder
			autoReply.PolicyEvaluator = services.GroundedPolicyEngine{}

			chatwootService.AutoReply = &services.ChatwootAutoReplyBridge{

				Resolver: services.ChatwootProviderReferenceResolver{
					References:  referenceRepository,
					Connections: postgres.NewChannelConnectionRepository(database),
				},
				AutoReply: autoReply,
			}
		}
		dependencies.IngestChatwootWebhook = chatwootService
	}
	_, mux := contract.BuildAPIWithHandlers(handlers.NewServer(dependencies))
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
				_, _ = writer.Write([]byte(`{"status":"failed"}`))
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
	return &APIRuntime{HTTP: server, Database: database, Dependencies: dependencies, EventStore: eventStore, Outbox: outboxStore, SocialAPI: external.SocialAPI, Chatwoot: external.Chatwoot, SocialWebhook: external.SocialWebhook, ChatwootWebhook: external.ChatwootWebhook, ChannelProvisioning: provisioningService}, nil
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
