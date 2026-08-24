//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestOutboxStoreAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()
	outbox := NewPostgresOutboxStore(adapter)
	outbound := NewOutboundMessageRepository(adapter)

	businessA := uuid.NewString()
	businessB := uuid.NewString()
	connectionA := uuid.NewString()
	connectionB := uuid.NewString()
	customerA := uuid.NewString()
	conversationA := uuid.NewString()
	referenceA := uuid.NewString()
	base := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Outbox A', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Outbox B', $2, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessA, businessB, base); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM outbox_entries WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM outbound_messages WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	}()
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'outbox-connection-a', 'active', 'secret-a', $3, $3), ($4::uuid, $5::uuid, 'socialapi', 'instagram', 'outbox-connection-b', 'active', 'secret-b', $3, $3)`, connectionA, businessA, base, connectionB, businessB); err != nil {
		t.Fatalf("insert connections: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', $3, $3)`, customerA, businessA, base); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationA, businessA, customerA, base); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'provider-conversation-a', $4::uuid, 'dm', true, 'active', $5, $5)`, referenceA, businessA, conversationA, connectionA, base); err != nil {
		t.Fatalf("insert conversation reference: %v", err)
	}

	makePair := func(messageID, entryID, dedupe string, availableAt time.Time) (ports.OutboundMessageRecord, ports.OutboxEntryRecord, error) {
		var message ports.OutboundMessageRecord
		var entry ports.OutboxEntryRecord
		err := adapter.Within(ctx, func(txCtx context.Context) error {
			createdMessage, err := outbound.CreatePending(txCtx, ports.OutboundMessageDraft{ID: messageID, BusinessID: businessA, ConversationID: conversationA, ConversationReferenceID: referenceA, ConnectionID: connectionA, ProviderRef: "socialapi", Channel: "whatsapp", Origin: "human", Transport: "provider", ContentReference: "content://" + messageID, ProviderIdempotencyKey: "provider-key-" + messageID})
			if err != nil {
				return err
			}
			createdEntry, err := outbox.Enqueue(txCtx, ports.OutboxEntryDraft{ID: entryID, BusinessID: businessA, OutboundMessageID: messageID, CommandType: "send_outbound_message", DedupeKey: dedupe, AvailableAt: availableAt, CreatedAt: base, UpdatedAt: base})
			if err != nil {
				return err
			}
			message, entry = createdMessage, createdEntry
			return nil
		})
		return message, entry, err
	}

	message1, entry1, err := makePair(uuid.NewString(), uuid.NewString(), "logical-send-1", base)
	if err != nil || message1.Status != "pending" || entry1.Status != "pending" {
		t.Fatalf("atomic create pair: message=%#v entry=%#v err=%v", message1, entry1, err)
	}
	duplicateMessage, err := outbound.CreatePending(ctx, ports.OutboundMessageDraft{ID: uuid.NewString(), BusinessID: businessA, ConversationID: conversationA, ConversationReferenceID: referenceA, ConnectionID: connectionA, ProviderRef: "socialapi", Channel: "whatsapp", Origin: "human", Transport: "provider", ContentReference: "content://duplicate", ProviderIdempotencyKey: "provider-key-duplicate"})
	if err != nil {
		t.Fatalf("duplicate outbound message: %v", err)
	}
	if _, err := outbox.Enqueue(ctx, ports.OutboxEntryDraft{ID: uuid.NewString(), BusinessID: businessA, OutboundMessageID: duplicateMessage.ID, CommandType: "send_outbound_message", DedupeKey: "logical-send-1", AvailableAt: base, CreatedAt: base, UpdatedAt: base}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("outbox dedupe conflict: %v", err)
	}
	if _, err := outbox.Get(ctx, businessB, entry1.ID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("outbox tenant isolation: %v", err)
	}
	if _, err := outbox.Get(ctx, businessA, uuid.NewString()); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("outbox not_found: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, _, err := makePair(uuid.NewString(), uuid.NewString(), fmt.Sprintf("logical-page-%d", i), base.Add(time.Duration(i+1)*time.Minute)); err != nil {
			t.Fatalf("page pair %d: %v", i, err)
		}
	}
	page, err := outbox.List(ctx, ports.OutboxFilter{BusinessID: businessA, Limit: 2})
	if err != nil || len(page.Items) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("outbox keyset: %#v err=%v", page, err)
	}
	if _, err := outbox.List(ctx, ports.OutboxFilter{BusinessID: businessA, Limit: 1, Cursor: "malformed"}); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("outbox malformed cursor: %v", err)
	}

	claimJob := func(entryID string) ports.OutboxClaimResult {
		result, err := outbox.Claim(ctx, entryID, ports.OutboxLease{Owner: "worker", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
		if err != nil {
			t.Fatalf("claim %s: %v", entryID, err)
		}
		return result
	}
	claim := claimJob(entry1.ID)
	if !claim.Claimed || claim.Record.Status != "processing" || claim.Record.AttemptCount != 1 || claim.Record.LeaseToken == nil {
		t.Fatalf("first outbox claim: %#v", claim)
	}
	activeDuplicate, err := outbox.Claim(ctx, entry1.ID, ports.OutboxLease{Owner: "worker-2", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
	if err != nil || activeDuplicate.Claimed {
		t.Fatalf("active duplicate claim: %#v err=%v", activeDuplicate, err)
	}
	completed, err := outbox.MarkCompleted(ctx, entry1.ID, ports.OutboxCompletion{Owner: *claim.Record.LeaseOwner, Token: *claim.Record.LeaseToken, ResultCode: "attempt_recorded", CompletedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	if err != nil || completed.Status != "completed" || completed.CompletedAt == nil || completed.LeaseOwner != nil || completed.LeaseToken != nil {
		t.Fatalf("complete outbox: %#v err=%v", completed, err)
	}
	if _, err := outbox.MarkCompleted(ctx, entry1.ID, ports.OutboxCompletion{Owner: "old-worker", Token: uuid.NewString(), ResultCode: "late", CompletedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("stale completion: %v", err)
	}

	concurrentMessage, concurrentEntry, err := makePair(uuid.NewString(), uuid.NewString(), "logical-concurrent", base)
	if err != nil {
		t.Fatalf("concurrent pair: %v", err)
	}
	_ = concurrentMessage
	workers := 32
	start := make(chan struct{})
	results := make(chan ports.OutboxClaimResult, workers)
	errorsCh := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := outbox.Claim(ctx, concurrentEntry.ID, ports.OutboxLease{Owner: "concurrent-worker", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	claimedCount := 0
	for result := range results {
		if result.Claimed {
			claimedCount++
		}
		if result.Record.ID != concurrentEntry.ID {
			t.Fatalf("concurrent claim id mismatch: %s", result.Record.ID)
		}
	}
	for err := range errorsCh {
		t.Fatalf("concurrent outbox claim: %v", err)
	}
	if claimedCount != 1 {
		t.Fatalf("concurrent outbox claim winners=%d, want 1", claimedCount)
	}

	retryMessage, retryEntry, err := makePair(uuid.NewString(), uuid.NewString(), "logical-retry", base)
	if err != nil {
		t.Fatalf("retry pair: %v", err)
	}
	_ = retryMessage
	retryClaim := claimJob(retryEntry.ID)
	nextAttempt := time.Now().UTC().Add(time.Hour)
	retryFailed, err := outbox.MarkRetryableFailure(ctx, retryEntry.ID, ports.OutboxFailure{Owner: *retryClaim.Record.LeaseOwner, Token: *retryClaim.Record.LeaseToken, ErrorCode: "provider_timeout", NextAttempt: &nextAttempt, UpdatedAt: time.Now().UTC()})
	if err != nil || retryFailed.Status != "retryable_failed" || retryFailed.LastErrorCode == nil || retryFailed.AvailableAt.Before(nextAttempt.Add(-time.Second)) {
		t.Fatalf("retryable failure: %#v err=%v", retryFailed, err)
	}
	earlyRetry, err := outbox.Claim(ctx, retryEntry.ID, ports.OutboxLease{Owner: "early", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
	if err != nil || earlyRetry.Claimed {
		t.Fatalf("early retry claim: %#v err=%v", earlyRetry, err)
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE outbox_entries SET available_at = $2 WHERE id = $1::uuid`, retryEntry.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("make retry due: %v", err)
	}
	retryClaimAgain, err := outbox.Claim(ctx, retryEntry.ID, ports.OutboxLease{Owner: "retry-worker", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
	if err != nil || !retryClaimAgain.Claimed || retryClaimAgain.Record.AttemptCount != 2 {
		t.Fatalf("due retry claim: %#v err=%v", retryClaimAgain, err)
	}
	dead, err := outbox.MoveToDeadLetter(ctx, retryEntry.ID, ports.OutboxFailure{Owner: *retryClaimAgain.Record.LeaseOwner, Token: *retryClaimAgain.Record.LeaseToken, ErrorCode: "max_attempts", UpdatedAt: time.Now().UTC()})
	if err != nil || dead.Status != "dead_letter" || dead.LeaseOwner != nil || dead.LeaseToken != nil {
		t.Fatalf("dead letter: %#v err=%v", dead, err)
	}
	requeued, err := outbox.Requeue(ctx, retryEntry.ID, time.Now().UTC().Add(-time.Minute), time.Now().UTC())
	if err != nil || requeued.Status != "pending" || requeued.CompletedAt != nil {
		t.Fatalf("requeue: %#v err=%v", requeued, err)
	}

	expiredMessage, expiredEntry, err := makePair(uuid.NewString(), uuid.NewString(), "logical-expired", base)
	if err != nil {
		t.Fatalf("expired pair: %v", err)
	}
	_ = expiredMessage
	expiredClaim, err := outbox.Claim(ctx, expiredEntry.ID, ports.OutboxLease{Owner: "expired-worker", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
	if err != nil || !expiredClaim.Claimed {
		t.Fatalf("expired initial claim: %#v err=%v", expiredClaim, err)
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE outbox_entries SET lease_expires_at = $2 WHERE id = $1::uuid`, expiredEntry.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	reclaimed, err := outbox.Claim(ctx, expiredEntry.ID, ports.OutboxLease{Owner: "new-worker", Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(30 * time.Minute)})
	if err != nil || !reclaimed.Claimed || reclaimed.Record.LeaseOwner == nil || *reclaimed.Record.LeaseOwner != "new-worker" {
		t.Fatalf("lease recovery: %#v err=%v", reclaimed, err)
	}

	rollbackBusiness := uuid.NewString()
	rollbackConnection := uuid.NewString()
	rollbackCustomer := uuid.NewString()
	rollbackConversation := uuid.NewString()
	rollbackReference := uuid.NewString()
	rollbackMessage := uuid.NewString()
	rollbackEntry := uuid.NewString()
	rollbackErr := errors.New("outbox atomic rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Rollback Outbox', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, rollbackBusiness); err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'rollback-outbox-connection', 'active', 'secret', now(), now())`, rollbackConnection, rollbackBusiness); err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', now(), now())`, rollbackCustomer, rollbackBusiness); err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', now(), now(), now())`, rollbackConversation, rollbackBusiness, rollbackCustomer); err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'rollback-reference', $4::uuid, 'dm', true, 'active', now(), now())`, rollbackReference, rollbackBusiness, rollbackConversation, rollbackConnection); err != nil {
			return err
		}
		if _, err := outbound.CreatePending(txCtx, ports.OutboundMessageDraft{ID: rollbackMessage, BusinessID: rollbackBusiness, ConversationID: rollbackConversation, ConversationReferenceID: rollbackReference, ConnectionID: rollbackConnection, ProviderRef: "socialapi", Channel: "whatsapp", Origin: "human", Transport: "provider", ContentReference: "content://rollback", ProviderIdempotencyKey: "rollback-provider-key"}); err != nil {
			return err
		}
		if _, err := outbox.Enqueue(txCtx, ports.OutboxEntryDraft{ID: rollbackEntry, BusinessID: rollbackBusiness, OutboundMessageID: rollbackMessage, CommandType: "send_outbound_message", DedupeKey: "rollback-dedupe", AvailableAt: base, CreatedAt: base, UpdatedAt: base}); err != nil {
			return err
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected atomic rollback: %v", err)
	}
	var remaining int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM outbox_entries WHERE id = $1::uuid`, rollbackEntry).Scan(&remaining); err != nil {
		t.Fatalf("rollback outbox query: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("outbox rollback leaked %d rows", remaining)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM outbound_messages WHERE id = $1::uuid`, rollbackMessage).Scan(&remaining); err != nil {
		t.Fatalf("rollback outbound query: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("outbound rollback leaked %d rows", remaining)
	}
}

var _ ports.OutboxStore = (*PostgresOutboxStore)(nil)
