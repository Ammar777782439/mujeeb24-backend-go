// Package services — Catalog Batch Controller (contracts ② and ⑨ §22)
//
// Implements contract ② Catalog Evaluation + Batching — CLOSED.
//
// Per contract ② §2, batch size is TOKEN-BASED, not item-count-based.
// Per contract ② §3, every item enters evaluation AT LEAST ONCE; the
// Controller guarantees coverage (Sent == Completed == Total).
// Per contract ② §4, each batch carries the AttributeSchemas its items use.
// Per contract ② §5, AI model returns ONLY candidates (item_id, variant_ids,
// offer_ids, reason) — not the full items back.
// Per contract ② §6, after all batches complete, a Final AI model Evaluation
// runs over the candidate set + customer message + context.
// Per contract ② §7, candidates are split by token budget, not by count.
// Per contract ② §8, batches are NOT chained via previous_interaction_id;
// they are independent stateless AI model requests over the same verified Conversation Context.
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
// It is invoked when AI model's first Interaction indicates that catalog data
// is needed. The Controller:
//  1. Builds the Catalog AI Projection (per contract ①) for the business scope.
//  2. Token-counts the serialized Projection.
//  3. Splits into batches by token budget.
//  4. Sends each batch as an independent stateless AI model request.
//  5. Collects candidates per batch.
//  6. Enforces coverage (Total == Sent == Completed).
//  7. Hands the candidate set to the Final AI model Evaluation.
//
// Per contract ② §1, Mujeeb:
//   - Determines the Catalog scope allowed for the merchant
//   - Builds the Projection
//   - Splits into Batches by TOKENS
//   - Guarantees every Item in scope was sent to AI model
//   - Tracks which Batches completed
//   - Aggregates evaluation results
//
// Evaluator:
//   - Understands the customer message
//   - Understands each Batch's data
//   - Infers and compares
//   - Identifies candidates
//   - Explains why each candidate is in the candidate set
type CatalogBatchController struct {
	// CatalogAI supplies complete tenant-scoped projection pages.
	CatalogAI ports.CatalogAIReadRepository
	Evaluator CatalogEvaluationClient
	RunRepo   ports.AIRunRepository
	Now       func() time.Time
	NewID     func() string

	// TokenBudget is the per-batch token cap. Per contract ② §2, this is
	// token-based not item-count-based. Set via runtime config.
	TokenBudget int

	// AIUsageRepository records per-batch telemetry to ai_usage_records.
	// Per AIUsageTokenTelemetry.md §6: every AI model call's tokens + cost
	// must be persisted. Batch calls record final_ai_replies=0.
	AIUsage   ports.AIUsageRepository
	AIPricing ports.AIProviderPricingRepository

	GatewayFXRateYER string

	Subscriptions ports.SubscriptionRepository
}

// CatalogEvaluationClient is the per-batch AI model interaction contract.
// Each batch is one independent stateless AI model request per contract ② §8 — no
// previous_interaction_id chaining between batches.
type CatalogEvaluationClient interface {
	// EvaluateBatch sends one batch to AI model and returns the candidate set.
	EvaluateBatch(ctx context.Context, input BatchEvaluationInput) (ports.CatalogBatchResult, error)

	// FinalEvaluate runs the contract ② §6 final evaluation with a custom
	// user prompt that includes BOTH candidate IDs AND full product details.
	// This allows AI model to compose a response with names, prices, descriptions.
	// Returns the proposal + usage telemetry for cost recording.
	FinalEvaluateWithDetails(ctx context.Context, input FinalEvaluationInput, userPrompt string) (ports.CapabilityDecision, ports.CustomerSalesUsageTelemetry, error)
}

type BatchRequestTokenEstimator interface {
	CountBatchTokens(ctx context.Context, input BatchEvaluationInput) (int, error)
}

type FinalRequestTokenEstimator interface {
	CountFinalTokens(ctx context.Context, input FinalEvaluationInput, userPrompt string) (int, error)
}

// BatchEvaluationInput is one batch's input to AI model.
type BatchEvaluationInput struct {
	AIRunID             string
	AttemptID           string
	BatchNumber         int
	BusinessID          string
	ConversationID      string
	CustomerMessage     string
	ConversationContext ports.CustomerSalesContext
	EntityContract      CatalogEntityContractPayload
	Batch               CatalogAIBatchPayload
	// Reduction switches the provider prompt from full-catalog evaluation
	// to token-driven candidate reduction. It never changes tenant scope.
	Reduction bool
}

// FinalEvaluationInput is the post-batch final AI model call.
type FinalEvaluationInput struct {
	AIRunID             string
	AttemptID           string
	BusinessID          string
	ConversationID      string
	CustomerMessage     string
	ConversationContext ports.CustomerSalesContext
	EntityContract      CatalogEntityContractPayload
	CandidateResults    []ports.CatalogBatchCandidate
}

