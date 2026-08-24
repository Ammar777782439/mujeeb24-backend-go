package services

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type CatalogCommandServices struct {
	Repository   ports.CatalogRepository
	Transactions ports.TransactionManager
	Now          func() time.Time
	NewID        func() string
}

func NewCatalogCommandServices(repository ports.CatalogRepository, transactions ports.TransactionManager) CatalogCommandServices {
	return CatalogCommandServices{Repository: repository, Transactions: transactions, Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}
}

func (s CatalogCommandServices) ready() error {
	if s.Repository == nil || s.Transactions == nil {
		return appErrors.NotImplemented()
	}
	return nil
}

func (s CatalogCommandServices) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s CatalogCommandServices) id() string {
	if s.NewID == nil {
		return uuid.NewString()
	}
	return s.NewID()
}

func (s CatalogCommandServices) within(ctx context.Context, fn func(context.Context) error) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.Transactions.Within(ctx, fn)
}

type CreateCatalogCommandService struct{ CatalogCommandServices }

func (s CreateCatalogCommandService) Handle(ctx context.Context, command commands.CreateCatalogCommand) (commands.CatalogResult, error) {
	var result commands.CatalogResult
	if command.Meta.Actor.BusinessID == "" || command.Name == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business and catalog name are required")
	}
	created := s.now()
	var record ports.CatalogRecord
	err := s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.CreateCatalog(txCtx, ports.CatalogDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), Name: command.Name, Description: optionalText(command.Description), Status: "draft", CreatedAt: created, UpdatedAt: created})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Catalog = catalogView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type UpdateCatalogCommandService struct{ CatalogCommandServices }

func (s UpdateCatalogCommandService) Handle(ctx context.Context, command commands.UpdateCatalogCommand) (commands.CatalogResult, error) {
	var result commands.CatalogResult
	version, err := parseExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.CatalogID == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business and catalog id are required")
	}
	var record ports.CatalogRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.UpdateCatalog(txCtx, ports.CatalogPatch{ID: string(command.CatalogID), BusinessID: string(command.Meta.Actor.BusinessID), Name: command.Name, Description: command.Description, Status: command.Status, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Catalog = catalogView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type CreateAttributeSchemaVersionCommandService struct{ CatalogCommandServices }

func (s CreateAttributeSchemaVersionCommandService) Handle(ctx context.Context, command commands.CreateAttributeSchemaVersionCommand) (commands.AttributeSchemaResult, error) {
	var result commands.AttributeSchemaResult
	if command.Meta.Actor.BusinessID == "" || command.Name == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business and schema name are required")
	}
	definitionDrafts := make([]ports.AttributeDefinitionDraft, 0, len(command.Definitions))
	for index, definition := range command.Definitions {
		if definition.Key == "" || definition.Label == "" || definition.DataType == "" || definition.DisplayOrder < 0 {
			return result, appErrors.New(appErrors.CodeValidation, "attribute definition is invalid at index "+strconv.Itoa(index))
		}
		definitionDrafts = append(definitionDrafts, ports.AttributeDefinitionDraft{ID: s.id(), Key: definition.Key, Label: definition.Label, DataType: definition.DataType, Required: definition.Required, Searchable: definition.Searchable, DisplayOrder: definition.DisplayOrder, ValidationRules: []byte(`{}`), CreatedAt: s.now(), UpdatedAt: s.now()})
	}
	now := s.now()
	var record ports.AttributeSchemaRecord
	err := s.within(ctx, func(txCtx context.Context) error {
		version, err := s.Repository.NextAttributeSchemaVersion(txCtx, string(command.Meta.Actor.BusinessID), command.Name)
		if err != nil {
			return mapCatalogRepositoryError(err)
		}
		record, err = s.Repository.CreateAttributeSchemaVersion(txCtx, ports.AttributeSchemaDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), Name: command.Name, Version: version, Definitions: definitionDrafts, CreatedAt: now, UpdatedAt: now})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Schema = attributeSchemaView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.Itoa(record.Version))
	result.Status = "published"
	return result, nil
}

