package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// MerchantAICapabilityRegistry builds a complete registry of tools specifically for the Merchant AI Copilot.
func NewMerchantAICapabilityRegistry(
	catalogRepo ports.CatalogRepository,
	authorItemHandler commands.AuthorCatalogItemHandler,
	createCatalogHandler commands.CreateCatalogHandler,
	updateItemHandler commands.UpdateCatalogItemHandler,
	updateOfferHandler commands.UpdateOfferHandler,
	createVariantHandler commands.CreateVariantHandler,
) *CapabilityRegistry {
	registry := NewCapabilityRegistry()

	_ = registry.Register(&merchantListCatalogsCapability{repo: catalogRepo})
	_ = registry.Register(&merchantCreateCatalogCapability{handler: createCatalogHandler, repo: catalogRepo})
	_ = registry.Register(&merchantAuthorCatalogItemCapability{handler: authorItemHandler, repo: catalogRepo})
	_ = registry.Register(&merchantSearchCatalogCapability{repo: catalogRepo})
	_ = registry.Register(&merchantGetItemDetailsCapability{repo: catalogRepo})
	_ = registry.Register(&merchantCreateVariantCapability{handler: createVariantHandler, repo: catalogRepo})
	_ = registry.Register(&merchantUpdateCatalogItemCapability{updateItem: updateItemHandler, updateOffer: updateOfferHandler, repo: catalogRepo})
	_ = registry.Register(&merchantArchiveCatalogItemCapability{updateItem: updateItemHandler, repo: catalogRepo})
	_ = registry.Register(&merchantParseFileCapability{authorHandler: authorItemHandler, repo: catalogRepo})

	return registry
}

// ----------------------------------------------------------------------
// 1. list_catalogs
// ----------------------------------------------------------------------
type merchantListCatalogsCapability struct {
	repo ports.CatalogRepository
}

func (c *merchantListCatalogsCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "list_catalogs",
		Description: "جلب قائمة الكتالوجات المتاحة لمتجر التاجر لمعرفة أسمائها وحالاتها وسؤال التاجر في أي كتالوج يود الإضافة.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "عدد الكتالوجات المطلوبة (افتراضي 20)",
				},
			},
		},
	}
}

func (c *merchantListCatalogsCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	if c.repo == nil {
		return ports.AICapabilityResult{}, errors.New("catalog repository is not configured")
	}
	limit := 20
	if len(rawParams) > 0 {
		var input struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(rawParams, &input)
		if input.Limit > 0 {
			limit = input.Limit
		}
	}

	page, err := c.repo.ListCatalogs(ctx, execCtx.BusinessID, "", limit, "")
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	type catalogSummary struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	items := make([]catalogSummary, 0, len(page.Items))
	for _, cat := range page.Items {
		items = append(items, catalogSummary{
			ID:     cat.ID,
			Name:   cat.Name,
			Status: cat.Status,
		})
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"catalogs": items,
			"total":    len(items),
		},
	}, nil
}

// ----------------------------------------------------------------------
// 2. create_catalog
// ----------------------------------------------------------------------
type merchantCreateCatalogCapability struct {
	handler commands.CreateCatalogHandler
	repo    ports.CatalogRepository
}

func (c *merchantCreateCatalogCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "create_catalog",
		Description: "إنشاء كتالوج جديد لمتجر التاجر بالاسم والوصف.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "اسم الكتالوج الجديد (إلزامي)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "وصف مختصر للكتالوج (اختياري)",
				},
			},
			"required": []string{"name"},
		},
	}
}

