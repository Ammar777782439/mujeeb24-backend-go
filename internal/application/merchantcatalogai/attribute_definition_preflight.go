package merchantcatalogai

import (
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// SchemaDefinitionPreflightError deliberately contains only verified type names
// and a model definition's array index, never model-authored labels or values.
type SchemaDefinitionPreflightError struct {
	Index        int
	Reason       string
	ExpectedType string
	ProposedType string
}

func (e *SchemaDefinitionPreflightError) Error() string {
	return fmt.Sprintf("attribute_definitions[%d]: %s", e.Index, e.Reason)
}

// preflightAttributeDefinitionBindings mirrors the no-write compatibility
// rules enforced by resolveApprovedAttributeDefinitions during approval.
// It also exposes aliases reusing existing managed keys, so that the preflight
// checks the effective values, not just the raw model-created aliases.
func preflightAttributeDefinitionBindings(
	current *ports.AttributeSchemaRecord,
	schemaName string,
	specs []ProposedAttributeDefinition,
) (map[string]string, bool, error) {
	aliases := make(map[string]string, len(specs))
	if len(specs) == 0 {
		return aliases, false, nil
	}
	byLabel := map[string]ports.AttributeDefinitionRecord{}
	byKey := map[string]ports.AttributeDefinitionRecord{}
	if current != nil {
		for _, d := range current.Definitions {
			byLabel[strings.TrimSpace(d.Label)] = d
			byKey[d.Key] = d
		}
	}
	seenAliases := map[string]bool{}
	seenLabels := map[string]bool{}
	hasNew := false
	for i, spec := range specs {
		reject := func(reason, expected string) (map[string]string, bool, error) {
			return nil, false, &SchemaDefinitionPreflightError{
				Index: i, Reason: reason, ExpectedType: expected, ProposedType: spec.DataType,
			}
		}
		if !attributeKeyPattern.MatchString(spec.Key) || strings.HasPrefix(spec.Key, "attr_") {
			return reject("invalid_alias", "")
		}
		if seenAliases[spec.Key] {
			return reject("duplicate_alias", "")
		}
		seenAliases[spec.Key] = true
		label := strings.TrimSpace(spec.Label)
		if label == "" {
			return reject("missing_label", "")
		}
		if seenLabels[label] {
			return reject("duplicate_label", "")
		}
		seenLabels[label] = true
		rules := spec.ValidationRules
		if rules == nil {
			rules = map[string]any{}
		}
		if err := validateAIAttributeRules(spec.DataType, rules); err != nil {
			return reject("invalid_rules", "")
		}
		if existing, ok := byKey[spec.Key]; ok {
			if strings.TrimSpace(existing.Label) != label || existing.DataType != spec.DataType {
				return reject("existing_key_conflict", existing.DataType)
			}
			if err := verifyExistingAttributeRules(existing.ValidationRules, rules); err != nil {
				return reject("existing_rules_conflict", existing.DataType)
			}
			aliases[spec.Key] = existing.Key
			continue
		}
		if existing, ok := byLabel[label]; ok {
			if existing.DataType != spec.DataType {
				return reject("existing_type_conflict", existing.DataType)
			}
			if err := verifyExistingAttributeRules(existing.ValidationRules, rules); err != nil {
				return reject("existing_rules_conflict", existing.DataType)
			}
			aliases[spec.Key] = existing.Key
			continue
		}
		hasNew = true
	}
	if hasNew {
		if current != nil {
			if schemaName != "" && strings.TrimSpace(schemaName) != current.Name {
				return nil, false, &SchemaDefinitionPreflightError{Reason: "existing_schema_name_conflict", Index: -1}
			}
		} else if strings.TrimSpace(schemaName) == "" {
			return nil, false, &SchemaDefinitionPreflightError{Reason: "new_schema_name_required", Index: -1}
		}
	}
	return aliases, hasNew, nil
}

// preflightRemapAttributes maps only known aliases to stored template keys.
// No IDs are created and no mutation of the model's proposal is performed.
func preflightRemapAttributes(attrs map[string]any, aliases map[string]string) (map[string]any, error) {
	if len(aliases) == 0 || attrs == nil {
		return attrs, nil
	}
	mapped, err := remapApprovedAttributes(attrs, aliases)
	if err != nil {
		return nil, &SchemaDefinitionPreflightError{Index: -1, Reason: "alias_mapping_collision"}
	}
	return mapped, nil
}
