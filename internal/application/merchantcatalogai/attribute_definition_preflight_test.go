package merchantcatalogai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func proposedColorConflict(pType string, attrs any) Proposal {
	return Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationUpdate,
		ResponseText:  "مراجعة تعديل اللون",
		Update: &UpdateOperation{ItemID: "item",
			NewVariants: []VariantCreate{{
				Ref: "yellow", Name: "أصفر",
				Attributes: map[string]any{"color": attrs},
			}},
		},
		AttributeDefinitions: []ProposedAttributeDefinition{{
			Key: "color", Label: "اللون", DataType: pType,
			ValidationRules: map[string]any{"enum": []any{"أصفر", "أسود"}},
		}},
	}
}

func TestPreflightRejectsExistingLabelTypeConflictBeforeApproval(t *testing.T) {
	repo := schemaPreflightFixture()
	p := proposedColorConflict("select", "أصفر")
	if err := p.Validate(); err != nil {
		t.Fatalf("wire should pass its own isolated rules: %v", err)
	}
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p)
	var conflict *SchemaDefinitionPreflightError
	if !errors.As(err, &conflict) || conflict.Reason != "existing_type_conflict" ||
		conflict.ExpectedType != "multi_select" || conflict.ProposedType != "select" ||
		conflict.Index != 0 {
		t.Fatalf("stored template conflict was not caught before review: %v", err)
	}
	if repo.writes != 0 {
		t.Fatal("preflight performed a write")
	}
}

func TestPreflightCanReuseExistingAliasOnlyWithMatchingTypeRulesAndValues(t *testing.T) {
	repo := schemaPreflightFixture()
	p := proposedColorConflict("multi_select", []any{"أسود"})
	p.AttributeDefinitions[0].ValidationRules = nil
	if err := p.Validate(); err == nil {
		t.Fatal("multi_select model definition without enum should fail standalone validation")
	}
	// A model alias that repeats an existing label must carry identical
	// rules when defined, or remove the duplicate definition and use attr_.
	p.AttributeDefinitions[0].ValidationRules = map[string]any{
		"enum": []any{"أسود", "أزرق", "أبيض"},
	}
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p); err != nil {
		t.Fatalf("compatible model alias should map to stored attr_color: %v", err)
	}
	if _, ok := p.Update.NewVariants[0].Attributes["attr_color"]; ok {
		t.Fatal("preflight mutated the proposal instead of using a copy")
	}
	p.Update.NewVariants[0].Attributes["color"] = "أسود"
	var shape *SchemaAttributePreflightError
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p)
	if !errors.As(err, &shape) || shape.DataType != "multi_select" ||
		!strings.Contains(shape.Path, "attr_color") {
		t.Fatalf("wrong scalar shape not caught after alias mapping: %v", err)
	}
	p.Update.NewVariants[0].Attributes["color"] = []any{"أصفر"}
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p); err == nil {
		t.Fatal("new color not present in existing enum was silently allowed")
	}
}

func TestPreflightRejectsRuleOverrideAndDuplicateDefinitionLabel(t *testing.T) {
	repo := schemaPreflightFixture()
	p := proposedColorConflict("multi_select", []any{"أسود"})
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p)
	var conflict *SchemaDefinitionPreflightError
	if !errors.As(err, &conflict) || conflict.Reason != "existing_rules_conflict" {
		t.Fatalf("model replaced stored choices: %v", err)
	}
	p.AttributeDefinitions[0].ValidationRules = map[string]any{
		"enum": []any{"أسود", "أزرق", "أبيض"},
	}
	p.AttributeDefinitions = append(p.AttributeDefinitions, ProposedAttributeDefinition{
		Key: "shade", Label: "اللون", DataType: "multi_select",
		ValidationRules: map[string]any{"enum": []any{"أسود", "أزرق", "أبيض"}},
	})
	err = ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p)
	if !errors.As(err, &conflict) || conflict.Reason != "duplicate_label" {
		t.Fatalf("same display label repeated in two aliases: %v", err)
	}
}

func TestPreflightRejectsExistingAliasCollisionWithoutMutation(t *testing.T) {
	repo := schemaPreflightFixture()
	p := proposedColorConflict("multi_select", []any{"أسود"})
	p.AttributeDefinitions[0].ValidationRules = map[string]any{
		"enum": []any{"أسود", "أزرق", "أبيض"},
	}
	p.Update.NewVariants[0].Attributes["attr_color"] = []any{"أزرق"}
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p)
	var conflict *SchemaDefinitionPreflightError
	if !errors.As(err, &conflict) || conflict.Reason != "alias_mapping_collision" {
		t.Fatalf("two aliases resolving to one stable key were accepted: %v", err)
	}
}

func TestPreflightRejectsExistingColorSchemaConflictForCreate(t *testing.T) {
	repo := schemaPreflightFixture()
	schemaID := repo.schema.ID
	p := Proposal{
		Status: StatusResolved, Operation: OperationCreate,
		Create: &ItemCreate{
			AttributeSchemaID: &schemaID,
			Attributes: map[string]any{"attr_brand": "Samsung", "color": "أصفر"},
		},
		AttributeDefinitions: []ProposedAttributeDefinition{{
			Key: "color", Label: "اللون", DataType: "select",
			ValidationRules: map[string]any{"enum": []any{"أصفر"}},
		}},
	}
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", p)
	var conflict *SchemaDefinitionPreflightError
	if !errors.As(err, &conflict) || conflict.Reason != "existing_type_conflict" {
		t.Fatalf("create misrepresented stored color type: %v", err)
	}
}

func TestPreflightRejectsConflictBeforeExecutionPrepareStoresProposal(t *testing.T) {
	s, original, store := executionFixture()
	repo := schemaPreflightFixture()
	repo.CatalogRepository = original
	s.Catalogs = repo
	original.item = repo.item
	in := executionUpdate()
	in.Proposal = proposedColorConflict("select", "أصفر")
	if _, err := s.Prepare(context.Background(), in); err == nil {
		t.Fatal("invalid attribute definition became an approvable proposal")
	}
	if store.p.ID != "" {
		t.Fatal("rejected definition changed stored approval state")
	}
}

func TestPreflightExistingSchemaNameIsNotSilentlyReplaced(t *testing.T) {
	schema := &ports.AttributeSchemaRecord{ID: "schema-1", Name: "مواصفات الهواتف"}
	_, _, err := preflightAttributeDefinitionBindings(schema, "قالب آخر", []ProposedAttributeDefinition{{
		Key: "warranty", Label: "الضمان", DataType: "text",
	}})
	var conflict *SchemaDefinitionPreflightError
	if !errors.As(err, &conflict) || conflict.Reason != "existing_schema_name_conflict" {
		t.Fatalf("model renamed a stored template: %v", err)
	}
}
