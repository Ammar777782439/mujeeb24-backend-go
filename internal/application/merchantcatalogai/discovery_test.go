package merchantcatalogai

import (
	"context"
	"encoding/json"
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

func TestValidateProposalReferencesSupportsDirectOfferWithoutVariant(t *testing.T) {
	registry := NewReadOnlyCapabilityRegistry(nil, "")
	registry.evidenceReferences["item-1"] = struct{}{}
	registry.evidenceReferences["offer-1"] = struct{}{}

	blank := ""
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationUpdate,
		ResponseText:  "أعددت اقتراح تعديل السعر.",
		Update: &UpdateOperation{
			ItemID: "item-1",
			ExistingOffers: []OfferUpdate{{
				ID: "offer-1", VariantID: &blank, Amount: stringPtr("5000"),
			}},
		},
	}.Normalize()

	if proposal.Update.ExistingOffers[0].VariantID != nil {
		t.Fatal("direct offer must not retain an empty variant reference")
	}
	if err := registry.ValidateProposalReferences(proposal); err != nil {
		t.Fatalf("direct offer without variant should pass evidence validation: %v", err)
	}
	if err := proposal.Validate(); err != nil {
		t.Fatalf("direct offer price update should remain a valid proposal: %v", err)
	}
}

func TestValidateProposalReferencesAllowsEvidenceBackedOfferVariant(t *testing.T) {
	registry := NewReadOnlyCapabilityRegistry(nil, "")
	registry.evidenceReferences["item-1"] = struct{}{}
	registry.evidenceReferences["offer-1"] = struct{}{}
	registry.evidenceReferences["variant-1"] = struct{}{}

	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationUpdate,
		ResponseText:  "أعددت اقتراح تعديل السعر.",
		Update: &UpdateOperation{
			ItemID: "item-1",
			ExistingOffers: []OfferUpdate{{
				ID: "offer-1", VariantID: stringPtr("variant-1"), Amount: stringPtr("5000"),
			}},
		},
	}.Normalize()

	if err := registry.ValidateProposalReferences(proposal); err != nil {
		t.Fatalf("evidence-backed variant should be accepted: %v", err)
	}
}

func TestValidateProposalReferencesRejectsInventedOfferVariant(t *testing.T) {
	registry := NewReadOnlyCapabilityRegistry(nil, "")
	registry.evidenceReferences["item-1"] = struct{}{}
	registry.evidenceReferences["offer-1"] = struct{}{}

	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationUpdate,
		ResponseText:  "أعددت اقتراح تعديل السعر.",
		Update: &UpdateOperation{
			ItemID: "item-1",
			ExistingOffers: []OfferUpdate{{
				ID: "offer-1", VariantID: stringPtr("variant-invented"), Amount: stringPtr("5000"),
			}},
		},
	}.Normalize()

	if err := registry.ValidateProposalReferences(proposal); err == nil {
		t.Fatal("invented non-empty variant reference must remain fail-closed")
	}
}

type cursorDiscoveryStub struct {
	name  string
	calls int
}

func (s *cursorDiscoveryStub) Definition() MerchantCatalogDiscoveryToolDefinition {
	return MerchantCatalogDiscoveryToolDefinition{Name: s.name}
}

func (s *cursorDiscoveryStub) Execute(_ context.Context, _ MerchantCatalogDiscoveryExecutionContext, raw []byte) (MerchantCatalogDiscoveryResult, error) {
	s.calls++
	var params struct {
		Cursor *string `json:"cursor"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}
	if params.Cursor != nil && *params.Cursor != "" {
		return MerchantCatalogDiscoveryResult{Data: []string{"last-item"}, Operation: s.name}, nil
	}
	return MerchantCatalogDiscoveryResult{Data: []string{"first-item"}, HasMore: true, NextCursor: "server-issued-page-2", Operation: s.name}, nil
}

func TestMerchantDiscoveryCursorIsIssuedForMatchingReadScope(t *testing.T) {
	for _, toolName := range []string{
		"merchant_catalog_list_items",
		"merchant_catalog_list_variants",
		"merchant_catalog_list_offers",
		"merchant_catalog_list_attribute_schemas",
	} {
		t.Run(toolName, func(t *testing.T) {
			r := NewReadOnlyCapabilityRegistry(nil, "catalog-one")
			stub := &cursorDiscoveryStub{name: toolName}
			r.capabilities[toolName] = stub
			scope := MerchantCatalogDiscoveryExecutionContext{BusinessID: "business-one", PrincipalID: "owner", ConversationID: "session"}
			first := []byte(`{"status":"active","limit":50,"item_id":"item-one","cursor":null}`)
			second := []byte(`{"status":"active","limit":50,"item_id":"item-one","cursor":"server-issued-page-2"}`)
			invented := []byte(`{"status":"active","limit":50,"item_id":"item-one","cursor":"invented"}`)
			if _, err := r.Execute(context.Background(), scope, toolName, invented); err == nil || stub.calls != 0 {
				t.Fatal("invented cursor reached repository tool")
			}
			result, err := r.Execute(context.Background(), scope, toolName, first)
			if err != nil || !result.HasMore || result.NextCursor != "server-issued-page-2" {
				t.Fatalf("first page not recorded: %+v %v", result, err)
			}
			for _, bad := range []struct {
				scope MerchantCatalogDiscoveryExecutionContext
				raw   []byte
			}{
				{scope, invented},
				{scope, []byte(`{"status":"inactive","limit":50,"item_id":"item-one","cursor":"server-issued-page-2"}`)},
				{scope, []byte(`{"status":"active","limit":25,"item_id":"item-one","cursor":"server-issued-page-2"}`)},
				{scope, []byte(`{"status":"active","limit":50,"item_id":"item-two","cursor":"server-issued-page-2"}`)},
				{MerchantCatalogDiscoveryExecutionContext{BusinessID: "business-two", PrincipalID: "owner", ConversationID: "session"}, second},
				{MerchantCatalogDiscoveryExecutionContext{BusinessID: "business-one", PrincipalID: "other", ConversationID: "session"}, second},
				{MerchantCatalogDiscoveryExecutionContext{BusinessID: "business-one", PrincipalID: "owner", ConversationID: "other"}, second},
			} {
				if _, err := r.Execute(context.Background(), bad.scope, toolName, bad.raw); err == nil {
					t.Fatal("unissued or cross-scope cursor accepted")
				}
			}
			if stub.calls != 1 {
				t.Fatal("invalid cursor invoked the tool")
			}
			result, err = r.Execute(context.Background(), scope, toolName, second)
			if err != nil || result.HasMore || result.NextCursor != "" || stub.calls != 2 {
				t.Fatalf("valid continuation failed: %+v %v", result, err)
			}
			if _, err := r.Execute(context.Background(), scope, toolName, second); err == nil || stub.calls != 2 {
				t.Fatal("cursor reused after has_more=false")
			}
		})
	}
}
