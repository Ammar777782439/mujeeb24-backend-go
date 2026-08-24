package ports

import "context"

type OutboundDelivery struct {
	ConnectionID           string
	ProviderAccountID      string
	ProviderConversationID string
	Text                   string
	IdempotencyKey         string
}

// OutboundDeliveryResolver resolves a claimed outbox obligation into the
// provider-neutral data required for one external send. It must read Mujeeb
// state only; the provider call happens after resolution and outside a DB tx.
type OutboundDeliveryResolver interface {
	Resolve(ctx context.Context, businessID, outboundMessageID string) (OutboundDelivery, error)
}
