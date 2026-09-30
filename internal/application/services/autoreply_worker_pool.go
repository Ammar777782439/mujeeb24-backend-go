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
// Per Item 5: the pool enforces a configurable maximum concurrency.
// Tasks beyond the limit queue up (bounded by a buffered channel).
// When the queue is full, Submit returns false IMMEDIATELY
// (non-blocking).
//
// Shutdown is graceful:
//  1. Stop() sets the stopped flag (under write lock) + closes the
//     tasks channel. No new submissions accepted after this point.
//  2. Workers continue draining queued tasks until the channel is
//     empty + closed.
//  3. Stop() waits for all workers to finish (in-flight + queued).
//
// Per Item 5 race fix: Submit uses RLock to prevent the
// send-on-closed-channel panic. Stop() acquires the write lock
// BEFORE closing the channel — so no Submit call can be in-flight
// on the channel send when it closes.
type AutoReplyWorkerPool struct {
	tasks chan autoReplyTask
	wg    sync.WaitGroup
	// mu protects the stopped flag + the tasks channel close.
	// Submit takes RLock (concurrent readers OK); Stop takes Lock
	// (exclusive — waits for all RLock holders to release before
	// closing the channel).
	mu       sync.RWMutex
	stopped  bool
	stopOnce sync.Once
}

type autoReplyTask struct {
	cmd              commands.AutoReplyCommand
	businessID       string
	conversationID   string
	handler          commands.AutoReplyHandler
	executionTimeout time.Duration
}

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

func (p *AutoReplyWorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		p.runTask(task)
	}
}

func (p *AutoReplyWorkerPool) runTask(task autoReplyTask) {
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
// full OR the pool is stopped, returns false immediately.
//
// Per Item 5 race fix: uses RLock to synchronize with Stop(). Stop()
// acquires the write lock before closing the channel — so this Submit
// call finishes its channel send (or hits the default branch) BEFORE
// Stop() can close the channel. No "send on closed channel" panic.
func (p *AutoReplyWorkerPool) Submit(task autoReplyTask) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.stopped {
		return false
	}
	select {
	case p.tasks <- task:
		return true
	default:
		return false
	}
}

// Stop initiates graceful shutdown. Idempotent.
//
// Per Item 5: acquires the write lock, sets stopped=true, closes the
// channel, then waits for all workers to drain queued + in-flight
// tasks. The write lock ensures no Submit call is concurrently
// sending on the channel when it's closed.
func (p *AutoReplyWorkerPool) Stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		close(p.tasks)
		p.mu.Unlock()
		p.wg.Wait()
	})
}

var _ = (*AutoReplyWorkerPool)(nil)
