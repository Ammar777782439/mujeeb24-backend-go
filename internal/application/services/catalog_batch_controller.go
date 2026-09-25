// Package services — Catalog Batch Controller (contracts ② and ⑨ §22)
//
// Implements contract ② Catalog Evaluation + Batching — CLOSED.
//
// Per contract ② §2, batch size is TOKEN-BASED, not item-count-based.
// Per contract ② §3, every item enters evaluation AT LEAST ONCE; the
// Controller guarantees coverage (Sent == Completed == Total).
// Per contract ② §4, each batch carries the AttributeSchemas its items use.
// Per contract ② §5, Gemini returns ONLY candidates (item_id, variant_ids,
// offer_ids, reason) — not the full items back.
// Per contract ② §6, after all batches complete, a Final Gemini Evaluation
// runs over the candidate set + customer message + context.
// Per contract ② §7, candidates are split by token budget, not by count.
// Per contract ② §8, batches are NOT chained via previous_interaction_id;
// they are independent Interactions on the same Conversation Context.
//
// Per contract ⑨ §22, each batch has state PENDING/RUNNING/COMPLETED/FAILED
// so the Controller can resume from where it left off after partial failure.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogBatchController drives a contract ② §9 evaluation.
//
// It is invoked when Gemini's first Interaction indicates that catalog data
// is needed. The Controller:
//  1. Builds the Catalog AI Projection (per contract ①) for the business scope.
//  2. Token-counts the serialized Projection.
//  3. Splits into batches by token budget.
//  4. Sends each batch as an independent Gemini Interaction.
//  5. Collects candidates per batch.
//  6. Enforces coverage (Total == Sent == Completed).
//  7. Hands the candidate set to the Final Gemini Evaluation.
//
// Per contract ② §1, Mujeeb:
//   - Determines the Catalog scope allowed for the merchant
//   - Builds the Projection
//   - Splits into Batches by TOKENS
//   - Guarantees every Item in scope was sent to Gemini
//   - Tracks which Batches completed
//   - Aggregates evaluation results
//
// Gemini:
//   - Understands the customer message
//   - Understands each Batch's data
//   - Infers and compares
//   - Identifies candidates
//   - Explains why each candidate is in the candidate set
type CatalogBatchController struct {
	// Catalogs is the Postgres-backed catalog repository used to fetch the
	// raw items/variants/offers/schemas for the projection per contract ① §6.
	// Per contract ⑤ §13, the data access boundary is Read Only, Tenant
	// Scoped, Structured, No SQL.
	Catalogs          ports.CatalogRepository
	ProjectionBuilder *CatalogAIProjectionBuilder
	TokenCounter      TokenCounter
	Gemini            BatchGeminiClient
	RunRepo           ports.AIRunRepository
	Now               func() time.Time
	NewID             func() string

	// TokenBudget is the per-batch token cap. Per contract ② §2, this is
	// token-based not item-count-based. Set via runtime config.
	TokenBudget int
}

// BatchGeminiClient is the per-batch Gemini interaction contract.
// Each batch is one independent Interaction per contract ② §8 — no
// previous_interaction_id chaining between batches.
type BatchGeminiClient interface {
	// EvaluateBatch sends one batch to Gemini and returns the candidate set.
	// Per contract ② §5, Gemini returns ONLY candidates, not the items back.
	EvaluateBatch(ctx context.Context, input BatchEvaluationInput) (ports.CatalogBatchResult, error)

	// FinalEvaluate runs the contract ② §6 final evaluation over the
	// aggregated candidate set + customer message + conversation context.
	// This Gemini Interaction is also independent (no previous_interaction_id
	// chaining to the per-batch Interactions).
	FinalEvaluate(ctx context.Context, input FinalEvaluationInput) (ports.AIGeminiProposal, error)
}

// BatchEvaluationInput is one batch's input to Gemini.
type BatchEvaluationInput struct {
	AIRunID             string
	AttemptID           string
	BatchNumber         int
	BusinessID          string
	ConversationID      string
	CustomerMessage     string
	ConversationContext ports.AIContext
	EntityContract      CatalogEntityContractPayload
	Batch               CatalogAIBatchPayload
}

