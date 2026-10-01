package merchantcatalogai

import (
	"errors"
	"fmt"
	"strings"
)

func normalizeAttributeSchemaReferences(data any) ([]string, error) {
	payload, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("attribute schema discovery payload must be object, got %T", data)
	}

	raw, ok := payload["attribute_schemas"]
	if !ok {
		return nil, errors.New("attribute schema discovery payload is missing attribute_schemas")
	}

	ids := make([]string, 0)
	appendSchema := func(index int, schema map[string]any) error {
		id, ok := schema["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return fmt.Errorf("attribute_schemas[%d].id is required", index)
		}
		ids = append(ids, strings.TrimSpace(id))
		return nil
	}

	switch schemas := raw.(type) {
	case []map[string]any:
		ids = make([]string, 0, len(schemas))
		for index, schema := range schemas {
			if err := appendSchema(index, schema); err != nil {
				return nil, err
			}
		}
	case []any:
		ids = make([]string, 0, len(schemas))
		for index, rawSchema := range schemas {
			schema, ok := rawSchema.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("attribute_schemas[%d] must be object, got %T", index, rawSchema)
			}
			if err := appendSchema(index, schema); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("attribute_schemas must be an array, got %T", raw)
	}
	return ids, nil
}

// ValidateProposalReferences enforces the closed B2B reference boundary.
// Every existing resource referenced by a mutation must have been returned by
// a tenant-scoped read capability during this turn.
func (r *ReadOnlyCapabilityRegistry) ValidateProposalReferences(p Proposal) error {
	if p.Status != StatusResolved || !p.IsMutation() {
		return nil
	}

	require := func(id string, kind string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New(kind + " reference is required")
		}
		if _, ok := r.evidenceReferences[id]; !ok {
			return errors.New("proposal references a " + kind + " that was not returned by a catalog read tool: " + id)
		}
		return nil
	}

	switch p.Operation {
	case OperationCreate:
		if p.Create != nil && p.Create.AttributeSchemaID != nil {
			schemaID := strings.TrimSpace(*p.Create.AttributeSchemaID)
			if schemaID == "" {
				return errors.New("create attribute_schema_id cannot be empty")
			}
			if _, ok := r.attributeSchemaReferences[schemaID]; !ok {
				return errors.New("create proposal references an attribute schema that was not returned by attribute schema discovery: " + schemaID)
			}
		}
	case OperationUpdate:
		if p.Update == nil {
			return errors.New("update proposal is missing update payload")
		}
		if err := require(p.Update.ItemID, "catalog item"); err != nil {
			return err
		}
		for _, variant := range p.Update.ExistingVariants {
			if err := require(variant.ID, "variant"); err != nil {
				return err
			}
		}
		for _, offer := range p.Update.ExistingOffers {
			if err := require(offer.ID, "offer"); err != nil {
				return err
			}
		}
		for _, offer := range p.Update.NewOffers {
			if offer.VariantID != nil {
				if err := require(*offer.VariantID, "variant"); err != nil {
					return err
				}
			}
		}
	case OperationDelete:
		if p.Delete == nil {
			return errors.New("delete proposal is missing delete payload")
		}
		return require(p.Delete.ItemID, "catalog item")
	}
	return nil
}
