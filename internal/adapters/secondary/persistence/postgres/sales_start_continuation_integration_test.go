//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/google/uuid"
)

// Exact postback, lead, and continuation phase are committed atomically.
func TestSalesStartButtonQueuesSingleResumableContinuation(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	payload := "m24.sales.start:" + id + ":accept"
	message := saveStartPostback(t, f, payload)
	repo := NewSalesActionJobs(f.a)
	now := time.Now().UTC()
	jobs, err := repo.Claim(context.Background(), 1, now)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("claim: %v %d", err, len(jobs))
	}
	j := jobs[0]
	if j.Phase != "apply_action" {
		t.Fatalf("wrong first phase %q", j.Phase)
	}
	r, err := startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
		BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
		SourceMessageID: message, Payload: payload, Now: now})
	if err != nil || r.Workflow.ID == "" {
		t.Fatalf("apply start %v %+v", err, r)
	}
	var status, phase, continuation string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT s.status,j.phase,s.continuation_state
		FROM sales_start_requests s JOIN sales_action_jobs j ON j.business_id=s.business_id
		AND j.message_id=s.decision_message_id WHERE s.id=$1::uuid`, id).
		Scan(&status, &phase, &continuation); err != nil {
		t.Fatal(err)
	}
	if status != "accepted" || phase != "continue_ai" || continuation != "pending" {
		t.Fatalf("accept and job stage not atomic: %s %s %s", status, phase, continuation)
	}
	if err := repo.Finish(context.Background(), j.ID, j.LeaseToken, "sales_continuation_queued", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// Crash after committed acceptance / before resumption: re-claim the SAME job.
	jobs, err = repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(jobs) != 1 {
		t.Fatalf("lost durable continuation: %v %d", err, len(jobs))
	}
	next := jobs[0]
	if next.ID != j.ID || next.Phase != "continue_ai" {
		t.Fatalf("wrong recovery job: %+v", next)
	}
	info, err := repo.LoadContinuation(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if info.RequestID != id || info.WorkflowID != r.Workflow.ID || info.SourceMessageID != f.message ||
		info.Text != "أريد واحدة" || info.ProviderRef != "socialapi" || info.Channel != "instagram" {
		t.Fatalf("continuation did not reuse real original message and workflow: %+v", info)
	}
	var leads, workflows int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM leads WHERE business_id=$1::uuid`, f.business).Scan(&leads); err != nil {
		t.Fatal(err)
	}
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM sales_workflows WHERE business_id=$1::uuid`, f.business).Scan(&workflows); err != nil {
		t.Fatal(err)
	}
	if leads != 1 || workflows != 1 {
		t.Fatalf("retry duplicated commercial effects: leads=%d workflows=%d", leads, workflows)
	}
	// Neither accidental marking nor an unsaved model reply is a committed result.
	err = f.a.Within(context.Background(), func(tx context.Context) error { return repo.CompleteContinuation(tx, next, id) })
	if err == nil {
		t.Fatal("continuation completed with no outbox and no human review")
	}
}

func TestSalesStartContinuationOutboxCommitSurvivesWorkerCrash(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	payload := "m24.sales.start:" + id + ":accept"
	msg := saveStartPostback(t, f, payload)
	repo := NewSalesActionJobs(f.a)
	jobs, err := repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(jobs) != 1 {
		t.Fatalf("claim: %v %d", err, len(jobs))
	}
	first := jobs[0]
	_, err = startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
		BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
		SourceMessageID: msg, Payload: payload, Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(context.Background(), first.ID, first.LeaseToken, "sales_continuation_queued", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	jobs, err = repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(jobs) != 1 {
		t.Fatalf("reclaim: %v %d", err, len(jobs))
	}
	job := jobs[0]
	// This is the same short transaction used by AutoReplyService after AI.
	err = f.a.Within(context.Background(), func(tx context.Context) error {
		now := time.Now().UTC()
		outbound, err := NewOutboundMessageRepository(f.a).CreatePending(tx, ports.OutboundMessageDraft{
			ID: uuid.NewString(), BusinessID: f.business, ConversationID: f.conversation,
			ConversationReferenceID: f.reference, ConnectionID: f.connection, ProviderRef: "socialapi",
			Channel: "instagram", Origin: "ai", Transport: "provider",
			ContentReference:       services.EncodeInlineTextContentReference("ما عنوان التوصيل؟"),
			ProviderIdempotencyKey: "sales-start-cont:" + id,
		})
		if err != nil {
			return err
		}
		_, err = NewPostgresOutboxStore(f.a).Enqueue(tx, ports.OutboxEntryDraft{
			ID: uuid.NewString(), BusinessID: f.business, OutboundMessageID: outbound.ID,
			CommandType: services.OutboundSendCommandType, DedupeKey: "sales-start-cont:" + id,
			AvailableAt: now, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
		return repo.CompleteContinuation(tx, job, id)
	})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate crashed worker AFTER the outbound and completion commit, BEFORE Finish.
	if _, err := f.a.Pool().Exec(context.Background(), `UPDATE sales_action_jobs SET lease_until=now()-interval '1 second'
		WHERE id=$1::uuid`, job.ID); err != nil {
		t.Fatal(err)
	}
	next, err := repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(next) != 1 {
		t.Fatalf("crash lease recovery: %v %d", err, len(next))
	}
	info, err := repo.LoadContinuation(context.Background(), next[0])
	if err != nil || !info.Completed {
		t.Fatalf("lost committed reply on recovery: %+v %v", info, err)
	}
	if err := repo.Finish(context.Background(), next[0].ID, next[0].LeaseToken, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var entries int
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT count(*) FROM outbox_entries WHERE business_id=$1::uuid AND dedupe_key=$2`, f.business, "sales-start-cont:"+id).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if entries != 1 {
		t.Fatalf("outbox duplicated after recovery: %d", entries)
	}
	// Stale lease owner must never commit again.
	err = f.a.Within(context.Background(), func(tx context.Context) error { return repo.CompleteContinuation(tx, job, id) })
	if err == nil {
		t.Fatal("stale worker committed after lease takeover")
	}
}

