package services

import (
	"context"
	"encoding/json"
	"math/big"
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
	catalogs map[string][]catalogCustomerItem
	empty    string
	runes    int
}

type catalogCustomerItem struct {
	name   string
	lines  []string
	prices []catalogCustomerPrice
}

type catalogCustomerPrice struct {
	amount   string
	currency string
}

func (p *catalogHierarchyLines) add(message string) {
	p.empty = message
	p.runes += utf8.RuneCountInString(message)
}

func (p *catalogHierarchyLines) tooLong() bool {
	// The overview is bounded before enqueueing; avoid a huge per-turn
	// response. Source catalog facts remain available in the repository.
	return p.runes > 300000
}

// messages forms coherent customer-facing sections. A product is never
// split across messages: when a product doesn't fit, we continue from
// that product in the next part with the catalog heading repeated.
func (p *catalogHierarchyLines) messages() ([]string, error) {
	if len(p.catalogs) == 0 {
		text := p.empty
		if text == "" {
			text = "لا توجد منتجات أو خدمات معروضة حاليًا."
		}
		return []string{text}, nil
	}
	names := make([]string, 0, len(p.catalogs))
	total := 0
	for name, items := range p.catalogs {
		names = append(names, name)
		total += len(items)
	}
	sort.Strings(names)
	for _, name := range names {
		sort.SliceStable(p.catalogs[name], func(i, j int) bool {
			return p.catalogs[name][i].name < p.catalogs[name][j].name
		})
	}
	const maximumMessages = 4
	var chunks []string
	var current strings.Builder
	begin := func() {
		current.WriteString(fmt.Sprintf("أهلًا وسهلًا! لدينا %d منتجًا وخدمة، وهذه نظرة مرتبة حسب الأقسام وأسعار الخيارات.\n", total))
	}
	begin()
	for _, name := range names {
		items := p.catalogs[name]
		header := "\n📂 " + name + "\n"
		headerWritten := false
		for _, item := range items {
			block := "• " + item.name + "\n" + strings.Join(item.lines, "\n") + "\n"
			if utf8.RuneCountInString(block)+utf8.RuneCountInString(header) > catalogHierarchyPartRunes {
				return nil, fmt.Errorf("catalog item %q cannot fit in one customer-facing message without splitting its commercial offers", item.name)
			}
			needed := utf8.RuneCountInString(block)
			if !headerWritten {
				needed += utf8.RuneCountInString(header)
			}
			if current.Len() > 0 && utf8.RuneCountInString(current.String())+needed > catalogHierarchyPartRunes {
				chunks = append(chunks, strings.TrimSpace(current.String()))
				current.Reset()
				headerWritten = false
			}
			if !headerWritten {
				current.WriteString(header)
				headerWritten = true
			}
			current.WriteString(block)
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	if len(chunks) == 0 || len(chunks) > maximumMessages {
		return nil, fmt.Errorf("complete customer-facing catalog requires %d messages; use a paginated category overview instead of a multi-message flood", len(chunks))
	}
	footer := "\nالأسعار المعروضة من بيانات المتجر؛ السعر أو التوفر الذي لم يُتحقق منه يحتاج تأكيدًا قبل إتمام الطلب. إذا رغبت، أشرح لك الفروقات والمميزات التفصيلية لأي قسم أو منتج."
	if utf8.RuneCountInString(chunks[len(chunks)-1])+utf8.RuneCountInString(footer) <= catalogHierarchyPartRunes {
		chunks[len(chunks)-1] += footer
	} else {
		chunks[len(chunks)-1] += "\nللمعلومات التفصيلية أو التأكيد، اسألني عن القسم الذي يهمك."
	}
	for i := range chunks {
		if len(chunks) > 1 {
			chunks[i] = fmt.Sprintf("الخيارات المتاحة (%d/%d)\n%s", i+1, len(chunks), chunks[i])
		}
	}
	return chunks, nil
}

func (p *catalogHierarchyLines) appendItem(catalogName string, item CatalogAIItem, schema *CatalogAIAttributeSchema) error {
	if strings.TrimSpace(catalogName) == "" || strings.TrimSpace(item.Name) == "" {
		return errors.New("catalog or item has no customer-facing name")
	}
	if p.catalogs == nil {
		p.catalogs = make(map[string][]catalogCustomerItem)
	}
	info := catalogCustomerItem{name: item.Name}
	if item.ShortDescription != nil && strings.TrimSpace(*item.ShortDescription) != "" {
		info.lines = append(info.lines, "  "+strings.TrimSpace(*item.ShortDescription))
	} else if item.LongDescription != nil && strings.TrimSpace(*item.LongDescription) != "" {
		// Avoid sending hundreds of description words. The complete merchant
		// description remains in the catalog and can be requested explicitly.
		info.lines = append(info.lines, "  "+catalogPreview(*item.LongDescription, 150))
	}
	if len(item.Attributes) > 0 {
		info.lines = append(info.lines, "  أبرز المواصفات: "+formatHierarchyAttributes(item.Attributes, schema))
	}
	variants := map[string]CatalogAIVariant{}
	for _, v := range item.Variants {
		if v.CatalogItemID != item.ID {
			return fmt.Errorf("variant %s belongs to another catalog item", v.ID)
		}
		if _, found := variants[v.ID]; found {
			return fmt.Errorf("duplicate catalog variant %s", v.ID)
		}
		variants[v.ID] = v
	}
	for _, offer := range item.Offers {
		if offer.CatalogItemID != item.ID {
			return fmt.Errorf("offer %s belongs to another catalog item", offer.ID)
		}
		name := strings.TrimSpace(offer.Name)
		if offer.VariantID != nil && *offer.VariantID != "" {
			v, found := variants[*offer.VariantID]
			if !found {
				return fmt.Errorf("offer %s has no valid item variant", offer.ID)
			}
			if strings.TrimSpace(v.Name) != "" {
				name = strings.TrimSpace(v.Name)
			}
		}
		if name == "" {
			name = "العرض"
		}
		price := formatCatalogCustomerPrice(offer.Amount, offer.Currency)
		info.lines = append(info.lines, "  - "+name+": "+price+catalogOfferQualifier(offer))
		if offer.Amount != nil && offer.Currency != nil {
			info.prices = append(info.prices, catalogCustomerPrice{amount: *offer.Amount, currency: *offer.Currency})
		}
	}
	if len(item.Offers) == 0 {
		info.lines = append(info.lines, "  السعر غير معلن؛ يمكن الاستفسار عنه.")
	}
	if item.RequiresConfirmation {
		info.lines = append(info.lines, "  يحتاج تأكيدًا قبل إتمام الطلب.")
	}
	p.catalogs[catalogName] = append(p.catalogs[catalogName], info)
	p.runes += utf8.RuneCountInString(item.Name)
	for _, line := range info.lines {
		p.runes += utf8.RuneCountInString(line) + 1
	}
	return nil
}

func catalogPreview(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	r := []rune(text)
	return strings.TrimSpace(string(r[:max])) + "…"
}

// Attribute labels come from Universal Catalog schema definitions, never from
// an industry-specific keyword classifier. Fallback keys are converted into
// readable words rather than exposing snake_case.
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
		value, err := json.Marshal(attributes[key])
		if err != nil {
			continue
		}
		label := labels[key]
		if label == "" {
			label = strings.ReplaceAll(key, "_", " ")
		}
		rendered := strings.Trim(string(value), "\"")
		switch rendered {
		case "true":
			rendered = "نعم"
		case "false":
			rendered = "لا"
		}
		parts = append(parts, label+": "+rendered)
	}
	return catalogPreview(strings.Join(parts, "، "), 170)
}

