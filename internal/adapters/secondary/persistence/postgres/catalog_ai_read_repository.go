package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogAIReadRepository is the read-only, tenant-scoped catalog reader used
// by AI. It bulk-loads nested records to avoid per-item N+1 queries.
type CatalogAIReadRepository struct {
	adapter *Adapter
}

func NewCatalogAIReadRepository(adapter *Adapter) *CatalogAIReadRepository {
	return &CatalogAIReadRepository{adapter: adapter}
}

func (r *CatalogAIReadRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func (r *CatalogAIReadRepository) GetManifest(ctx context.Context, businessID string) (ports.CatalogAIManifest, error) {
	if strings.TrimSpace(businessID) == "" {
		return ports.CatalogAIManifest{}, invalidRepositoryInput("catalog_ai.manifest", "business id is required")
	}
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.CatalogAIManifest{}, err
	}

	var manifest ports.CatalogAIManifest
	rows, err := executor.Query(ctx, `
		SELECT c.id::text, c.name, c.description, COUNT(ci.id)::int,
		       COALESCE(array_agg(DISTINCT ci.item_type ORDER BY ci.item_type)
		         FILTER (WHERE ci.id IS NOT NULL), ARRAY[]::text[])
		FROM catalogs c
		LEFT JOIN catalog_items ci
		  ON ci.business_id = c.business_id
		 AND ci.catalog_id = c.id
		 AND ci.status = 'active'
		WHERE c.business_id = $1::uuid AND c.status = 'active'
		GROUP BY c.id, c.name, c.description
		ORDER BY c.name, c.id`, businessID)
	if err != nil {
		return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest", err)
	}
	for rows.Next() {
		var c ports.CatalogAIManifestCatalog
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.ItemCount, &c.ItemTypes); err != nil {
			rows.Close()
			return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest", err)
		}
		manifest.TotalActiveItems += c.ItemCount
		manifest.Catalogs = append(manifest.Catalogs, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest", err)
	}
	rows.Close()

	sRows, err := executor.Query(ctx, `
		SELECT s.id::text, s.name, s.version, COUNT(ci.id)::int
		FROM attribute_schemas s
		JOIN catalog_items ci
		  ON ci.business_id = s.business_id
		 AND ci.attribute_schema_id = s.id
		 AND ci.status = 'active'
		WHERE s.business_id = $1::uuid
		GROUP BY s.id, s.name, s.version
		ORDER BY s.name, s.version, s.id`, businessID)
	if err != nil {
		return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest.schemas", err)
	}
	schemaIndex := map[string]int{}
	var schemaIDs []string
	for sRows.Next() {
		var s ports.CatalogAIManifestSchema
		if err := sRows.Scan(&s.ID, &s.Name, &s.Version, &s.UsageCount); err != nil {
			sRows.Close()
			return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest.schemas", err)
		}
		schemaIndex[s.ID] = len(manifest.Schemas)
		schemaIDs = append(schemaIDs, s.ID)
		manifest.Schemas = append(manifest.Schemas, s)
	}
	if err := sRows.Err(); err != nil {
		sRows.Close()
		return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest.schemas", err)
	}
	sRows.Close()
	if len(schemaIDs) == 0 {
		return manifest, nil
	}

	dRows, err := executor.Query(ctx, `
		SELECT d.schema_id::text, d.attribute_key, d.label, d.data_type,
		       d.is_required, d.is_searchable, d.validation_rules, d.display_order
		FROM attribute_definitions d
		JOIN attribute_schemas s ON s.id = d.schema_id
		WHERE s.business_id = $1::uuid AND d.schema_id = ANY($2::uuid[])
		ORDER BY d.schema_id, d.display_order, d.id`, businessID, schemaIDs)
	if err != nil {
		return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest.definitions", err)
	}
	for dRows.Next() {
		var schemaID string
		var d ports.CatalogAIManifestAttributeDefinition
		var rules []byte
		if err := dRows.Scan(&schemaID, &d.Key, &d.Label, &d.DataType, &d.Required, &d.Searchable, &rules, &d.DisplayOrder); err != nil {
			dRows.Close()
			return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest.definitions", err)
		}
		if len(rules) > 0 {
			_ = json.Unmarshal(rules, &d.ValidationRules)
		}
		if idx, ok := schemaIndex[schemaID]; ok {
			manifest.Schemas[idx].Definitions = append(manifest.Schemas[idx].Definitions, d)
		}
	}
	if err := dRows.Err(); err != nil {
		dRows.Close()
		return ports.CatalogAIManifest{}, catalogRepositoryError("catalog_ai.manifest.definitions", err)
	}
	dRows.Close()
	return manifest, nil
}

