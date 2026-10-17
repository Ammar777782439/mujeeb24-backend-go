package ports

import (
	"context"
	"time"
)

// NativeAIPlatformConfig exposes only deployment model identifiers and
// credential-presence flags. Provider secrets never enter HTTP responses.
type NativeAIPlatformConfig struct {
	GeminiModel string
	DeepSeekModel string
	CustomerModel string
	CatalogModel string
	MerchantModel string
	MaxOutputTokens int
	MaxInputCharacters int
	GeminiConfigured bool
	DeepSeekConfigured bool
}

// AIProviderModel is provider-neutral model discovery metadata. It is retained
// to preserve Platform Admin's existing model-list DTO and API contract.
type AIProviderModel struct {
	ID                string
	Provider          string
	ModelName         string
	DisplayName       *string
	Description       *string
	InputTokenLimit   *int
	OutputTokenLimit  *int
	SupportedMethods  []string
	ThinkingSupported bool
	TemperatureMin    *float64
	TemperatureMax    *float64
	TopPMin           *float64
	TopPMax           *float64
	TopKMin           *int
	TopKMax           *int
	Version           *string
	BaseModel         *string
	DiscoveredAt      time.Time
}

// ModelDiscoveryClient queries native providers' model catalogs and tests a configured
// provider directly. It never reads merchant conversations for probes.
type ModelDiscoveryClient interface {
	DiscoverModels(ctx context.Context, apiKey, baseURL string) ([]AIProviderModel, error)
	TestConnection(ctx context.Context, apiKey, model, baseURL string) (bool, int64, string)
}
