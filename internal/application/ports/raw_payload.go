package ports

import "context"

// RawPayloadStore durably stores the verified raw webhook bytes outside the
// event ledger and returns an opaque reference plus the hash of the exact bytes.
// The application must not fabricate the reference when this port is absent.
type RawPayloadStore interface {
	Put(ctx context.Context, provider, deliveryID string, payload []byte) (RawPayload, error)
}

type RawPayload struct {
	Reference string
	SHA256    string
}