type CreateCatalogItemCommandService struct{ CatalogCommandServices }

func (s CreateCatalogItemCommandService) Handle(ctx context.Context, command commands.CreateCatalogItemCommand) (commands.CatalogItemResult, error) {
	var result commands.CatalogItemResult
	if command.Meta.Actor.BusinessID == "" || command.CatalogID == "" || command.ItemType == "" || command.Name == "" || command.PricingMode == "" || command.AvailabilityMode == "" || command.FulfillmentMode == "" {
		return result, appErrors.New(appErrors.CodeValidation, "required catalog item fields are missing")
	}
	attributes, err := encodeObject(command.Attributes)
	if err != nil {
		return result, err
	}
	var schemaVersion *int
	if command.AttributeSchemaID != nil {
		if *command.AttributeSchemaID == "" {
			return result, appErrors.New(appErrors.CodeValidation, "attribute schema id is invalid")
		}
		schema, err := s.Repository.GetAttributeSchema(ctx, string(command.Meta.Actor.BusinessID), string(*command.AttributeSchemaID))
		if err != nil {
			return result, mapCatalogRepositoryError(err)
		}
		schemaVersion = &schema.Version
	}
	now := s.now()
	var record ports.CatalogItemRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.CreateCatalogItem(txCtx, ports.CatalogItemDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), CatalogID: string(command.CatalogID), AttributeSchemaID: optionalID(command.AttributeSchemaID), AttributeSchemaVersion: schemaVersion, ItemType: command.ItemType, Name: command.Name, PricingMode: command.PricingMode, AvailabilityMode: command.AvailabilityMode, FulfillmentMode: command.FulfillmentMode, RequiresConfirmation: command.RequiresConfirmation, Attributes: attributes, CreatedAt: now, UpdatedAt: now})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Item = catalogItemView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type UpdateCatalogItemCommandService struct{ CatalogCommandServices }

