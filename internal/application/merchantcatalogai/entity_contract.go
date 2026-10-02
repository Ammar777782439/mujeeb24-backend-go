package merchantcatalogai

import (
	"context"
	"encoding/json"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

type CanonicalEntityContractProvider struct{}

func (CanonicalEntityContractProvider) Payload(_ context.Context) ([]byte, error) {
	payload := services.BuildCatalogEntityContractPayload()
	return json.Marshal(payload)
}

var _ EntityContractProvider = CanonicalEntityContractProvider{}
