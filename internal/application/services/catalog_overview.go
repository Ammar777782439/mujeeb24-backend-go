package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogOverviewClient delegates ALL semantic summarization and customer
// wording to Gemini. The controller verifies coverage, tenant pages, model
// capacity, catalog revision and output size; it never writes a sales reply.
type CatalogOverviewClient interface {
	CountOverviewTokens(context.Context, CatalogOverviewRequest) (int, error)
	ComposeOverview(context.Context, CatalogOverviewRequest) (CatalogOverviewResponse, error)
}

type CatalogOverviewRequest struct {
	Stage                 string
	CustomerMessage       string
	ConversationContext   ports.CustomerSalesContext
	Batch                 *CatalogAIBatchPayload
	Summaries             []string
	RequestedPresentation string
}

type CatalogOverviewResponse struct {
	Text  string
	Usage ports.CustomerSalesUsageTelemetry
}

const (
	overviewBatchMaxRunes  = 700
	overviewMergeMaxRunes  = 700
	overviewFinalMaxRunes  = 1800
	overviewMaxMergeRounds = 12
)

// runCatalogOverview visits EVERY active catalog item in the business,
// including all relevant item fragments and pages. Model-generated batch
// digests are summarized in a bounded tree, then Gemini writes exactly
// ONE customer response. Batches are never sent as customer messages.
func (c *CatalogBatchController) runCatalogOverview(ctx context.Context, input CatalogEvaluationInput) (CatalogEvaluationResult, error) {
	ai, ok := c.Gemini.(CatalogOverviewClient)
	if !ok {
		return CatalogEvaluationResult{}, errors.New("configured catalog model cannot author full-catalog overviews")
	}
	if c.TokenBudget <= 0 {
		return CatalogEvaluationResult{}, errors.New("catalog overview requires a positive verified token budget")
	}
	before, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision: %w", err)
	}
	cursor := ""
	seenCursors := map[string]struct{}{}
	seenItems := map[string]struct{}{}
	var summaries []string
	batchNumber := 0
	itemCount := 0
	// Coverage is proved by Go's lossless batch validation and successful
	// submission of EVERY original item fragment, never by Gemini echoing
	// UUIDs in a JSON array.
	reviewedItems := make(map[string]struct{})
	for {
		if err := ctx.Err(); err != nil {
			return CatalogEvaluationResult{}, err
		}
		page, err := c.CatalogAI.ListProjectionPage(ctx, ports.CatalogAIProjectionRequest{
			BusinessID: input.BusinessID, CatalogID: input.CatalogScope, Limit: 200, Cursor: cursor,
		})
		if err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("read overview catalog page: %w", err)
		}
		projection := ProjectionFromBundles(page.Items)
		appendCatalogRecordsToProjection(&projection, page.Catalogs)
		for _, item := range projection.Items {
			if _, duplicate := seenItems[item.ID]; duplicate {
				return CatalogEvaluationResult{}, fmt.Errorf("catalog overview duplicated item %s across pages", item.ID)
			}
			seenItems[item.ID] = struct{}{}
			itemCount++
		}
		if len(projection.Items) > 0 {
			batches, err := c.splitIntoBatches(ctx, projection, input, batchNumber+1)
			if err != nil {
				return CatalogEvaluationResult{}, fmt.Errorf("lossless catalog overview batching: %w", err)
			}
			for _, batch := range batches {
				// The existing batch token contract can differ from the
				// overview schema. Split again against the EXACT overview
				// request, never by silently dropping commercial fragments.
				chunks, err := c.fitOverviewBatch(ctx, ai, input, batch)
				if err != nil {
					return CatalogEvaluationResult{}, err
				}
				if err := validateCatalogBatchCoverage(
					CatalogAIProjection{Catalogs: batch.Catalogs, AttributeSchemas: batch.AttributeSchemas, Items: batch.Items},
					chunks,
				); err != nil {
					return CatalogEvaluationResult{}, fmt.Errorf("overview re-batching changed commercial evidence: %w", err)
				}
				for _, chunk := range chunks {
					batchNumber++
					request := CatalogOverviewRequest{
						Stage: "batch", CustomerMessage: input.CustomerMessage,
						ConversationContext: input.ConversationContext,
						Batch:               &chunk, RequestedPresentation: input.Presentation,
					}
					result, err := ai.ComposeOverview(ctx, request)
					if err != nil {
						return CatalogEvaluationResult{}, fmt.Errorf("Gemini catalog overview batch %d: %w", batchNumber, err)
					}
					c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, result.Usage, fmt.Sprintf("overview_batch_%d", batchNumber))
					if strings.TrimSpace(result.Text) == "" || utf8.RuneCountInString(result.Text) > overviewBatchMaxRunes {
						return CatalogEvaluationResult{}, fmt.Errorf("overview batch %d returned empty or oversized evidence summary", batchNumber)
					}
					// Only mark these input facts reviewed after the provider
					// successfully returns a usable intermediate summary.
					for _, item := range chunk.Items {
						reviewedItems[item.ID] = struct{}{}
					}
					summaries = append(summaries, result.Text)
				}
			}
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return CatalogEvaluationResult{}, errors.New("catalog overview cursor did not advance")
		}
		if _, duplicate := seenCursors[page.NextCursor]; duplicate {
			return CatalogEvaluationResult{}, errors.New("catalog overview cursor cycled")
		}
		seenCursors[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	if len(reviewedItems) != len(seenItems) {
		return CatalogEvaluationResult{}, fmt.Errorf("overview input coverage incomplete: reviewed=%d expected=%d", len(reviewedItems), len(seenItems))
	}
	log.Printf("[CatalogBatch] OVERVIEW_COVERAGE items=%d reviewed=%d batches=%d", itemCount, len(reviewedItems), batchNumber)
	// Keep intermediate reduction strictly bounded. A 100-catalog store
	// produces many AI *processing* calls but one customer-facing answer.
	for round := 0; len(summaries) > 1; round++ {
		if round >= overviewMaxMergeRounds {
			return CatalogEvaluationResult{}, errors.New("catalog overview did not converge within bounded synthesis rounds")
		}
		var next []string
		for i := 0; i < len(summaries); {
			limit := i + 4
			if limit > len(summaries) {
				limit = len(summaries)
			}
			end, err := c.fitOverviewSummaries(ctx, ai, input, summaries[i:limit])
			if err != nil {
				return CatalogEvaluationResult{}, err
			}
			if end == 1 {
				// A singleton summary is already checked and need not
				// incur a useless AI request.
				next = append(next, summaries[i])
				i++
				continue
			}
			request := CatalogOverviewRequest{
				Stage: "merge", CustomerMessage: input.CustomerMessage,
				ConversationContext: input.ConversationContext,
				Summaries:           summaries[i : i+end], RequestedPresentation: input.Presentation,
			}
			result, err := ai.ComposeOverview(ctx, request)
			if err != nil {
				return CatalogEvaluationResult{}, fmt.Errorf("merge catalog overview round %d: %w", round+1, err)
			}
			c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, result.Usage, fmt.Sprintf("overview_merge_round_%d_group_%d", round+1, i))
			if strings.TrimSpace(result.Text) == "" || utf8.RuneCountInString(result.Text) > overviewMergeMaxRunes {
				return CatalogEvaluationResult{}, errors.New("Gemini returned an invalid intermediate overview digest")
			}
			next = append(next, result.Text)
			i += end
		}
		if len(next) >= len(summaries) {
			return CatalogEvaluationResult{}, errors.New("overview synthesis failed to reduce its summary count")
		}
		summaries = next
	}
	final := CatalogOverviewRequest{
		Stage: "final", CustomerMessage: input.CustomerMessage,
		ConversationContext: input.ConversationContext,
		Summaries:           summaries, RequestedPresentation: input.Presentation,
	}
	count, err := ai.CountOverviewTokens(ctx, final)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("count final overview input: %w", err)
	}
	if count > c.TokenBudget {
		return CatalogEvaluationResult{}, fmt.Errorf("final catalog overview exceeds verified model token budget: %d > %d", count, c.TokenBudget)
	}
	response, err := ai.ComposeOverview(ctx, final)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("Gemini final customer overview: %w", err)
	}
	c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, response.Usage, "overview_final")
	if strings.TrimSpace(response.Text) == "" || utf8.RuneCountInString(response.Text) > overviewFinalMaxRunes {
		return CatalogEvaluationResult{}, errors.New("Gemini final overview exceeded reply limit or returned invalid references")
	}
	after, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision after overview: %w", err)
	}
	if before != after {
		return CatalogEvaluationResult{}, errors.New("catalog changed during full overview; refusing stale information")
	}
	log.Printf("[CatalogBatch] OVERVIEW_AI_READY items=%d reviewed_batches=%d messages=1", itemCount, batchNumber)
	return CatalogEvaluationResult{
		Decision: ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityRespondWithoutAction, ResponseText: response.Text},
		Messages: []string{response.Text},
		Evidence: ports.NewCatalogAIEvidenceSet(),
	}, nil
}

