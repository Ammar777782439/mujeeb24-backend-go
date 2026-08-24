package bootstrap

import (
	"context"
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type WorkerRuntime struct {
	Database   *postgres.Adapter
	EventStore ports.EventStore
	Outbox     ports.OutboxStore
}

func BuildWorker(ctx context.Context, cfg config.ProcessConfig) (*WorkerRuntime, error) {
	database, err := postgres.Open(ctx, cfg.DatabaseURL, postgres.PoolConfig{MaxConns: cfg.DBMaxConns, MinConns: cfg.DBMinConns, MaxConnLifetime: cfg.DBMaxConnLifetime, MaxConnIdleTime: cfg.DBMaxConnIdleTime, HealthCheckPeriod: cfg.DBHealthCheckPeriod, ConnectTimeout: cfg.DBConnectTimeout})
	if err != nil {
		return nil, err
	}
	return &WorkerRuntime{Database: database, EventStore: postgres.NewInboundEventStore(database), Outbox: postgres.NewPostgresOutboxStore(database)}, nil
}

func (r *WorkerRuntime) Run(ctx context.Context) error {
	if r == nil || r.Database == nil || r.EventStore == nil || r.Outbox == nil {
		return errors.New("worker runtime is not configured")
	}
	<-ctx.Done()
	return nil
}

func (r *WorkerRuntime) Shutdown() {
	if r != nil && r.Database != nil {
		r.Database.Close()
	}
}
