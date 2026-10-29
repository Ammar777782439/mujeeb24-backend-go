package merchantcatalogai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/catalogvalidation"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// resolveApprovedAttributeDefinitions runs only in ExecuteApproved's transaction.
// The model supplies semantic aliases, labels and value types; Go owns stable keys,
// schema versions and persistence. All writes roll back if execution fails.
func resolveApprovedAttributeDefinitions(
	ctx context.Context,
	catalogs ports.CatalogRepository,
	business, modelSchemaName string,
	current *ports.AttributeSchemaRecord,
	specs []ProposedAttributeDefinition,
	now time.Time,
) (*ports.AttributeSchemaRecord, map[string]string, error) {
	keys := map[string]string{}
	if len(specs) == 0 {
		return current, keys, nil
	}
	if len(specs) > 100 {
		return nil, nil, errors.New("too many proposed attribute definitions")
	}

	byKey := map[string]ports.AttributeDefinitionRecord{}
	byLabel := map[string]ports.AttributeDefinitionRecord{}
	definitions := make([]ports.AttributeDefinitionDraft, 0, len(specs))
	if current != nil {
		for _, d := range current.Definitions {
			byKey[d.Key] = d
			byLabel[strings.TrimSpace(d.Label)] = d
			definitions = append(definitions, ports.AttributeDefinitionDraft{
				ID: uuid.NewString(), Key: d.Key, Label: d.Label,
				DataType: d.DataType, Required: d.Required,
				ValidationRules: d.ValidationRules, DisplayOrder: d.DisplayOrder,
				CreatedAt: now, UpdatedAt: now,
			})
		}
	}
	newDefs := 0
	seen := map[string]bool{}
	for _, spec := range specs {
		if !attributeKeyPattern.MatchString(spec.Key) || strings.HasPrefix(spec.Key, "attr_") {
			return nil, nil, fmt.Errorf("invalid proposed attribute alias %q", spec.Key)
		}
		label := strings.TrimSpace(spec.Label)
		if label == "" || len([]rune(label)) > 120 {
			return nil, nil, errors.New("invalid attribute display label")
		}
		hasArabic := false
		for _, r := range label {
			if unicode.Is(unicode.Arabic, r) {
				hasArabic = true
			}
			if unicode.IsControl(r) {
				return nil, nil, errors.New("attribute label contains control characters")
			}
		}
		if !hasArabic {
			return nil, nil, fmt.Errorf("attribute %q must have an Arabic display label", spec.Key)
		}
		if seen[spec.Key] {
			return nil, nil, fmt.Errorf("duplicate attribute alias %q", spec.Key)
		}
		seen[spec.Key] = true
		rules := spec.ValidationRules
		if rules == nil { rules = map[string]any{} }
		if err := validateAIAttributeRules(spec.DataType, rules); err != nil {
			return nil, nil, fmt.Errorf("attribute %q: %w", spec.Key, err)
		}
		rulesJSON, err := json.Marshal(rules)
		if err != nil { return nil, nil, err }
		if existing, ok := byKey[spec.Key]; ok {
			if existing.Label != label || existing.DataType != spec.DataType {
				return nil, nil, errors.New("existing attribute definition cannot be silently redefined")
			}
			if err := verifyExistingAttributeRules(existing.ValidationRules, rules); err != nil {
				return nil, nil, fmt.Errorf("existing attribute %q: %w", label, err)
			}
			keys[spec.Key] = existing.Key
			continue
		}
		if existing, ok := byLabel[label]; ok {
			if existing.DataType != spec.DataType {
				return nil, nil, fmt.Errorf("attribute label %q has conflicting data type", label)
			}
			if err := verifyExistingAttributeRules(existing.ValidationRules, rules); err != nil {
				return nil, nil, fmt.Errorf("existing attribute %q: %w", label, err)
			}
			keys[spec.Key] = existing.Key
			continue
		}
		newID := uuid.NewString()
		sum := sha256.Sum256([]byte(newID))
		stableKey := "attr_" + hex.EncodeToString(sum[:8])
		for {
			if _, exists := byKey[stableKey]; !exists {
				break
			}
			newID = uuid.NewString()
			sum = sha256.Sum256([]byte(newID))
			stableKey = "attr_" + hex.EncodeToString(sum[:8])
		}
		definitions = append(definitions, ports.AttributeDefinitionDraft{
			ID: newID, Key: stableKey, Label: label, DataType: spec.DataType,
			Required: false, ValidationRules: rulesJSON, DisplayOrder: len(definitions),
			CreatedAt: now, UpdatedAt: now,
		})
		byKey[stableKey] = ports.AttributeDefinitionRecord{Key: stableKey, Label: label, DataType: spec.DataType}
		byLabel[label] = byKey[stableKey]
		keys[spec.Key] = stableKey
		newDefs++
	}
	if newDefs == 0 {
		return current, keys, nil
	}
	name := strings.TrimSpace(modelSchemaName)
	if current != nil {
		// A pre-existing template wins; the model may not rename it.
		if name != "" && name != current.Name {
			return nil, nil, errors.New("selected schema name does not match existing template")
		}
		name = current.Name
	} else {
		// Neither a template name nor its meaning is ever inferred by Go.
		// A new Arabic template name must be explicitly authored by the model.
		if name == "" || len([]rune(name)) > 120 {
			return nil, nil, errors.New("new attribute template requires a model-authored Arabic name")
		}
		hasArabic := false
		for _, char := range name {
			if unicode.Is(unicode.Arabic, char) {
				hasArabic = true
			}
			if unicode.IsControl(char) {
				return nil, nil, errors.New("attribute template name has control characters")
			}
		}
		if !hasArabic {
			return nil, nil, errors.New("attribute template name must be Arabic")
		}
		// An already-named template cannot be silently shadowed by a new
		// version lacking its previous definitions. The model must select
		// that existing template by a verified ID instead.
		existing, err := catalogs.ListAttributeSchemas(ctx, business, name, nil, 1, "")
		if err != nil {
			return nil, nil, err
		}
		if len(existing.Items) != 0 {
			return nil, nil, errors.New("attribute template name already exists: select an existing template from catalog evidence")
		}
	}
	version, err := catalogs.NextAttributeSchemaVersion(ctx, business, name)
	if err != nil {
		return nil, nil, err
	}
	record, err := catalogs.CreateAttributeSchemaVersion(ctx, ports.AttributeSchemaDraft{
		ID: uuid.NewString(), BusinessID: business, Name: name,
		Version: version, Definitions: definitions, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return nil, nil, err
	}
	return &record, keys, nil
}

// Map a proposal's semantic aliases to server-issued machine IDs at the
// storage boundary. Preserves unmapped legacy/custom attributes verbatim.
func remapApprovedAttributes(input map[string]any, keys map[string]string) (map[string]any, error) {
	if input == nil {
		return nil, nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		target := key
		if actual, ok := keys[key]; ok {
			target = actual
		}
		if _, exists := out[target]; exists {
			return nil, fmt.Errorf("attribute mapping collision on %q", target)
		}
		out[target] = value
	}
	return out, nil
}

// An existing template remains authoritative: new model-produced rules must
// not overwrite the stored definition. Omitted rules preserve those on reuse.
func verifyExistingAttributeRules(stored []byte, proposed map[string]any) error {
	if len(proposed) == 0 { return nil }
	var original map[string]any
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &original); err != nil { return err }
	}
	if original == nil { original = map[string]any{} }
	current, err := json.Marshal(original)
	if err != nil { return err }
	requested, err := json.Marshal(proposed)
	if err != nil { return err }
	if string(current) != string(requested) {
		return errors.New("validation rules differ from the selected template; do not overwrite existing choices")
	}
	return nil
}
