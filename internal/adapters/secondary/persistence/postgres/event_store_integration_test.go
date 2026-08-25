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

func TestInboundEventStoreAgainstPostgres(t *testing.T) {
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
	store := NewInboundEventStore(adapter)

	businessA := uuid.NewString()
	businessB := uuid.NewString()
	connectionA := uuid.NewString()
	connectionB := uuid.NewString()
	base := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Event A', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Event B', $2, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessA, businessB, base); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM inbound_event_ledger WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	}()
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'connection-a', 'active', 'secret-ref-a', $3, $3), ($4::uuid, $5::uuid, 'socialapi', 'instagram', 'connection-b', 'active', 'secret-ref-b', $3, $3)`, connectionA, businessA, base, connectionB, businessB); err != nil {
		t.Fatalf("insert connections: %v", err)
	}

	businessAPtr := stringPointerForEventStore(businessA)
	connectionAPtr := stringPointerForEventStore(connectionA)
	draft := ports.InboundEventDraft{ID: uuid.NewString(), ProviderRef: "socialapi", ProviderConnectionRef: "connection-a", ProviderEventID: "evt-sequential", DedupeStrategy: "provider_event_id", BusinessID: businessAPtr, ConnectionID: connectionAPtr, EventType: "interaction_received", InteractionKind: stringPointerForEventStore("dm"), ProviderMessageID: stringPointerForEventStore("msg-1"), ProviderConversationID: stringPointerForEventStore("conv-1"), ExternalUserID: stringPointerForEventStore("user-1"), ContentReference: stringPointerForEventStore("content-ref-1"), ReceivedAt: base, RawPayloadReference: "raw://event/1", PayloadHash: "hash-1", SignatureVerified: true, ProcessingState: "received", CreatedAt: base, UpdatedAt: base}
	created, first, err := store.RecordIfAbsent(ctx, draft)
	if err != nil || !created || first.ID != draft.ID || first.ProcessingState != "received" {
		t.Fatalf("first record: created=%v record=%#v err=%v", created, first, err)
	}
	created, duplicate, err := store.RecordIfAbsent(ctx, draft)
	if err != nil || created || duplicate.ID != draft.ID {
		t.Fatalf("sequential duplicate: created=%v record=%#v err=%v", created, duplicate, err)
	}

	otherConnectionDraft := draft
	otherConnectionDraft.ID = uuid.NewString()
	otherConnectionDraft.ProviderConnectionRef = "connection-b"
	otherConnectionDraft.BusinessID = stringPointerForEventStore(businessB)
	otherConnectionDraft.ConnectionID = stringPointerForEventStore(connectionB)
	created, otherConnection, err := store.RecordIfAbsent(ctx, otherConnectionDraft)
	if err != nil || !created || otherConnection.ID != otherConnectionDraft.ID {
		t.Fatalf("provider connection scoped identity: created=%v record=%#v err=%v", created, otherConnection, err)
	}

	var concurrentDraft = draft
	concurrentDraft.ID = uuid.NewString()
	concurrentDraft.ProviderEventID = "evt-concurrent"
	workers := 32
	start := make(chan struct{})
	results := make(chan bool, workers)
	errorsCh := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			created, record, recordErr := store.RecordIfAbsent(ctx, concurrentDraft)
			if recordErr != nil {
				errorsCh <- recordErr
				return
			}
			if record.ID != concurrentDraft.ID {
				errorsCh <- fmt.Errorf("concurrent winner returned unexpected id %s", record.ID)
				return
			}
			results <- created
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	createdCount := 0
	for created := range results {
		if created {
			createdCount++
		}
	}
	for workerErr := range errorsCh {
		t.Fatalf("concurrent record: %v", workerErr)
	}
	if createdCount != 1 {
		t.Fatalf("concurrent dedupe created %d rows, want 1", createdCount)
	}

	claimDraft := draft
	claimDraft.ID = uuid.NewString()
	claimDraft.ProviderEventID = "evt-concurrent-claim"
	if created, _, err := store.RecordIfAbsent(ctx, claimDraft); err != nil || !created {
		t.Fatalf("concurrent claim record: created=%v err=%v", created, err)
	}
	leaseExpiry := time.Now().UTC().Add(2 * time.Hour)
	claimWorkers := 32
	claimStart := make(chan struct{})
	claimResults := make(chan ports.InboundEventClaimResult, claimWorkers)
	claimErrors := make(chan error, claimWorkers)
	var claimWait sync.WaitGroup
	for i := 0; i < claimWorkers; i++ {
		claimWait.Add(1)
		go func() {
			defer claimWait.Done()
			<-claimStart
			claimResult, claimErr := store.Claim(ctx, claimDraft.ID, ports.InboundEventLease{Owner: "claim-worker", Token: uuid.NewString(), ExpiresAt: leaseExpiry})
			if claimErr != nil {
				claimErrors <- claimErr
				return
			}
			claimResults <- claimResult
		}()
	}
	close(claimStart)
	claimWait.Wait()
	close(claimResults)
	close(claimErrors)
	claimedCount := 0
	for claimResult := range claimResults {
		if claimResult.Claimed {
			claimedCount++
		}
		if claimResult.Record.ID != claimDraft.ID {
			t.Fatalf("concurrent claim returned unexpected id %s", claimResult.Record.ID)
		}
	}
	for claimErr := range claimErrors {
		t.Fatalf("concurrent claim: %v", claimErr)
	}
	if claimedCount != 1 {
		t.Fatalf("concurrent claim won %d times, want 1", claimedCount)
	}

	page, err := store.List(ctx, ports.InboundEventFilter{BusinessID: businessA, ProviderRef: "socialapi", Limit: 1})
	if err != nil || len(page.Items) != 1 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("event keyset page: %#v err=%v", page, err)
	}
	if _, err := store.List(ctx, ports.InboundEventFilter{BusinessID: businessA, Limit: 1, Cursor: "malformed"}); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("event malformed cursor: %v", err)
	}
	if _, err := store.Get(ctx, businessB, draft.ID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("event tenant isolation: %v", err)
	}
	if _, err := store.Get(ctx, businessA, uuid.NewString()); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("event not_found: %v", err)
	}

	owner := "worker-a"
	token := uuid.NewString()
	claim, err := store.Claim(ctx, draft.ID, ports.InboundEventLease{Owner: owner, Token: token, ExpiresAt: leaseExpiry})
	if err != nil || !claim.Claimed || claim.Record.ProcessingState != "processing" || claim.Record.AttemptCount != 1 || claim.Record.ProcessingLeaseToken == nil {
		t.Fatalf("first claim: %#v err=%v", claim, err)
	}
	secondClaim, err := store.Claim(ctx, draft.ID, ports.InboundEventLease{Owner: "worker-b", Token: uuid.NewString(), ExpiresAt: leaseExpiry})
	if err != nil || secondClaim.Claimed || secondClaim.Record.ID != draft.ID {
		t.Fatalf("duplicate active claim: %#v err=%v", secondClaim, err)
	}
	processed, err := store.MarkProcessed(ctx, draft.ID, ports.InboundEventCompletion{Owner: owner, Token: token, ResultCode: "normalized", ProcessedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(2 * time.Minute)})
	if err != nil || processed.ProcessingState != "processed" || processed.ProcessedAt == nil || processed.ProcessingOwner != nil || processed.ProcessingLeaseToken != nil {
		t.Fatalf("mark processed: %#v err=%v", processed, err)
	}
	processedClaim, err := store.Claim(ctx, draft.ID, ports.InboundEventLease{Owner: owner, Token: uuid.NewString(), ExpiresAt: leaseExpiry})
	if err != nil || processedClaim.Claimed || processedClaim.Record.ProcessingState != "processed" {
		t.Fatalf("processed claim: %#v err=%v", processedClaim, err)
	}
	if _, err := store.MarkProcessed(ctx, draft.ID, ports.InboundEventCompletion{Owner: owner, Token: token, ResultCode: "old-worker", ProcessedAt: base.Add(3 * time.Minute), UpdatedAt: base.Add(3 * time.Minute)}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("old lease mutation: %v", err)
	}

	retryDraft := draft
	retryDraft.ID = uuid.NewString()
	retryDraft.ProviderEventID = "evt-retry"
	if created, _, err := store.RecordIfAbsent(ctx, retryDraft); err != nil || !created {
		t.Fatalf("retry record: created=%v err=%v", created, err)
	}
	retryToken := uuid.NewString()
	if result, err := store.Claim(ctx, retryDraft.ID, ports.InboundEventLease{Owner: owner, Token: retryToken, ExpiresAt: leaseExpiry}); err != nil || !result.Claimed {
		t.Fatalf("retry claim: %#v err=%v", result, err)
	}
	nextAttempt := leaseExpiry
	retryFailed, err := store.MarkRetryableFailure(ctx, retryDraft.ID, ports.InboundEventFailure{Owner: owner, Token: retryToken, ErrorCode: "normalization_failed", NextAttempt: &nextAttempt, UpdatedAt: base.Add(3 * time.Minute)})
	if err != nil || retryFailed.ProcessingState != "retryable_failed" || retryFailed.LastErrorCode == nil || retryFailed.NextAttemptAt == nil {
		t.Fatalf("retryable failure: %#v err=%v", retryFailed, err)
	}
	beforeRetry, err := store.Claim(ctx, retryDraft.ID, ports.InboundEventLease{Owner: owner, Token: uuid.NewString(), ExpiresAt: leaseExpiry})
	if err != nil || beforeRetry.Claimed {
		t.Fatalf("early retry claim: %#v err=%v", beforeRetry, err)
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE inbound_event_ledger SET next_attempt_at = $2 WHERE id = $1::uuid`, retryDraft.ID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("make retry due: %v", err)
	}
	retryClaim, err := store.Claim(ctx, retryDraft.ID, ports.InboundEventLease{Owner: owner, Token: uuid.NewString(), ExpiresAt: leaseExpiry})
	if err != nil || !retryClaim.Claimed || retryClaim.Record.AttemptCount != 2 {
		t.Fatalf("due retry claim: %#v err=%v", retryClaim, err)
	}
	deadLetter, err := store.MoveToDeadLetter(ctx, retryDraft.ID, ports.InboundEventFailure{Owner: owner, Token: *retryClaim.Record.ProcessingLeaseToken, ErrorCode: "max_attempts", UpdatedAt: base.Add(4 * time.Minute)})
	if err != nil || deadLetter.ProcessingState != "dead_letter" || deadLetter.ProcessingOwner != nil || deadLetter.LeaseExpiresAt != nil {
		t.Fatalf("dead letter: %#v err=%v", deadLetter, err)
	}

	unverified := draft
	unverified.ID = uuid.NewString()
	unverified.ProviderEventID = "evt-unverified"
	unverified.SignatureVerified = false
	unverified.ProcessingState = "received"
	if _, _, err := store.RecordIfAbsent(ctx, unverified); !IsRepositoryKind(err, RepositoryInvalid) {
		t.Fatalf("unverified normal event was accepted: %v", err)
	}
	unverified.ProcessingState = "rejected"
	if created, _, err := store.RecordIfAbsent(ctx, unverified); err != nil || !created {
		t.Fatalf("unverified rejected event: created=%v err=%v", created, err)
	}

	rollbackBusiness := uuid.NewString()
	rollbackConnection := uuid.NewString()
	rollbackEvent := uuid.NewString()
	rollbackErr := errors.New("event store rollback")
	if err := adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Rollback Event', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, rollbackBusiness); err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'rollback-connection', 'active', 'secret-ref', now(), now())`, rollbackConnection, rollbackBusiness); err != nil {
			return err
		}
		business := rollbackBusiness
		connection := rollbackConnection
		_, _, err = store.RecordIfAbsent(txCtx, ports.InboundEventDraft{ID: rollbackEvent, ProviderRef: "socialapi", ProviderConnectionRef: "rollback-connection", ProviderEventID: "evt-rollback", DedupeStrategy: "provider_event_id", BusinessID: &business, ConnectionID: &connection, EventType: "interaction_received", RawPayloadReference: "raw://rollback", PayloadHash: "rollback-hash", SignatureVerified: true, ProcessingState: "received", ReceivedAt: base, CreatedAt: base, UpdatedAt: base})
		if err != nil {
			return err
		}
		return rollbackErr
	}); !errors.Is(err, rollbackErr) {
		t.Fatalf("expected event rollback: %v", err)
	}
	var count int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM inbound_event_ledger WHERE id = $1::uuid`, rollbackEvent).Scan(&count); err != nil {
		t.Fatalf("rollback event query: %v", err)
	}
	if count != 0 {
		t.Fatalf("event rollback leaked %d rows", count)
	}
}

func stringPointerForEventStore(value string) *string { return &value }

var _ ports.EventStore = (*InboundEventStore)(nil)
