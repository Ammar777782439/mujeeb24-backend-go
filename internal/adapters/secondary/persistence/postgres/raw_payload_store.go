package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RawPayloadStore struct{ adapter *Adapter }

func NewRawPayloadStore(adapter *Adapter) *RawPayloadStore {
	return &RawPayloadStore{adapter: adapter}
}

func (s *RawPayloadStore) Put(ctx context.Context, provider, deliveryID string, payload []byte) (ports.RawPayload, error) {
	if s == nil || s.adapter == nil {
		return ports.RawPayload{}, ErrPoolClosed
	}
	provider = strings.TrimSpace(provider)
	deliveryID = strings.TrimSpace(deliveryID)
	if provider == "" || len(payload) == 0 {
		return ports.RawPayload{}, invalidRepositoryInput("raw_payload.put", "provider and payload are required")
	}
	digest := sha256.Sum256(payload)
	hash := hex.EncodeToString(digest[:])
	if deliveryID == "" {
		deliveryID = "sha256:" + hash
	}
	executor, err := s.adapter.Executor(ctx)
	if err != nil {
		return ports.RawPayload{}, err
	}
	id := uuid.NewString()
	createdAt := time.Now().UTC()
	var storedID string
	var storedHash string
	err = executor.QueryRow(ctx, `INSERT INTO inbound_webhook_payloads (id, provider_ref, delivery_id, payload, payload_hash, created_at) VALUES ($1::uuid, $2, $3, $4, $5, $6) ON CONFLICT (provider_ref, delivery_id) DO UPDATE SET provider_ref = EXCLUDED.provider_ref RETURNING id::text, payload_hash`, id, provider, deliveryID, payload, hash, createdAt).Scan(&storedID, &storedHash)
	if err != nil {
		return ports.RawPayload{}, classifyRepositoryWriteError("raw_payload.put", err)
	}
	if storedHash != hash {
		return ports.RawPayload{}, &RepositoryError{Operation: "raw_payload.put", Kind: RepositoryConflict, Err: fmt.Errorf("delivery id was reused with different payload")}
	}
	if storedID == "" {
		return ports.RawPayload{}, &RepositoryError{Operation: "raw_payload.put", Kind: RepositoryInvalid, Err: errors.New("stored payload id is empty")}
	}
	return ports.RawPayload{Reference: "db://inbound_webhook_payloads/" + storedID, SHA256: storedHash}, nil
}

var _ ports.RawPayloadStore = (*RawPayloadStore)(nil)

var _ = pgx.ErrNoRows
