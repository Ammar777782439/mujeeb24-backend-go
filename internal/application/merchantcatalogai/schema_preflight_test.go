package merchantcatalogai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type schemaPreflightRepo struct {
	ports.CatalogRepository
	item    ports.CatalogItemRecord
	variant ports.VariantRecord
	schema  ports.AttributeSchemaRecord
	reads   int
	writes  int
}

func (r *schemaPreflightRepo) GetAttributeSchema(_ context.Context, business, id string) (ports.AttributeSchemaRecord, error) {
	r.reads++
	if business != "business" || id != r.schema.ID {
		return ports.AttributeSchemaRecord{}, errors.New("schema not in tenant")
	}
	return r.schema, nil
}
func (r *schemaPreflightRepo) GetCatalogItem(_ context.Context, business, catalog, id string) (ports.CatalogItemRecord, error) {
	r.reads++
	if business != "business" || catalog != "catalog" || id != r.item.ID {
		return ports.CatalogItemRecord{}, errors.New("item not in catalog")
	}
	return r.item, nil
}
func (r *schemaPreflightRepo) GetVariant(_ context.Context, business, id string) (ports.VariantRecord, error) {
	r.reads++
	if business != "business" || id != r.variant.ID {
		return ports.VariantRecord{}, errors.New("variant not found")
	}
	return r.variant, nil
}

func schemaPreflightFixture() *schemaPreflightRepo {
	schemaID := "schema"
	return &schemaPreflightRepo{
		schema: ports.AttributeSchemaRecord{ID: schemaID, BusinessID: "business", Version: 1, Definitions: []ports.AttributeDefinitionRecord{
			{Key: "attr_brand", Label: "العلامة التجارية", DataType: "text", Required: true, ValidationRules: []byte("{}")},
			{Key: "attr_color", Label: "اللون", DataType: "multi_select", Required: false, ValidationRules: []byte(`{"enum":["أسود","أزرق","أبيض"]}`)},
		}},
		item: ports.CatalogItemRecord{ID: "item", BusinessID: "business", CatalogID: "catalog", AttributeSchemaID: &schemaID,
			Attributes: []byte(`{"attr_brand":"Samsung","attr_color":["أسود","أزرق"]}`), ResourceVersion: 5},
		variant: ports.VariantRecord{ID: "variant", BusinessID: "business", CatalogItemID: "item", Attributes: []byte(`{"attr_color":["أزرق"]}`)},
	}
}

func TestPreflightRejectsMissingRequiredBrandBeforeApproval(t *testing.T) {
	repo := schemaPreflightFixture()
	id := repo.schema.ID
	proposal := Proposal{Status: StatusResolved, Operation: OperationCreate,
		Create: &ItemCreate{AttributeSchemaID: &id, Attributes: map[string]any{"attr_color": []any{"أسود"}}},
	}
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal)
	var field *SchemaAttributePreflightError
	if !errors.As(err, &field) || !field.Required || !strings.Contains(field.Path, "attr_brand") {
		t.Fatalf("required catalog template brand was not detected: %v", err)
	}
	if repo.reads != 1 || repo.writes != 0 {
		t.Fatalf("preflight must only read schema once, reads=%d writes=%d", repo.reads, repo.writes)
	}
	proposal.Create.Attributes["attr_brand"] = "Samsung"
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err != nil {
		t.Fatalf("valid required brand rejected: %v", err)
	}
}

func TestPreflightRejectsStringForMultiSelectVariantAndAcceptsArray(t *testing.T) {
	repo := schemaPreflightFixture()
	proposal := Proposal{Status: StatusResolved, Operation: OperationUpdate, Update: &UpdateOperation{
		ItemID: "item", NewVariants: []VariantCreate{
			{Ref: "black", Name: "أسود", Attributes: map[string]any{"attr_color": "أسود"}},
		},
	}}
	err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal)
	var field *SchemaAttributePreflightError
	if !errors.As(err, &field) || field.DataType != "multi_select" ||
		!strings.Contains(field.Path, "update.new_variants[0].attributes.attr_color") {
		t.Fatalf("wrong type was not caught before approval: %v", err)
	}
	proposal.Update.NewVariants[0].Attributes["attr_color"] = []any{"أسود"}
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err != nil {
		t.Fatalf("valid single-element multi_select array rejected: %v", err)
	}
	proposal.Update.NewVariants[0].Attributes["attr_color"] = []any{"غير مدرج"}
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err == nil {
		t.Fatal("unverified enum choice accepted")
	}
}

func TestPreflightExistingVariantChecksMergedAttributesAndOwnership(t *testing.T) {
	repo := schemaPreflightFixture()
	proposal := Proposal{Status: StatusResolved, Operation: OperationUpdate, Update: &UpdateOperation{
		ItemID: "item", ExistingVariants: []VariantUpdate{{
			ID: "variant", Attributes: map[string]any{"attr_color": "أسود"},
		}},
	}}
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err == nil {
		t.Fatal("existing variant wrong attribute shape reached approval")
	}
	proposal.Update.ExistingVariants[0].Attributes = map[string]any{"attr_color": []any{"أسود"}}
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err != nil {
		t.Fatalf("valid variant patch rejected: %v", err)
	}
	repo.variant.CatalogItemID = "foreign-item"
	if err := ValidateProposalSchemaAttributes(context.Background(), repo, "business", "catalog", proposal); err == nil {
		t.Fatal("variant from another item accepted")
	}
}

func TestPreflightRejectsSchemaInvalidProposalBeforeStoring(t *testing.T) {
	s, original, store := executionFixture()
	repo := schemaPreflightFixture()
	repo.CatalogRepository = original
	s.Catalogs = repo
	original.item = repo.item
	in := executionUpdate()
	in.Proposal.Update.NewVariants[0].Attributes = map[string]any{"attr_color": "أسود"}
	if _, err := s.Prepare(context.Background(), in); err == nil {
		t.Fatal("invalid variant proposal was stored for merchant approval")
	}
	if store.p.ID != "" {
		t.Fatal("preflight failure stored a proposal")
	}
}