func (c *merchantCreateCatalogCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	var input struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid create_catalog parameters: %w", err)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ports.AICapabilityResult{}, errors.New("catalog name is required")
	}

	if c.handler != nil {
		desc := ""
		if input.Description != nil {
			desc = *input.Description
		}
		cmd := commands.CreateCatalogCommand{
			Meta: commands.CommandMeta{
				Actor: commands.ActorContext{
					BusinessID: commands.BusinessID(execCtx.BusinessID),
					Role:       "owner",
				},
			},
			Name:        name,
			Description: desc,
		}
		res, err := c.handler.Handle(ctx, cmd)
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		return ports.AICapabilityResult{
			Data: map[string]any{
				"success":    true,
				"catalog_id": string(res.Catalog.ID),
				"name":       res.Catalog.Name,
				"status":     res.Catalog.Status,
			},
		}, nil
	}

	if c.repo != nil {
		now := time.Now().UTC()
		rec, err := c.repo.CreateCatalog(ctx, ports.CatalogDraft{
			ID:          uuid.NewString(),
			BusinessID:  execCtx.BusinessID,
			Name:        name,
			Description: input.Description,
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		return ports.AICapabilityResult{
			Data: map[string]any{
				"success":    true,
				"catalog_id": rec.ID,
				"name":       rec.Name,
				"status":     rec.Status,
			},
		}, nil
	}

	return ports.AICapabilityResult{}, errors.New("create_catalog handler is not configured")
}

// ----------------------------------------------------------------------
// 3. author_catalog_item
// ----------------------------------------------------------------------
type merchantAuthorCatalogItemCapability struct {
	handler commands.AuthorCatalogItemHandler
	repo    ports.CatalogRepository
}

func (c *merchantAuthorCatalogItemCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "author_catalog_item",
		Description: "إنشاء منتج أو خدمة جديدة في كتالوج المتجر مع كافة المتغيرات والعروض والأسعار والخصائص دفعة واحدة في معاملة ذرية تامة.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"catalog_id": map[string]any{
					"type":        "string",
					"description": "معرف الكتالوج المراد الإضافة فيه (UUID). إذا لم يحدد يُستخدم الكتالوج الافتراضي تلقائياً.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "اسم المنتج أو الخدمة التجاري الأساسي (مثال: عسل سدر دوعني، تيشرت قطني، شقة مفروشة VIP). إلزامي.",
				},
				"item_type": map[string]any{
					"type":        "string",
					"enum":        []string{"product", "service", "dish", "apartment", "car", "tour", "custom"},
					"description": "نوع الصنف التجاري بدقة: product (بضاعة مادية: عسل، ملابس، عطور، هواتف)، service (خدمة أو صيانة أو استشارة)، dish (وجبة أو مشروب في مطعم/كافيه)، apartment (شقة مفروشة أو فندق أو عقار)، car (تأجير سيارات أو نقل)، tour (رحلة سياحية أو حجز)، custom (صنف مخصص).",
				},
				"pricing_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "quote_required", "dynamic"},
					"description": "نمط التسعير العام: fixed (سعر مقطوع ثابت للصنف)، per_unit (سعر بالوحدة/الكيلو/العلبة)، per_day (سعر باليوم للشقق والسيارات)، per_person (سعر للفرد)، starting_from (سعر يبدأ من حد أدنى).",
				},
				"availability_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"stock", "schedule", "supplier_check", "always_available", "unknown"},
					"description": "نمط توفر المخزون: stock (بضاعة مادية موجودة في المخزن أو المحل)، schedule (مواعيد وجدولة زمنية)، supplier_check (طلب من المورد)، always_available (متاح دائماً كالمنتجات الرقمية).",
				},
				"fulfillment_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"delivery", "pickup", "digital", "appointment", "travel", "manual"},
					"description": "نمط التسليم والاستلام: delivery (توصيل وشحن للعميل)، pickup (استلام محلي من مقر المحل)، appointment (حضور بموعد للفرع)، digital (تسليم رقمي).",
				},
				"requires_confirmation": map[string]any{
					"type":        "boolean",
					"description": "هل يتطلب تأكيد يدوي من التاجر عند الطلب؟ (افتراضي false)",
				},
				"attributes": map[string]any{
					"type":        "object",
					"description": "خصائص إضافية مخصصة للقطاع ككائن JSON (مثال: {\"category\": \"honey\", \"origin\": \"دوعن\", \"grade\": \"درجة أولى\", \"color\": \"أحمر\", \"model_year\": 2024})",
				},
				"variants": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "اسم المتغير الفرعي بالكامل (مثال: دبة كبيرة، علبة صغيرة، نص كيلو، أحمر / XL، 256GB)",
							},
							"attributes": map[string]any{
								"type":        "object",
								"description": "كائن JSON لخصائص المتغير (مثال: {\"size\": \"large\", \"unit\": \"دبة\", \"color\": \"red\"})",
							},
						},
						"required": []string{"name"},
					},
					"description": "قائمة المتغيرات والأحجام والمقاسات والألوان المتاحة للمنتج في حال وجود أكثر من خيار.",
				},
				"offers": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "اسم عرض السعر (مثال: سعر الدبة الكبيرة، سعر العلبة الصغيرة، سعر الحبة)",
							},
							"variant_name": map[string]any{
								"type":        "string",
								"description": "اسم المتغير المطابق تماماً للحقل name في مصفوفة variants لتخصيص هذا السعر له حصراً (مثال: دبة كبيرة)",
							},
							"amount": map[string]any{
								"type":        "number",
								"description": "المبلغ النقدي بالعملة الأساسية بدقة (مثال: 60000 أو 15000 أو 8000)",
							},
							"currency": map[string]any{
								"type":        "string",
								"enum":        []string{"YER", "SAR", "USD"},
								"description": "رمز العملة: YER (ريال يمني - افتراضي)، SAR (ريال سعودي)، USD (دولار أمريكي)",
							},
							"pricing_mode": map[string]any{
								"type":        "string",
								"enum":        []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "quote_required", "dynamic"},
								"description": "طريقة التسعير لهذا العرض",
							},
							"pricing_unit": map[string]any{
								"type":        "string",
								"description": "وحدة التسعير المربوطة بالسعر (مثال: دبة، علبة، حبة، كيلو، يوم، شهر، شخص)",
							},
							"availability_status": map[string]any{
								"type":        "string",
								"enum":        []string{"available", "unavailable", "requires_check", "stale"},
								"description": "حالة التوفر الحالية (افتراضي available)",
							},
						},
						"required": []string{"name", "amount", "currency"},
					},
					"description": "قائمة عروض الأسعار والتسعير المربوطة بالمنتج أو بالمتغيرات.",
				},
			},
			"required": []string{"name"},
		},
	}
}

func (c *merchantAuthorCatalogItemCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	if c.handler == nil {
		return ports.AICapabilityResult{}, errors.New("author catalog item handler is not configured")
	}

	var input struct {
		CatalogID            string         `json:"catalog_id"`
		Name                 string         `json:"name"`
		ItemType             string         `json:"item_type"`
		PricingMode          string         `json:"pricing_mode"`
		AvailabilityMode     string         `json:"availability_mode"`
		FulfillmentMode      string         `json:"fulfillment_mode"`
		RequiresConfirmation bool           `json:"requires_confirmation"`
		Attributes           map[string]any `json:"attributes"`
		Variants             []struct {
			Name       string         `json:"name"`
			Attributes map[string]any `json:"attributes"`
		} `json:"variants"`
		Offers []struct {
			Name               string   `json:"name"`
			VariantName        *string  `json:"variant_name"`
			Amount             *float64 `json:"amount"`
			Currency           *string  `json:"currency"`
			PricingMode        string   `json:"pricing_mode"`
			PricingUnit        *string  `json:"pricing_unit"`
			AvailabilityStatus string   `json:"availability_status"`
		} `json:"offers"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid author_catalog_item parameters: %w", err)
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ports.AICapabilityResult{}, errors.New("item name is required")
	}

	// Resolve Catalog ID: if not provided or empty, find or create the default active catalog for the business
	catalogID := strings.TrimSpace(input.CatalogID)
	if catalogID == "" && c.repo != nil {
		page, err := c.repo.ListCatalogs(ctx, execCtx.BusinessID, "active", 1, "")
		if err == nil && len(page.Items) > 0 {
			catalogID = page.Items[0].ID
		} else {
			now := time.Now().UTC()
			defaultCat, createErr := c.repo.CreateCatalog(ctx, ports.CatalogDraft{
				ID:         uuid.NewString(),
				BusinessID: execCtx.BusinessID,
				Name:       "الكتالوج الرئيسي",
				Status:     "active",
				CreatedAt:  now,
				UpdatedAt:  now,
			})
			if createErr == nil {
				catalogID = defaultCat.ID
			}
		}
	}
	if catalogID == "" {
		return ports.AICapabilityResult{}, errors.New("catalog_id could not be resolved; please create a catalog first")
	}

	itemType := strings.TrimSpace(input.ItemType)
	if itemType == "" {
		itemType = "product"
	}
	pricingMode := strings.TrimSpace(input.PricingMode)
	if pricingMode == "" {
		pricingMode = "fixed"
	}
	availabilityMode := strings.TrimSpace(input.AvailabilityMode)
	if availabilityMode == "" {
		availabilityMode = "stock"
	}
	fulfillmentMode := strings.TrimSpace(input.FulfillmentMode)
	if fulfillmentMode == "" {
		fulfillmentMode = "delivery"
	}

	variants := make([]commands.AuthorVariantInput, 0, len(input.Variants))
	for _, v := range input.Variants {
		vName := strings.TrimSpace(v.Name)
		if vName != "" {
			variants = append(variants, commands.AuthorVariantInput{
				Name:       vName,
				Attributes: v.Attributes,
			})
		}
	}

	offers := make([]commands.AuthorOfferInput, 0, len(input.Offers))
	for _, o := range input.Offers {
		var amountMinor *int64
		if o.Amount != nil {
			minor := int64(*o.Amount * 100)
			amountMinor = &minor
		}
		curr := "YER"
		if o.Currency != nil && strings.TrimSpace(*o.Currency) != "" {
			curr = strings.ToUpper(strings.TrimSpace(*o.Currency))
		}
		oPricingMode := strings.TrimSpace(o.PricingMode)
		if oPricingMode == "" {
			oPricingMode = pricingMode
		}
		oAvailStatus := strings.TrimSpace(o.AvailabilityStatus)
		if oAvailStatus == "" {
			oAvailStatus = "available"
		}
		oName := strings.TrimSpace(o.Name)
		if oName == "" {
			oName = name
		}
		offers = append(offers, commands.AuthorOfferInput{
			Name:               oName,
			VariantName:        o.VariantName,
			PricingMode:        oPricingMode,
			AmountMinor:        amountMinor,
			Currency:           &curr,
			PricingUnit:        o.PricingUnit,
			AvailabilityMode:   availabilityMode,
			AvailabilityStatus: oAvailStatus,
			FulfillmentMode:    fulfillmentMode,
			Status:             "active",
		})
	}

	cmd := commands.AuthorCatalogItemCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID: commands.BusinessID(execCtx.BusinessID),
				Role:       "owner",
			},
		},
		CatalogID:            commands.CatalogID(catalogID),
		Name:                 name,
		ItemType:             itemType,
		PricingMode:          pricingMode,
		AvailabilityMode:     availabilityMode,
		FulfillmentMode:      fulfillmentMode,
		RequiresConfirmation: input.RequiresConfirmation,
		Attributes:           input.Attributes,
		Variants:             variants,
		Offers:               offers,
	}

	result, err := c.handler.Handle(ctx, cmd)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"success":    true,
			"item_id":    string(result.Item.ID),
			"name":       result.Item.Name,
			"status":     result.Item.Status,
			"variants":   len(result.Variants),
			"offers":     len(result.Offers),
			"catalog_id": catalogID,
		},
	}, nil
}

// ----------------------------------------------------------------------
// 4. search_catalog
// ----------------------------------------------------------------------
type merchantSearchCatalogCapability struct {
	repo ports.CatalogRepository
}

func (c *merchantSearchCatalogCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "search_catalog",
		Description: "البحث في كتالوج المتجر للتحقق من وجود صنف أو معرفة سعره أو جلب معرفه (ID).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "كلمة البحث (اسم الصنف أو جزء منه)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "عدد النتائج الأقصى (افتراضي 10)",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (c *merchantSearchCatalogCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	if c.repo == nil {
		return ports.AICapabilityResult{}, errors.New("catalog repository is not configured")
	}
	var input struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(rawParams, &input)
	query := strings.TrimSpace(input.Query)
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	page, err := c.repo.ListCatalogItems(ctx, execCtx.BusinessID, "", query, "", limit, "")
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	type foundItem struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		ItemType string `json:"item_type"`
		Status   string `json:"status"`
	}
	items := make([]foundItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, foundItem{
			ID:       item.ID,
			Name:     item.Name,
			ItemType: item.ItemType,
			Status:   item.Status,
		})
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"items": items,
			"count": len(items),
			"query": query,
		},
	}, nil
}

// ----------------------------------------------------------------------
// 5. get_item_details
// ----------------------------------------------------------------------
type merchantGetItemDetailsCapability struct {
	repo ports.CatalogRepository
}

func (c *merchantGetItemDetailsCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "get_item_details",
		Description: "جلب التفاصيل الكاملة لصنف معين (الاسم، الوصف، الخصائص، جميع المتغيرات والمقاسات، وجميع الأسعار والعروض).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{
					"type":        "string",
					"description": "معرف الصنف في قاعدة البيانات (UUID)",
				},
				"catalog_id": map[string]any{
					"type":        "string",
					"description": "معرف الكتالوج إن وجد",
				},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c *merchantGetItemDetailsCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	if c.repo == nil {
		return ports.AICapabilityResult{}, errors.New("catalog repository is not configured")
	}
	var input struct {
		ItemID    string `json:"item_id"`
		CatalogID string `json:"catalog_id"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid get_item_details parameters: %w", err)
	}
	itemID := strings.TrimSpace(input.ItemID)
	if itemID == "" {
		return ports.AICapabilityResult{}, errors.New("item_id is required")
	}

	catalogID := strings.TrimSpace(input.CatalogID)
	var itemRec ports.CatalogItemRecord
	var err error

	if catalogID != "" {
		itemRec, err = c.repo.GetCatalogItem(ctx, execCtx.BusinessID, catalogID, itemID)
	} else {
		// Auto-resolve item across active catalogs of the business
		catalogs, listErr := c.repo.ListCatalogs(ctx, execCtx.BusinessID, "active", 50, "")
		if listErr != nil {
			return ports.AICapabilityResult{}, listErr
		}
		found := false
		for _, cat := range catalogs.Items {
			rec, getErr := c.repo.GetCatalogItem(ctx, execCtx.BusinessID, cat.ID, itemID)
			if getErr == nil {
				itemRec = rec
				found = true
				break
			}
		}
		if !found {
			return ports.AICapabilityResult{}, errors.New("catalog item not found")
		}
	}
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	// Fetch offers for this item
	offersPage, _ := c.repo.ListOffers(ctx, execCtx.BusinessID, itemID, "", 50, "")
	// Fetch variants for this item
	variantsPage, _ := c.repo.ListVariants(ctx, execCtx.BusinessID, itemID, "", 50, "")

	var attrs map[string]any
	if len(itemRec.Attributes) > 0 {
		_ = json.Unmarshal(itemRec.Attributes, &attrs)
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"item": map[string]any{
				"id":         itemRec.ID,
				"catalog_id": itemRec.CatalogID,
				"name":       itemRec.Name,
				"item_type":  itemRec.ItemType,
				"status":     itemRec.Status,
				"attributes": attrs,
			},
			"variants": variantsPage.Items,
			"offers":   offersPage.Items,
		},
	}, nil
}

