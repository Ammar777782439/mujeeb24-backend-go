package handlers

import (
	"sync"
	"time"
)

// IdempotencyStore is an in-memory cache for platform command idempotency.
// Per Contract §59: "Business creation, Subscription creation, Payment
// recording, Support message creation" must support idempotency — a retry
// with the same Idempotency-Key must NOT produce a duplicate.
//
// The store is process-local (single-instance V1). Multi-instance would
// require a shared store (Redis/DB). Entries expire after 24 hours.
type IdempotencyStore struct {
	mu    sync.RWMutex
	store map[string]idempotencyEntry
	ttl   time.Duration
}

type idempotencyEntry struct {
	result   any
	Err      error
	cachedAt time.Time
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{
		store: map[string]idempotencyEntry{},
		ttl:   24 * time.Hour,
	}
}

// Get returns the cached result for the given key, or false if not found
// or expired. The key is operationID + "|" + idempotencyKey.
func (s *IdempotencyStore) Get(key string) (any, error, bool) {
	if s == nil || key == "" {
		return nil, nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.store[key]
	if !ok {
		return nil, nil, false
	}
	if time.Since(entry.cachedAt) > s.ttl {
		return nil, nil, false
	}
	return entry.result, entry.Err, true
}

// Set caches the result for the given key.
func (s *IdempotencyStore) Set(key string, result any, err error) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = idempotencyEntry{result: result, Err: err, cachedAt: time.Now()}
	// Lazy GC: periodically purge expired entries (every 100 writes).
	if len(s.store)%100 == 0 {
		now := time.Now()
		for k, v := range s.store {
			if now.Sub(v.cachedAt) > s.ttl {
				delete(s.store, k)
			}
		}
	}
}

// makeIdempotencyKey builds the cache key from operationID + idempotencyKey.
func makeIdempotencyKey(operationID, idempotencyKey string) string {
	if idempotencyKey == "" {
		return ""
	}
	return operationID + "|" + idempotencyKey
}