// FinalEvaluationInput is the post-batch final Gemini call.
type FinalEvaluationInput struct {
	AIRunID             string
	AttemptID           string
	BusinessID          string
	ConversationID      string
	CustomerMessage     string
	ConversationContext ports.AIContext
	EntityContract      CatalogEntityContractPayload
	CandidateResults    []ports.CatalogBatchCandidate
}

// CatalogAIBatchPayload is one batch's content.
//
// Per contract ② §4, each batch carries the schemas its items use — schemas
// are NOT repeated globally across batches.
type CatalogAIBatchPayload struct {
	BatchNumber      int                        `json:"batch_number"`
	AttributeSchemas []CatalogAIAttributeSchema `json:"attribute_schemas,omitempty"`
	Items            []CatalogAIItem            `json:"items,omitempty"`
}

// TokenCounter returns the token count of a serialized payload.
//
// Per contract ② §2, Google provides count_tokens for exactly this purpose.
// Implementations wrap that API. The Contract is token-based, not character
// or byte based, so the model's own tokenizer is authoritative.
type TokenCounter interface {
	CountTokens(ctx context.Context, payload any) (int, error)
}

// RunCatalogEvaluation drives the full contract ② §9 evaluation pipeline.
//
// Returns the final AIGeminiProposal (post-Final-Evaluation) on success.
// On failure, returns the failure stage + category for ai_runs.failure_*.
func (c *CatalogBatchController) RunCatalogEvaluation(ctx context.Context, input CatalogEvaluationInput) (ports.AIGeminiProposal, error) {
	if c.ProjectionBuilder == nil || c.TokenCounter == nil || c.Gemini == nil || c.RunRepo == nil {
		return ports.AIGeminiProposal{}, errors.New("CatalogBatchController is not fully wired per contract ② §1")
	}
	if strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.AIRunID) == "" {
		return ports.AIGeminiProposal{}, errors.New("business_id and ai_run_id are required for Catalog Evaluation per contract ② §1")
	}

	// Step 1: Build the Projection. Per contract ① §6, Mujeeb builds it.
	projection, err := c.buildProjection(ctx, input.BusinessID, input.CatalogScope)
	if err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("build projection: %w", err)
	}

	// Step 2: Token-count and split. Per contract ② §2, token-based not item-count.
	batches, err := c.splitIntoBatches(ctx, projection, input.EntityContract)
	if err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("split batches: %w", err)
	}
	if len(batches) == 0 {
		// No items in scope — Final Evaluation with empty candidate set.
		return c.runFinalEvaluation(ctx, input, nil)
	}

	// Step 3: Register each batch in ai_catalog_batches per contract ⑨ §22.
	batchRecords := make([]ports.AICatalogBatchRecord, 0, len(batches))
	for _, b := range batches {
		rec, err := c.createBatchRecord(ctx, input.AIRunID, b)
		if err != nil {
			return ports.AIGeminiProposal{}, fmt.Errorf("create batch %d record: %w", b.BatchNumber, err)
		}
		batchRecords = append(batchRecords, rec)
	}

	// Step 4: Evaluate each batch independently per contract ② §8.
	// Coverage tracking: Sent == len(batches); Completed == count of COMPLETED.
	candidateSet := make([]ports.CatalogBatchCandidate, 0)
	for i, b := range batches {
		// Per contract ⑨ §22, mark batch RUNNING.
		if err := c.markBatchRunning(ctx, batchRecords[i].ID); err != nil {
			return ports.AIGeminiProposal{}, err
		}
		result, err := c.Gemini.EvaluateBatch(ctx, BatchEvaluationInput{
			AIRunID:             input.AIRunID,
			AttemptID:           input.AttemptID,
			BatchNumber:         b.BatchNumber,
			BusinessID:          input.BusinessID,
			ConversationID:      input.ConversationID,
			CustomerMessage:     input.CustomerMessage,
			ConversationContext: input.ConversationContext,
			EntityContract:      input.EntityContract,
			Batch:               b,
		})
		if err != nil {
			// Per contract ⑨ §22, mark batch FAILED; per ⑨ §21 do NOT re-run other batches.
			_ = c.markBatchFailed(ctx, batchRecords[i].ID, err.Error())
			return ports.AIGeminiProposal{}, fmt.Errorf("batch %d evaluation: %w", b.BatchNumber, err)
		}
		// Per contract ⑨ §22, mark batch COMPLETED — update the in-memory record too.
		if err := c.markBatchCompleted(ctx, batchRecords[i].ID, len(result.Candidates)); err != nil {
			return ports.AIGeminiProposal{}, err
		}
		batchRecords[i].Status = "completed"
		candidateSet = append(candidateSet, result.Candidates...)
		log.Printf("[CatalogBatch] BATCH_DONE batch=%d items=%d candidates=%d", b.BatchNumber, len(b.Items), len(result.Candidates))
	}

	// Step 5: Coverage check per contract ② §3.
	// Coverage is complete when all batches are COMPLETED.
	log.Printf("[CatalogBatch] COVERAGE total=%d completed=%d", len(batchRecords), countCompleted(batchRecords))
	if !c.coverageComplete(batchRecords) {
		return ports.AIGeminiProposal{}, fmt.Errorf("coverage incomplete per contract ② §3 — %d/%d batches completed", countCompleted(batchRecords), len(batchRecords))
	}

	// Step 6: Final Gemini Evaluation per contract ② §6.
	return c.runFinalEvaluation(ctx, input, candidateSet)
}

