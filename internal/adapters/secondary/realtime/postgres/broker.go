package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RealtimeNotificationChannel = "mujeeb_realtime_events"
)

type Broker struct {
	pool       *pgxpool.Pool
	Hub        *LocalHub
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	startOnce  sync.Once
	closeOnce  sync.Once
	LogWarning func(format string, args ...any)
}

func NewBroker(pool *pgxpool.Pool, hub *LocalHub) *Broker {
	if hub == nil {
		hub = NewLocalHub()
	}
	return &Broker{
		pool: pool,
		Hub:  hub,
		LogWarning: func(format string, args ...any) {
			log.Printf("[RealtimeBroker] "+format, args...)
		},
	}
}

// Publish serializes and sends a realtime event notification via PostgreSQL LISTEN/NOTIFY.
// It must only be invoked after the corresponding database transaction has committed.
func (b *Broker) Publish(ctx context.Context, event ports.RealtimeEvent) error {
	if strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.EventType) == "" || strings.TrimSpace(event.BusinessID) == "" {
		return errors.New("event_id, event_type, and business_id are required")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal realtime event: %w", err)
	}

	// If database pool is unavailable (e.g. isolated unit tests), deliver directly to local hub
	if b.pool == nil {
		if b.Hub != nil {
			b.Hub.Broadcast(event)
		}
		return nil
	}

	// Execute pg_notify on PostgreSQL
	_, err = b.pool.Exec(ctx, "SELECT pg_notify($1, $2)", RealtimeNotificationChannel, string(encoded))
	if err != nil {
		if b.LogWarning != nil {
			b.LogWarning("pg_notify failed for event %s (%s): %v", event.EventID, event.EventType, err)
		}
		// Also deliver locally as best-effort if notification query failed
		if b.Hub != nil {
			b.Hub.Broadcast(event)
		}
		return fmt.Errorf("pg_notify: %w", err)
	}
	return nil
}

// StartListener begins the background PostgreSQL LISTEN loop and forwards received events to the LocalHub.
func (b *Broker) StartListener(ctx context.Context) {
	b.startOnce.Do(func() {
		if b.pool == nil {
			return
		}
		listenerCtx, cancel := context.WithCancel(ctx)
		b.cancel = cancel
		b.wg.Add(1)
		go b.runListener(listenerCtx)
	})
}

func (b *Broker) runListener(ctx context.Context) {
	defer b.wg.Done()
	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		conn, err := b.pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if b.LogWarning != nil {
				b.LogWarning("acquire connection for LISTEN failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			}
		}

		backoff = 100 * time.Millisecond // reset backoff on successful acquire
		pgxConn := conn.Conn()

		_, err = pgxConn.Exec(ctx, "LISTEN "+RealtimeNotificationChannel)
		if err != nil {
			conn.Release()
			if ctx.Err() != nil {
				return
			}
			if b.LogWarning != nil {
				b.LogWarning("exec LISTEN failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}

		// Inner notification loop
		for {
			if ctx.Err() != nil {
				conn.Release()
				return
			}

			notification, waitErr := pgxConn.WaitForNotification(ctx)
			if waitErr != nil {
				conn.Release()
				if ctx.Err() != nil {
					return
				}
				if b.LogWarning != nil {
					b.LogWarning("WaitForNotification returned error: %v", waitErr)
				}
				break // break inner loop to reconnect
			}

			if notification != nil && strings.TrimSpace(notification.Payload) != "" {
				var event ports.RealtimeEvent
				if jsonErr := json.Unmarshal([]byte(notification.Payload), &event); jsonErr != nil {
					if b.LogWarning != nil {
						b.LogWarning("discarding malformed notification payload: %v", jsonErr)
					}
					continue
				}
				if b.Hub != nil {
					b.Hub.Broadcast(event)
				}
			}
		}
	}
}

func (b *Broker) Close() {
	b.closeOnce.Do(func() {
		if b.cancel != nil {
			b.cancel()
		}
		b.wg.Wait()
		if b.Hub != nil {
			b.Hub.Close()
		}
	})
}

var _ ports.RealtimePublisher = (*Broker)(nil)