func formatCatalogCustomerPrice(amount, currency *string) string {
	if amount == nil || strings.TrimSpace(*amount) == "" {
		return "السعر حسب الطلب"
	}
	v := strings.TrimSpace(*amount)
	n := new(big.Rat)
	if _, ok := n.SetString(v); ok {
		bits := strings.SplitN(v, ".", 2)
		intPart := bits[0]
		sign := ""
		if strings.HasPrefix(intPart, "-") {
			sign, intPart = "-", strings.TrimPrefix(intPart, "-")
		}
		for i := len(intPart)-3; i > 0; i -= 3 {
			intPart = intPart[:i] + "," + intPart[i:]
		}
		v = sign + intPart
		if len(bits) == 2 {
			decimal := strings.TrimRight(bits[1], "0")
			if decimal != "" {
				v += "." + decimal
			}
		}
	}
	unit := ""
	if currency != nil {
		switch strings.ToUpper(*currency) {
		case "YER":
			unit = " ريال يمني"
		case "SAR":
			unit = " ريال سعودي"
		case "USD":
			unit = " دولار أمريكي"
		default:
			if *currency != "" {
				unit = " " + *currency
			}
		}
	}
	return v + unit
}

func catalogOfferQualifier(offer CatalogAIOffer) string {
	var labels []string
	if offer.PricingUnit != nil && strings.TrimSpace(*offer.PricingUnit) != "" {
		switch *offer.PricingUnit {
		case "person", "per_person":
			labels = append(labels, "للشخص")
		case "per_day", "day":
			labels = append(labels, "لليوم")
		case "each":
			labels = append(labels, "للوحدة")
		case "per_unit":
			labels = append(labels, "لكل وحدة")
		default:
			// Unknown merchant-defined units are not interpreted as promises.
		}
	}
	if offer.PriceVerificationStatus != nil && *offer.PriceVerificationStatus != "verified" {
		labels = append(labels, "السعر يحتاج تأكيدًا")
	}
	switch {
	case offer.AvailabilityStatus == nil:
		labels = append(labels, "التوفر يحتاج تأكيدًا")
	case *offer.AvailabilityStatus == "requires_check", *offer.AvailabilityStatus == "stale", *offer.AvailabilityStatus == "unknown":
		labels = append(labels, "التوفر يحتاج تأكيدًا")
	case *offer.AvailabilityStatus == "unavailable":
		labels = append(labels, "غير متوفر حاليًا")
	}
	if len(labels) == 0 {
		return ""
	}
	return " (" + strings.Join(labels, "، ") + ")"
}