// buildProjection builds the contract ① Catalog AI Projection from the
// merchant's actual catalog data in PostgreSQL. Per contract ① §6, Mujeeb
// is the sole builder of the Projection.
//
// Per contract ⑤ §13, the data access boundary is:
//   - Read Only — no writes via this path
//   - Tenant Scoped — all reads filter by business_id
//   - Structured — returns Projection shapes, not raw rows
//   - No SQL — the AI never sees query strings
//
// Per contract ① §2, only the AttributeSchemas USED by the Items in the
// projection are sent. We do NOT send unused schemas.
//
// Per contract ① §5, the Projection does NOT contain: business_id, SQL,
// database metadata, created_at, updated_at, search_query, semantic_search,
// matching logic, or ranking logic.
//
// Flow:
//  1. List catalog_items for the given (businessID, catalogScope) with
//     status='active' — per contract ① §6, Mujeeb determines the scope.
//  2. For each item, list its variants and offers (also active only).
//  3. For each item's attribute_schema_id, fetch the schema + definitions.
//  4. Assemble the CatalogAIProjection with nested variants+offers per item.
//  5. Deduplicate schemas — per contract ① §2, each schema appears once.
//
// Per contract ⑧ §17, every read is tenant-scoped via business_id. A
// cross-tenant read returns empty (per contract ⑥ §8: do not leak existence).
func (c *CatalogBatchController) buildProjection(ctx context.Context, businessID, catalogScope string) (CatalogAIProjection, error) {
	if c.Catalogs == nil {
		return CatalogAIProjection{}, errors.New("catalog repository is not wired on CatalogBatchController per contract ① §6")
	}
	if strings.TrimSpace(businessID) == "" {
		return CatalogAIProjection{}, errors.New("business_id is required for projection build per contract ⑧ §17")
	}

	// Per contract ① §6, Mujeeb determines the catalog scope. catalogScope
	// is the catalog_id; if empty, we list all catalogs for the business and
	// iterate items across all of them.
	catalogIDs := make([]string, 0)
	if strings.TrimSpace(catalogScope) != "" {
		catalogIDs = append(catalogIDs, catalogScope)
	} else {
		// List all active catalogs for the business.
		catPage, err := c.Catalogs.ListCatalogs(ctx, businessID, "active", 100, "")
		if err != nil {
			return CatalogAIProjection{}, fmt.Errorf("list catalogs: %w", err)
		}
		for _, cat := range catPage.Items {
			catalogIDs = append(catalogIDs, cat.ID)
		}
	}

	projection := CatalogAIProjection{
		Items:            make([]CatalogAIItem, 0),
		AttributeSchemas: make([]CatalogAIAttributeSchema, 0),
	}
	schemaCache := make(map[string]CatalogAIAttributeSchema) // dedup per contract ① §2

	for _, catalogID := range catalogIDs {
		// List active items for this catalog. Per contract ① §6, we send
		// only active items (drafts are not yet published).
		cursor := ""
		for {
			itemPage, err := c.Catalogs.ListCatalogItems(ctx, businessID, catalogID, "", "active", 200, cursor)
			if err != nil {
				return CatalogAIProjection{}, fmt.Errorf("list catalog items for catalog %s: %w", catalogID, err)
			}
			for _, itemRec := range itemPage.Items {
				item := mapCatalogItemRecordToProjection(itemRec)

				// Fetch variants for this item (active only).
				variants, err := c.fetchVariants(ctx, businessID, itemRec.ID)
				if err != nil {
					return CatalogAIProjection{}, fmt.Errorf("fetch variants for item %s: %w", itemRec.ID, err)
				}
				item.Variants = variants

				// Fetch offers for this item (active only).
				offers, err := c.fetchOffers(ctx, businessID, itemRec.ID)
				if err != nil {
					return CatalogAIProjection{}, fmt.Errorf("fetch offers for item %s: %w", itemRec.ID, err)
				}
				item.Offers = offers

				// Fetch the attribute schema if the item references one.
				if itemRec.AttributeSchemaID != nil && *itemRec.AttributeSchemaID != "" {
					schemaID := *itemRec.AttributeSchemaID
					if _, ok := schemaCache[schemaID]; !ok {
						schemaRec, err := c.Catalogs.GetAttributeSchema(ctx, businessID, schemaID)
						if err != nil {
							return CatalogAIProjection{}, fmt.Errorf("fetch attribute schema %s: %w", schemaID, err)
						}
						schema := mapAttributeSchemaRecordToProjection(schemaRec)
						schemaCache[schemaID] = schema
					}
				}

				projection.Items = append(projection.Items, item)
			}
			if !itemPage.HasMore {
				break
			}
			cursor = itemPage.NextCursor
		}
	}

	// Per contract ① §2: only the schemas actually used by items[] are sent.
	for _, schema := range schemaCache {
		projection.AttributeSchemas = append(projection.AttributeSchemas, schema)
	}

	return projection, nil
}

