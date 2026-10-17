package gateway

import (
	"testing"
)

func TestMerchantDiscoveryStrictSchemaRequiredAndNullable(t *testing.T) {
	schema, err := strictDiscoverySchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"item_id": map[string]any{"type": "string"},
			"cursor": map[string]any{"type": "string"},
			"limit": map[string]any{"type": "integer"},
		},
		"required": []string{"item_id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateStrictSchema(schema); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		input string
		valid bool
	}{
		{name: "nullable-options", input: `{"item_id":"item-1","cursor":null,"limit":null}`, valid: true},
		{name: "bounded-options", input: `{"item_id":"item-1","cursor":"next","limit":20}`, valid: true},
		{name: "omitted-options", input: `{"item_id":"item-1"}`, valid: false},
		{name: "extra-authority", input: `{"item_id":"item-1","cursor":null,"limit":null,"approved":true}`, valid: false},
		{name: "missing-identity", input: `{"item_id":null,"cursor":null,"limit":null}`, valid: false},
		{name: "invalid-limit", input: `{"item_id":"item-1","cursor":null,"limit":"twenty"}`, valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStrictArguments([]byte(tc.input), schema)
			if (err == nil) != tc.valid {
				t.Fatalf("schema validation did not match expectation: %v", err)
			}
		})
	}
}

func TestMerchantDiscoveryRejectsOpenEndedSchema(t *testing.T) {
	for _, bad := range []map[string]any{
		{"type":"object", "properties": map[string]any{"free": map[string]any{"type":"object"}}},
		{"type":"object", "properties": map[string]any{}, "additionalProperties": true},
	} {
		if _, err := strictDiscoverySchema(bad); err == nil {
			t.Fatal("dynamic or open-ended merchant tool schema accepted")
		}
	}
}