// ----------------------------------------------------------------------
// 6. create_variant
// ----------------------------------------------------------------------
type merchantCreateVariantCapability struct {
	handler commands.CreateVariantHandler
	repo    ports.CatalogRepository
}

func (c *merchantCreateVariantCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "create_variant",
		Description: "إضافة متغير جديد (مقاس، لون، سعة...) لصنف قائم في الكتالوج مع إمكانية تحديد سعره فوراً.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{
					"type":        "string",
					"description": "معرف الصنف الأساسي (UUID)",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "اسم المتغير (مثال: مقاس XL، أسود، 256GB)",
				},
				"attributes": map[string]any{
					"type":        "object",
					"description": "خصائص المتغير إن وجدت (كائن JSON)",
				},
				"price_amount": map[string]any{
					"type":        "number",
					"description": "سعر هذا المتغير (اختياري، مثلاً 5000)",
				},
				"currency": map[string]any{
					"type":        "string",
					"enum":        []string{"YER", "SAR", "USD"},
					"description": "عملة السعر (افتراضي YER)",
				},
				"pricing_unit": map[string]any{
					"type":        "string",
					"description": "وحدة التسعير (مثلاً حبة، كيلو، علبة)",
				},
				"pricing_mode": map[string]any{
					"type":        "string",
					"enum":        []string{"fixed", "per_unit", "per_day", "per_person", "starting_from"},
					"description": "نمط التسعير (افتراضي fixed)",
				},
			},
			"required": []string{"item_id", "name"},
		},
	}
}