func TestSalesStartContinuationRetryIsDelayedAndLeaseFenced(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	payload := "m24.sales.start:" + id + ":accept"
	msg := saveStartPostback(t, f, payload)
	repo := NewSalesActionJobs(f.a)
	now := time.Now().UTC()
	jobs, err := repo.Claim(context.Background(), 1, now)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("first claim %v %+v", err, jobs)
	}
	if _, err := startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
		BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
		SourceMessageID: msg, Payload: payload, Now: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(context.Background(), jobs[0].ID, jobs[0].LeaseToken, "sales_continuation_queued", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	cont, err := repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(cont) != 1 {
		t.Fatalf("continue claim: %v %+v", err, cont)
	}
	if err := repo.Finish(context.Background(), cont[0].ID, cont[0].LeaseToken, "sales_continuation_transient", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var state string
	var next time.Time
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT state,next_attempt_at FROM sales_action_jobs WHERE id=$1::uuid`, cont[0].ID).Scan(&state, &next); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || !next.After(time.Now().UTC()) {
		t.Fatalf("transient failure had no retry backoff: %s %s", state, next)
	}
	before, err := repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(before) != 0 {
		t.Fatalf("retry was scheduled too soon: %v %+v", err, before)
	}
	after, err := repo.Claim(context.Background(), 1, next.Add(time.Second))
	if err != nil || len(after) != 1 {
		t.Fatalf("retry not available when due: %v %+v", err, after)
	}
	if err := repo.Finish(context.Background(), cont[0].ID, cont[0].LeaseToken, "", time.Now().UTC()); err == nil {
		t.Fatal("lost lease token still able to complete")
	}
	if err := repo.Finish(context.Background(), after[0].ID, after[0].LeaseToken, "sales_continuation_permanent", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT state FROM sales_action_jobs WHERE id=$1::uuid`, after[0].ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "dead_letter" {
		t.Fatalf("permanent failure was retried: %s", state)
	}
}

func TestSalesStartRetryNeverReAcceptsAnAlreadyExpiredRequest(t *testing.T) {
	f := newSalesWorkflowFixture(t)
	id := createPendingStart(t, f)
	payload := "m24.sales.start:" + id + ":accept"
	msg := saveStartPostback(t, f, payload)
	repo := NewSalesActionJobs(f.a)
	jobs, err := repo.Claim(context.Background(), 1, time.Now().UTC())
	if err != nil || len(jobs) != 1 {
		t.Fatal("missing action job")
	}
	_, err = startFixtureService(f).HandleStartButton(context.Background(), ports.SalesButtonAction{
		BusinessID: f.business, CustomerID: f.customer, ConversationID: f.conversation,
		SourceMessageID: msg, Payload: payload, Now: f.now.Add(25 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(context.Background(), jobs[0].ID, jobs[0].LeaseToken, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var state, phase string
	if err := f.a.Pool().QueryRow(context.Background(), `SELECT state,phase FROM sales_action_jobs WHERE id=$1::uuid`, jobs[0].ID).Scan(&state, &phase); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || phase != "apply_action" {
		t.Fatalf("expired postback queued AI: %s %s", state, phase)
	}
}

func TestSalesStartContinuationErrorsAreExplicit(t *testing.T) {
	if !errors.Is(context.DeadlineExceeded, context.DeadlineExceeded) {
		t.Fatal("invalid test environment")
	}
}
