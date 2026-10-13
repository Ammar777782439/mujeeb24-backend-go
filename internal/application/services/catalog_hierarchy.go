package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// The provider-specific text limit is intentionally below the smallest
// Messenger-family text envelope. All parts are pre-built before enqueueing:
// exceeding the safety ceiling fails the entire run without a partial catalog.
const catalogHierarchyPartRunes = 1800
const catalogHierarchyMaxParts = 80

// runHierarchicalCatalog is an explicit display operation selected by Gemini,
// NOT keyword-based intent inference in Go. It reads each active catalog item
// using the existing tenant-scoped Universal Catalog projection. It never uses
// candidate reduction or asks a model to reproduce merchant commercial facts.
func (c *CatalogBatchController) runHierarchicalCatalog(ctx context.Context, input CatalogEvaluationInput) (CatalogEvaluationResult, error) {
	revision, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision before hierarchy: %w", err)
	}
	cursor := ""
	seenCursors := map[string]struct{}{}
	seenItems := map[string]struct{}{}
	catalogs := map[string]CatalogAICatalog{}
	schemas := map[string]CatalogAIAttributeSchema{}
	parts := &catalogHierarchyLines{}
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return CatalogEvaluationResult{}, err
		}
		page, err := c.CatalogAI.ListProjectionPage(ctx, ports.CatalogAIProjectionRequest{
			BusinessID: input.BusinessID, CatalogID: input.CatalogScope, Limit: 200, Cursor: cursor,
		})
		if err != nil {
			return CatalogEvaluationResult{}, fmt.Errorf("read full hierarchy page: %w", err)
		}
		projection := ProjectionFromBundles(page.Items)
		appendCatalogRecordsToProjection(&projection, page.Catalogs)
		for _, catalog := range projection.Catalogs {
			catalogs[catalog.ID] = catalog
		}
		for _, schema := range projection.AttributeSchemas {
			schemas[schema.ID] = schema
		}
		for _, item := range projection.Items {
			if _, exists := seenItems[item.ID]; exists {
				return CatalogEvaluationResult{}, fmt.Errorf("duplicate catalog item %s across hierarchy pages", item.ID)
			}
			seenItems[item.ID] = struct{}{}
			catalog, ok := catalogs[item.CatalogID]
			if !ok {
				return CatalogEvaluationResult{}, fmt.Errorf("missing catalog %s for hierarchy item %s", item.CatalogID, item.ID)
			}
			var schema *CatalogAIAttributeSchema
			if item.AttributeSchemaID != nil {
				found, ok := schemas[*item.AttributeSchemaID]
				if !ok {
					return CatalogEvaluationResult{}, fmt.Errorf("missing attribute schema for item %s", item.ID)
				}
				schema = &found
			}
			if err := parts.appendItem(catalog.Name, item, schema); err != nil {
				return CatalogEvaluationResult{}, err
			}
			total++
			if parts.tooLong() {
				return CatalogEvaluationResult{}, errors.New("hierarchical catalog exceeds safe per-turn message limit; complete listing requires paginated customer navigation")
			}
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return CatalogEvaluationResult{}, errors.New("hierarchical catalog cursor did not advance")
		}
		if _, exists := seenCursors[page.NextCursor]; exists {
			return CatalogEvaluationResult{}, errors.New("hierarchical catalog cursor cycle")
		}
		seenCursors[page.NextCursor] = struct{}{}
		cursor = page.NextCursor
	}
	revisionAfter, err := c.CatalogAI.GetRevision(ctx, input.BusinessID)
	if err != nil {
		return CatalogEvaluationResult{}, fmt.Errorf("read catalog revision after hierarchy: %w", err)
	}
	if revisionAfter != revision {
		return CatalogEvaluationResult{}, errors.New("catalog changed while building hierarchy; refusing mixed commercial facts")
	}
	if total == 0 {
		parts.add("لا توجد منتجات أو خدمات نشطة معروضة حاليًا.")
	}
	messages, err := parts.messages()
	if err != nil {
		return CatalogEvaluationResult{}, err
	}
	log.Printf("[CatalogBatch] HIERARCHY_READY items=%d parts=%d", total, len(messages))
	return CatalogEvaluationResult{
		Decision: ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityRespondWithoutAction, ResponseText: messages[0]},
		Messages: messages,
		Evidence: ports.NewCatalogAIEvidenceSet(),
	}, nil
}

type catalogHierarchyLines struct {
	lines []string
	runes int
}
func (p *catalogHierarchyLines) add(value string) {
	p.lines = append(p.lines, value)
	p.runes += utf8.RuneCountInString(value) + 1
}
func (p *catalogHierarchyLines) tooLong() bool {
	return p.runes > catalogHierarchyPartRunes*catalogHierarchyMaxParts
}

