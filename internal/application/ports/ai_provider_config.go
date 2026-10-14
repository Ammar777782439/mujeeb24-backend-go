package ports

import (
	"context"
	"time"
)

// LiteLLMPlatformConfig describes gateway-owned route settings. APIKey is
// secret server-side configuration and MUST never be returned to HTTP clients.
type LiteLLMPlatformConfig struct {
	BaseURL string
	APIKey string
	CustomerModel string
	CatalogModel string
	MerchantModel string
	MaxOutputTokens int
	MaxInputCharacters int
	USDToYERRate string
}

// AIProviderModel is provider-neutral model discovery metadata. It is retained
// to preserve Platform Admin's existing model-list DTO and API contract.
type AIProviderModel struct {
	ID string
	Provider string
	ModelName string
	DisplayName *string
	Description *string
	InputTokenLimit *int
	OutputTokenLimit *int
	SupportedMethods []string
	ThinkingSupported bool
	TemperatureMin *float64
	TemperatureMax *float64
	TopPMin *float64
	TopPMax *float64
	TopKMin *int
	TopKMax *int
	Version *string
	BaseModel *string
	DiscoveredAt time.Time
}

// ModelDiscoveryClient queries LiteLLM's model catalog and tests a configured
// route via the gateway. It never reads merchant conversations for probes.
type ModelDiscoveryClient interface {
	DiscoverModels(ctx context.Context, apiKey, baseURL string) ([]AIProviderModel, error)
	TestConnection(ctx context.Context, apiKey, model, baseURL string) (bool, int64, string)
}
