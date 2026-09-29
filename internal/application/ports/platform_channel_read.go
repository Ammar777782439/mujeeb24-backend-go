package ports

import "context"

// PlatformChannelReadPort reads channel_connections in a Platform-scoped
// manner. Per Contract §93: the view exposes business_id, connection_id,
// channel, provider, status, health, provider_account_ref,
// provider_connection_ref, last_health_check_at.
//
// It NEVER exposes: access_token, secret, secret_reference, customer
// messages, conversation body, merchant catalog (per §93).
type PlatformChannelReadPort interface {
	ListChannels(ctx context.Context, filter PlatformChannelListFilter) ([]PlatformChannelRecord, error)
	GetChannelByID(ctx context.Context, connectionID string) (PlatformChannelRecord, error)
}

type PlatformChannelListFilter struct {
	BusinessID string
	Status     string
	Channel    string
	Limit      int
}

// PlatformChannelRecord is the safe view of a channel connection.
// The secret_reference column is intentionally absent — per Contract §93,
// the Dashboard does NOT show secrets.
type PlatformChannelRecord struct {
	ID                    string
	BusinessID            string
	ProviderRef           string
	Channel               string
	ProviderAccountRef    *string
	ProviderConnectionRef string
	Status                string
	LastHealthCheckAt     *string // RFC 3339
}