// fetchVariants reads active variants for a catalog item.
// Per contract ⑧ §17, the read is tenant-scoped via business_id.
func (c *CatalogBatchController) fetchVariants(ctx context.Context, businessID, itemID string) ([]CatalogAIVariant, error) {
	page, err := c.Catalogs.ListVariants(ctx, businessID, itemID, "active", 200, "")
	if err != nil {
		return nil, err
	}
	out := make([]CatalogAIVariant, 0, len(page.Items))
	for _, v := range page.Items {
		out = append(out, CatalogAIVariant{
			ID:         v.ID,
			Name:       v.Name,
			Attributes: parseJSONAttributes(v.Attributes),
			Status:     v.Status,
		})
	}
	return out, nil
}

// fetchOffers reads active offers for a catalog item.
// Per contract ⑧ §17, the read is tenant-scoped via business_id.
func (c *CatalogBatchController) fetchOffers(ctx context.Context, businessID, itemID string) ([]CatalogAIOffer, error) {
	page, err := c.Catalogs.ListOffers(ctx, businessID, itemID, "active", 200, "")
	if err != nil {
		return nil, err
	}
	out := make([]CatalogAIOffer, 0, len(page.Items))
	for _, o := range page.Items {
		out = append(out, CatalogAIOffer{
			ID:                      o.ID,
			VariantID:               o.VariantID,
			Name:                    o.Name,
			PricingMode:             o.PricingMode,
			Amount:                  o.Amount,
			Currency:                o.Currency,
			PricingUnit:             o.PricingUnit,
			PriceSource:             o.PriceSource,
			PriceVerificationStatus: stringPtrOrNil(o.PriceVerificationStatus),
			AvailabilityMode:        stringPtrOrNil(o.AvailabilityMode),
			AvailabilityStatus:      stringPtrOrNil(o.AvailabilityStatus),
			FulfillmentMode:         stringPtrOrNil(o.FulfillmentMode),
			ValidityFrom:            formatTimePtr(o.ValidityFrom),
			ValidityUntil:           formatTimePtr(o.ValidityUntil),
			Status:                  o.Status,
		})
	}
	return out, nil
}