// messages preserves all generated facts. Text exceeding one part is split
// on rune boundaries (not byte boundaries), without truncating descriptions.
func (p *catalogHierarchyLines) messages() ([]string, error) {
	chunks := make([]string, 0, 4)
	var part strings.Builder
	flush := func() {
		if part.Len() > 0 {
			chunks = append(chunks, part.String())
			part.Reset()
		}
	}
	partRunes := 0
	for _, line := range p.lines {
		remaining := []rune(line + "\n")
		for len(remaining) > 0 {
			room := catalogHierarchyPartRunes - partRunes
			if room == 0 {
				flush()
				partRunes = 0
				room = catalogHierarchyPartRunes
			}
			take := len(remaining)
			if take > room {
				take = room
			}
			part.WriteString(string(remaining[:take]))
			partRunes += take
			remaining = remaining[take:]
		}
	}
	flush()
	if len(chunks) == 0 || len(chunks) > catalogHierarchyMaxParts {
		return nil, errors.New("hierarchical catalog exceeds safe multi-message limit")
	}
	out := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		header := "📚 الكتالوج الكامل"
		if len(chunks) > 1 {
			header = fmt.Sprintf("📚 الكتالوج الكامل — الجزء %d من %d", i+1, len(chunks))
		}
		out = append(out, header+"\n"+chunk)
	}
	return out, nil
}

func (p *catalogHierarchyLines) appendItem(catalogName string, item CatalogAIItem, schema *CatalogAIAttributeSchema) error {
	p.add("")
	p.add("📁 " + catalogName)
	p.add("  ◉ " + item.Name)
	if item.ItemType != "" {
		p.add("    النوع: " + item.ItemType)
	}
	if item.ShortDescription != nil && *item.ShortDescription != "" {
		p.add("    الوصف: " + *item.ShortDescription)
	}
	if item.LongDescription != nil && *item.LongDescription != "" {
		p.add("    التفاصيل: " + *item.LongDescription)
	}
	if len(item.Attributes) > 0 {
		p.add("    المواصفات: " + formatHierarchyAttributes(item.Attributes, schema))
	}
	if item.RequiresConfirmation {
		p.add("    يتطلب التأكيد قبل التنفيذ")
	}
	variantByID := make(map[string]CatalogAIVariant, len(item.Variants))
	for _, variant := range item.Variants {
		if variant.CatalogItemID != item.ID {
			return fmt.Errorf("variant %s has wrong parent item", variant.ID)
		}
		variantByID[variant.ID] = variant
	}
	for _, offer := range item.Offers {
		if offer.CatalogItemID != item.ID {
			return fmt.Errorf("offer %s has wrong parent item", offer.ID)
		}
		if offer.VariantID != nil && *offer.VariantID != "" {
			if _, ok := variantByID[*offer.VariantID]; !ok {
				return fmt.Errorf("offer %s references missing item variant", offer.ID)
			}
		}
	}
	for _, variant := range item.Variants {
		name := variant.Name
		if name == "" {
			name = "خيار"
		}
		p.add("    ├─ " + name)
		if len(variant.Attributes) > 0 {
			p.add("    │  المواصفات: " + formatHierarchyAttributes(variant.Attributes, schema))
		}
		for _, offer := range item.Offers {
			if offer.VariantID != nil && *offer.VariantID == variant.ID {
				p.add("    │  └─ " + formatHierarchyOffer(offer))
			}
		}
	}
	for _, offer := range item.Offers {
		if offer.VariantID == nil || *offer.VariantID == "" {
			p.add("    └─ " + formatHierarchyOffer(offer))
		}
	}
	return nil
}

func formatHierarchyAttributes(attributes map[string]any, schema *CatalogAIAttributeSchema) string {
	labels := make(map[string]string)
	if schema != nil {
		for _, def := range schema.Definitions {
			if def.Label != "" {
				labels[def.AttributeKey] = def.Label
			}
		}
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		label := key
		if known := labels[key]; known != "" {
			label = known
		}
		value, err := json.Marshal(attributes[key])
		if err != nil {
			value = []byte("null")
		}
		parts = append(parts, label+": "+strings.Trim(string(value), "\"" ))
	}
	return strings.Join(parts, "، ")
}

func formatHierarchyOffer(offer CatalogAIOffer) string {
	var p []string
	if offer.Name != "" {
		p = append(p, offer.Name)
	} else {
		p = append(p, "عرض")
	}
	if offer.Amount != nil && *offer.Amount != "" {
		price := *offer.Amount
		if offer.Currency != nil && *offer.Currency != "" {
			price += " " + *offer.Currency
		}
		p = append(p, "السعر: "+price)
	} else {
		p = append(p, "السعر: غير محدد")
	}
	if offer.PricingMode != "" {
		p = append(p, "نوع التسعير: "+offer.PricingMode)
	}
	if offer.PricingUnit != nil && *offer.PricingUnit != "" {
		p = append(p, "وحدة السعر: "+*offer.PricingUnit)
	}
	if offer.PriceVerificationStatus == nil || *offer.PriceVerificationStatus != "verified" {
		status := "غير مؤكد"
		if offer.PriceVerificationStatus != nil && *offer.PriceVerificationStatus != "" {
			status = *offer.PriceVerificationStatus
		}
		p = append(p, "تحقق السعر: "+status)
	}
	if offer.AvailabilityStatus != nil && *offer.AvailabilityStatus != "" {
		p = append(p, "التوفر: "+*offer.AvailabilityStatus)
	} else {
		p = append(p, "التوفر: غير مؤكد")
	}
	if offer.ValidityUntil != nil && *offer.ValidityUntil != "" {
		p = append(p, "صالح حتى: "+*offer.ValidityUntil)
	}
	if offer.FulfillmentMode != nil && *offer.FulfillmentMode != "" {
		p = append(p, "التسليم: "+*offer.FulfillmentMode)
	}
	return strings.Join(p, " | ")
}