type CatalogEvaluationResult struct {
	Decision ports.CapabilityDecision
	Evidence ports.CatalogAIEvidenceSet
}

// CatalogAIBatchPayload is one batch's content.
//
// Per contract ② §4, each batch carries the schemas its items use — schemas
// are NOT repeated globally across batches.
type CatalogAIBatchPayload struct {
	BatchNumber      int                        `json:"batch_number"`
	Catalogs         []CatalogAICatalog         `json:"catalogs,omitempty"`
	AttributeSchemas []CatalogAIAttributeSchema `json:"attribute_schemas,omitempty"`
	Items            []CatalogAIItem            `json:"items,omitempty"`
}

// RunCatalogEvaluation drives the full contract ② §9 evaluation pipeline.
//
// Returns the final CapabilityDecision (post-Final-Evaluation) on success.
// On failure, returns the failure stage + category for ai_runs.failure_*.
func (c *CatalogBatchController) RunCatalogEvaluation(ctx context.Context, input CatalogEvaluationInput) (CatalogEvaluationResult, error) {
	if !c.Ready() {
		return CatalogEvaluationResult{}, errors.New("CatalogBatchController is not fully wired with a token counter")
	}
	if strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.AIRunID) == "" {
		return CatalogEvaluationResult{}, errors.New("business_id and ai_run_id are required for Catalog Evaluation per contract ② §1")
	}

	return c.runPagedCatalogEvaluation(ctx, input)
}

func (c *CatalogBatchController) runPagedCatalogEvaluation(ctx context.Context, input CatalogEvaluationInput) (CatalogEvaluationResult, error) {
	revisionStart, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision before evaluation: %w", err)
	}
	cursor := ""
	nextBatchNumber := 1
	batchRecords := make([]ports.AICatalogBatchRecord, 0)
	candidateSet := make([]ports.CatalogBatchCandidate, 0)
	candidateProjection := CatalogAIProjection{}
	candidateItemsSeen := make(map[string]struct{})
	for {
		page, err := c.CatalogAI.ListProjectionPage(ctx, ports.CatalogAIProjectionRequest{
			BusinessID: input.BusinessID,
			CatalogID:  input.CatalogScope,
			Limit:      200,
			Cursor:     cursor,
		})
		if err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("list catalog projection page: %w", err)
		}

		if len(page.Items) > 0 {
			pageProjection := ProjectionFromBundles(page.Items)
			appendCatalogRecordsToProjection(&pageProjection, page.Catalogs)
			batches, err := c.splitIntoBatches(ctx, pageProjection, input, nextBatchNumber)
			if err != nil {
				return CatalogEvaluationResult{}, fmt.Errorf("split catalog page into budgeted batches: %w", err)
			}
			pageCandidates := make([]ports.CatalogBatchCandidate, 0)

			for _, batch := range batches {
				rec, err := c.createBatchRecord(ctx, input.AIRunID, batch)
				if err != nil {
					return CatalogEvaluationResult{}, fmt.Errorf("create batch %d record: %w", batch.BatchNumber, err)
				}
				batchRecords = append(batchRecords, rec)
				recordIndex := len(batchRecords) - 1

				if err := c.markBatchRunning(ctx, rec.ID); err != nil {
					return CatalogEvaluationResult{}, err
				}
				result, err := c.Evaluator.EvaluateBatch(ctx, BatchEvaluationInput{
					AIRunID:             input.AIRunID,
					AttemptID:           input.AttemptID,
					BatchNumber:         batch.BatchNumber,
					BusinessID:          input.BusinessID,
					ConversationID:      input.ConversationID,
					CustomerMessage:     input.CustomerMessage,
					ConversationContext: input.ConversationContext,
					EntityContract:      input.EntityContract,
					Batch:               batch,
				})
				if err != nil {
					_ = c.markBatchFailed(ctx, rec.ID, err.Error())
					return CatalogEvaluationResult{}, fmt.Errorf("batch %d evaluation: %w", batch.BatchNumber, err)
				}
				// Successful model calls incur usage even if their output is
				// rejected by the evidence validator.
				c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, result.Usage, fmt.Sprintf("batch_%d", batch.BatchNumber))
				if err := validateBatchCandidates(batch, result.Candidates); err != nil {
					_ = c.markBatchFailed(ctx, rec.ID, err.Error())
					return CatalogEvaluationResult{}, fmt.Errorf("batch %d returned invalid candidate evidence: %w", batch.BatchNumber, err)
				}
				if err := c.markBatchCompleted(ctx, rec.ID, len(result.Candidates)); err != nil {
					return CatalogEvaluationResult{}, err
				}
				batchRecords[recordIndex].Status = "completed"
				candidateSet = append(candidateSet, result.Candidates...)
				pageCandidates = append(pageCandidates, result.Candidates...)
				log.Printf("[CatalogBatch] BATCH_DONE batch=%d items=%d candidates=%d", batch.BatchNumber, len(batch.Items), len(result.Candidates))
			}
			if len(pageCandidates) > 0 {
				appendCandidateProjection(
					&candidateProjection,
					CatalogAIBatchPayload{
						Catalogs:         pageProjection.Catalogs,
						AttributeSchemas: pageProjection.AttributeSchemas,
						Items:            pageProjection.Items,
					},
					normalizeBatchCandidates(pageCandidates),
					candidateItemsSeen,
				)
			}
			nextBatchNumber += len(batches)
		}

		if !page.HasMore {
			break
		}
		if strings.TrimSpace(page.NextCursor) == "" || page.NextCursor == cursor {
			return CatalogEvaluationResult{}, errors.New("catalog projection cursor did not advance")
		}
		cursor = page.NextCursor
	}

	log.Printf("[CatalogBatch] COVERAGE total=%d completed=%d", len(batchRecords), countCompleted(batchRecords))
	if !c.coverageComplete(batchRecords) {
		return CatalogEvaluationResult{}, fmt.Errorf("coverage incomplete per contract ② §3 — %d/%d batches completed", countCompleted(batchRecords), len(batchRecords))
	}

	revisionEnd, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision after evaluation: %w", err)
	}
	if revisionEnd != revisionStart {
		return CatalogEvaluationResult{}, errors.New("catalog changed during full evaluation; refusing mixed-snapshot AI result")
	}

	proposal, finalProjection, err := c.runFinalEvaluation(ctx, input, candidateSet, candidateProjection, nextBatchNumber)
	if err != nil {
		return CatalogEvaluationResult{}, err
	}
	// The final LLM call (including any candidate-reduction rounds) may run
	// after the first revision check. A merchant can change an offer or price
	// during that time. Never publish a decision from a stale catalog.
	revisionAfterFinal, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision after final evaluation: %w", err)
	}
	if revisionAfterFinal != revisionStart {
		return CatalogEvaluationResult{}, errors.New("catalog changed during final evaluation; refusing stale commercial AI result")
	}
	return CatalogEvaluationResult{
		Decision: proposal,
		// Validation trusts exactly the projection exposed to the final model
		// after any token-driven reduction, never discarded catalog pages.
		Evidence: EvidenceFromProjection(finalProjection),
	}, nil
}

