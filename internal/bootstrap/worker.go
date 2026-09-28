package bootstrap

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type WorkerRuntime struct {
	Database     *postgres.Adapter
	EventStore   ports.EventStore
	Outbox       ports.OutboxStore
	SocialAPI    ports.ChannelProvider
	Processor    *services.OutboxProcessor
	PollInterval time.Duration
	CycleTimeout time.Duration
	BatchSize    int
	WorkerOwner  string
}

func BuildWorker(ctx context.Context, cfg config.ProcessConfig) (*WorkerRuntime, error) {
	database, err := postgres.Open(ctx, cfg.DatabaseURL, postgres.PoolConfig{MaxConns: cfg.DBMaxConns, MinConns: cfg.DBMinConns, MaxConnLifetime: cfg.DBMaxConnLifetime, MaxConnIdleTime: cfg.DBMaxConnIdleTime, HealthCheckPeriod: cfg.DBHealthCheckPeriod, ConnectTimeout: cfg.DBConnectTimeout})
	if err != nil {
		return nil, err
	}
	external := BuildExternalAdapters(cfg)
	outbox := postgres.NewPostgresOutboxStore(database)
	runtime := &WorkerRuntime{Database: database, EventStore: postgres.NewInboundEventStore(database), Outbox: outbox, SocialAPI: external.SocialAPI, PollInterval: cfg.WorkerPollInterval, CycleTimeout: cfg.ShutdownTimeout, BatchSize: cfg.WorkerBatchSize, WorkerOwner: cfg.WorkerOwner}
	if external.SocialAPI != nil {
		runtime.Processor = &services.OutboxProcessor{
			Outbox: outbox,
			Resolver: services.MujeebOutboundDeliveryResolver{
				OutboundRepository:   postgres.NewOutboundMessageRepository(database),
				ReferenceRepository:  postgres.NewConversationReferenceRepository(database),
				ConnectionRepository: postgres.NewChannelConnectionRepository(database),
			},
			Provider: external.SocialAPI,
			Messages: postgres.NewOutboundMessageRepository(database),
			Owner:    runtime.WorkerOwner,
		}
	}
	return runtime, nil
}

func (r *WorkerRuntime) Run(ctx context.Context) error {
	if r == nil || r.Database == nil || r.EventStore == nil || r.Outbox == nil {
		return errors.New("worker runtime is not configured")
	}
	interval := r.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			cycleContext, cancel := context.WithTimeout(context.Background(), r.cycleTimeout())
			_, err := r.RunOnce(cycleContext)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func (r *WorkerRuntime) cycleTimeout() time.Duration {
	if r != nil && r.CycleTimeout > 0 {
		return r.CycleTimeout
	}
	return 10 * time.Second
}

func (r *WorkerRuntime) RunOnce(ctx context.Context) (int, error) {
	if r == nil || r.Outbox == nil {
		return 0, errors.New("worker runtime is not configured")
	}
	limit := r.BatchSize
	if limit <= 0 {
		limit = 20
	}
	processed := 0
	if r.Processor != nil {
		entries, err := r.Outbox.ListClaimable(ctx, limit)
		if err != nil {
			log.Printf("[Worker] LIST_ERROR err=%v", err)
			return processed, err
		}
		if len(entries) > 0 {
			log.Printf("[Worker] POLL found=%d entries", len(entries))
		}
		for _, entry := range entries {
			if err := r.Processor.Process(ctx, entry.ID); err != nil {
				log.Printf("[Worker] PROCESS_ERROR outbox=%s err=%v", entry.ID, err)
				return processed, err
			}
			processed++
		}
	}
	return processed, nil
}

func (r *WorkerRuntime) Shutdown() {
	if r != nil && r.Database != nil {
		r.Database.Close()
	}
}