func (r *CatalogAIReadRepository) SearchProjection(ctx context.Context, request ports.CatalogAISearchRequest) ([]ports.CatalogAIProjectionBundle, error) {
	if strings.TrimSpace(request.BusinessID) == "" {
		return nil, invalidRepositoryInput("catalog_ai.search", "business id is required")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, nil
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	executor, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, `
		WITH q AS (SELECT websearch_to_tsquery('simple'::regconfig, $2) query),
		hits AS (
		  SELECT ci.id item_id, ts_rank_cd(ci.ai_search_document, q.query) score
		  FROM catalog_items ci, q
		  WHERE ci.business_id = $1::uuid AND ci.status = 'active'
		    AND ci.ai_search_document @@ q.query
		  UNION ALL
		  SELECT v.catalog_item_id, ts_rank_cd(v.ai_search_document, q.query)
		  FROM variants v
		  JOIN catalog_items ci ON ci.business_id=v.business_id AND ci.id=v.catalog_item_id AND ci.status='active'
		  CROSS JOIN q
		  WHERE v.business_id=$1::uuid AND v.status='active' AND v.ai_search_document @@ q.query
		  UNION ALL
		  SELECT o.catalog_item_id, ts_rank_cd(o.ai_search_document, q.query)
		  FROM offers o
		  JOIN catalog_items ci ON ci.business_id=o.business_id AND ci.id=o.catalog_item_id AND ci.status='active'
		  CROSS JOIN q
		  WHERE o.business_id=$1::uuid AND o.status='active' AND o.ai_search_document @@ q.query
		)
		SELECT item_id::text
		FROM hits
		GROUP BY item_id
		ORDER BY MAX(score) DESC, item_id
		LIMIT $3`, request.BusinessID, query, limit)
	if err != nil {
		return nil, catalogRepositoryError("catalog_ai.search", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, catalogRepositoryError("catalog_ai.search", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, catalogRepositoryError("catalog_ai.search", err)
	}
	rows.Close()
	return r.loadBundles(ctx, executor, request.BusinessID, ids)
}

func (r *CatalogAIReadRepository) ListProjectionPage(ctx context.Context, request ports.CatalogAIProjectionRequest) (ports.CatalogAIProjectionPage, error) {
	if strings.TrimSpace(request.BusinessID) == "" {
		return ports.CatalogAIProjectionPage{}, invalidRepositoryInput("catalog_ai.page", "business id is required")
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.CatalogAIProjectionPage{}, err
	}
	rows, err := executor.Query(ctx, `
		SELECT id::text, business_id::text, catalog_id::text,
		       attribute_schema_id::text, attribute_schema_version,
		       item_type, name, short_description, long_description,
		       status, pricing_mode, availability_mode, fulfillment_mode,
		       requires_confirmation, attributes, resource_version, created_at, updated_at
		FROM catalog_items
		WHERE business_id=$1::uuid AND status='active'
		  AND (NULLIF($2,'')::uuid IS NULL OR catalog_id=NULLIF($2,'')::uuid)
		  AND (NULLIF($3,'')::uuid IS NULL OR id>NULLIF($3,'')::uuid)
		ORDER BY id
		LIMIT $4`, request.BusinessID, request.CatalogID, request.Cursor, limit+1)
	if err != nil {
		return ports.CatalogAIProjectionPage{}, catalogRepositoryError("catalog_ai.page", err)
	}
	var items []ports.CatalogItemRecord
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			rows.Close()
			return ports.CatalogAIProjectionPage{}, catalogRepositoryError("catalog_ai.page", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ports.CatalogAIProjectionPage{}, catalogRepositoryError("catalog_ai.page", err)
	}
	rows.Close()

	page := ports.CatalogAIProjectionPage{}
	if len(items) > limit {
		page.HasMore = true
		items = items[:limit]
	}
	if len(items) == 0 {
		return page, nil
	}
	if page.HasMore {
		page.NextCursor = items[len(items)-1].ID
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	bundles, err := r.hydrate(ctx, executor, request.BusinessID, items, ids)
	if err != nil {
		return ports.CatalogAIProjectionPage{}, err
	}
	page.Items = bundles
	return page, nil
}

func (r *CatalogAIReadRepository) loadBundles(ctx context.Context, executor SQLExecutor, businessID string, ids []string) ([]ports.CatalogAIProjectionBundle, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := executor.Query(ctx, `
		SELECT id::text, business_id::text, catalog_id::text,
		       attribute_schema_id::text, attribute_schema_version,
		       item_type, name, short_description, long_description,
		       status, pricing_mode, availability_mode, fulfillment_mode,
		       requires_confirmation, attributes, resource_version, created_at, updated_at
		FROM catalog_items
		WHERE business_id=$1::uuid AND status='active' AND id=ANY($2::uuid[])`, businessID, ids)
	if err != nil {
		return nil, catalogRepositoryError("catalog_ai.load", err)
	}
	byID := map[string]ports.CatalogItemRecord{}
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			rows.Close()
			return nil, catalogRepositoryError("catalog_ai.load", err)
		}
		byID[item.ID] = item
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, catalogRepositoryError("catalog_ai.load", err)
	}
	rows.Close()
	items := make([]ports.CatalogItemRecord, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			items = append(items, item)
		}
	}
	return r.hydrate(ctx, executor, businessID, items, ids)
}