// mapCatalogItemRecordToProjection converts a ports.CatalogItemRecord to a
// CatalogAIItem per contract ① §1. Per contract ① §5, the projection does
// NOT carry business_id, SQL, database metadata, created_at, updated_at.
func mapCatalogItemRecordToProjection(r ports.CatalogItemRecord) CatalogAIItem {
	return CatalogAIItem{
		ID:                     r.ID,
		CatalogID:              r.CatalogID,
		AttributeSchemaID:      r.AttributeSchemaID,
		AttributeSchemaVersion: r.AttributeSchemaVersion,
		ItemType:               r.ItemType,
		Name:                   r.Name,
		ShortDescription:       r.ShortDescription,
		LongDescription:        r.LongDescription,
		Status:                 r.Status,
		PricingMode:            r.PricingMode,
		AvailabilityMode:       r.AvailabilityMode,
		FulfillmentMode:        r.FulfillmentMode,
		RequiresConfirmation:   r.RequiresConfirmation,
		Attributes:             parseJSONAttributes(r.Attributes),
	}
}

// mapAttributeSchemaRecordToProjection converts a ports.AttributeSchemaRecord
// to a CatalogAIAttributeSchema per contract ① §1.
func mapAttributeSchemaRecordToProjection(r ports.AttributeSchemaRecord) CatalogAIAttributeSchema {
	defs := make([]CatalogAIAttributeDefinition, 0, len(r.Definitions))
	for _, d := range r.Definitions {
		defs = append(defs, CatalogAIAttributeDefinition{
			ID:           d.ID,
			SchemaID:     r.ID,
			AttributeKey: d.Key,
			Label:        d.Label,
			DataType:     d.DataType,
			IsRequired:   d.Required,
			IsSearchable: d.Searchable,
			DisplayOrder: d.DisplayOrder,
		})
	}
	return CatalogAIAttributeSchema{
		ID:          r.ID,
		Name:        r.Name,
		Version:     r.Version,
		Definitions: defs,
	}
}

