package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/handlers"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type APIRuntime struct {
	HTTP            *http.Server
	Database        *postgres.Adapter
	Dependencies    handlers.Dependencies
	EventStore      ports.EventStore
	Outbox          ports.OutboxStore
	SocialAPI       ports.ChannelProvider
	Chatwoot        ports.CommunicationWorkspace
	SocialWebhook   ports.WebhookReceiver
	ChatwootWebhook ports.WebhookReceiver
	closeOnce       sync.Once
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
	eventStore := postgres.NewInboundEventStore(database)
	outboxStore := postgres.NewPostgresOutboxStore(database)
	if external.ChatwootWebhook != nil {
		chatwootService := services.ChatwootWebhookService{Receiver: external.ChatwootWebhook, Inbound: postgres.NewChatwootInboundStore(database)}
		if external.ChatwootAutoReplyEnabled {
			if external.AIRuntime == nil {
				return nil, errors.New("Chatwoot AutoReply requires a configured LLM runtime")
			}
			referenceRepository := postgres.NewConversationReferenceRepository(database)
			chatwootService.AutoReply = &services.ChatwootAutoReplyBridge{
				Resolver: services.ChatwootProviderReferenceResolver{
					References:  referenceRepository,
					Connections: postgres.NewChannelConnectionRepository(database),
				},
				AutoReply: services.NewAutoReplyService(
					external.AIRuntime,

					postgres.NewAIDecisionRepository(database),
					referenceRepository,
					postgres.NewOutboundMessageRepository(database),
					outboxStore,
					database,
				),
			}
		}
		dependencies.IngestChatwootWebhook = chatwootService
	}
	_, mux := contract.BuildAPIWithHandlers(handlers.NewServer(dependencies))
	mux.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok","service":"mujeeb24-api"}`))
	})
	return &APIRuntime{HTTP: &http.Server{Addr: address, Handler: mux}, Database: database, Dependencies: dependencies, EventStore: eventStore, Outbox: outboxStore, SocialAPI: external.SocialAPI, Chatwoot: external.Chatwoot, SocialWebhook: external.SocialWebhook, ChatwootWebhook: external.ChatwootWebhook}, nil
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
