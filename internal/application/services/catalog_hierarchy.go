package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogReplyComposer is implemented by an AI provider, not Go's renderer.
// The provider decides the Arabic customer-facing wording, explanation and
// comparison. Go only supplies verified complete commercial evidence.
type CatalogReplyComposer interface {
	CountCatalogReplyTokens(context.Context, CatalogReplyInput) (int, error)
	ComposeCatalogReply(context.Context, CatalogReplyInput) (CatalogReplyOutput, error)
}

type CatalogReplyInput struct {
	CustomerMessage     string
	ConversationContext ports.CustomerSalesContext
	Projection          CatalogAIProjection
	PartNumber          int
}

type CatalogReplyOutput struct {
	Text           string
	CoveredItemIDs []string
	Usage          ports.CustomerSalesUsageTelemetry
}

// Catalog replies must stay inside the text transport limit, including UTF-8
// Arabic characters. Never slice AI text or split an item in Go.
const catalogReplyMaxRunes = 1800
const catalogReplyMaxParts = 12

func (c *CatalogBatchController) runHierarchicalCatalog(ctx context.Context, input CatalogEvaluationInput) (CatalogEvaluationResult, error) {
	composer, ok := c.Gemini.(CatalogReplyComposer)
	if !ok {
		return CatalogEvaluationResult{}, errors.New("catalog model does not support AI-authored customer replies; refusing hardcoded hierarchy")
	}
	revision, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision before AI reply: %w", err)
	}
	var all CatalogAIProjection
	seen := make(map[string]struct{})
	cursor := ""
	visitedCursors := map[string]struct{}{}
	for {
		if err := ctx.Err(); err != nil {
			return CatalogEvaluationResult{}, err
		}
		page, err := c.CatalogAI.ListProjectionPage(ctx, ports.CatalogAIProjectionRequest{BusinessID: input.BusinessID, CatalogID: input.CatalogScope, Limit: 200, Cursor: cursor})
		if err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("read full catalog for AI reply: %w", err)
		}
		projection := ProjectionFromBundles(page.Items)
		appendCatalogRecordsToProjection(&projection, page.Catalogs)
		if err := validateCatalogBatchCoverage(projection, []CatalogAIBatchPayload{{Catalogs: projection.Catalogs, AttributeSchemas: projection.AttributeSchemas, Items: projection.Items}}); err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("verify customer-visible catalog evidence: %w", err)
		}
		for _, item := range projection.Items {
			if _, exists := seen[item.ID]; exists {
				return CatalogEvaluationResult{}, fmt.Errorf("repeated catalog item %s between pages", item.ID)
			}
			seen[item.ID] = struct{}{}
		}
		appendCatalogProjectionComplete(&all, projection)
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return CatalogEvaluationResult{}, errors.New("full catalog page cursor did not advance")
		}
		if _, exists := visitedCursors[page.NextCursor]; exists {
			return CatalogEvaluationResult{}, errors.New("full catalog page cursor cycled")
		}
		visitedCursors[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	if len(all.Items) == 0 {
		// Empty catalog still gets an AI-authored answer, with verified empty
		// evidence rather than a hardcoded customer message.
		all.Catalogs = nil
	}
	sort.SliceStable(all.Items, func(i, j int) bool {
		if all.Items[i].CatalogID != all.Items[j].CatalogID {
			return all.Items[i].CatalogID < all.Items[j].CatalogID
		}
		return all.Items[i].Name < all.Items[j].Name
	})

	parts, err := c.partitionCatalogForReply(ctx, composer, input, all)
	if err != nil {
		return CatalogEvaluationResult{}, err
	}
	if len(parts) > catalogReplyMaxParts {
		return CatalogEvaluationResult{}, fmt.Errorf("full catalog requires %d AI reply parts; request a category-level presentation instead of flooding the conversation", len(parts))
	}

	messages := make([]string, 0, len(parts))
	for i, projection := range parts {
		request := CatalogReplyInput{
			CustomerMessage: input.CustomerMessage, ConversationContext: input.ConversationContext,
			Projection: projection, PartNumber: i + 1,
		}
		result, err := composer.ComposeCatalogReply(ctx, request)
		if err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("Gemini catalog reply part %d: %w", i+1, err)
		}
		// Meter every successful call, even when its response is rejected.
		c.recordBatchUsage(ctx, input.BusinessID, input.AIRunID, result.Usage, fmt.Sprintf("catalog_reply_part_%d", i+1))
		if err := validateCatalogReplyCoverage(projection, result); err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("Gemini reply part %d failed coverage: %w", i+1, err)
		}
		if utf8.RuneCountInString(result.Text) > catalogReplyMaxRunes {
			return CatalogEvaluationResult{}, fmt.Errorf("Gemini reply part %d is longer than the channel text limit", i+1)
		}
		messages = append(messages, result.Text)
	}
	after, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision after AI reply: %w", err)
	}
	if after != revision {
		return CatalogEvaluationResult{}, errors.New("catalog changed while Gemini was composing the reply; refusing stale prices or offers")
	}
	log.Printf("[CatalogBatch] HIERARCHY_AI_READY items=%d parts=%d", len(all.Items), len(messages))
	return CatalogEvaluationResult{
		Decision: ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityRespondWithoutAction, ResponseText: messages[0]},
		Evidence: EvidenceFromProjection(all),
		Messages: messages,
	}, nil
}