func (r *CatalogAIReadRepository) hydrate(ctx context.Context, executor SQLExecutor, businessID string, items []ports.CatalogItemRecord, ids []string) ([]ports.CatalogAIProjectionBundle, error) {
	bundles := make([]ports.CatalogAIProjectionBundle, len(items))
	index := map[string]int{}
	var schemaIDs []string
	seenSchema := map[string]bool{}
	for i, item := range items {
		bundles[i].Item = item
		index[item.ID] = i
		if item.AttributeSchemaID != nil && !seenSchema[*item.AttributeSchemaID] {
			seenSchema[*item.AttributeSchemaID] = true
			schemaIDs = append(schemaIDs, *item.AttributeSchemaID)
		}
	}

	vRows, err := executor.Query(ctx, `
		SELECT id::text,business_id::text,catalog_item_id::text,name,attributes,status,resource_version,created_at,updated_at
		FROM variants
		WHERE business_id=$1::uuid AND catalog_item_id=ANY($2::uuid[]) AND status='active'
		ORDER BY catalog_item_id,id`, businessID, ids)
	if err != nil {
		return nil, catalogRepositoryError("catalog_ai.variants", err)
	}
	for vRows.Next() {
		var v ports.VariantRecord
		if err := vRows.Scan(&v.ID,&v.BusinessID,&v.CatalogItemID,&v.Name,&v.Attributes,&v.Status,&v.ResourceVersion,&v.CreatedAt,&v.UpdatedAt); err != nil {
			vRows.Close()
			return nil, catalogRepositoryError("catalog_ai.variants", err)
		}
		if i, ok := index[v.CatalogItemID]; ok {
			bundles[i].Variants = append(bundles[i].Variants, v)
		}
	}
	if err := vRows.Err(); err != nil {
		vRows.Close()
		return nil, catalogRepositoryError("catalog_ai.variants", err)
	}
	vRows.Close()

	oRows, err := executor.Query(ctx, `
		SELECT id::text,business_id::text,catalog_item_id::text,variant_id::text,name,pricing_mode,amount::text,currency,
		       pricing_unit,price_source,price_verification_status,price_checked_at,availability_mode,availability_source,
		       availability_checked_at,availability_valid_until,availability_evidence_ref,fulfillment_mode,validity_from,
		       validity_until,availability_status,status,resource_version,created_at,updated_at
		FROM offers
		WHERE business_id=$1::uuid AND catalog_item_id=ANY($2::uuid[]) AND status='active'
		ORDER BY catalog_item_id,id`, businessID, ids)
	if err != nil {
		return nil, catalogRepositoryError("catalog_ai.offers", err)
	}
	for oRows.Next() {
		var o ports.OfferRecord
		if err := oRows.Scan(&o.ID,&o.BusinessID,&o.CatalogItemID,&o.VariantID,&o.Name,&o.PricingMode,&o.Amount,&o.Currency,
			&o.PricingUnit,&o.PriceSource,&o.PriceVerificationStatus,&o.PriceCheckedAt,&o.AvailabilityMode,&o.AvailabilitySource,
			&o.AvailabilityCheckedAt,&o.AvailabilityValidUntil,&o.AvailabilityEvidenceRef,&o.FulfillmentMode,&o.ValidityFrom,
			&o.ValidityUntil,&o.AvailabilityStatus,&o.Status,&o.ResourceVersion,&o.CreatedAt,&o.UpdatedAt); err != nil {
			oRows.Close()
			return nil, catalogRepositoryError("catalog_ai.offers", err)
		}
		if i, ok := index[o.CatalogItemID]; ok {
			bundles[i].Offers = append(bundles[i].Offers, o)
		}
	}
	if err := oRows.Err(); err != nil {
		oRows.Close()
		return nil, catalogRepositoryError("catalog_ai.offers", err)
	}
	oRows.Close()

	if len(schemaIDs) == 0 {
		return bundles, nil
	}
	sRows, err := executor.Query(ctx, `
		SELECT id::text,business_id::text,name,version
		FROM attribute_schemas
		WHERE business_id=$1::uuid AND id=ANY($2::uuid[])`, businessID, schemaIDs)
	if err != nil {
		return nil, catalogRepositoryError("catalog_ai.schemas", err)
	}
	schemas := map[string]*ports.AttributeSchemaRecord{}
	for sRows.Next() {
		var s ports.AttributeSchemaRecord
		if err := sRows.Scan(&s.ID,&s.BusinessID,&s.Name,&s.Version); err != nil {
			sRows.Close()
			return nil, catalogRepositoryError("catalog_ai.schemas", err)
		}
		copy := s
		schemas[s.ID] = &copy
	}
	if err := sRows.Err(); err != nil {
		sRows.Close()
		return nil, catalogRepositoryError("catalog_ai.schemas", err)
	}
	sRows.Close()

	dRows, err := executor.Query(ctx, `
		SELECT d.schema_id::text,d.id::text,d.attribute_key,d.label,d.data_type,d.is_required,d.is_searchable,d.validation_rules,d.display_order
		FROM attribute_definitions d
		JOIN attribute_schemas s ON s.id=d.schema_id
		WHERE s.business_id=$1::uuid AND d.schema_id=ANY($2::uuid[])
		ORDER BY d.schema_id,d.display_order,d.id`, businessID, schemaIDs)
	if err != nil {
		return nil, catalogRepositoryError("catalog_ai.definitions", err)
	}
	for dRows.Next() {
		var schemaID string
		var d ports.AttributeDefinitionRecord
		if err := dRows.Scan(&schemaID,&d.ID,&d.Key,&d.Label,&d.DataType,&d.Required,&d.Searchable,&d.ValidationRules,&d.DisplayOrder); err != nil {
			dRows.Close()
			return nil, catalogRepositoryError("catalog_ai.definitions", err)
		}
		if s := schemas[schemaID]; s != nil {
			s.Definitions = append(s.Definitions, d)
		}
	}
	if err := dRows.Err(); err != nil {
		dRows.Close()
		return nil, catalogRepositoryError("catalog_ai.definitions", err)
	}
	dRows.Close()
	for i := range bundles {
		if bundles[i].Item.AttributeSchemaID == nil {
			continue
		}
		if s := schemas[*bundles[i].Item.AttributeSchemaID]; s != nil {
			copy := *s
			copy.Definitions = append([]ports.AttributeDefinitionRecord(nil), s.Definitions...)
			bundles[i].AttributeSchema = &copy
		}
	}
	return bundles, nil
}

var _ ports.CatalogAIReadRepository = (*CatalogAIReadRepository)(nil)