func (c *CatalogBatchController) fitOverviewBatch(ctx context.Context, ai CatalogOverviewClient, input CatalogEvaluationInput, batch CatalogAIBatchPayload) ([]CatalogAIBatchPayload, error) {
	var fit func(CatalogAIBatchPayload) ([]CatalogAIBatchPayload, error)
	fit = func(part CatalogAIBatchPayload) ([]CatalogAIBatchPayload, error) {
		request := CatalogOverviewRequest{
			Stage: "batch", CustomerMessage: input.CustomerMessage,
			ConversationContext: input.ConversationContext,
			Batch:               &part, RequestedPresentation: input.Presentation,
		}
		count, err := ai.CountOverviewTokens(ctx, request)
		if err != nil {
			return nil, err
		}
		if count <= c.TokenBudget {
			return []CatalogAIBatchPayload{part}, nil
		}
		if len(part.Items) < 2 {
			return nil, fmt.Errorf("catalog overview item fragment exceeds verified model budget: item=%s estimated=%d", part.Items[0].ID, count)
		}
		mid := len(part.Items) / 2
		sub := CatalogAIProjection{Catalogs: part.Catalogs, AttributeSchemas: part.AttributeSchemas}
		left := subsetCatalogForReply(sub, part.Items[:mid])
		right := subsetCatalogForReply(sub, part.Items[mid:])
		a, err := fit(CatalogAIBatchPayload{Catalogs: left.Catalogs, AttributeSchemas: left.AttributeSchemas, Items: left.Items})
		if err != nil {
			return nil, err
		}
		b, err := fit(CatalogAIBatchPayload{Catalogs: right.Catalogs, AttributeSchemas: right.AttributeSchemas, Items: right.Items})
		if err != nil {
			return nil, err
		}
		return append(a, b...), nil
	}
	return fit(batch)
}

func (c *CatalogBatchController) fitOverviewSummaries(ctx context.Context, ai CatalogOverviewClient, input CatalogEvaluationInput, group []string) (int, error) {
	for n := len(group); n >= 1; n-- {
		request := CatalogOverviewRequest{
			Stage: "merge", CustomerMessage: input.CustomerMessage,
			ConversationContext: input.ConversationContext,
			Summaries:           group[:n], RequestedPresentation: input.Presentation,
		}
		count, err := ai.CountOverviewTokens(ctx, request)
		if err != nil {
			return 0, err
		}
		if count <= c.TokenBudget {
			return n, nil
		}
	}
	return 0, errors.New("a single catalog digest exceeds the verified overview context")
}
