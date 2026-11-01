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
	ready      chan struct{}
	readyOnce  sync.Once
}

func NewBroker(pool *pgxpool.Pool, hub *LocalHub) *Broker {
	if hub == nil {
		hub = NewLocalHub()
	}
	return &Broker{
		pool:  pool,
		Hub:   hub,
		ready: make(chan struct{}),
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
	// This stream invalidates HTTP views; it does not transport message bodies.
	// Keep private text out of notifications and stay below PostgreSQL NOTIFY's
	// payload limit even when the original message is long.
	event.Data = nil
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
			b.readyOnce.Do(func() { close(b.ready) })
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
	reconnectListener(ctx, b.listenOnce, b.LogWarning)
}

// Recover per connection attempt, not around the entire goroutine: recovering
// at the outer goroutine boundary returns forever and silently stops realtime.
func runListenerAttempt(ctx context.Context, attempt func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("realtime listener recovered from an internal failure")
		}
	}()
	return attempt(ctx)
}

func reconnectListener(ctx context.Context, attempt func(context.Context) error, warn func(string, ...any)) {
	backoff := 100 * time.Millisecond
	for ctx.Err() == nil {
		started := time.Now()
		err := runListenerAttempt(ctx, attempt)
		if ctx.Err() != nil {
			return
		}
		if warn != nil {
			warn("LISTEN connection ended; reconnecting: %v", err)
		}
		if time.Since(started) >= time.Second {
			backoff = 100 * time.Millisecond
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

func (b *Broker) listenOnce(ctx context.Context) error {
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire LISTEN connection: %w", err)
	}
	defer conn.Release()
	pgxConn := conn.Conn()
	// Never return a subscribed or possibly interrupted connection to query
	// borrowers. Closing it also clears its PostgreSQL LISTEN registration.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pgxConn.Close(cleanupCtx)
	}()
	if _, err := pgxConn.Exec(ctx, "LISTEN "+RealtimeNotificationChannel); err != nil {
		return fmt.Errorf("exec LISTEN: %w", err)
	}
	if b.Hub != nil {
		b.Hub.ResyncAll()
	}
	b.readyOnce.Do(func() { close(b.ready) })
	for {
		notification, err := pgxConn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait for notification: %w", err)
		}
		if notification == nil || strings.TrimSpace(notification.Payload) == "" {
			continue
		}
		var event ports.RealtimeEvent
		if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
			if b.LogWarning != nil {
				b.LogWarning("discarding malformed notification payload: %v", err)
			}
			continue
		}
		if b.Hub != nil {
			b.Hub.Broadcast(event)
		}
	}
}

// WaitReady is useful for startup probes and deterministic cross-instance tests.
// It never reports readiness until PostgreSQL has acknowledged LISTEN.
func (b *Broker) WaitReady(ctx context.Context) error {
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
