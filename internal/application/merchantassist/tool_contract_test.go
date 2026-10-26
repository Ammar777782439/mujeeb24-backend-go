package merchantassist

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestAllMerchantNativeToolContractsHaveStrictRequiredArrays(t *testing.T) {
	offered := []string{
		"platform_guide", "dashboard_overview", "find_customers",
		"list_conversations", "list_leads", "list_transactions", "list_catalogs",
		"list_channels", "subscription", "customer_activity",
		"published_policies", "published_knowledge", "conversation_history",
		"customer_record", "transaction_record", "lead_evidence",
	}
	defs := toolDefinitions(offered)
	if len(defs) != len(offered) {
		t.Fatalf("got %d definitions for %d authorized tools", len(defs), len(offered))
	}
	for _, spec := range defs {
		t.Run(spec.Name, func(t *testing.T) {
			raw, err := json.Marshal(spec.Parameters)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte(`"required":null`)) {
				t.Fatalf("DeepSeek strict tools must never contain required:null: %s", raw)
			}
			var schema struct {
				Type string `json:"type"`
				Properties map[string]json.RawMessage `json:"properties"`
				Required []string `json:"required"`
				AdditionalProperties bool `json:"additionalProperties"`
			}
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			if schema.Type != "object" || schema.Properties == nil || schema.Required == nil ||
				schema.AdditionalProperties || len(schema.Required) != len(schema.Properties) {
				t.Fatalf("invalid strict tool schema: %s", raw)
			}
			for _, name := range schema.Required {
				if _, exists := schema.Properties[name]; !exists {
					t.Fatalf("required property %q is missing: %s", name, raw)
				}
			}
			if len(schema.Properties) == 0 && !bytes.Contains(raw, []byte(`"required":[]`)) {
				t.Fatalf("no-argument tool is not encoded with an empty array: %s", raw)
			}
		})
	}
}

func TestParameterlessMerchantToolCanValidateEmptyArgumentObject(t *testing.T) {
	defs := toolDefinitions([]string{"dashboard_overview", "platform_guide"})
	for _, spec := range defs {
		args, err := decodeReadArgs(spec, json.RawMessage(`{}`))
		if err != nil {
			t.Fatalf("%s empty arguments rejected: %v", spec.Name, err)
		}
		if args.Search != "" || args.RecordID != "" {
			t.Fatalf("%s empty object was not preserved", spec.Name)
		}
	}
}
