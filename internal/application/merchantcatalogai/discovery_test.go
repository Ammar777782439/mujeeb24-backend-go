package merchantcatalogai

import (
	"testing"
)

func TestNormalizeAttributeSchemaReferences(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []string
		wantErr bool
	}{
		{
			name: "typed map slice",
			input: map[string]any{
				"attribute_schemas": []map[string]any{
					{"id": "schema-1", "name": "Watch"},
					{"id": "schema-2", "name": "Clothing"},
				},
			},
			want: []string{"schema-1", "schema-2"},
		},
		{
			name: "decoded generic slice",
			input: map[string]any{
				"attribute_schemas": []any{
					map[string]any{"id": "schema-1"},
					map[string]any{"id": "schema-2"},
				},
			},
			want: []string{"schema-1", "schema-2"},
		},
		{
			name: "missing id",
			input: map[string]any{
				"attribute_schemas": []map[string]any{
					{"name": "broken"},
				},
			},
			wantErr: true,
		},
		{
			name: "unexpected shape",
			input: map[string]any{
				"attribute_schemas": "not-an-array",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAttributeSchemaReferences(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected normalization error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected normalization error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d schema refs, want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got schema ref %q at index %d, want %q", got[i], i, tt.want[i])
				}
			}
		})
	}
}

func TestValidateProposalReferencesAllowsDynamicAttributesWithoutSchema(t *testing.T) {
	registry := NewReadOnlyCapabilityRegistry(nil, "")
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت اقتراح إضافة المنتج.",
		Create: &ItemCreate{
			Name: "ساعة",
			Attributes: map[string]any{
				"water_resistant": true,
			},
		},
	}

	if err := registry.ValidateProposalReferences(proposal); err != nil {
		t.Fatalf("dynamic attributes without schema must not require discovery: %v", err)
	}
}

func TestValidateProposalReferencesRequiresDiscoveredSchemaWhenSchemaIDIsPresent(t *testing.T) {
	registry := NewReadOnlyCapabilityRegistry(nil, "")
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت الاقتراح.",
		Create: &ItemCreate{
			Name:              "ساعة",
			AttributeSchemaID: stringPtr("schema-unknown"),
		},
	}

	if err := registry.ValidateProposalReferences(proposal); err == nil {
		t.Fatal("expected unknown attribute schema reference to be rejected")
	}

	registry.attributeSchemaReferences["schema-known"] = struct{}{}
	proposal.Create.AttributeSchemaID = stringPtr("schema-known")
	if err := registry.ValidateProposalReferences(proposal); err != nil {
		t.Fatalf("discovered attribute schema reference must be accepted: %v", err)
	}
}