func (c *merchantCreateVariantCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	var input struct {
		ItemID      string         `json:"item_id"`
		Name        string         `json:"name"`
		Attributes  map[string]any `json:"attributes"`
		PriceAmount *float64       `json:"price_amount"`
		Currency    *string        `json:"currency"`
		PricingUnit *string        `json:"pricing_unit"`
		PricingMode *string        `json:"pricing_mode"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid create_variant parameters: %w", err)
	}
	itemID := strings.TrimSpace(input.ItemID)
	name := strings.TrimSpace(input.Name)
	if itemID == "" || name == "" {
		return ports.AICapabilityResult{}, errors.New("item_id and name are required")
	}

	if c.handler != nil {
		cmd := commands.CreateVariantCommand{
			Meta: commands.CommandMeta{
				Actor: commands.ActorContext{
					BusinessID: commands.BusinessID(execCtx.BusinessID),
					Role:       "owner",
				},
			},
			CatalogItemID: commands.CatalogItemID(itemID),
			Name:          name,
			Attributes:    input.Attributes,
		}
		res, err := c.handler.Handle(ctx, cmd)
		if err != nil {
			return ports.AICapabilityResult{}, err
		}

		variantID := string(res.Variant.ID)
		var offerID string

		// If price is specified, create offer for this variant
		if input.PriceAmount != nil && c.repo != nil {
			minor := int64(*input.PriceAmount * 100)
			curr := "YER"
			if input.Currency != nil && strings.TrimSpace(*input.Currency) != "" {
				curr = strings.ToUpper(strings.TrimSpace(*input.Currency))
			}
			pMode := "fixed"
			if input.PricingMode != nil && strings.TrimSpace(*input.PricingMode) != "" {
				pMode = strings.TrimSpace(*input.PricingMode)
			}
			now := time.Now().UTC()
			offRec, offErr := c.repo.CreateOffer(ctx, ports.OfferDraft{
				ID:                 uuid.NewString(),
				BusinessID:         execCtx.BusinessID,
				CatalogItemID:      itemID,
				VariantID:          &variantID,
				Name:               name,
				PricingMode:        pMode,
				AmountMinor:        &minor,
				Currency:           &curr,
				PricingUnit:        input.PricingUnit,
				AvailabilityMode:   "stock",
				AvailabilityStatus: "available",
				FulfillmentMode:    "delivery",
				Status:             "active",
				CreatedAt:          now,
				UpdatedAt:          now,
			})
			if offErr == nil {
				offerID = offRec.ID
			}
		}

		return ports.AICapabilityResult{
			Data: map[string]any{
				"success":    true,
				"variant_id": variantID,
				"name":       res.Variant.Name,
				"item_id":    itemID,
				"offer_id":   offerID,
			},
		}, nil
	}

	return ports.AICapabilityResult{}, errors.New("create_variant handler is not configured")
}

// ----------------------------------------------------------------------
// 7. update_catalog_item
// ----------------------------------------------------------------------
type merchantUpdateCatalogItemCapability struct {
	updateItem  commands.UpdateCatalogItemHandler
	updateOffer commands.UpdateOfferHandler
	repo        ports.CatalogRepository
}

func (c *merchantUpdateCatalogItemCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "update_catalog_item",
		Description: "تعديل اسم أو حالة أو خصائص أو سعر صنف موجود في الكتالوج باستخدام معرّفه (item_id) مع إمكانية استهداف متغير معين.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{
					"type":        "string",
					"description": "معرف الصنف في قاعدة البيانات (UUID)",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "الاسم الجديد للصنف إن وجد",
				},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"draft", "active", "inactive", "archived"},
					"description": "حالة الصنف",
				},
				"attributes": map[string]any{
					"type":        "object",
					"description": "تحديث الخصائص المخصصة (كائن JSON: ألوان، مقاسات، موديل...)",
				},
				"variant_name": map[string]any{
					"type":        "string",
					"description": "اسم المتغير لتعديل سعره حصراً (مثلاً: دبة كبيرة، علبة صغيرة)",
				},
				"offer_id": map[string]any{
					"type":        "string",
					"description": "معرف العرض (UUID) إن رغب بتعديل عرض محدد",
				},
				"price_amount": map[string]any{
					"type":        "number",
					"description": "السعر الجديد للصنف أو المتغير إن رغب التاجر بتعديله",
				},
				"currency": map[string]any{
					"type":        "string",
					"enum":        []string{"YER", "SAR", "USD"},
					"description": "العملة (YER, SAR, USD)",
				},
				"availability_status": map[string]any{
					"type":        "string",
					"enum":        []string{"available", "unavailable", "requires_check", "stale"},
					"description": "تحديث حالة التوفر الفورية (نفد / متوفر)",
				},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c *merchantUpdateCatalogItemCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	var input struct {
		ItemID             string         `json:"item_id"`
		Name               *string        `json:"name"`
		Status             *string        `json:"status"`
		Attributes         map[string]any `json:"attributes"`
		VariantName        *string        `json:"variant_name"`
		OfferID            *string        `json:"offer_id"`
		PriceAmount        *float64       `json:"price_amount"`
		Currency           *string        `json:"currency"`
		AvailabilityStatus *string        `json:"availability_status"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid update_catalog_item parameters: %w", err)
	}
	itemID := strings.TrimSpace(input.ItemID)
	if itemID == "" {
		return ports.AICapabilityResult{}, errors.New("item_id is required")
	}

	updatedFields := make([]string, 0)
	if c.updateItem != nil && (input.Name != nil || input.Status != nil || input.Attributes != nil) {
		cmd := commands.UpdateCatalogItemCommand{
			Meta: commands.CommandMeta{
				Actor: commands.ActorContext{
					BusinessID: commands.BusinessID(execCtx.BusinessID),
					Role:       "owner",
				},
			},
			CatalogItemID: commands.CatalogItemID(itemID),
			Name:          input.Name,
			Status:        input.Status,
			Attributes:    input.Attributes,
		}
		_, err := c.updateItem.Handle(ctx, cmd)
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		if input.Name != nil {
			updatedFields = append(updatedFields, "name")
		}
		if input.Status != nil {
			updatedFields = append(updatedFields, "status")
		}
		if input.Attributes != nil {
			updatedFields = append(updatedFields, "attributes")
		}
	}

	// Update offer price or availability status if specified
	if (input.PriceAmount != nil || input.AvailabilityStatus != nil) && c.repo != nil {
		offersPage, err := c.repo.ListOffers(ctx, execCtx.BusinessID, itemID, "", 50, "")
		if err == nil && len(offersPage.Items) > 0 {
			var minor *int64
			if input.PriceAmount != nil {
				m := int64(*input.PriceAmount * 100)
				minor = &m
			}

			targetVariantName := ""
			if input.VariantName != nil {
				targetVariantName = strings.TrimSpace(*input.VariantName)
			}
			targetOfferID := ""
			if input.OfferID != nil {
				targetOfferID = strings.TrimSpace(*input.OfferID)
			}

			var targetVariantID string
			if targetVariantName != "" {
				variantsPage, _ := c.repo.ListVariants(ctx, execCtx.BusinessID, itemID, "", 50, "")
				for _, v := range variantsPage.Items {
					if strings.EqualFold(strings.TrimSpace(v.Name), targetVariantName) {
						targetVariantID = v.ID
						break
					}
				}
			}

			if c.updateOffer != nil {
				for _, o := range offersPage.Items {
					if targetOfferID != "" && o.ID != targetOfferID {
						continue
					}
					if targetVariantID != "" && (o.VariantID == nil || *o.VariantID != targetVariantID) {
						continue
					}

					offerCmd := commands.UpdateOfferCommand{
						Meta: commands.CommandMeta{
							Actor: commands.ActorContext{
								BusinessID: commands.BusinessID(execCtx.BusinessID),
								Role:       "owner",
							},
						},
						OfferID:            commands.OfferID(o.ID),
						AmountMinor:        minor,
						AvailabilityStatus: input.AvailabilityStatus,
					}
					_, _ = c.updateOffer.Handle(ctx, offerCmd)
				}
				if input.PriceAmount != nil {
					updatedFields = append(updatedFields, "price")
				}
				if input.AvailabilityStatus != nil {
					updatedFields = append(updatedFields, "availability_status")
				}
			}
		}
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"success":        true,
			"item_id":        itemID,
			"updated_fields": updatedFields,
		},
	}, nil
}

