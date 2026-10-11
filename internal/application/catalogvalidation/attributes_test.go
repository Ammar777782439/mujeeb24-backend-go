package catalogvalidation

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func schemaFixture() ports.AttributeSchemaRecord {
	return ports.AttributeSchemaRecord{
		ID: "schema",
		Definitions: []ports.AttributeDefinitionRecord{
			{Key: "attr_color", Label: "اللون", DataType: "select", Required: true, ValidationRules: []byte(`{"enum":["أسود","أبيض"]}`)},
			{Key: "attr_capacity", Label: "السعة", DataType: "number", ValidationRules: []byte(`{"min":64,"max":1024}`)},
			{Key: "attr_code", Label: "الرمز", DataType: "text", ValidationRules: []byte(`{"regex":"^[A-Z]{2}[0-9]{2}$"}`)},
		},
	}
}

func TestValidateAttributesEnforcesManagedSchemaValues(t *testing.T) {
	schema := schemaFixture()
	valid := map[string]any{
		"attr_color": "أسود",
		"attr_capacity": 256,
		"attr_code": "AB12",
		"legacy_note": "kept",
	}
	if err := ValidateAttributes(&schema, valid); err != nil {
		t.Fatalf("valid attributes rejected: %v", err)
	}
}

func TestValidateAttributesRejectsMissingWrongAndUnknownManagedKeys(t *testing.T) {
	schema := schemaFixture()
	cases := []map[string]any{
		{"attr_capacity": 256},
		{"attr_color": "أحمر"},
		{"attr_color": "أسود", "attr_capacity": "256"},
		{"attr_color": "أسود", "attr_missing": "ambiguous"},
	}
	for index, attributes := range cases {
		if err := ValidateAttributes(&schema, attributes); err == nil {
			t.Fatalf("case %d unexpectedly passed: %#v", index, attributes)
		}
	}
}

func TestValidateAttributesPreservesLegacyCustomKeys(t *testing.T) {
	if err := ValidateAttributes(nil, map[string]any{"memory_size": "256GB"}); err != nil {
		t.Fatalf("legacy custom key rejected: %v", err)
	}
	if err := ValidateAttributes(nil, map[string]any{"حجم الذاكرة": "256GB"}); err == nil {
		t.Fatal("non machine-safe custom key accepted")
	}
	if err := ValidateAttributes(nil, map[string]any{"attr_unknown": "256GB"}); err == nil {
		t.Fatal("reserved managed key accepted without definition")
	}
}

func TestValidateDefinitionRejectsInvalidKnownRules(t *testing.T) {
	if err := ValidateDefinition("number", map[string]any{"min": 10, "max": 1}); err == nil {
		t.Fatal("invalid min/max accepted")
	}
	if err := ValidateDefinition("text", map[string]any{"regex": "["}); err == nil {
		t.Fatal("invalid regex accepted")
	}
	if err := ValidateDefinition("unknown", nil); err == nil {
		t.Fatal("unsupported data type accepted")
	}
}

func TestValidatePartialAttributesDoesNotRequireEveryItemField(t *testing.T) {
	schema := schemaFixture()
	if err := ValidatePartialAttributes(&schema, map[string]any{"attr_capacity": 256}); err != nil {
		t.Fatalf("valid variant subset rejected: %v", err)
	}
	if err := ValidatePartialAttributes(&schema, map[string]any{"attr_capacity": "wrong"}); err == nil {
		t.Fatal("variant type mismatch was accepted")
	}
}