func validateBatchCandidates(batch CatalogAIBatchPayload, candidates []ports.CatalogBatchCandidate) error {
	items := make(map[string]CatalogAIItem, len(batch.Items))
	for _, item := range batch.Items {
		items[item.ID] = item
	}
	for _, candidate := range candidates {
		item, ok := items[candidate.ItemID]
		if !ok {
			return fmt.Errorf("candidate item_id %s was not present in batch", candidate.ItemID)
		}
		variants := make(map[string]struct{}, len(item.Variants))
		for _, variant := range item.Variants {
			variants[variant.ID] = struct{}{}
		}
		offers := make(map[string]CatalogAIOffer, len(item.Offers))
		for _, offer := range item.Offers {
			offers[offer.ID] = offer
		}
		selectedVariants := make(map[string]struct{}, len(candidate.VariantIDs))
		for _, variantID := range candidate.VariantIDs {
			if _, ok := variants[variantID]; !ok {
				return fmt.Errorf("candidate variant_id %s was not present under item %s", variantID, candidate.ItemID)
			}
			selectedVariants[variantID] = struct{}{}
		}
		for _, offerID := range candidate.OfferIDs {
			offer, ok := offers[offerID]
			if !ok {
				return fmt.Errorf("candidate offer_id %s was not present under item %s", offerID, candidate.ItemID)
			}
			if len(selectedVariants) > 0 && offer.VariantID != nil && *offer.VariantID != "" {
				if _, ok := selectedVariants[*offer.VariantID]; !ok {
					return fmt.Errorf(
						"candidate offer_id %s belongs to variant %s, not one of the selected variants for item %s",
						offerID, *offer.VariantID, candidate.ItemID,
					)
				}
			}
		}
	}
	return nil
}