func (s UpdateCatalogItemCommandService) Handle(ctx context.Context, command commands.UpdateCatalogItemCommand) (commands.CatalogItemResult, error) {
	var result commands.CatalogItemResult
	version, err := parseExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	attributes, err := optionalObject(command.Attributes)
	if err != nil {
		return result, err
	}
	var record ports.CatalogItemRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.UpdateCatalogItem(txCtx, ports.CatalogItemPatch{ID: string(command.CatalogItemID), BusinessID: string(command.Meta.Actor.BusinessID), Name: command.Name, Status: command.Status, Attributes: attributes, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Item = catalogItemView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type CreateOfferCommandService struct{ CatalogCommandServices }

func (s CreateOfferCommandService) Handle(ctx context.Context, command commands.CreateOfferCommand) (commands.OfferResult, error) {
	var result commands.OfferResult
	if command.Meta.Actor.BusinessID == "" || command.CatalogItemID == "" || command.Name == "" || command.PricingMode == "" || command.AvailabilityMode == "" || command.AvailabilityStatus == "" || command.FulfillmentMode == "" || command.Status == "" {
		return result, appErrors.New(appErrors.CodeValidation, "required offer fields are missing")
	}
	now := s.now()
	var record ports.OfferRecord
	err := s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.CreateOffer(txCtx, ports.OfferDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), CatalogItemID: string(command.CatalogItemID), VariantID: optionalID(command.VariantID), Name: command.Name, PricingMode: command.PricingMode, AmountMinor: command.AmountMinor, Currency: command.Currency, AvailabilityMode: command.AvailabilityMode, AvailabilityStatus: command.AvailabilityStatus, FulfillmentMode: command.FulfillmentMode, Status: command.Status, CreatedAt: now, UpdatedAt: now})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Offer = offerView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type UpdateOfferCommandService struct{ CatalogCommandServices }

func (s UpdateOfferCommandService) Handle(ctx context.Context, command commands.UpdateOfferCommand) (commands.OfferResult, error) {
	var result commands.OfferResult
	version, err := parseExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	var record ports.OfferRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.UpdateOffer(txCtx, ports.OfferPatch{ID: string(command.OfferID), BusinessID: string(command.Meta.Actor.BusinessID), Name: command.Name, AmountMinor: command.AmountMinor, AvailabilityStatus: command.AvailabilityStatus, Status: command.Status, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Offer = offerView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type CreateVariantCommandService struct{ CatalogCommandServices }

func (s CreateVariantCommandService) Handle(ctx context.Context, command commands.CreateVariantCommand) (commands.VariantResult, error) {
	var result commands.VariantResult
	if command.Meta.Actor.BusinessID == "" || command.CatalogItemID == "" || command.Name == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business, item, and variant name are required")
	}
	attributes, err := encodeObject(command.Attributes)
	if err != nil {
		return result, err
	}
	now := s.now()
	var record ports.VariantRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.CreateVariant(txCtx, ports.VariantDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), CatalogItemID: string(command.CatalogItemID), Name: command.Name, Attributes: attributes, Status: "active", CreatedAt: now, UpdatedAt: now})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Variant = variantView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type UpdateVariantCommandService struct{ CatalogCommandServices }

func (s UpdateVariantCommandService) Handle(ctx context.Context, command commands.UpdateVariantCommand) (commands.VariantResult, error) {
	var result commands.VariantResult
	version, err := parseExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	attributes, err := optionalObject(command.Attributes)
	if err != nil {
		return result, err
	}
	var record ports.VariantRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.Repository.UpdateVariant(txCtx, ports.VariantPatch{ID: string(command.VariantID), BusinessID: string(command.Meta.Actor.BusinessID), Name: command.Name, Attributes: attributes, Status: command.Status, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapCatalogRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Variant = variantView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

func parseExpectedVersion(value *commands.ResourceVersion) (int64, error) {
	if value == nil || *value == "" {
		return 0, appErrors.New(appErrors.CodeValidation, "If-Match resource version is required")
	}
	version, err := strconv.ParseInt(string(*value), 10, 64)
	if err != nil || version <= 0 {
		return 0, appErrors.New(appErrors.CodeValidation, "If-Match resource version must be a positive integer")
	}
	return version, nil
}

func encodeObject(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, appErrors.New(appErrors.CodeValidation, "attributes must be valid JSON")
	}
	return encoded, nil
}

func optionalObject(value map[string]any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return encodeObject(value)
}

func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalID[T ~string](value *T) *string {
	if value == nil || *value == "" {
		return nil
	}
	id := string(*value)
	return &id
}

func mapCatalogRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "catalog resource was not found")
	case "stale":
		return appErrors.New(appErrors.CodeStaleResource, "catalog resource version is stale")
	case "conflict":
		return appErrors.New(appErrors.CodeConflict, "catalog resource conflicts with an existing record")
	case "invalid":
		return appErrors.New(appErrors.CodeValidation, "catalog persistence rejected the request")
	default:
		return err
	}
}

var _ commands.CreateCatalogHandler = CreateCatalogCommandService{}
var _ commands.UpdateCatalogHandler = UpdateCatalogCommandService{}
var _ commands.CreateAttributeSchemaVersionHandler = CreateAttributeSchemaVersionCommandService{}
var _ commands.CreateCatalogItemHandler = CreateCatalogItemCommandService{}
var _ commands.UpdateCatalogItemHandler = UpdateCatalogItemCommandService{}
var _ commands.CreateOfferHandler = CreateOfferCommandService{}
var _ commands.UpdateOfferHandler = UpdateOfferCommandService{}
var _ commands.CreateVariantHandler = CreateVariantCommandService{}
var _ commands.UpdateVariantHandler = UpdateVariantCommandService{}
