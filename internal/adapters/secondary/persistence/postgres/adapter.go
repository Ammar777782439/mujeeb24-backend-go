package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPoolClosed        = errors.New("postgres pool is closed")
	ErrNestedTransaction = errors.New("nested transactions are not supported")
)

type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
}

func DefaultPoolConfig() PoolConfig {
	return PoolConfig{MaxConns: 10, MinConns: 1, MaxConnLifetime: time.Hour, MaxConnIdleTime: 30 * time.Minute, HealthCheckPeriod: time.Minute, ConnectTimeout: 5 * time.Second}
}

type tx interface {
	Commit(context.Context) error
	Rollback(context.Context) error
}
type beginner interface {
	Begin(context.Context) (tx, error)
}
type closer interface{ Close() }
type pinger interface{ Ping(context.Context) error }

type poolBeginner struct{ pool *pgxpool.Pool }

func (p poolBeginner) Begin(ctx context.Context) (tx, error) { return p.pool.Begin(ctx) }

type Adapter struct {
	pool     *pgxpool.Pool
	beginner beginner
	closer   closer
	pinger   pinger
}

func Open(ctx context.Context, databaseURL string, cfg PoolConfig) (*Adapter, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database URL is required")
	}
	parsed, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	applyPoolConfig(parsed, cfg)
	if cfg.ConnectTimeout > 0 {
		parsed.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	}
	pool, err := pgxpool.NewWithConfig(ctx, parsed)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	adapter := NewFromPool(pool)
	if err := adapter.Ping(ctx); err != nil {
		adapter.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return adapter, nil
}

func applyPoolConfig(cfg *pgxpool.Config, values PoolConfig) {
	if values.MaxConns > 0 {
		cfg.MaxConns = values.MaxConns
	}
	if values.MinConns > 0 {
		cfg.MinConns = values.MinConns
	}
	if values.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = values.MaxConnLifetime
	}
	if values.MaxConnIdleTime > 0 {
		cfg.MaxConnIdleTime = values.MaxConnIdleTime
	}
	if values.HealthCheckPeriod > 0 {
		cfg.HealthCheckPeriod = values.HealthCheckPeriod
	}
}

func NewFromPool(pool *pgxpool.Pool) *Adapter {
	if pool == nil {
		return &Adapter{}
	}
	return &Adapter{pool: pool, beginner: poolBeginner{pool: pool}, closer: pool, pinger: pool}
}

func (a *Adapter) Pool() *pgxpool.Pool {
	if a == nil {
		return nil
	}
	return a.pool
}
func (a *Adapter) Ping(ctx context.Context) error {
	if a == nil || a.pinger == nil {
		return ErrPoolClosed
	}
	return a.pinger.Ping(ctx)
}
func (a *Adapter) Close() {
	if a != nil && a.closer != nil {
		a.closer.Close()
	}
}

// Within owns exactly one database transaction. It never runs network calls
// itself; the callback is the application unit of work and commit happens only
// after the callback succeeds.
func (a *Adapter) Within(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil || a.beginner == nil {
		return ErrPoolClosed
	}
	if _, exists := transactionFromContext(ctx); exists {
		return ErrNestedTransaction
	}
	tx, err := a.beginner.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	callbackErr := fn(txCtx)
	if callbackErr != nil {
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return fmt.Errorf("%w; rollback transaction: %v", callbackErr, rollbackErr)
		}
		return callbackErr
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

type txContextKey struct{}

func transactionFromContext(ctx context.Context) (tx, bool) {
	value, ok := ctx.Value(txContextKey{}).(tx)
	return value, ok
}
func TransactionFromContext(ctx context.Context) (any, bool) {
	value, ok := transactionFromContext(ctx)
	return value, ok
}

var _ interface {
	Within(context.Context, func(context.Context) error) error
} = (*Adapter)(nil)
