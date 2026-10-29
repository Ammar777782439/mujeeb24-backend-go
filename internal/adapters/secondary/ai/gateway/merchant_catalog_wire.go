package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

// The provider sees a closed, fully typed hierarchy. Only individual field values
// use JSON text, which permits arbitrary merchant attributes without weakening the
// structural contract for items, variants, offers, or operations.
type catalogWireField struct {
	Name      string `json:"name"`
	ValueJSON string `json:"value_json"`
}
type catalogWireEntity struct {
	Fields []catalogWireField `json:"fields"`
}
type catalogWireExistingEntity struct {
	ID     string             `json:"id"`
	Fields []catalogWireField `json:"fields"`
}
type catalogWireCreate struct {
	Fields   []catalogWireField  `json:"fields"`
	Variants []catalogWireEntity `json:"variants"`
	Offers   []catalogWireEntity `json:"offers"`
}
type catalogWireUpdate struct {
	ItemID           string                      `json:"item_id"`
	Changes          []catalogWireField          `json:"changes"`
	ExistingVariants []catalogWireExistingEntity `json:"existing_variants"`
	NewVariants      []catalogWireEntity         `json:"new_variants"`
	ExistingOffers   []catalogWireExistingEntity `json:"existing_offers"`
	NewOffers        []catalogWireEntity         `json:"new_offers"`
}
type catalogWireDelete struct {
	ID          string `json:"id"`
	ReasonGiven string `json:"reason_given"`
}
type catalogWireProposal struct {
	AttributeDefinitions []merchantcatalogai.ProposedAttributeDefinition `json:"attribute_definitions"`
	SchemaVersion      int                              `json:"schema_version"`
	Status             merchantcatalogai.ProposalStatus `json:"status"`
	Operation          merchantcatalogai.Operation      `json:"operation"`
	ResponseText       string                           `json:"response_text"`
	EvidenceReferences []string                         `json:"evidence_references"`
	MissingInformation []merchantcatalogai.MissingField `json:"missing_information"`
	Create             catalogWireCreate                `json:"create"`
	Update             catalogWireUpdate                `json:"update"`
	Delete             catalogWireDelete                `json:"delete"`
}

func catalogWireObject(properties map[string]any) map[string]any {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return map[string]any{"type": "object", "properties": properties, "required": names, "additionalProperties": false}
}
func catalogWireArray(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}
func catalogWireString() map[string]any { return map[string]any{"type": "string"} }
func catalogWireFieldSchema(sample any, excluded ...string) map[string]any {
	skip := map[string]bool{}
	for _, name := range excluded {
		skip[name] = true
	}
	t := reflect.TypeOf(sample)
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if !skip[name] && name != "" && name != "-" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return catalogWireArray(catalogWireObject(map[string]any{
		"name":       map[string]any{"type": "string", "enum": names},
		"value_json": catalogWireString(),
	}))
}

// All object keys are required for DeepSeek strict mode; inactive operations
// are expressed by empty arrays/strings, not by nullable partial objects.
func merchantCatalogWireSchema() map[string]any {
	entity := func(fields map[string]any) map[string]any {
		return catalogWireObject(map[string]any{"fields": fields})
	}
	existing := func(fields map[string]any) map[string]any {
		return catalogWireObject(map[string]any{"id": catalogWireString(), "fields": fields})
	}
	missing := catalogWireObject(map[string]any{
		"path": catalogWireString(), "display_name": catalogWireString(),
		"data_type": catalogWireString(), "reason": catalogWireString(),
	})
	return catalogWireObject(map[string]any{
		"schema_version":      map[string]any{"type": "integer", "enum": []any{merchantcatalogai.ProposalSchemaVersion}},
		"status":              map[string]any{"type": "string", "enum": []string{"resolved", "needs_more_data", "not_found", "ambiguous"}},
		"operation":           map[string]any{"type": "string", "enum": []string{"create", "update", "delete", "ask_merchant"}},
		"response_text":       catalogWireString(),
		"evidence_references": catalogWireArray(catalogWireString()),
		"missing_information": catalogWireArray(missing),
		"attribute_definitions": catalogWireArray(catalogWireObject(map[string]any{
			"key": catalogWireString(), "label": catalogWireString(), "data_type": catalogWireString(),
		})),
		"create": catalogWireObject(map[string]any{
			"fields":   catalogWireFieldSchema(merchantcatalogai.ItemCreate{}, "variants", "offers"),
			"variants": catalogWireArray(entity(catalogWireFieldSchema(merchantcatalogai.VariantCreate{}))),
			"offers":   catalogWireArray(entity(catalogWireFieldSchema(merchantcatalogai.OfferCreate{}))),
		}),
		"update": catalogWireObject(map[string]any{
			"item_id":           catalogWireString(),
			"changes":           catalogWireFieldSchema(merchantcatalogai.ItemChanges{}),
			"existing_variants": catalogWireArray(existing(catalogWireFieldSchema(merchantcatalogai.VariantUpdate{}, "id"))),
			"new_variants":      catalogWireArray(entity(catalogWireFieldSchema(merchantcatalogai.VariantCreate{}))),
			"existing_offers":   catalogWireArray(existing(catalogWireFieldSchema(merchantcatalogai.OfferUpdate{}, "id"))),
			"new_offers":        catalogWireArray(entity(catalogWireFieldSchema(merchantcatalogai.OfferCreate{}))),
		}),
		"delete": catalogWireObject(map[string]any{
			"id": catalogWireString(), "reason_given": catalogWireString(),
		}),
	})
}

