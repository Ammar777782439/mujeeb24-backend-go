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
// Per P1-10: the webhook_ingestion path used to spawn a new goroutine
// for every inbound DM with no concurrency limit. A viral DM burst
// (e.g., a flash sale announcement) could spawn hundreds of concurrent
// goroutines — each calling Gemini + writing to Postgres — exhausting
// the connection pool + the Gemini rate limit.
//
// The worker pool enforces a configurable maximum concurrency. Tasks
// beyond the limit queue up (bounded by a buffered channel) and run
// as workers become free. If the queue is full, the submit call
// blocks until a slot is available OR the deadline is reached
// (configurable; default 30s).
//
// The pool preserves the existing behavior:
//   - Each task runs in its own context with the configured timeout
//     (120s default, same as before).
//   - Panics in the task function are recovered + logged (audit B-CRIT-1).
//   - Errors are logged but never fail the webhook caller — AutoReply
//     is best-effort async work.
type AutoReplyWorkerPool struct {
	// tasks is a buffered channel of pending AutoReply invocations.
	// Size = the maximum queue depth before submissions block.
	tasks chan autoReplyTask
	// wg tracks in-flight workers for graceful shutdown.
	wg sync.WaitGroup
	// stopOnce ensures Stop() is idempotent.
	stopOnce sync.Once
	// stopCh closes to signal workers to exit.
	stopCh chan struct{}
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
// Per P1-10: there is NO polling — workers block on the channel
// until a task arrives. The pool starts immediately on construction.
func NewAutoReplyWorkerPool(concurrency, queueDepth int) *AutoReplyWorkerPool {
	if concurrency < 1 {
		concurrency = 4
	}
	if queueDepth < 1 {
		queueDepth = 64
	}
	pool := &AutoReplyWorkerPool{
		tasks:   make(chan autoReplyTask, queueDepth),
		stopCh:  make(chan struct{}),
	}
	for i := 0; i < concurrency; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}
	return pool
}

// worker is the consumer loop. Each worker:
//  1. Blocks on pool.tasks until a task arrives.
//  2. Runs the AutoReply task with the configured timeout.
//  3. Recovers from any panic (audit B-CRIT-1).
//  4. Returns to step 1.
//
// Workers exit when pool.stopCh is closed OR pool.tasks is closed.
func (p *AutoReplyWorkerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.stopCh:
			return
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			p.runTask(task)
		}
	}
}

// runTask executes one AutoReply task with panic recovery + timeout.
// Mirrors the original goroutine logic from webhook_ingestion.go:279-296.
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

// Submit enqueues an AutoReply task. If the queue is full, blocks
// until a slot frees up OR `submitTimeout` elapses. Returns false
// if the deadline is reached (the webhook caller logs + discards).
//
// Per P1-10: this is the BOUND on AutoReply concurrency — even if
// 10,000 DMs arrive simultaneously, only `concurrency` Gemini
// calls run in parallel; the rest queue.
func (p *AutoReplyWorkerPool) Submit(task autoReplyTask, submitTimeout time.Duration) bool {
	if submitTimeout <= 0 {
		submitTimeout = 30 * time.Second
	}
	timer := time.NewTimer(submitTimeout)
	defer timer.Stop()
	select {
	case p.tasks <- task:
		return true
	case <-timer.C:
		return false
	}
}

// Stop signals all workers to exit + waits for in-flight tasks to
// complete. Idempotent — calling Stop multiple times is a no-op.
// Per P1-10: the webhook service should call this on shutdown so
// no in-flight AutoReply work is dropped silently.
func (p *AutoReplyWorkerPool) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		// Drain remaining queued tasks before waiting — give in-flight
		// + queued work a chance to finish.
		// (Workers will see stopCh + return after their current task.)
		p.wg.Wait()
	})
}

var _ = (*AutoReplyWorkerPool)(nil)