func appendCandidateProjection(target *CatalogAIProjection, batch CatalogAIBatchPayload, candidates []ports.CatalogBatchCandidate, seen map[string]struct{}) {
	if target == nil {
		return
	}

	candidateByItem := make(map[string]ports.CatalogBatchCandidate, len(candidates))
	for _, candidate := range normalizeBatchCandidates(candidates) {
		candidateByItem[candidate.ItemID] = candidate
	}

	for _, item := range batch.Items {
		if _, selected := candidateByItem[item.ID]; !selected {
			continue
		}
		if _, exists := seen[item.ID]; exists {
			continue // already included as an indivisible complete item
		}
		target.Items = append(target.Items, item)
		seen[item.ID] = struct{}{}
	}

	knownCatalogs := make(map[string]struct{}, len(target.Catalogs))
	for _, catalog := range target.Catalogs {
		knownCatalogs[catalog.ID] = struct{}{}
	}
	for _, catalog := range batch.Catalogs {
		if _, ok := knownCatalogs[catalog.ID]; ok {
			continue
		}
		for _, item := range target.Items {
			if item.CatalogID == catalog.ID {
				target.Catalogs = append(target.Catalogs, catalog)
				knownCatalogs[catalog.ID] = struct{}{}
				break
			}
		}
	}

	knownSchemas := make(map[string]struct{}, len(target.AttributeSchemas))
	for _, schema := range target.AttributeSchemas {
		knownSchemas[schema.ID] = struct{}{}
	}
	for _, schema := range batch.AttributeSchemas {
		if _, ok := knownSchemas[schema.ID]; ok {
			continue
		}
		for _, item := range target.Items {
			if item.AttributeSchemaID != nil && *item.AttributeSchemaID == schema.ID {
				target.AttributeSchemas = append(target.AttributeSchemas, schema)
				knownSchemas[schema.ID] = struct{}{}
				break
			}
		}
	}
}