func catalogWireDecodeFields(fields []catalogWireField, sample any) (map[string]any, error) {
	allowed := map[string]bool{}
	t := reflect.TypeOf(sample)
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			allowed[name] = true
		}
	}
	if len(fields) > 200 {
		return nil, errors.New("merchant proposal has too many field changes")
	}
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		if !allowed[field.Name] {
			return nil, fmt.Errorf("unsupported merchant proposal field %q", field.Name)
		}
		if _, duplicate := out[field.Name]; duplicate {
			return nil, fmt.Errorf("duplicate merchant proposal field %q", field.Name)
		}
		if len(field.ValueJSON) == 0 || len(field.ValueJSON) > 65536 {
			return nil, errors.New("merchant proposal field value has invalid size")
		}
		dec := json.NewDecoder(strings.NewReader(field.ValueJSON))
		dec.UseNumber()
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("invalid merchant field %q: %w", field.Name, err)
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("merchant field %q has trailing JSON", field.Name)
		}
		if value == nil && field.Name != "amount" {
			return nil, fmt.Errorf("merchant field %q cannot be null: omit unchanged fields or use explicit clear flags", field.Name)
		}
		out[field.Name] = value
	}
	return out, nil
}

func catalogWireEntities(entries []catalogWireEntity, sample any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		fields, err := catalogWireDecodeFields(entry.Fields, sample)
		if err != nil {
			return nil, err
		}
		out = append(out, fields)
	}
	return out, nil
}
func catalogWireExistingEntities(entries []catalogWireExistingEntity, sample any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			return nil, errors.New("existing merchant entity lacks ID")
		}
		fields, err := catalogWireDecodeFields(entry.Fields, sample)
		if err != nil {
			return nil, err
		}
		fields["id"] = entry.ID
		out = append(out, fields)
	}
	return out, nil
}
func catalogWireParse(raw string) (merchantcatalogai.Proposal, error) {
	if len(raw) > 524288 {
		return merchantcatalogai.Proposal{}, errors.New("merchant wire proposal exceeds size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire catalogWireProposal
	if err := decoder.Decode(&wire); err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("decode merchant wire proposal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return merchantcatalogai.Proposal{}, errors.New("merchant wire proposal has trailing JSON")
	}
	// The complete provider-facing schema is verified independently.
	if err := validateStrictArguments([]byte(raw), merchantCatalogWireSchema()); err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("merchant wire schema: %w", err)
	}
	proposal := map[string]any{
		"schema_version": wire.SchemaVersion, "status": wire.Status, "operation": wire.Operation,
		"response_text": wire.ResponseText, "evidence_references": wire.EvidenceReferences,
		"missing_information": wire.MissingInformation,
		"attribute_definitions": wire.AttributeDefinitions,
		"create":              nil, "update": nil, "delete": nil,
	}
	switch wire.Operation {
	case merchantcatalogai.OperationCreate:
		if wire.Update.ItemID != "" || len(wire.Update.Changes) != 0 ||
			len(wire.Update.ExistingVariants) != 0 || len(wire.Update.NewVariants) != 0 ||
			len(wire.Update.ExistingOffers) != 0 || len(wire.Update.NewOffers) != 0 ||
			wire.Delete.ID != "" || wire.Delete.ReasonGiven != "" {
			return merchantcatalogai.Proposal{}, errors.New("inactive merchant proposal operation has data")
		}
		create, err := catalogWireDecodeFields(wire.Create.Fields, merchantcatalogai.ItemCreate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		variants, err := catalogWireEntities(wire.Create.Variants, merchantcatalogai.VariantCreate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		offers, err := catalogWireEntities(wire.Create.Offers, merchantcatalogai.OfferCreate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		create["variants"], create["offers"] = variants, offers
		proposal["create"] = create
	case merchantcatalogai.OperationUpdate:
		if len(wire.Create.Fields) != 0 || len(wire.Create.Variants) != 0 ||
			len(wire.Create.Offers) != 0 || wire.Delete.ID != "" || wire.Delete.ReasonGiven != "" {
			return merchantcatalogai.Proposal{}, errors.New("inactive merchant proposal operation has data")
		}
		if wire.Update.ItemID == "" {
			return merchantcatalogai.Proposal{}, errors.New("update missing item ID")
		}
		changes, err := catalogWireDecodeFields(wire.Update.Changes, merchantcatalogai.ItemChanges{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		oldVariants, err := catalogWireExistingEntities(wire.Update.ExistingVariants, merchantcatalogai.VariantUpdate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		newVariants, err := catalogWireEntities(wire.Update.NewVariants, merchantcatalogai.VariantCreate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		oldOffers, err := catalogWireExistingEntities(wire.Update.ExistingOffers, merchantcatalogai.OfferUpdate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		newOffers, err := catalogWireEntities(wire.Update.NewOffers, merchantcatalogai.OfferCreate{})
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		proposal["update"] = map[string]any{
			"item_id": wire.Update.ItemID, "changes": changes,
			"existing_variants": oldVariants, "new_variants": newVariants,
			"existing_offers": oldOffers, "new_offers": newOffers,
		}
	case merchantcatalogai.OperationDelete:
		if len(wire.Create.Fields) != 0 || len(wire.Create.Variants) != 0 ||
			len(wire.Create.Offers) != 0 || wire.Update.ItemID != "" ||
			len(wire.Update.Changes) != 0 || len(wire.Update.ExistingVariants) != 0 ||
			len(wire.Update.NewVariants) != 0 || len(wire.Update.ExistingOffers) != 0 ||
			len(wire.Update.NewOffers) != 0 {
			return merchantcatalogai.Proposal{}, errors.New("inactive merchant proposal operation has data")
		}
		proposal["delete"] = map[string]any{"id": wire.Delete.ID, "reason_given": wire.Delete.ReasonGiven}
	case merchantcatalogai.OperationAskMerchant:
		if len(wire.Create.Fields) != 0 || len(wire.Create.Variants) != 0 ||
			len(wire.Create.Offers) != 0 || wire.Update.ItemID != "" ||
			len(wire.Update.Changes) != 0 || len(wire.Update.ExistingVariants) != 0 ||
			len(wire.Update.NewVariants) != 0 || len(wire.Update.ExistingOffers) != 0 ||
			len(wire.Update.NewOffers) != 0 || wire.Delete.ID != "" || wire.Delete.ReasonGiven != "" {
			return merchantcatalogai.Proposal{}, errors.New("ask merchant operation contains mutations")
		}
	default:
		return merchantcatalogai.Proposal{}, errors.New("unknown merchant proposal operation")
	}
	encoded, err := json.Marshal(proposal)
	if err != nil {
		return merchantcatalogai.Proposal{}, err
	}
	if !json.Valid(encoded) {
		return merchantcatalogai.Proposal{}, errors.New("cannot encode merchant proposal")
	}
	// Keep the authoritative parser, reference validation, and commercial
	// authorization boundaries unchanged.
	proposalValue, err := parseMerchantCatalogProposal(string(encoded))
	if err != nil { return merchantcatalogai.Proposal{}, err }
	if err := proposalValue.ValidateAttributeDefinitions(true); err != nil { return merchantcatalogai.Proposal{}, err }
	return proposalValue, nil
}
