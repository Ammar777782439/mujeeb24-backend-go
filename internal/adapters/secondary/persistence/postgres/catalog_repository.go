package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type CatalogRepository struct{ adapter *Adapter }

func NewCatalogRepository(adapter *Adapter) *CatalogRepository {
	return &CatalogRepository{adapter: adapter}
}

type catalogCursor struct {
	UpdatedAt time.Time
	ID        string
}

type schemaCursor struct {
	Version int
	ID      string
}

func (r *CatalogRepository) ListCatalogs(ctx context.Context, businessID, status string, limit int, cursor string) (ports.CatalogPage, error) {
	executor, err := r.catalogExecutor(ctx, "catalog.list")
	if err != nil {
		return ports.CatalogPage{}, err
	}
	limit, decoded, err := catalogPageArgs(limit, cursor, "catalog.list")
	if err != nil {
		return ports.CatalogPage{}, err
	}
	var updatedAt any
	var id any
	if decoded != nil {
		updatedAt, id = decoded.UpdatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, name, description, status, created_at, updated_at FROM catalogs WHERE business_id = $1::uuid AND ($2 = '' OR status = $2) AND ($3::timestamptz IS NULL OR (updated_at, id) < ($3::timestamptz, $4::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $5`, businessID, status, updatedAt, id, limit+1)
	if err != nil {
		return ports.CatalogPage{}, catalogRepositoryError("catalog.list", err)
	}
	defer rows.Close()
	items := make([]ports.CatalogRecord, 0, limit)
	for rows.Next() {
		var item ports.CatalogRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.Name, &item.Description, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ports.CatalogPage{}, catalogRepositoryError("catalog.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.CatalogPage{}, catalogRepositoryError("catalog.list", err)
	}
	page := ports.CatalogPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeCatalogCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CatalogRepository) GetCatalog(ctx context.Context, businessID, catalogID string) (ports.CatalogRecord, error) {
	executor, err := r.catalogExecutor(ctx, "catalog.get")
	if err != nil {
		return ports.CatalogRecord{}, err
	}
	if businessID == "" || catalogID == "" {
		return ports.CatalogRecord{}, invalidRepositoryInput("catalog.get", "business and catalog ids are required")
	}
	var item ports.CatalogRecord
	if err := executor.QueryRow(ctx, `SELECT id::text, business_id::text, name, description, status, created_at, updated_at FROM catalogs WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, catalogID).Scan(&item.ID, &item.BusinessID, &item.Name, &item.Description, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, classifyRepositoryGetError("catalog.get", err)
	}
	return item, nil
}

func (r *CatalogRepository) ListCatalogItems(ctx context.Context, businessID, catalogID, search, status string, limit int, cursor string) (ports.CatalogItemPage, error) {
	executor, err := r.catalogExecutor(ctx, "catalog_item.list")
	if err != nil {
		return ports.CatalogItemPage{}, err
	}
	if err := r.ensureScopedParent(ctx, executor, "catalog_item.list", `SELECT EXISTS (SELECT 1 FROM catalogs WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, catalogID); err != nil {
		return ports.CatalogItemPage{}, err
	}
	limit, decoded, err := catalogPageArgs(limit, cursor, "catalog_item.list")
	if err != nil {
		return ports.CatalogItemPage{}, err
	}
	var updatedAt any
	var id any
	if decoded != nil {
		updatedAt, id = decoded.UpdatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, catalog_id::text, attribute_schema_id::text, attribute_schema_version, item_type, name, status, attributes, created_at, updated_at FROM catalog_items WHERE business_id = $1::uuid AND catalog_id = $2::uuid AND ($3 = '' OR status = $3) AND ($4 = '' OR name ILIKE '%' || $4 || '%') AND ($5::timestamptz IS NULL OR (updated_at, id) < ($5::timestamptz, $6::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $7`, businessID, catalogID, status, search, updatedAt, id, limit+1)
	if err != nil {
		return ports.CatalogItemPage{}, catalogRepositoryError("catalog_item.list", err)
	}
	defer rows.Close()
	items := make([]ports.CatalogItemRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanCatalogItem(rows)
		if scanErr != nil {
			return ports.CatalogItemPage{}, catalogRepositoryError("catalog_item.list", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.CatalogItemPage{}, catalogRepositoryError("catalog_item.list", err)
	}
	page := ports.CatalogItemPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeCatalogItemCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CatalogRepository) GetCatalogItem(ctx context.Context, businessID, catalogID, itemID string) (ports.CatalogItemRecord, error) {
	executor, err := r.catalogExecutor(ctx, "catalog_item.get")
	if err != nil {
		return ports.CatalogItemRecord{}, err
	}
	if businessID == "" || catalogID == "" || itemID == "" {
		return ports.CatalogItemRecord{}, invalidRepositoryInput("catalog_item.get", "business, catalog, and item ids are required")
	}
	var item ports.CatalogItemRecord
	if err := executor.QueryRow(ctx, `SELECT id::text, business_id::text, catalog_id::text, attribute_schema_id::text, attribute_schema_version, item_type, name, status, attributes, created_at, updated_at FROM catalog_items WHERE business_id = $1::uuid AND catalog_id = $2::uuid AND id = $3::uuid`, businessID, catalogID, itemID).Scan(&item.ID, &item.BusinessID, &item.CatalogID, &item.AttributeSchemaID, &item.AttributeSchemaVersion, &item.ItemType, &item.Name, &item.Status, &item.Attributes, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return item, classifyRepositoryGetError("catalog_item.get", err)
	}
	return item, nil
}

func (r *CatalogRepository) ListOffers(ctx context.Context, businessID, itemID, status string, limit int, cursor string) (ports.OfferPage, error) {
	executor, err := r.catalogExecutor(ctx, "offer.list")
	if err != nil {
		return ports.OfferPage{}, err
	}
	if err := r.ensureScopedParent(ctx, executor, "offer.list", `SELECT EXISTS (SELECT 1 FROM catalog_items WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, itemID); err != nil {
		return ports.OfferPage{}, err
	}
	limit, decoded, err := catalogPageArgs(limit, cursor, "offer.list")
	if err != nil {
		return ports.OfferPage{}, err
	}
	var updatedAt any
	var id any
	if decoded != nil {
		updatedAt, id = decoded.UpdatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, catalog_item_id::text, variant_id::text, name, pricing_mode, amount::text, currency, availability_status, status, created_at, updated_at FROM offers WHERE business_id = $1::uuid AND catalog_item_id = $2::uuid AND ($3 = '' OR status = $3) AND ($4::timestamptz IS NULL OR (updated_at, id) < ($4::timestamptz, $5::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $6`, businessID, itemID, status, updatedAt, id, limit+1)
	if err != nil {
		return ports.OfferPage{}, catalogRepositoryError("offer.list", err)
	}
	defer rows.Close()
	items := make([]ports.OfferRecord, 0, limit)
	for rows.Next() {
		var item ports.OfferRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CatalogItemID, &item.VariantID, &item.Name, &item.PricingMode, &item.Amount, &item.Currency, &item.AvailabilityStatus, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ports.OfferPage{}, catalogRepositoryError("offer.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.OfferPage{}, catalogRepositoryError("offer.list", err)
	}
	page := ports.OfferPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeOfferCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CatalogRepository) ListVariants(ctx context.Context, businessID, itemID, status string, limit int, cursor string) (ports.VariantPage, error) {
	executor, err := r.catalogExecutor(ctx, "variant.list")
	if err != nil {
		return ports.VariantPage{}, err
	}
	if err := r.ensureScopedParent(ctx, executor, "variant.list", `SELECT EXISTS (SELECT 1 FROM catalog_items WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, itemID); err != nil {
		return ports.VariantPage{}, err
	}
	limit, decoded, err := catalogPageArgs(limit, cursor, "variant.list")
	if err != nil {
		return ports.VariantPage{}, err
	}
	var updatedAt any
	var id any
	if decoded != nil {
		updatedAt, id = decoded.UpdatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, catalog_item_id::text, name, attributes, status, created_at, updated_at FROM variants WHERE business_id = $1::uuid AND catalog_item_id = $2::uuid AND ($3 = '' OR status = $3) AND ($4::timestamptz IS NULL OR (updated_at, id) < ($4::timestamptz, $5::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $6`, businessID, itemID, status, updatedAt, id, limit+1)
	if err != nil {
		return ports.VariantPage{}, catalogRepositoryError("variant.list", err)
	}
	defer rows.Close()
	items := make([]ports.VariantRecord, 0, limit)
	for rows.Next() {
		var item ports.VariantRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CatalogItemID, &item.Name, &item.Attributes, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ports.VariantPage{}, catalogRepositoryError("variant.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.VariantPage{}, catalogRepositoryError("variant.list", err)
	}
	page := ports.VariantPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeVariantCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CatalogRepository) ListAttributeSchemas(ctx context.Context, businessID, name string, version *int, limit int, cursor string) (ports.AttributeSchemaPage, error) {
	executor, err := r.catalogExecutor(ctx, "attribute_schema.list")
	if err != nil {
		return ports.AttributeSchemaPage{}, err
	}
	limit, decoded, err := schemaPageArgs(limit, cursor, "attribute_schema.list")
	if err != nil {
		return ports.AttributeSchemaPage{}, err
	}
	var versionValue any
	if version != nil {
		versionValue = *version
	}
	var cursorVersion any
	var cursorID any
	if decoded != nil {
		cursorVersion, cursorID = decoded.Version, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, name, version FROM attribute_schemas WHERE business_id = $1::uuid AND ($2 = '' OR name = $2) AND ($3::int IS NULL OR version = $3) AND ($4::int IS NULL OR (version, id) < ($4::int, $5::uuid)) ORDER BY version DESC, id DESC LIMIT $6`, businessID, name, versionValue, cursorVersion, cursorID, limit+1)
	if err != nil {
		return ports.AttributeSchemaPage{}, catalogRepositoryError("attribute_schema.list", err)
	}
	defer rows.Close()
	items := make([]ports.AttributeSchemaRecord, 0, limit)
	for rows.Next() {
		var item ports.AttributeSchemaRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.Name, &item.Version); err != nil {
			return ports.AttributeSchemaPage{}, catalogRepositoryError("attribute_schema.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.AttributeSchemaPage{}, catalogRepositoryError("attribute_schema.list", err)
	}
	page := ports.AttributeSchemaPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeSchemaCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CatalogRepository) GetAttributeSchema(ctx context.Context, businessID, schemaID string) (ports.AttributeSchemaRecord, error) {
	executor, err := r.catalogExecutor(ctx, "attribute_schema.get")
	if err != nil {
		return ports.AttributeSchemaRecord{}, err
	}
	if businessID == "" || schemaID == "" {
		return ports.AttributeSchemaRecord{}, invalidRepositoryInput("attribute_schema.get", "business and schema ids are required")
	}
	var item ports.AttributeSchemaRecord
	if err := executor.QueryRow(ctx, `SELECT id::text, business_id::text, name, version FROM attribute_schemas WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, schemaID).Scan(&item.ID, &item.BusinessID, &item.Name, &item.Version); err != nil {
		return item, classifyRepositoryGetError("attribute_schema.get", err)
	}
	rows, err := executor.Query(ctx, `SELECT id::text, attribute_key, label, data_type, is_required, is_searchable, display_order FROM attribute_definitions WHERE schema_id = $1::uuid ORDER BY display_order ASC, id ASC`, schemaID)
	if err != nil {
		return ports.AttributeSchemaRecord{}, catalogRepositoryError("attribute_schema.get", err)
	}
	defer rows.Close()
	item.Definitions = make([]ports.AttributeDefinitionRecord, 0)
	for rows.Next() {
		var definition ports.AttributeDefinitionRecord
		if err := rows.Scan(&definition.ID, &definition.Key, &definition.Label, &definition.DataType, &definition.Required, &definition.Searchable, &definition.DisplayOrder); err != nil {
			return ports.AttributeSchemaRecord{}, catalogRepositoryError("attribute_schema.get", err)
		}
		item.Definitions = append(item.Definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return ports.AttributeSchemaRecord{}, catalogRepositoryError("attribute_schema.get", err)
	}
	return item, nil
}

func (r *CatalogRepository) catalogExecutor(ctx context.Context, operation string) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	return executor, nil
}

func (r *CatalogRepository) ensureScopedParent(ctx context.Context, executor SQLExecutor, operation, query, businessID, parentID string) error {
	if businessID == "" || parentID == "" {
		return invalidRepositoryInput(operation, "business and parent ids are required")
	}
	var exists bool
	if err := executor.QueryRow(ctx, query, businessID, parentID).Scan(&exists); err != nil {
		return classifyRepositoryGetError(operation, err)
	}
	if !exists {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
	}
	return nil
}

func catalogPageArgs(limit int, cursor, operation string) (int, *catalogCursor, error) {
	limit, err := normalizeCatalogLimit(limit, operation)
	if err != nil {
		return 0, nil, err
	}
	decoded, err := decodeCatalogCursor(cursor)
	if err != nil {
		return 0, nil, invalidRepositoryInput(operation, err.Error())
	}
	return limit, decoded, nil
}

func schemaPageArgs(limit int, cursor, operation string) (int, *schemaCursor, error) {
	limit, err := normalizeCatalogLimit(limit, operation)
	if err != nil {
		return 0, nil, err
	}
	decoded, err := decodeSchemaCursor(cursor)
	if err != nil {
		return 0, nil, invalidRepositoryInput(operation, err.Error())
	}
	return limit, decoded, nil
}

func normalizeCatalogLimit(limit int, operation string) (int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return 0, invalidRepositoryInput(operation, "limit must not exceed 100")
	}
	return limit, nil
}

func encodeCatalogCursor(item ports.CatalogRecord) string {
	return encodeCursorParts(item.UpdatedAt, item.ID)
}
func encodeCatalogItemCursor(item ports.CatalogItemRecord) string {
	return encodeCursorParts(item.UpdatedAt, item.ID)
}
func encodeOfferCursor(item ports.OfferRecord) string {
	return encodeCursorParts(item.UpdatedAt, item.ID)
}
func encodeVariantCursor(item ports.VariantRecord) string {
	return encodeCursorParts(item.UpdatedAt, item.ID)
}
func encodeCursorParts(updatedAt time.Time, id string) string {
	raw := updatedAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func encodeSchemaCursor(item ports.AttributeSchemaRecord) string {
	raw := strconv.Itoa(item.Version) + "|" + item.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCatalogCursor(value string) (*catalogCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid cursor encoding")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 || parts[1] == "" {
		return nil, errors.New("invalid cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid cursor updated_at")
	}
	return &catalogCursor{UpdatedAt: updatedAt, ID: parts[1]}, nil
}

func decodeSchemaCursor(value string) (*schemaCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid cursor encoding")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 || parts[1] == "" {
		return nil, errors.New("invalid cursor")
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil || version <= 0 {
		return nil, errors.New("invalid cursor version")
	}
	return &schemaCursor{Version: version, ID: parts[1]}, nil
}

func scanCatalogItem(row interface{ Scan(...any) error }) (ports.CatalogItemRecord, error) {
	var item ports.CatalogItemRecord
	err := row.Scan(&item.ID, &item.BusinessID, &item.CatalogID, &item.AttributeSchemaID, &item.AttributeSchemaVersion, &item.ItemType, &item.Name, &item.Status, &item.Attributes, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func catalogRepositoryError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: fmt.Errorf("%s: %w", operation, err)}
}

var _ ports.CatalogRepository = (*CatalogRepository)(nil)