func normalizeBatchCandidates(candidates []ports.CatalogBatchCandidate) []ports.CatalogBatchCandidate {
	if len(candidates) == 0 {
		return nil
	}
	index := make(map[string]int, len(candidates))
	out := make([]ports.CatalogBatchCandidate, 0, len(candidates))
	variantSeen := make(map[string]map[string]struct{})
	offerSeen := make(map[string]map[string]struct{})

	for _, candidate := range candidates {
		pos, exists := index[candidate.ItemID]
		if !exists {
			pos = len(out)
			index[candidate.ItemID] = pos
			out = append(out, ports.CatalogBatchCandidate{ItemID: candidate.ItemID, Reason: candidate.Reason})
			variantSeen[candidate.ItemID] = make(map[string]struct{})
			offerSeen[candidate.ItemID] = make(map[string]struct{})
		} else if out[pos].Reason == "" && candidate.Reason != "" {
			out[pos].Reason = candidate.Reason
		}
		for _, variantID := range candidate.VariantIDs {
			if _, ok := variantSeen[candidate.ItemID][variantID]; ok {
				continue
			}
			variantSeen[candidate.ItemID][variantID] = struct{}{}
			out[pos].VariantIDs = append(out[pos].VariantIDs, variantID)
		}
		for _, offerID := range candidate.OfferIDs {
			if _, ok := offerSeen[candidate.ItemID][offerID]; ok {
				continue
			}
			offerSeen[candidate.ItemID][offerID] = struct{}{}
			out[pos].OfferIDs = append(out[pos].OfferIDs, offerID)
		}
	}
	return out
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
			ID:              d.ID,
			SchemaID:        r.ID,
			AttributeKey:    d.Key,
			Label:           d.Label,
			DataType:        d.DataType,
			IsRequired:      d.Required,
			ValidationRules: parseJSONAttributes(d.ValidationRules),
			DisplayOrder:    d.DisplayOrder,
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
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber() // Preserve decimal/integer attribute values without float64 coercion.
	if err := decoder.Decode(&out); err != nil {
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
func (c *CatalogBatchController) splitIntoBatches(ctx context.Context, projection CatalogAIProjection, input CatalogEvaluationInput, startBatchNumber int) ([]CatalogAIBatchPayload, error) {
	return c.splitIntoBatchesMode(ctx, projection, input, startBatchNumber, false)
}

func (c *CatalogBatchController) splitIntoBatchesMode(ctx context.Context, projection CatalogAIProjection, input CatalogEvaluationInput, startBatchNumber int, reduction bool) ([]CatalogAIBatchPayload, error) {
	if len(projection.Items) == 0 {
		return nil, nil
	}
	budget := c.TokenBudget
	if budget <= 0 {
		budget = 8000
	}

	estimator, hasEstimator := c.Evaluator.(BatchRequestTokenEstimator)
	if !hasEstimator {
		return nil, errors.New("full provider request token estimator is required; no legacy payload-only counter")
	}

	buildBatch := func(items []CatalogAIItem, batchNumber int) CatalogAIBatchPayload {
		schemaByID := make(map[string]CatalogAIAttributeSchema)
		for _, item := range items {
			if item.AttributeSchemaID == nil {
				continue
			}
			for _, schema := range projection.AttributeSchemas {
				if schema.ID == *item.AttributeSchemaID {
					schemaByID[schema.ID] = schema
					break
				}
			}
		}
		return CatalogAIBatchPayload{
			BatchNumber:      batchNumber,
			Catalogs:         collectCatalogsForItems(items, projection.Catalogs),
			AttributeSchemas: collectSchemas(schemaByID),
			Items:            append([]CatalogAIItem(nil), items...),
		}
	}

	nextBatchNumber := startBatchNumber
	if nextBatchNumber <= 0 {
		nextBatchNumber = 1
	}
	batches := make([]CatalogAIBatchPayload, 0)
	countBatch := func(batch CatalogAIBatchPayload) (int, error) {
		return estimator.CountBatchTokens(ctx, BatchEvaluationInput{
			AIRunID:             input.AIRunID,
			AttemptID:           input.AttemptID,
			BatchNumber:         batch.BatchNumber,
			BusinessID:          input.BusinessID,
			ConversationID:      input.ConversationID,
			CustomerMessage:     input.CustomerMessage,
			ConversationContext: input.ConversationContext,
			EntityContract:      input.EntityContract,
			Batch:               batch,
			Reduction:           reduction,
		})
	}

	var split func([]CatalogAIItem) error
	split = func(items []CatalogAIItem) error {
		if len(items) == 0 {
			return nil
		}
		batch := buildBatch(items, nextBatchNumber)
		tokens, err := countBatch(batch)
		if err != nil {
			return fmt.Errorf("count tokens for batch %d: %w", nextBatchNumber, err)
		}
		if tokens <= budget {
			batches = append(batches, batch)
			nextBatchNumber++
			return nil
		}
		if len(items) == 1 {
			return fmt.Errorf("catalog item %s exceeds token budget %d with complete request estimate %d: indivisible commercial entity; configure a verified larger-context route", items[0].ID, budget, tokens)
		}
		mid := len(items) / 2
		if err := split(items[:mid]); err != nil {
			return err
		}
		return split(items[mid:])
	}

	if err := split(projection.Items); err != nil {
		return nil, err
	}
	// Verify each source item appears once, unchanged, with all its child
	// relationships and schema before any batch is sent to the provider.
	if err := validateCatalogBatchCoverage(projection, batches); err != nil {
		return nil, fmt.Errorf("catalog batch coverage: %w", err)
	}
	return batches, nil
}

func collectCatalogsForItems(items []CatalogAIItem, catalogs []CatalogAICatalog) []CatalogAICatalog {
	if len(items) == 0 || len(catalogs) == 0 {
		return nil
	}
	needed := make(map[string]struct{}, len(items))
	for _, item := range items {
		needed[item.CatalogID] = struct{}{}
	}
	out := make([]CatalogAICatalog, 0, len(needed))
	for _, catalog := range catalogs {
		if _, ok := needed[catalog.ID]; ok {
			out = append(out, catalog)
		}
	}
	return out
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

// runFinalEvaluation calls the contract ② §6 final AI model evaluation.
// Per contract ② §6, the Final AI model sees:
//   - Customer Message
//   - Conversation Context
//   - Candidate Results (IDs + reason)
//   - الدليل التجاري المرتبط بالمرشحين (full product details for each candidate)
//
// Without the full product details, AI model only sees IDs and cannot
// compose a response with names, prices, descriptions.
func (c *CatalogBatchController) runFinalEvaluation(
	ctx context.Context,
	input CatalogEvaluationInput,
	candidates []ports.CatalogBatchCandidate,
	projection CatalogAIProjection,
	startBatchNumber int,
) (ports.CapabilityDecision, CatalogAIProjection, error) {
	candidates = normalizeBatchCandidates(candidates)
	candidateProjection := CatalogAIProjection{}
	appendCandidateProjection(
		&candidateProjection,
		CatalogAIBatchPayload{
			Catalogs:         projection.Catalogs,
			AttributeSchemas: projection.AttributeSchemas,
			Items:            projection.Items,
		},
		candidates,
		make(map[string]struct{}),
	)

	nextBatchNumber := startBatchNumber
	if nextBatchNumber <= 0 {
		nextBatchNumber = 1
	}
	budget := c.TokenBudget
	if budget <= 0 {
		budget = 8000
	}
	lastOversizeTokens := 0
	reductionRounds := 0
	const maxReductionRounds = 3

	for {
		if err := ctx.Err(); err != nil {
			return ports.CapabilityDecision{}, CatalogAIProjection{}, err
		}

		finalInput, userPrompt := buildFinalEvaluationPayload(input, candidates, candidateProjection)

		if estimator, ok := c.Evaluator.(FinalRequestTokenEstimator); ok {
			tokens, err := estimator.CountFinalTokens(ctx, finalInput, userPrompt)
			if err != nil {
				return ports.CapabilityDecision{}, CatalogAIProjection{}, fmt.Errorf("count final evaluation tokens: %w", err)
			}
			if tokens > budget {
				if len(candidateProjection.Items) == 0 {
					return ports.CapabilityDecision{}, CatalogAIProjection{}, fmt.Errorf(
						"final evaluation base context exceeds token budget %d with estimated request size %d",
						budget, tokens,
					)
				}
				if lastOversizeTokens > 0 && tokens >= lastOversizeTokens {
					return ports.CapabilityDecision{}, CatalogAIProjection{}, fmt.Errorf(
						"candidate reduction made no token progress: previous=%d current=%d budget=%d",
						lastOversizeTokens, tokens, budget,
					)
				}
				lastOversizeTokens = tokens
				if reductionRounds >= maxReductionRounds {
					return ports.CapabilityDecision{}, CatalogAIProjection{}, fmt.Errorf("candidate reduction exceeded %d bounded rounds with %d estimated tokens; use a validated larger-context route for complete comparison", maxReductionRounds, tokens)
				}
				reductionRounds++

				reducedCandidates, reducedProjection, nextNumber, err := c.reduceCandidateRound(
					ctx,
					input,
					candidateProjection,
					nextBatchNumber,
				)
				if err != nil {
					return ports.CapabilityDecision{}, CatalogAIProjection{}, err
				}
				candidates = normalizeBatchCandidates(reducedCandidates)
				candidateProjection = reducedProjection
				nextBatchNumber = nextNumber
				continue
			}
		}

		proposal, usage, err := c.Evaluator.FinalEvaluateWithDetails(ctx, finalInput, userPrompt)
		if err != nil {
			return ports.CapabilityDecision{}, CatalogAIProjection{}, err
		}
		if !isTerminalCustomerSalesCapability(proposal.Capability) {
			return ports.CapabilityDecision{}, CatalogAIProjection{}, errors.New("final catalog evaluation returned a non-terminal or unavailable capability after complete catalog coverage")
		}
		c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, usage, "final_evaluation")
		return proposal, candidateProjection, nil
	}
}

func buildFinalEvaluationPayload(
	input CatalogEvaluationInput,
	candidates []ports.CatalogBatchCandidate,
	candidateProjection CatalogAIProjection,
) (FinalEvaluationInput, string) {
	contextPayload := struct {
		Sales                  *ports.SalesWorkflowContext                 `json:"sales,omitempty"`
		SchemaVersion          int                                         `json:"schema_version"`
		Freshness              string                                      `json:"freshness"`
		Business               ports.CustomerSalesContextBusiness          `json:"business"`
		Conversation           ports.CustomerSalesContextConversation      `json:"conversation"`
		Customer               finalEvaluationCustomer                     `json:"customer"`
		KnowledgeEvidence      []ports.CustomerSalesKnowledgeEvidence      `json:"knowledge_evidence,omitempty"`
		BusinessPolicyEvidence []ports.CustomerSalesBusinessPolicyEvidence `json:"business_policy_evidence,omitempty"`
		RecentMessages         []ports.CustomerSalesRecentMessageEvidence  `json:"recent_messages,omitempty"`
		PolicyEvidence         ports.CustomerSalesPolicyEvidence           `json:"policy_evidence"`
		KnowledgeState         string                                      `json:"knowledge_state"`
		ConversationState      *ports.ConversationStateRecord              `json:"conversation_state,omitempty"`
		ConversationSummary    string                                      `json:"conversation_summary,omitempty"`
		GeneratedAt            time.Time                                   `json:"generated_at"`
		ExpiresAt              time.Time                                   `json:"expires_at"`
	}{
		Sales:         input.ConversationContext.Sales,
		SchemaVersion: input.ConversationContext.SchemaVersion,
		Freshness:     input.ConversationContext.Freshness,
		Business:      input.ConversationContext.Business,
		Conversation:  input.ConversationContext.Conversation,
		Customer: finalEvaluationCustomer{
			Reference:        input.ConversationContext.Customer.Reference,
			LocalePreference: input.ConversationContext.Customer.LocalePreference,
			Status:           input.ConversationContext.Customer.Status,
		},
		KnowledgeEvidence:      input.ConversationContext.KnowledgeEvidence,
		BusinessPolicyEvidence: input.ConversationContext.BusinessPolicyEvidence,
		RecentMessages:         input.ConversationContext.RecentMessages,
		PolicyEvidence:         input.ConversationContext.PolicyEvidence,
		KnowledgeState:         input.ConversationContext.KnowledgeState,
		ConversationState:      input.ConversationContext.ConversationState,
		ConversationSummary:    input.ConversationContext.ConversationSummary,
		GeneratedAt:            input.ConversationContext.GeneratedAt,
		ExpiresAt:              input.ConversationContext.ExpiresAt,
	}

	userPrompt := fmt.Sprintf(
		"Customer message: %s\n\nVerified conversation context:\n%s\n\nAggregated candidates:\n%s\n\nCandidate catalog projection:\n%s",
		input.CustomerMessage,
		string(mustMarshal(contextPayload)),
		string(mustMarshal(candidates)),
		string(mustMarshal(candidateProjection)),
	)

	return FinalEvaluationInput{
		AIRunID:             input.AIRunID,
		AttemptID:           input.AttemptID,
		BusinessID:          input.BusinessID,
		ConversationID:      input.ConversationID,
		CustomerMessage:     input.CustomerMessage,
		ConversationContext: input.ConversationContext,
		EntityContract:      input.EntityContract,
		CandidateResults:    candidates,
	}, userPrompt
}

// finalEvaluationCustomer deliberately excludes profile and contact payloads.
// Those fields are not catalog evidence and can be both large and sensitive;
// the final model only needs the same bounded customer identity fields used by
// the normal customer-sales prompt.
type finalEvaluationCustomer struct {
	Reference        string `json:"reference"`
	LocalePreference string `json:"locale_preference"`
	Status           string `json:"status"`
}

func (c *CatalogBatchController) reduceCandidateRound(
	ctx context.Context,
	input CatalogEvaluationInput,
	projection CatalogAIProjection,
	startBatchNumber int,
) ([]ports.CatalogBatchCandidate, CatalogAIProjection, int, error) {
	batches, err := c.splitIntoBatchesMode(ctx, projection, input, startBatchNumber, true)
	if err != nil {
		return nil, CatalogAIProjection{}, startBatchNumber, fmt.Errorf("split candidate reduction batches: %w", err)
	}

	reducedCandidates := make([]ports.CatalogBatchCandidate, 0)
	reducedProjection := CatalogAIProjection{}
	seenItems := make(map[string]struct{})

	for _, batch := range batches {
		rec, err := c.createBatchRecord(ctx, input.AIRunID, batch)
		if err != nil {
			return nil, CatalogAIProjection{}, startBatchNumber, fmt.Errorf("create reduction batch %d: %w", batch.BatchNumber, err)
		}
		if err := c.markBatchRunning(ctx, rec.ID); err != nil {
			return nil, CatalogAIProjection{}, startBatchNumber, err
		}

		result, err := c.Evaluator.EvaluateBatch(ctx, BatchEvaluationInput{
			AIRunID:             input.AIRunID,
			AttemptID:           input.AttemptID,
			BatchNumber:         batch.BatchNumber,
			BusinessID:          input.BusinessID,
			ConversationID:      input.ConversationID,
			CustomerMessage:     input.CustomerMessage,
			ConversationContext: input.ConversationContext,
			EntityContract:      input.EntityContract,
			Batch:               batch,
			Reduction:           true,
		})
		if err != nil {
			_ = c.markBatchFailed(ctx, rec.ID, err.Error())
			return nil, CatalogAIProjection{}, startBatchNumber, fmt.Errorf("candidate reduction batch %d: %w", batch.BatchNumber, err)
		}
		// Meter successful provider calls before checking their references.
		// In particular, rejected candidate IDs must not hide spent tokens.
		c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, result.Usage, fmt.Sprintf("candidate_reduction_batch_%d", batch.BatchNumber))
		if err := validateBatchCandidates(batch, result.Candidates); err != nil {
			_ = c.markBatchFailed(ctx, rec.ID, err.Error())
			return nil, CatalogAIProjection{}, startBatchNumber, fmt.Errorf("candidate reduction batch %d returned invalid evidence: %w", batch.BatchNumber, err)
		}
		if err := c.markBatchCompleted(ctx, rec.ID, len(result.Candidates)); err != nil {
			return nil, CatalogAIProjection{}, startBatchNumber, err
		}

		reducedCandidates = append(reducedCandidates, result.Candidates...)
		appendCandidateProjection(&reducedProjection, batch, result.Candidates, seenItems)
		log.Printf(
			"[CatalogBatch] REDUCTION_DONE batch=%d items=%d candidates=%d",
			batch.BatchNumber,
			len(batch.Items),
			len(result.Candidates),
		)
	}

	return normalizeBatchCandidates(reducedCandidates), reducedProjection, startBatchNumber + len(batches), nil
}

// mustMarshal marshals v to JSON, panicking on error (should never fail).
func mustMarshal(v any) []byte {
	buf, err := json.Marshal(v)
	if err != nil {
		return []byte(`[]`)
	}
	return buf
}

// CatalogEvaluationInput is the controller's input.
type CatalogEvaluationInput struct {
	AIRunID             string
	AttemptID           string
	BusinessID          string
	ConversationID      string
	CatalogScope        string // the catalog scope to evaluate (e.g., a specific catalog_id)
	CustomerMessage     string
	ConversationContext ports.CustomerSalesContext
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

// recordBatchUsage persists per-batch AI model call telemetry to ai_usage_records.
//
// Per AIUsageTokenTelemetry.md §6: every AI model call's tokens + cost must be
// recorded. Batch calls and the final evaluation call record:
//   - model_requests = 1
//   - final_ai_replies = 0 (the final reply is counted by AutoReply.recordAIUsage)
//   - tokens (input/cached/output) from AI model's usageMetadata
//   - provider_cost_yer computed from the pricing table
//   - correlation_id = runID + "|" + phase (for dedup)
//
// Best-effort: errors are logged but never fail the catalog evaluation.
func (c *CatalogBatchController) recordBatchUsage(ctx context.Context, businessID, runID string, usage ports.CustomerSalesUsageTelemetry, phase string) {
	if c.AIUsage == nil {
		return
	}
	// Find the business's active subscription ID.
	var subscriptionID string
	if c.Subscriptions != nil {
		page, err := c.Subscriptions.List(ctx, ports.SubscriptionListFilter{
			BusinessID: businessID,
			Status:     "ACTIVE",
			Limit:      1,
		})
		if err == nil && len(page.Items) > 0 {
			subscriptionID = page.Items[0].ID
		}
	}
	if subscriptionID == "" {
		log.Printf("[CatalogBatch] AI_USAGE_SKIP business=%s reason=no_active_subscription phase=%s", businessID, phase)
		return
	}
	// Compute provider_cost from the pricing table.
	// Same anti-double-counting logic as auto_reply.go:
	// promptTokenCount INCLUDES cachedContentTokenCount, so we subtract.
	totalInput := usage.InputTokens
	cachedInput := usage.CachedTokens
	nonCachedInput := totalInput - cachedInput
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	providerCostYER := 0
	pricingVersion, gatewayCostUSD, gatewayFXRate, pricingFailed := gatewayNativeSpend(usage, c.GatewayFXRateYER)
	recordStatus := "success"
	if pricingFailed {
		recordStatus = "pricing_failed"
	}
	// Use correlation_id = runID + "|" + phase for dedup.
	// If the same phase is recorded twice for the same run, it's a retry —
	// the AppendRecord generates a new UUID so we rely on the caller not
	// calling recordBatchUsage twice for the same phase. The catalog batch
	// controller calls it exactly once per batch + once for final evaluation.
	correlationID := runID + "|" + phase
	recordID := c.NewID()
	now := c.Now()
	// Per P2-13: use the REAL latency from the AI model call
	// (usage.LatencyMs) to compute StartedAt — NOT EstimatedCostMicros
	// (which was always 0, resulting in StartedAt == CompletedAt +
	// latency=0 for every batch record).
	startedAt := now.Add(-time.Duration(usage.LatencyMs) * time.Millisecond)
	usageRecord := ports.AIUsageAppend{
		ID:                recordID,
		BusinessID:        businessID,
		SubscriptionID:    subscriptionID,
		Provider:          usageProvider(usage.Provider),
		Model:             usage.Model,
		InputTokens:       int64(nonCachedInput),
		CachedInputTokens: int64(cachedInput),
		OutputTokens:      int64(usage.OutputTokens),
		ModelRequests:     1,
		ToolCalls:         0,
		FinalAIReplies:    0, // batch calls never count as final replies
		ProviderCostYER:   providerCostYER,

		PricingVersion: pricingVersion,
		Status:         recordStatus,
		CorrelationID:  &correlationID,
		StartedAt:      startedAt,
		CompletedAt:    now,
		Now:            now,
	}
	usageRecord.GatewayCostUSD, usageRecord.GatewayFXRateYER = gatewayCostUSD, gatewayFXRate
	usageRecord.GatewayCallID, usageRecord.GatewayDeploymentID = usage.GatewayCallID, usage.GatewayDeploymentID
	_, err := c.AIUsage.AppendRecord(ctx, usageRecord)
	if err != nil {
		log.Printf("[CatalogBatch] AI_USAGE_RECORD_FAILED business=%s phase=%s err=%v", businessID, phase, err)
		return
	}
	log.Printf("[CatalogBatch] AI_USAGE_RECORDED business=%s phase=%s tokens_in=%d cached=%d tokens_out=%d cost_yer=%d",
		businessID, phase, nonCachedInput, cachedInput, usage.OutputTokens, providerCostYER)
}

// Ready is the same local dependency check used when offering the capability.
func (c *CatalogBatchController) Ready() bool {
	if c == nil || c.Evaluator == nil || c.RunRepo == nil || c.CatalogAI == nil || c.Now == nil || c.NewID == nil {
		return false
	}
	_, estimated := c.Evaluator.(BatchRequestTokenEstimator)
	return estimated
}
