package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

// Provider-neutral strict proposal parser: no model can authorize its own mutation.
func parseMerchantCatalogProposal(raw string) (merchantcatalogai.Proposal, error) {
	if strings.TrimSpace(raw) == "" {
		return merchantcatalogai.Proposal{}, errors.New("AI provider returned no merchant catalog proposal")
	}
	var proposal merchantcatalogai.Proposal
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("decode merchant catalog proposal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return merchantcatalogai.Proposal{}, errors.New("AI provider returned trailing data after merchant catalog proposal")
	}
	return proposal, nil
}
