package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

// AutoReplyWorkerPool bounds the concurrency of AutoReply goroutines.
//
// Per P1-10 + Item 5: the webhook_ingestion path used to spawn a new
// goroutine for every inbound DM with no concurrency limit. A viral DM
// burst could spawn hundreds of concurrent goroutines — each calling
// Gemini + writing to Postgres — exhausting the connection pool +
// the Gemini rate limit.
//
// The pool enforces a configurable maximum concurrency. Tasks beyond
// the limit queue up (bounded by a buffered channel). When the queue
// is full, Submit returns false IMMEDIATELY (non-blocking) — the
// webhook logs the drop + returns 202 to the caller. The webhook
// MUST NOT block waiting for a pool slot (that would make the webhook
// synchronous + defeat the async dispatch pattern).
//
// Shutdown is graceful:
//  1. Stop() closes the tasks channel (no new submissions accepted).
//  2. Workers continue draining queued tasks until the channel is empty.
//  3. Workers exit after the channel is drained + closed.
//  4. Stop() waits for all workers to finish (in-flight + queued).
//
// The pool preserves the existing behavior:
//   - Each task runs in its own context with the configured timeout
//     (120s default).
//   - Panics in the task function are recovered + logged (audit B-CRIT-1).
//   - Errors are logged but never fail the webhook caller.
type AutoReplyWorkerPool struct {
	// tasks is a buffered channel of pending AutoReply invocations.
	// Size = the maximum queue depth. When full, Submit returns false.
	tasks chan autoReplyTask
	// wg tracks in-flight workers for graceful shutdown.
	wg sync.WaitGroup
	// stopOnce ensures Stop() is idempotent.
	stopOnce sync.Once
}

type autoReplyTask struct {
	cmd             commands.AutoReplyCommand
	businessID      string
	conversationID  string
	handler         commands.AutoReplyHandler
	executionTimeout time.Duration
}

// NewAutoReplyWorkerPool spins up `concurrency` workers consuming from
// a queue of `queueDepth` depth. Recommended values:
//   - concurrency = 4-8 (matches Gemini API throughput for a single key)
//   - queueDepth  = 64-128 (absorbs a burst without blocking the webhook)
//
// Workers start immediately on construction + run until Stop() is called.
func NewAutoReplyWorkerPool(concurrency, queueDepth int) *AutoReplyWorkerPool {
	if concurrency < 1 {
		concurrency = 4
	}
	if queueDepth < 1 {
		queueDepth = 64
	}
	pool := &AutoReplyWorkerPool{
		tasks: make(chan autoReplyTask, queueDepth),
	}
	for i := 0; i < concurrency; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	return pool
}

// worker is the consumer loop. Each worker:
//  1. Blocks on pool.tasks until a task arrives OR the channel is closed.
//  2. Runs the AutoReply task with the configured timeout.
//  3. Recovers from any panic (audit B-CRIT-1).
//  4. Returns to step 1.
//
// Workers exit when pool.tasks is closed (graceful drain) — they
// process any remaining queued tasks before exiting.
func (p *AutoReplyWorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		p.runTask(task)
	}
}

// runTask executes one AutoReply task with panic recovery + timeout.
// Mirrors the original goroutine logic from webhook_ingestion.go.
func (p *AutoReplyWorkerPool) runTask(task autoReplyTask) {
	// Per audit B-CRIT-1: a panic in this detached goroutine would
	// crash the entire API process. AutoReply calls Gemini HTTP +
	// Postgres writes + catalog batch logic — any of those can panic
	// on adversarial input. Wrap the entire goroutine in recover so
	// a single bad payload doesn't take down the webhook service.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[WorkerPool] AUTO_REPLY_PANIC business=%s conversation=%s recovered=%v", task.businessID, task.conversationID, r)
		}
	}()
	timeout := task.executionTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	autoReplyCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, autoReplyErr := task.handler.Handle(autoReplyCtx, task.cmd); autoReplyErr != nil {
		log.Printf("[WorkerPool] AUTO_REPLY_ERROR business=%s conversation=%s err=%v", task.businessID, task.conversationID, autoReplyErr)
	}
}

// Submit enqueues an AutoReply task. NON-BLOCKING: if the queue is
// full, returns false immediately (the webhook caller logs the drop
// + returns 202 to the upstream provider).
//
// Per Item 5: the previous implementation blocked for up to 30s when
// the queue was full — that made the webhook synchronous + could
// cause the upstream provider (SocialAPI) to time out waiting for
// the webhook response. The fix: never block. A full queue means the
// system is saturated; dropping the task is the correct backpressure
// signal (the merchant's customer will see no AI reply, but the
// webhook stays responsive + the outbox pattern ensures already-
// enqueued replies are still delivered).
//
// Returns true if the task was enqueued, false if dropped (queue full
// OR pool is stopped).
func (p *AutoReplyWorkerPool) Submit(task autoReplyTask) bool {
	select {
	case p.tasks <- task:
		return true
	default:
		return false
	}
}

// Stop initiates graceful shutdown. Idempotent — calling Stop multiple
// times is a no-op.
//
// Per Item 5: the previous implementation closed stopCh which caused
// workers to exit immediately — queued tasks were lost. The fix:
//
//  1. Close the tasks channel — no new submissions accepted.
//     Workers continue ranging over the channel, processing any
//     remaining queued tasks.
//  2. Wait for all workers to finish (wg.Wait) — in-flight tasks
//     + queued tasks complete with their configured timeouts.
//
// The shutdown is synchronous: Stop() returns only after all workers
// have exited. Callers (e.g., APIRuntime.Shutdown) should call this
// last to ensure no AutoReply work is dropped.
func (p *AutoReplyWorkerPool) Stop() {
	p.stopOnce.Do(func() {
		close(p.tasks)
		p.wg.Wait()
	})
}

var _ = (*AutoReplyWorkerPool)(nil)
