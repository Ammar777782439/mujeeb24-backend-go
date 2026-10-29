package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

// Decode only per-definition rules JSON, never an opaque full catalog proposal.
func catalogWireDefinitions(entries []catalogWireAttributeDefinition) ([]merchantcatalogai.ProposedAttributeDefinition, error) {
	out := make([]merchantcatalogai.ProposedAttributeDefinition, 0, len(entries))
	for _, def := range entries {
		if len(def.ValidationRulesJSON) == 0 || len(def.ValidationRulesJSON) > 16384 {
			return nil, errors.New("attribute validation rules missing or too large")
		}
		decoder := json.NewDecoder(strings.NewReader(def.ValidationRulesJSON))
		decoder.UseNumber()
		var rules map[string]any
		if err := decoder.Decode(&rules); err != nil {
			return nil, fmt.Errorf("invalid rules for %q: %w", def.Key, err)
		}
		if rules == nil {
			return nil, fmt.Errorf("attribute %q rules must be a JSON object", def.Key)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("attribute %q rules contain trailing data", def.Key)
		}
		out = append(out, merchantcatalogai.ProposedAttributeDefinition{
			Key: def.Key, Label: def.Label, DataType: def.DataType, ValidationRules: rules,
		})
	}
	return out, nil
}
