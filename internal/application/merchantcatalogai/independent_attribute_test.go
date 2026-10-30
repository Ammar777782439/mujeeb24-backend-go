package merchantcatalogai

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func independentAttributeFixture() (*schemaPreflightRepo, Proposal) {
	repo := schemaPreflightFixture()
	repo.schema.Name = "مواصفات الخدمات"
	repo.schema.Definitions = append(repo.schema.Definitions, ports.AttributeDefinitionRecord{
		Key: "attr_target_age", Label: "المنطقة المستهدفة", DataType: "multi_select",
		ValidationRules: []byte(`{"enum":["أطفال","يافعون","بالغون"]}`),
	})
	id := repo.schema.ID
	p := Proposal{
		Status: StatusResolved, Operation: OperationCreate,
		AttributeDefinitions: []ProposedAttributeDefinition{{
			Key: "target_anatomy", Label: "الأعضاء المستهدفة", DataType: "multi_select",
			ValidationRules: map[string]any{"enum": []any{"الأسنان", "اللثة", "الفم"}},
		}},
		Create: &ItemCreate{AttributeSchemaID: &id, Attributes: map[string]any{
			"attr_brand":      "Example",
			"attr_target_age": []any{"بالغون"},
			"target_anatomy":  []any{"الأسنان", "اللثة", "الفم"},
		}},
	}
	return repo, p
}

// A new merchant fact must not be forced into an unrelated existing enum just
// because both definitions were initially described with the same Arabic label.
func TestIndependentAttributeCanExtendSelectedTemplateWithoutRuleConflict(t *testing.T) {
	repo, proposal := independentAttributeFixture()
	if err := proposal.ValidateAttributeDefinitions(true); err != nil {
		t.Fatalf("new attribute's own values must be valid: %v", err)
	}
	before := append([]ports.AttributeDefinitionRecord(nil), repo.schema.Definitions...)
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err != nil {
		t.Fatalf("independent custom attribute must pass read-only preflight: %v", err)
	}
	if repo.writes != 0 || !reflect.DeepEqual(before, repo.schema.Definitions) {
		t.Fatal("preflight changed the stored template")
	}
	if proposal.Create.Attributes["target_anatomy"] == nil {
		t.Fatal("preflight silently discarded the merchant's requested values")
	}

	// Identical label plus conflicting enum is NOT a new independent attribute.
	// It must be disambiguated by the model, not accepted as an override.
	proposal.AttributeDefinitions[0].Label = "المنطقة المستهدفة"
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal)
	var conflict *SchemaDefinitionPreflightError
	if !errors.As(err, &conflict) || conflict.Reason != "existing_rules_conflict" {
		t.Fatalf("existing managed enum was silently overwritten: %v", err)
	}
}

func TestApprovedIndependentAttributeCreatesNewSchemaVersionPreservingOldRules(t *testing.T) {
	repo, proposal := independentAttributeFixture()
	current := repo.schema
	stub := &testAttributeCatalog{next: current.Version + 1}
	record, aliases, err := resolveApprovedAttributeDefinitions(
		context.Background(), stub, "business", "", &current,
		proposal.AttributeDefinitions, time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("approved independent attribute could not be stored: %v", err)
	}
	if record.ID == current.ID || record.Version != current.Version+1 ||
		record.Name != current.Name || len(record.Definitions) != len(current.Definitions)+1 {
		t.Fatalf("failed to extend the existing template version: %+v", record)
	}
	for i, original := range current.Definitions {
		persisted := record.Definitions[i]
		if persisted.Key != original.Key || persisted.Label != original.Label ||
			persisted.DataType != original.DataType ||
			!reflect.DeepEqual(persisted.ValidationRules, original.ValidationRules) {
			t.Fatalf("existing definition %d was overwritten: %+v", i, persisted)
		}
	}
	newKey := aliases["target_anatomy"]
	if len(newKey) <= len("attr_") || newKey[:len("attr_")] != "attr_" ||
		newKey == "attr_target_age" {
		t.Fatalf("new attribute was not issued its own stable key: %q", newKey)
	}
	mapped, err := remapApprovedAttributes(proposal.Create.Attributes, aliases)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mapped[newKey], []any{"الأسنان", "اللثة", "الفم"}) {
		t.Fatalf("new merchant values were lost: %+v", mapped)
	}
}