func appendCatalogProjectionComplete(dst *CatalogAIProjection, src CatalogAIProjection) {
	existingCatalogs := make(map[string]struct{}, len(dst.Catalogs))
	for _, catalog := range dst.Catalogs {
		existingCatalogs[catalog.ID] = struct{}{}
	}
	for _, catalog := range src.Catalogs {
		if _, ok := existingCatalogs[catalog.ID]; !ok {
			dst.Catalogs = append(dst.Catalogs, catalog)
			existingCatalogs[catalog.ID] = struct{}{}
		}
	}
	existingSchemas := make(map[string]struct{}, len(dst.AttributeSchemas))
	for _, schema := range dst.AttributeSchemas {
		existingSchemas[schema.ID] = struct{}{}
	}
	for _, schema := range src.AttributeSchemas {
		if _, ok := existingSchemas[schema.ID]; !ok {
			dst.AttributeSchemas = append(dst.AttributeSchemas, schema)
			existingSchemas[schema.ID] = struct{}{}
		}
	}
	dst.Items = append(dst.Items, src.Items...)
}

func (c *CatalogBatchController) partitionCatalogForReply(
	ctx context.Context, composer CatalogReplyComposer, input CatalogEvaluationInput, projection CatalogAIProjection,
) ([]CatalogAIProjection, error) {
	var output []CatalogAIProjection
	var split func([]CatalogAIItem) error
	split = func(items []CatalogAIItem) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		subset := subsetCatalogForReply(projection, items)
		req := CatalogReplyInput{CustomerMessage: input.CustomerMessage, ConversationContext: input.ConversationContext, Projection: subset}
		count, err := composer.CountCatalogReplyTokens(ctx, req)
		if err != nil {
			return fmt.Errorf("count Gemini reply tokens: %w", err)
		}
		if count <= c.TokenBudget {
			output = append(output, subset)
			if len(output) > catalogReplyMaxParts {
				return errors.New("too many complete AI-authored catalog messages for one conversation reply")
			}
			return nil
		}
		if len(items) < 2 {
			return fmt.Errorf("catalog item %s cannot fit the AI provider's verified context without losing commercial data", items[0].ID)
		}
		mid := len(items) / 2
		if err := split(items[:mid]); err != nil {
			return err
		}
		return split(items[mid:])
	}
	if len(projection.Items) == 0 {
		return []CatalogAIProjection{{}}, nil
	}
	if c.TokenBudget <= 0 {
		return nil, errors.New("catalog AI reply token budget must be positive")
	}
	if err := split(projection.Items); err != nil {
		return nil, err
	}
	for _, part := range output {
		if err := validateCatalogBatchCoverage(part, []CatalogAIBatchPayload{{Catalogs: part.Catalogs, AttributeSchemas: part.AttributeSchemas, Items: part.Items}}); err != nil {
			return nil, fmt.Errorf("lossless Gemini reply partition: %w", err)
		}
	}
	return output, nil
}

func subsetCatalogForReply(full CatalogAIProjection, items []CatalogAIItem) CatalogAIProjection {
	schemas := make(map[string]struct{})
	catalogs := make(map[string]struct{})
	for _, item := range items {
		catalogs[item.CatalogID] = struct{}{}
		if item.AttributeSchemaID != nil {
			schemas[*item.AttributeSchemaID] = struct{}{}
		}
	}
	subset := CatalogAIProjection{Items: items}
	for _, catalog := range full.Catalogs {
		if _, ok := catalogs[catalog.ID]; ok {
			subset.Catalogs = append(subset.Catalogs, catalog)
		}
	}
	for _, schema := range full.AttributeSchemas {
		if _, ok := schemas[schema.ID]; ok {
			subset.AttributeSchemas = append(subset.AttributeSchemas, schema)
		}
	}
	return subset
}

func validateCatalogReplyCoverage(source CatalogAIProjection, out CatalogReplyOutput) error {
	if strings.TrimSpace(out.Text) == "" {
		return errors.New("empty Gemini customer reply")
	}
	expected := make(map[string]struct{}, len(source.Items))
	for _, item := range source.Items {
		expected[item.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(out.CoveredItemIDs))
	for _, id := range out.CoveredItemIDs {
		if _, exists := expected[id]; !exists {
			return fmt.Errorf("Gemini referenced item %s outside this catalog page", id)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("Gemini repeated item evidence %s", id)
		}
		seen[id] = struct{}{}
	}
	for id := range expected {
		if _, covered := seen[id]; !covered {
			return fmt.Errorf("Gemini omitted item %s from its customer response", id)
		}
	}
	return nil
}
