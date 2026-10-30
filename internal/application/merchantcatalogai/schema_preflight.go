package merchantcatalogai

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/catalogvalidation"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// SchemaAttributePreflightError contains only safe structural facts.
// Never embed merchant-authored labels/values in trusted model instructions.
type SchemaAttributePreflightError struct {
	Path     string
	DataType string
	Required bool
}

func (e *SchemaAttributePreflightError) Error() string {
	if e.Required {
		return fmt.Sprintf("catalog schema attribute %s (%s) is required or invalid", e.Path, e.DataType)
	}
	return fmt.Sprintf("catalog schema attribute %s expects %s value from the selected template", e.Path, e.DataType)
}

var safeAttributePathSegment = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,95}$`)

func preflightSchemaAttributes(schema *ports.AttributeSchemaRecord, values map[string]any, path string, requireAll bool) error {
	if schema == nil {
		return nil
	}
	definitions := make(map[string]ports.AttributeDefinitionRecord, len(schema.Definitions))
	for _, d := range schema.Definitions {
		definitions[d.Key] = d
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def, ok := definitions[key]
		if !ok {
			continue
		}
		if err := catalogvalidation.ValidatePartialAttributes(schema, map[string]any{key: values[key]}); err != nil {
			field := path + ".attributes"
			if safeAttributePathSegment.MatchString(key) {
				field += "." + key
			}
			return &SchemaAttributePreflightError{Path: field, DataType: def.DataType, Required: def.Required}
		}
	}
	if requireAll {
		for _, def := range schema.Definitions {
			if !def.Required {
				continue
			}
			value, exists := values[def.Key]
			if !exists || value == nil || (func() bool {
				switch typed := value.(type) {
				case string:
					return strings.TrimSpace(typed) == ""
				case []any:
					return len(typed) == 0
				}
				return false
			})() {
				field := path + ".attributes"
				if safeAttributePathSegment.MatchString(def.Key) {
					field += "." + def.Key
				}
				return &SchemaAttributePreflightError{Path: field, DataType: def.DataType, Required: true}
			}
		}
	}
	if requireAll {
		if err := catalogvalidation.ValidateAttributes(schema, values); err != nil {
			return errors.New("catalog schema requires valid item attributes")
		}
		return nil
	}
	if err := catalogvalidation.ValidatePartialAttributes(schema, values); err != nil {
		return errors.New("catalog schema requires valid variant attributes")
	}
	return nil
}

// ValidateProposalSchemaAttributes is a read-only preflight; it runs before
// saving a merchant-approvable proposal. Execution revalidates after locking
// to defend against concurrent catalog/schema changes.
func ValidateProposalSchemaAttributes(
	ctx context.Context, catalog ports.CatalogRepository, businessID, catalogID string, p Proposal,
) error {
	if catalog == nil || p.Status != StatusResolved || !p.IsMutation() {
		return nil
	}
	var schema *ports.AttributeSchemaRecord
	if p.Create != nil {
		if p.Create.AttributeSchemaID != nil {
			loaded, err := catalog.GetAttributeSchema(ctx, businessID, *p.Create.AttributeSchemaID)
			if err != nil {
				return fmt.Errorf("preflight create template lookup: %w", err)
			}
			schema = &loaded
		}
		if err := preflightSchemaAttributes(schema, p.Create.Attributes, "create", true); err != nil {
			return err
		}
		for i, variant := range p.Create.Variants {
			if err := preflightSchemaAttributes(schema, variant.Attributes, fmt.Sprintf("create.variants[%d]", i), false); err != nil {
				return err
			}
		}
		return nil
	}
	if p.Update == nil {
		return nil
	}
	u := p.Update
	item, err := catalog.GetCatalogItem(ctx, businessID, catalogID, u.ItemID)
	if err != nil {
		return fmt.Errorf("preflight item lookup: %w", err)
	}
	if item.AttributeSchemaID != nil {
		loaded, err := catalog.GetAttributeSchema(ctx, businessID, *item.AttributeSchemaID)
		if err != nil {
			return fmt.Errorf("preflight item template lookup: %w", err)
		}
		schema = &loaded
	}
	if u.AttributeSchemaID != nil {
		if schema != nil && schema.ID != *u.AttributeSchemaID {
			return errors.New("cannot change the existing catalog attribute template by inference")
		}
		if schema == nil {
			loaded, err := catalog.GetAttributeSchema(ctx, businessID, *u.AttributeSchemaID)
			if err != nil {
				return fmt.Errorf("preflight updated template lookup: %w", err)
			}
			schema = &loaded
		}
	}
	if schema == nil {
		return nil
	}
	if u.Changes.Attributes != nil || (item.AttributeSchemaID == nil && u.AttributeSchemaID != nil) {
		raw, err := mergeAttributes(item.Attributes, u.Changes.Attributes)
		if err != nil {
			return errors.New("preflight cannot merge current item attributes")
		}
		if u.Changes.Attributes == nil {
			raw = item.Attributes
		}
		merged, err := decodeAttributes(raw)
		if err != nil {
			return errors.New("preflight cannot read current item attributes")
		}
		if err := preflightSchemaAttributes(schema, merged, "update.changes", true); err != nil {
			return err
		}
	}
	for i, v := range u.NewVariants {
		if err := preflightSchemaAttributes(schema, v.Attributes, fmt.Sprintf("update.new_variants[%d]", i), false); err != nil {
			return err
		}
	}
	for i, v := range u.ExistingVariants {
		if v.Attributes == nil {
			continue
		}
		current, err := catalog.GetVariant(ctx, businessID, v.ID)
		if err != nil {
			return fmt.Errorf("preflight current variant lookup: %w", err)
		}
		if current.CatalogItemID != u.ItemID {
			return errors.New("preflight variant belongs to a different catalog item")
		}
		raw, err := mergeAttributes(current.Attributes, v.Attributes)
		if err != nil {
			return errors.New("preflight cannot merge current variant attributes")
		}
		merged, err := decodeAttributes(raw)
		if err != nil {
			return errors.New("preflight cannot read current variant attributes")
		}
		if err := preflightSchemaAttributes(schema, merged, fmt.Sprintf("update.existing_variants[%d]", i), false); err != nil {
			return err
		}
	}
	return nil
}