// ----------------------------------------------------------------------
// 8. archive_catalog_item
// ----------------------------------------------------------------------
type merchantArchiveCatalogItemCapability struct {
	updateItem commands.UpdateCatalogItemHandler
	repo       ports.CatalogRepository
}

func (c *merchantArchiveCatalogItemCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "archive_catalog_item",
		Description: "أرشفة وتعطيل صنف في الكتالوج (حفظ آمن بدلاً من الحذف الفيزيائي) بعد تأكيد التاجر.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{
					"type":        "string",
					"description": "معرف الصنف المراد أرشفته (UUID)",
				},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c *merchantArchiveCatalogItemCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	if c.updateItem == nil {
		return ports.AICapabilityResult{}, errors.New("update catalog item handler is not configured")
	}
	var input struct {
		ItemID string `json:"item_id"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid archive_catalog_item parameters: %w", err)
	}
	itemID := strings.TrimSpace(input.ItemID)
	if itemID == "" {
		return ports.AICapabilityResult{}, errors.New("item_id is required")
	}

	statusArchived := "archived"
	cmd := commands.UpdateCatalogItemCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID: commands.BusinessID(execCtx.BusinessID),
				Role:       "owner",
			},
		},
		CatalogItemID: commands.CatalogItemID(itemID),
		Status:        &statusArchived,
	}

	res, err := c.updateItem.Handle(ctx, cmd)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"success": true,
			"item_id": string(res.Item.ID),
			"name":    res.Item.Name,
			"status":  res.Item.Status,
		},
	}, nil
}

// ----------------------------------------------------------------------
// 9. parse_and_import_file
// ----------------------------------------------------------------------
type merchantParseFileCapability struct {
	authorHandler commands.AuthorCatalogItemHandler
	repo          ports.CatalogRepository
}

func (c *merchantParseFileCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "parse_and_import_file",
		Description: "استيراد وتفكيك محتوى جدول نصي أو CSV أو إكسل وإضافة جميع الأصناف جماعياً إلى الكتالوج.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"catalog_id": map[string]any{
					"type":        "string",
					"description": "معرف الكتالوج المراد الإضافة فيه",
				},
				"raw_content": map[string]any{
					"type":        "string",
					"description": "المحتوى النصي أو الـ CSV المفصول بفاصلة أو سطر",
				},
			},
			"required": []string{"raw_content"},
		},
	}
}

func (c *merchantParseFileCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	var input struct {
		CatalogID  string `json:"catalog_id"`
		RawContent string `json:"raw_content"`
	}
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return ports.AICapabilityResult{}, fmt.Errorf("invalid parse_and_import_file parameters: %w", err)
	}

	rawContent := strings.TrimSpace(input.RawContent)
	if rawContent == "" {
		return ports.AICapabilityResult{}, errors.New("raw_content cannot be empty")
	}

	// Resolve default catalog
	catalogID := strings.TrimSpace(input.CatalogID)
	if catalogID == "" && c.repo != nil {
		page, err := c.repo.ListCatalogs(ctx, execCtx.BusinessID, "active", 1, "")
		if err == nil && len(page.Items) > 0 {
			catalogID = page.Items[0].ID
		} else {
			now := time.Now().UTC()
			defaultCat, createErr := c.repo.CreateCatalog(ctx, ports.CatalogDraft{
				ID:         uuid.NewString(),
				BusinessID: execCtx.BusinessID,
				Name:       "الكتالوج الرئيسي",
				Status:     "active",
				CreatedAt:  now,
				UpdatedAt:  now,
			})
			if createErr == nil {
				catalogID = defaultCat.ID
			}
		}
	}
	if catalogID == "" {
		return ports.AICapabilityResult{}, errors.New("catalog_id is required or could not be created")
	}

	// Parse CSV lines
	reader := csv.NewReader(strings.NewReader(rawContent))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	var importedCount int
	var itemsSummary []string

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) == 0 {
			continue
		}
		itemName := strings.TrimSpace(record[0])
		lowered := strings.ToLower(itemName)
		if itemName == "" ||
			lowered == "name" ||
			lowered == "item" ||
			lowered == "product" ||
			lowered == "title" ||
			lowered == "item name" ||
			lowered == "product name" ||
			itemName == "الاسم" ||
			itemName == "اسم" ||
			itemName == "اسم الصنف" ||
			itemName == "اسم المنتج" ||
			itemName == "الصنف" ||
			itemName == "المنتج" ||
			strings.HasPrefix(itemName, "اسم ") {
			continue
		}

		var priceVal *float64
		if len(record) > 1 {
			pStr := strings.TrimSpace(record[1])
			pStr = strings.ReplaceAll(pStr, ",", "")
			if val, pErr := strconv.ParseFloat(pStr, 64); pErr == nil && val >= 0 {
				priceVal = &val
			}
		}

		var amountMinor *int64
		if priceVal != nil {
			minor := int64(*priceVal * 100)
			amountMinor = &minor
		}
		curr := "YER"

		if c.authorHandler != nil {
			cmd := commands.AuthorCatalogItemCommand{
				Meta: commands.CommandMeta{
					Actor: commands.ActorContext{
						BusinessID: commands.BusinessID(execCtx.BusinessID),
						Role:       "owner",
					},
				},
				CatalogID:        commands.CatalogID(catalogID),
				Name:             itemName,
				ItemType:         "product",
				PricingMode:      "fixed",
				AvailabilityMode: "stock",
				FulfillmentMode:  "delivery",
				Offers: []commands.AuthorOfferInput{
					{
						Name:               itemName,
						PricingMode:        "fixed",
						AmountMinor:        amountMinor,
						Currency:           &curr,
						AvailabilityMode:   "stock",
						AvailabilityStatus: "available",
						FulfillmentMode:    "delivery",
						Status:             "active",
					},
				},
			}
			res, authorErr := c.authorHandler.Handle(ctx, cmd)
			if authorErr == nil {
				importedCount++
				itemsSummary = append(itemsSummary, res.Item.Name)
			}
		}
	}

	return ports.AICapabilityResult{
		Data: map[string]any{
			"success":        true,
			"imported_count": importedCount,
			"items":          itemsSummary,
			"catalog_id":     catalogID,
		},
	}, nil
}
