package native

import (
	"encoding/json"
	"strings"
	"testing"
)

const strictCatalogExample = `{"type":"object","properties":{"candidates":{"type":"array","items":{"type":"object","properties":{"item_id":{"type":"string","enum":["item-1"]},"variant_ids":{"type":"array","items":{"type":"string","enum":["variant-1"]}},"offer_ids":{"type":"array","items":{"type":"string","enum":["offer-1"]}},"reason":{"type":"string"}},"required":["item_id","variant_ids","offer_ids","reason"],"additionalProperties":false}}},"required":["candidates"],"additionalProperties":false}`

func TestNativeStrictCatalogSchemaAcceptsOnlyDeclaredRefs(t *testing.T) {
	for _, tc := range []struct {
		json string
		ok   bool
	}{
		{`{"candidates":[{"item_id":"item-1","variant_ids":["variant-1"],"offer_ids":["offer-1"],"reason":"matches"}]}`, true},
		{`{"candidates":[]}`, true},
		{`{"candidates":[{"item_id":"fabricated","variant_ids":[],"offer_ids":[],"reason":"fake"}]}`, false},
		{`{"candidates":[{"item_id":"item-1","variant_ids":["wrong"],"offer_ids":[],"reason":"bad"}]}`, false},
		{`{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":["wrong"],"reason":"bad"}]}`, false},
		{`{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":[]}]}`, false},
		{`{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":[],"reason":"ok","price":30}]}`, false},
		{`{"candidates":[{"item_id":"item-1","variant_ids":null,"offer_ids":[],"reason":"bad"}]}`, false},
		{`{"candidates":[],"admin":true}`, false},
		{`{"candidates":[]} true`, false},
	} {
		t.Run(tc.json, func(t *testing.T) {
			err := validateNativeSchemaResult(json.RawMessage(tc.json), json.RawMessage(strictCatalogExample))
			if (err == nil) != tc.ok {
				t.Fatalf("expected valid=%t error=%v", tc.ok, err)
			}
		})
	}
}
func TestNativeStrictSchemaRejectsUnsupportedKeywordsAndLooseRequired(t *testing.T) {
	bad := []string{
		`{"type":"object","properties":{"a":{"type":"string"}},"required":[],"additionalProperties":false}`,
		`{"type":"object","properties":{},"required":[],"additionalProperties":true}`,
		`{"type":"object","properties":{},"required":[],"additionalProperties":false,"anyOf":[{"type":"string"}]}`,
		`{"type":"array","items":{"type":"string"},"uniqueItems":true}`,
		`{"type":"object","properties":{},"required":null,"additionalProperties":false}`,
	}
	for _, v := range bad {
		if _, err := compileNativeSchema(json.RawMessage(v)); err == nil {
			t.Fatalf("unsafe schema accepted: %s", v)
		}
	}
}
func TestNativeStrictSchemaNullableNotCoerced(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"offer_id":{"type":["string","null"]}},"required":["offer_id"],"additionalProperties":false}`)
	for _, v := range []string{`{"offer_id":null}`, `{"offer_id":"offer-1"}`} {
		if err := validateNativeSchemaResult(json.RawMessage(v), schema); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []string{`{}`, `{"offer_id":"" ,"extra":true}`, `{"offer_id":0}`} {
		if err := validateNativeSchemaResult(json.RawMessage(v), schema); err == nil {
			t.Fatalf("allowed invalid value %s", v)
		}
	}
}
func TestNativeSchemaDiagnosticsDoNotIncludeCustomerValues(t *testing.T) {
	err := validateNativeSchemaResult(json.RawMessage(`{"candidates":[{"item_id":"secret-customer-text","variant_ids":[],"offer_ids":[],"reason":"a"}]}`), json.RawMessage(strictCatalogExample))
	if err == nil || strings.Contains(err.Error(), "secret-customer-text") {
		t.Fatalf("unsafe diagnostic: %v", err)
	}
}
