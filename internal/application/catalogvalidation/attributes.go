package catalogvalidation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

var supportedDataTypes = map[string]struct{}{
	"text": {}, "number": {}, "boolean": {}, "date": {}, "datetime": {},
	"select": {}, "multi_select": {}, "location": {}, "money": {},
}

// ValidateDefinition validates only the contract-defined parts of a definition.
// validation_rules is intentionally extensible; unknown rule keys are preserved.
func ValidateDefinition(dataType string, rules map[string]any) error {
	if _, ok := supportedDataTypes[dataType]; !ok {
		return fmt.Errorf("unsupported attribute data_type %q", dataType)
	}
	if raw, ok := rules["enum"]; ok {
		values, ok := sliceValues(raw)
		if !ok || len(values) == 0 {
			return fmt.Errorf("validation rule enum must be a non-empty array")
		}
	}
	var minValue *float64
	var maxValue *float64
	if raw, ok := rules["min"]; ok {
		value, ok := numericValue(raw)
		if !ok {
			return fmt.Errorf("validation rule min must be numeric")
		}
		minValue = &value
	}
	if raw, ok := rules["max"]; ok {
		value, ok := numericValue(raw)
		if !ok {
			return fmt.Errorf("validation rule max must be numeric")
		}
		maxValue = &value
	}
	if minValue != nil && maxValue != nil && *minValue > *maxValue {
		return fmt.Errorf("validation rule min cannot be greater than max")
	}
	if raw, ok := rules["regex"]; ok {
		pattern, ok := raw.(string)
		if !ok || strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("validation rule regex must be a non-empty string")
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("validation rule regex is invalid: %w", err)
		}
	}
	for _, key := range []string{"currency", "timezone"} {
		if raw, ok := rules[key]; ok {
			value, ok := raw.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("validation rule %s must be a non-empty string", key)
			}
		}
	}
	return nil
}

// ValidateAttributes enforces schema definitions without breaking free-form
// merchant attributes. Keys beginning with attr_ are reserved for managed
// schema definitions. Free-form keys keep their human-readable semantic label.
func ValidateAttributes(schema *ports.AttributeSchemaRecord, attributes map[string]any) error {
	return validateAttributes(schema, attributes, true)
}

// ValidatePartialAttributes validates only supplied values. It is used for
// Variant overrides, which may legitimately contain only a subset of the
// parent item's schema-defined attributes.
func ValidatePartialAttributes(schema *ports.AttributeSchemaRecord, attributes map[string]any) error {
	return validateAttributes(schema, attributes, false)
}

func validateAttributes(schema *ports.AttributeSchemaRecord, attributes map[string]any, enforceRequired bool) error {
	definitions := map[string]ports.AttributeDefinitionRecord{}
	if schema != nil {
		for _, definition := range schema.Definitions {
			if strings.TrimSpace(definition.Key) == "" {
				return fmt.Errorf("schema contains an empty attribute key")
			}
			if _, exists := definitions[definition.Key]; exists {
				return fmt.Errorf("schema contains duplicate attribute key %q", definition.Key)
			}
			rules, err := decodeRules(definition.ValidationRules)
			if err != nil {
				return fmt.Errorf("attribute %q has invalid validation_rules: %w", definition.Label, err)
			}
			if err := ValidateDefinition(definition.DataType, rules); err != nil {
				return fmt.Errorf("attribute %q definition is invalid: %w", definition.Label, err)
			}
			definitions[definition.Key] = definition
		}
		if enforceRequired {
			for _, definition := range schema.Definitions {
				if !definition.Required {
					continue
				}
				value, exists := attributes[definition.Key]
				if !exists || emptyRequiredValue(value) {
					return fmt.Errorf("required attribute %q is missing", definition.Label)
				}
			}
		}
	}

	for key, value := range attributes {
		definition, managed := definitions[key]
		if !managed {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				return fmt.Errorf("custom attribute name cannot be empty")
			}
			if strings.HasPrefix(trimmed, "attr_") {
				return fmt.Errorf("attribute key %q is reserved for a managed schema definition", key)
			}
			if utf8.RuneCountInString(trimmed) > 120 {
				return fmt.Errorf("custom attribute name is too long")
			}
			if strings.IndexFunc(trimmed, unicode.IsControl) >= 0 {
				return fmt.Errorf("custom attribute name contains control characters")
			}
			continue
		}
		if value == nil {
			if definition.Required {
				return fmt.Errorf("required attribute %q cannot be null", definition.Label)
			}
			continue
		}
		rules, err := decodeRules(definition.ValidationRules)
		if err != nil {
			return fmt.Errorf("attribute %q has invalid validation_rules: %w", definition.Label, err)
		}
		if err := validateValue(definition, value, rules); err != nil {
			return fmt.Errorf("attribute %q: %w", definition.Label, err)
		}
	}
	return nil
}

