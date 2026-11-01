package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

// Bind sensitive provisioning retries to the authenticated actor, target and
// body. Serialize lookup+execute+cache so simultaneous retries cannot create
// duplicate accounts. Only a digest is cached, never the credential payload.
func (s *Server) dispatchGovernanceIdempotency(ctx context.Context, operationID string, input any) (any, bool) {
	var body any
	var target, key string
	switch in := input.(type) {
	case *dto.CreateBusinessInput:
		if operationID != "platformCreateBusiness" {
			return nil, false
		}
		body, key = in.Body, in.IdempotencyKey
	case *dto.AssignBusinessOwnerInput:
		if operationID != "platformAssignBusinessOwner" {
			return nil, false
		}
		body, key, target = in.Body, in.IdempotencyKey, string(in.BusinessID)
	default:
		return nil, false
	}
	if key == "" || s.idempotency == nil {
		return nil, false
	}
	actor, ok := middleware.PlatformAdminID(ctx)
	if !ok {
		return mapApplicationError(appErrors.New(appErrors.CodeForbidden, "platform scope required")), true
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return mapApplicationError(appErrors.New(appErrors.CodeValidation, "invalid request body")), true
	}
	digest := sha256.Sum256(encoded)
	fingerprint := hex.EncodeToString(digest[:])
	cacheKey := "governance|" + string(actor) + "|" + operationID + "|" + target + "|" + key
	store := s.idempotency
	store.executionMu.Lock()
	defer store.executionMu.Unlock()
	store.mu.RLock()
	entry, exists := store.store[cacheKey]
	store.mu.RUnlock()
	if exists && time.Since(entry.cachedAt) <= store.ttl {
		if entry.fingerprint != fingerprint {
			return mapApplicationError(appErrors.New(appErrors.CodeIdempotencyConflict, "idempotency key was used with a different request")), true
		}
		return entry.result, true
	}
	result, handled := s.dispatchPlatformCommandInner(ctx, operationID, input)
	if handled {
		if _, failed := result.(error); !failed {
			store.mu.Lock()
			store.store[cacheKey] = idempotencyEntry{result: result, cachedAt: time.Now(), fingerprint: fingerprint}
			if len(store.store)%100 == 0 {
				for key, entry := range store.store {
					if time.Since(entry.cachedAt) > store.ttl {
						delete(store.store, key)
					}
				}
			}
			store.mu.Unlock()
		}
	}
	return result, handled
}