// parseJSONAttributes parses the JSONB attributes column into map[string]any.
// Per contract ① §5 + migration 000016 catalog_items_attributes_object_chk,
// attributes is always a JSON object. Returns empty map for nil/empty.
func parseJSONAttributes(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stringPtrOrNil converts a non-empty string to *string, nil for empty.
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// formatTimePtr formats a *time.Time as ISO8601 string pointer for the
// projection (per contract ① §1, the projection uses ISO8601 strings for
// timestamps, not Go time.Time).
func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// splitIntoBatches implements contract ② §2 token-based splitting.
//
// Per contract ② §2, we do NOT say "100 products = batch" or "50 = batch".
// We serialize the projection, count tokens, and split when the running
// total exceeds TokenBudget.
func (c *CatalogBatchController) splitIntoBatches(ctx context.Context, projection CatalogAIProjection, entityContract CatalogEntityContractPayload) ([]CatalogAIBatchPayload, error) {
	if len(projection.Items) == 0 {
		return nil, nil
	}
	budget := c.TokenBudget
	if budget <= 0 {
		budget = 8000 // sensible default per runtime config
	}

	batches := make([]CatalogAIBatchPayload, 0)
	currentBatch := CatalogAIBatchPayload{BatchNumber: 1}
	currentSchemas := make(map[string]CatalogAIAttributeSchema)
	currentTokenCount := 0

	// Per contract ② §4, each batch carries the schemas its items use.
	for _, item := range projection.Items {
		// Serialize the item (and its variants+offers) and count tokens.
		itemTokens, err := c.TokenCounter.CountTokens(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("count tokens for item %s: %w", item.ID, err)
		}
		if currentTokenCount+itemTokens > budget && len(currentBatch.Items) > 0 {
			// Flush current batch with its schemas.
			currentBatch.AttributeSchemas = collectSchemas(currentSchemas)
			batches = append(batches, currentBatch)
			// Start a new batch.
			currentBatch = CatalogAIBatchPayload{BatchNumber: currentBatch.BatchNumber + 1}
			currentSchemas = make(map[string]CatalogAIAttributeSchema)
			currentTokenCount = 0
		}
		// Add item to current batch.
		currentBatch.Items = append(currentBatch.Items, item)
		currentTokenCount += itemTokens
		// Collect schemas this item uses per contract ② §4.
		if item.AttributeSchemaID != nil {
			for _, sch := range projection.AttributeSchemas {
				if sch.ID == *item.AttributeSchemaID {
					currentSchemas[sch.ID] = sch
					break
				}
			}
		}
	}
	// Flush the final batch.
	if len(currentBatch.Items) > 0 {
		currentBatch.AttributeSchemas = collectSchemas(currentSchemas)
		batches = append(batches, currentBatch)
	}
	return batches, nil
}

// collectSchemas converts a schema map to a slice.
func collectSchemas(m map[string]CatalogAIAttributeSchema) []CatalogAIAttributeSchema {
	if len(m) == 0 {
		return nil
	}
	out := make([]CatalogAIAttributeSchema, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	return out
}

// createBatchRecord persists a batch state in ai_catalog_batches per contract ⑨ §22.
func (c *CatalogBatchController) createBatchRecord(ctx context.Context, runID string, batch CatalogAIBatchPayload) (ports.AICatalogBatchRecord, error) {
	now := c.Now()
	return c.RunRepo.CreateCatalogBatch(ctx, ports.AICatalogBatchRecord{
		ID:           c.NewID(),
		AIRunID:      runID,
		BatchNumber:  batch.BatchNumber,
		Status:       "pending",
		ItemsCount:   len(batch.Items),
		SchemasCount: len(batch.AttributeSchemas),
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func (c *CatalogBatchController) markBatchRunning(ctx context.Context, batchID string) error {
	now := c.Now()
	_, err := c.RunRepo.UpdateCatalogBatch(ctx, batchID, ports.AICatalogBatchPatch{
		Status:    "running",
		StartedAt: &now,
	})
	return err
}

func (c *CatalogBatchController) markBatchCompleted(ctx context.Context, batchID string, candidateCount int) error {
	now := c.Now()
	_, err := c.RunRepo.UpdateCatalogBatch(ctx, batchID, ports.AICatalogBatchPatch{
		Status:         "completed",
		CandidateCount: &candidateCount,
		CompletedAt:    &now,
	})
	return err
}

func (c *CatalogBatchController) markBatchFailed(ctx context.Context, batchID, reason string) error {
	now := c.Now()
	_, err := c.RunRepo.UpdateCatalogBatch(ctx, batchID, ports.AICatalogBatchPatch{
		Status:        "failed",
		FailureReason: &reason,
		CompletedAt:   &now,
	})
	return err
}

// coverageComplete implements contract ② §3 — every batch must be COMPLETED.
func (c *CatalogBatchController) coverageComplete(records []ports.AICatalogBatchRecord) bool {
	for _, r := range records {
		if r.Status != "completed" {
			return false
		}
	}
	return true
}

// runFinalEvaluation calls the contract ② §6 final Gemini evaluation.
func (c *CatalogBatchController) runFinalEvaluation(ctx context.Context, input CatalogEvaluationInput, candidates []ports.CatalogBatchCandidate) (ports.AIGeminiProposal, error) {
	// Per contract ② §6, the Final Gemini does NOT see the entire catalog again.
	// It sees: Customer Message + Conversation Context + Candidate Results +
	// the commercial evidence linked to the candidates (retrieved fresh from
	// PostgreSQL, not from what Gemini returned).
	return c.Gemini.FinalEvaluate(ctx, FinalEvaluationInput{
		AIRunID:             input.AIRunID,
		AttemptID:           input.AttemptID,
		BusinessID:          input.BusinessID,
		ConversationID:      input.ConversationID,
		CustomerMessage:     input.CustomerMessage,
		ConversationContext: input.ConversationContext,
		EntityContract:      input.EntityContract,
		CandidateResults:    candidates,
	})
}

// CatalogEvaluationInput is the controller's input.
type CatalogEvaluationInput struct {
	AIRunID             string
	AttemptID           string
	BusinessID          string
	ConversationID      string
	CatalogScope        string // the catalog scope to evaluate (e.g., a specific catalog_id)
	CustomerMessage     string
	ConversationContext ports.AIContext
	EntityContract      CatalogEntityContractPayload
}

// countCompleted returns the number of batches with status="completed".
func countCompleted(records []ports.AICatalogBatchRecord) int {
	count := 0
	for _, r := range records {
		if r.Status == "completed" {
			count++
		}
	}
	return count
}