func decodeRules(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var rules map[string]any
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	if rules == nil {
		return map[string]any{}, nil
	}
	return rules, nil
}

func validateValue(definition ports.AttributeDefinitionRecord, value any, rules map[string]any) error {
	switch definition.DataType {
	case "text":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("must be text")
		}
	case "number":
		if _, ok := numericValue(value); !ok {
			return fmt.Errorf("must be a finite number")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("must be true or false")
		}
	case "date":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("must be a date string")
		}
		if _, err := time.Parse("2006-01-02", text); err != nil {
			return fmt.Errorf("must use YYYY-MM-DD")
		}
	case "datetime":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("must be a datetime string")
		}
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return fmt.Errorf("must use RFC3339")
		}
	case "select":
		if !scalarValue(value) {
			return fmt.Errorf("must be one scalar value")
		}
	case "multi_select":
		values, ok := sliceValues(value)
		if !ok {
			return fmt.Errorf("must be an array")
		}
		for _, entry := range values {
			if !scalarValue(entry) {
				return fmt.Errorf("must contain only scalar values")
			}
		}
	case "location":
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				return fmt.Errorf("location cannot be empty")
			}
		case map[string]any:
			if typed == nil {
				return fmt.Errorf("location cannot be null")
			}
		default:
			return fmt.Errorf("must be a location string or object")
		}
	case "money":
		if _, ok := numericValue(value); ok {
			break
		}
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("must be a numeric amount or money object")
		}
		if amount, exists := object["amount"]; exists {
			if _, ok := numericValue(amount); !ok {
				return fmt.Errorf("money amount must be numeric")
			}
		}
		if currency, exists := object["currency"]; exists {
			if text, ok := currency.(string); !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("money currency must be a non-empty string")
			}
		}
	default:
		return fmt.Errorf("unsupported data type %q", definition.DataType)
	}

	if raw, ok := rules["enum"]; ok {
		allowed, _ := sliceValues(raw)
		if values, multi := sliceValues(value); multi && definition.DataType == "multi_select" {
			for _, selected := range values {
				if !containsScalar(allowed, selected) {
					return fmt.Errorf("contains a value outside the allowed choices")
				}
			}
		} else if !containsScalar(allowed, value) {
			return fmt.Errorf("value is outside the allowed choices")
		}
	}
	if definition.DataType == "number" {
		number, _ := numericValue(value)
		if raw, ok := rules["min"]; ok {
			minimum, _ := numericValue(raw)
			if number < minimum {
				return fmt.Errorf("must be greater than or equal to %v", minimum)
			}
		}
		if raw, ok := rules["max"]; ok {
			maximum, _ := numericValue(raw)
			if number > maximum {
				return fmt.Errorf("must be less than or equal to %v", maximum)
			}
		}
	}
	if raw, ok := rules["regex"]; ok {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("regex rule requires a text value")
		}
		pattern, _ := raw.(string)
		matched, err := regexp.MatchString(pattern, text)
		if err != nil || !matched {
			return fmt.Errorf("does not match the required format")
		}
	}
	if raw, ok := rules["currency"]; ok {
		requiredCurrency, _ := raw.(string)
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("currency rule requires a money object")
		}
		currency, ok := object["currency"].(string)
		if !ok || !strings.EqualFold(strings.TrimSpace(currency), strings.TrimSpace(requiredCurrency)) {
			return fmt.Errorf("currency must be %s", requiredCurrency)
		}
	}
	if raw, ok := rules["timezone"]; ok {
		requiredTimezone, _ := raw.(string)
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("timezone rule requires an object value")
		}
		timezone, ok := object["timezone"].(string)
		if !ok || strings.TrimSpace(timezone) != strings.TrimSpace(requiredTimezone) {
			return fmt.Errorf("timezone must be %s", requiredTimezone)
		}
	}
	return nil
}

func emptyRequiredValue(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	}
	if values, ok := sliceValues(value); ok {
		return len(values) == 0
	}
	return false
}

func scalarValue(value any) bool {
	if value == nil {
		return false
	}
	if _, ok := value.(string); ok {
		return true
	}
	if _, ok := value.(bool); ok {
		return true
	}
	_, ok := numericValue(value)
	return ok
}

func numericValue(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func sliceValues(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	if values, ok := value.([]any); ok {
		return values, true
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	values := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		values[i] = rv.Index(i).Interface()
	}
	return values, true
}

func containsScalar(values []any, target any) bool {
	for _, candidate := range values {
		if left, ok := numericValue(candidate); ok {
			if right, ok := numericValue(target); ok && left == right {
				return true
			}
			continue
		}
		if reflect.DeepEqual(candidate, target) {
			return true
		}
	}
	return false
}
