// Package services — Catalog Entity Contract (contract ⑤)
//
// This file is the CONTRACT ⑤ §2 definition of the six catalog entities.
// Per contract ⑤ §3, the Entity Contract is the DEFINITION of the catalog
// system; it is NOT merchant data.
//
// Per contract ⑤ §7, Gemini sees the Catalog Entity Contract from the start
// so it never has to guess the meaning of fields like pricing_mode or
// availability_status. Actual merchant data comes later via the Catalog AI
// Projection (contract ①) inside Batch evaluation.
//
// Per contract ⑤ §9, the Entity Contract is NOT repeated per batch — it is
// a shared definition sent once per AI Runtime invocation.
//
// Per contract ⑤ §17, Entity Contract does not contain merchant-specific
// values; e.g., Offer.amount = NUMERIC(20,4) is the definition, while
// amount=12000 currency=YER is merchant data (Actual Catalog Data).

package services

// CatalogEntityContract is the canonical definition of the six catalog entities
// per contract ⑤ §2. It is sent to Gemini as part of the system context.
//
// Per contract ⑤ §17, this structure contains ONLY type/meaning definitions;
// no merchant-specific data lives here.
type CatalogEntityContract struct {
	Catalog             CatalogEntityDefinition             `json:"catalog"`
	CatalogItem         CatalogItemEntityDefinition         `json:"catalog_item"`
	AttributeSchema     AttributeSchemaEntityDefinition     `json:"attribute_schema"`
	AttributeDefinition AttributeDefinitionEntityDefinition `json:"attribute_definition"`
	Variant             VariantEntityDefinition             `json:"variant"`
	Offer               OfferEntityDefinition               `json:"offer"`
}

// CatalogEntityDefinition is the Catalog root entity definition per contract ⑤ §2.
type CatalogEntityDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CatalogItemEntityDefinition is the CatalogItem entity per contract ⑤ §2.
type CatalogItemEntityDefinition struct {
	ID                     string         `json:"id"`
	CatalogID              string         `json:"catalog_id"`
	AttributeSchemaID      *string        `json:"attribute_schema_id,omitempty"`
	AttributeSchemaVersion *int           `json:"attribute_schema_version,omitempty"`
	ItemType               string         `json:"item_type"`
	Name                   string         `json:"name"`
	ShortDescription       *string        `json:"short_description,omitempty"`
	LongDescription        *string        `json:"long_description,omitempty"`
	Status                 string         `json:"status"`
	PricingMode            string         `json:"pricing_mode"`
	AvailabilityMode       string         `json:"availability_mode"`
	FulfillmentMode        string         `json:"fulfillment_mode"`
	RequiresConfirmation   bool           `json:"requires_confirmation"`
	Attributes             map[string]any `json:"attributes,omitempty"`
}

// AttributeSchemaEntityDefinition per contract ⑤ §2.
type AttributeSchemaEntityDefinition struct {
	ID          string                                `json:"id"`
	Name        string                                `json:"name"`
	Version     int                                   `json:"version"`
	Definitions []AttributeDefinitionEntityDefinition `json:"definitions"`
}

// AttributeDefinitionEntityDefinition per contract ⑤ §2.
type AttributeDefinitionEntityDefinition struct {
	ID              string         `json:"id"`
	SchemaID        string         `json:"schema_id"`
	AttributeKey    string         `json:"attribute_key"`
	Label           string         `json:"label"`
	DataType        string         `json:"data_type"`
	IsRequired      bool           `json:"is_required"`
	IsSearchable    bool           `json:"is_searchable"`
	ValidationRules map[string]any `json:"validation_rules,omitempty"`
	DisplayOrder    int            `json:"display_order"`
}

// VariantEntityDefinition per contract ⑤ §2.
type VariantEntityDefinition struct {
	ID            string         `json:"id"`
	CatalogItemID string         `json:"catalog_item_id"`
	Name          string         `json:"name"`
	Attributes    map[string]any `json:"attributes,omitempty"`
	Status        string         `json:"status"`
}

// OfferEntityDefinition per contract ⑤ §2.
type OfferEntityDefinition struct {
	ID                      string  `json:"id"`
	VariantID               *string `json:"variant_id,omitempty"`
	Name                    string  `json:"name"`
	PricingMode             string  `json:"pricing_mode"`
	Amount                  *string `json:"amount,omitempty"`
	Currency                *string `json:"currency,omitempty"`
	PricingUnit             *string `json:"pricing_unit,omitempty"`
	PriceSource             *string `json:"price_source,omitempty"`
	PriceVerificationStatus *string `json:"price_verification_status,omitempty"`
	PriceCheckedAt          *string `json:"price_checked_at,omitempty"`
	AvailabilityMode        *string `json:"availability_mode,omitempty"`
	AvailabilityStatus      *string `json:"availability_status,omitempty"`
	AvailabilitySource      *string `json:"availability_source,omitempty"`
	AvailabilityCheckedAt   *string `json:"availability_checked_at,omitempty"`
	AvailabilityValidUntil  *string `json:"availability_valid_until,omitempty"`
	AvailabilityEvidenceRef *string `json:"availability_evidence_ref,omitempty"`
	FulfillmentMode         *string `json:"fulfillment_mode,omitempty"`
	ValidityFrom            *string `json:"validity_from,omitempty"`
	ValidityUntil           *string `json:"validity_until,omitempty"`
	Status                  string  `json:"status"`
}

// CatalogEntityContractDescriptor is the human+machine-readable description
// that Mujeeb sends to Gemini as part of the system instruction. Per contract
// ⑤ §8, this tells Gemini the meaning of every enum value so it never has
// to guess.
//
// Per contract ⑤ §8, this descriptor covers at least:
//   - meaning of pricing_mode values
//   - meaning of availability_mode values
//   - meaning of availability_status values
//   - meaning of price_verification_status values
//   - meaning of fulfillment_mode values
//   - relationship between CatalogItem, Variant, Offer
//   - relationship between CatalogItem and AttributeSchema
//   - relationship between AttributeSchema and AttributeDefinition
type CatalogEntityContractDescriptor struct {
	PricingModes              map[string]string    `json:"pricing_modes"`
	AvailabilityModes         map[string]string    `json:"availability_modes"`
	AvailabilityStatuses      map[string]string    `json:"availability_statuses"`
	PriceVerificationStatuses map[string]string    `json:"price_verification_statuses"`
	FulfillmentModes          map[string]string    `json:"fulfillment_modes"`
	ItemTypes                 map[string]string    `json:"item_types"`
	AttributeDataTypes        map[string]string    `json:"attribute_data_types"`
	Relationships             []EntityRelationship `json:"relationships"`
}

// EntityRelationship describes one relationship between entities per contract ⑤ §2.
type EntityRelationship struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Cardinality string `json:"cardinality"` // one_to_many, many_to_one, etc.
	Description string `json:"description"`
}

// DefaultCatalogEntityContractDescriptor returns the canonical descriptor
// that all Mujeeb deployments use. Per contract ⑤ §8, this is fixed
// knowledge that Gemini needs; deployments do NOT customize it.
//
// Per contract ⑤, deployments do not invent new pricing_mode or
// availability_mode values. If new values are added, the contract
// itself must be amended via an ADR.
func DefaultCatalogEntityContractDescriptor() CatalogEntityContractDescriptor {
	return CatalogEntityContractDescriptor{
		PricingModes: map[string]string{
			"fixed":         "السعر ثابت لا يتغير ضمن نطاق العرض",
			"starting_from": "السعر يبدأ من هذه القيمة وقد يزيد حسب الخيارات",
			"range":         "السعر يقع بين حد أدنى وحد أعلى",
			"negotiable":    "السعر قابل للتفاوض ضمن نطاق محدد",
			"on_request":    "السعر يُحدد عند الطلب بناءً على التفاصيل",
			"free":          "لا توجد تكلفة على هذا العنصر ضمن العرض",
		},
		AvailabilityModes: map[string]string{
			"in_stock":      "متوفر فورًا",
			"limited":       "متوفر بكمية محدودة",
			"pre_order":     "متاح الحجز المسبق قبل التوفر",
			"made_to_order": "يُصنع/يُنفذ عند الطلب",
			"out_of_stock":  "غير متوفر حاليًا",
			"discontinued":  "توقف إنتاجه/توفيره",
		},
		AvailabilityStatuses: map[string]string{
			"available":    "متاح للبيع/الإستخدام",
			"backordered":  "متاح الحجز مع تأخير التسليم",
			"reserved":     "محجوز بالكامل لعميل آخر",
			"unavailable":  "غير متاح حاليًا",
			"discontinued": "متوقف نهائيًا",
		},
		PriceVerificationStatuses: map[string]string{
			"verified":    "تم التحقق من السعر في آخر تحديث للكتالوج",
			"unverified":  "السعر لم يُتحقق منه بعد؛ قد لا يكون دقيقًا",
			"stale":       "السعر قديم وقد لا يعكس السعر الحالي",
			"in_progress": "التحقق من السعر جارٍ",
		},
		FulfillmentModes: map[string]string{
			"physical_delivery": "تسليم مادي للعميل",
			"digital_delivery":  "تسليم رقمي/إلكتروني",
			"pickup":            "استلام من المتجر/الموقع",
			"service_execution": "تنفيذ خدمة في وقت/مكان محدد",
			"subscription":      "اشتراك مدفوع لفترة محددة",
		},
		ItemTypes: map[string]string{
			"physical_product": "منتج مادي ملموس",
			"digital_product":  "منتج رقمي",
			"service":          "خدمة تُنفذ عند الطلب",
			"bundle":           "حزمة من عدة منتجات/خدمات",
		},
		AttributeDataTypes: map[string]string{
			"text":         "نص حر قصير أو طويل",
			"number":       "قيمة عددية",
			"boolean":      "صح/خطأ",
			"date":         "تاريخ بدون وقت",
			"datetime":     "تاريخ مع وقت",
			"select":       "قيمة واحدة من قائمة محددة",
			"multi_select": "قيم متعددة من قائمة محددة",
			"location":     "موقع جغرافي",
			"money":        "مبلغ مالي مع عملة",
		},
		Relationships: []EntityRelationship{
			{From: "Catalog", To: "CatalogItem", Cardinality: "one_to_many", Description: "Catalog يحتوي على عدة CatalogItems"},
			{From: "CatalogItem", To: "AttributeSchema", Cardinality: "many_to_one", Description: "CatalogItem يشير إلى AttributeSchema واحدة (اختياريًا)"},
			{From: "AttributeSchema", To: "AttributeDefinition", Cardinality: "one_to_many", Description: "AttributeSchema تحوي عدة AttributeDefinitions"},
			{From: "CatalogItem", To: "Variant", Cardinality: "one_to_many", Description: "CatalogItem قد يحوي عدة Variants"},
			{From: "CatalogItem", To: "Offer", Cardinality: "one_to_many", Description: "CatalogItem قد يحوي عدة Offers"},
			{From: "Variant", To: "Offer", Cardinality: "one_to_many", Description: "Offer قد يرتبط بـVariant واحد (اختياريًا)"},
		},
	}
}

// CatalogEntityContractPayload is the JSON-serializable payload sent to Gemini
// as part of the system instruction. Per contract ⑤ §7, this is shared
// across all batches and is sent once per AI Runtime invocation.
//
// The payload contains ONLY definitions and descriptors; no merchant data.
type CatalogEntityContractPayload struct {
	Contract   CatalogEntityContract           `json:"entity_contract"`
	Descriptor CatalogEntityContractDescriptor `json:"descriptor"`
}

// BuildCatalogEntityContractPayload returns the canonical payload to send to
// Gemini. Per contract ⑤ §7, this is built once per AI Runtime invocation
// and reused across all Batches.
func BuildCatalogEntityContractPayload() CatalogEntityContractPayload {
	return CatalogEntityContractPayload{
		Contract: CatalogEntityContract{
			Catalog: CatalogEntityDefinition{
				ID:          "UUID",
				Name:        "TEXT",
				Description: "TEXT",
			},
			CatalogItem: CatalogItemEntityDefinition{
				ID:                     "UUID",
				CatalogID:              "UUID",
				AttributeSchemaID:      entityContractStrPtr("UUID?"),
				AttributeSchemaVersion: entityContractIntPtr(0),
				ItemType:               "physical_product|digital_product|service|bundle",
				Name:                   "TEXT",
				ShortDescription:       entityContractStrPtr("TEXT?"),
				LongDescription:        entityContractStrPtr("TEXT?"),
				Status:                 "active|draft|archived",
				PricingMode:            "fixed|starting_from|range|negotiable|on_request|free",
				AvailabilityMode:       "in_stock|limited|pre_order|made_to_order|out_of_stock|discontinued",
				FulfillmentMode:        "physical_delivery|digital_delivery|pickup|service_execution|subscription",
				RequiresConfirmation:   false,
				Attributes:             map[string]any{"attribute_key": "value_per_definition"},
			},
			AttributeSchema: AttributeSchemaEntityDefinition{
				ID:      "UUID",
				Name:    "TEXT",
				Version: 1,
				Definitions: []AttributeDefinitionEntityDefinition{{
					ID:              "UUID",
					SchemaID:        "UUID",
					AttributeKey:    "TEXT (snake_case)",
					Label:           "TEXT (display)",
					DataType:        "text|number|boolean|date|datetime|select|multi_select|location|money",
					IsRequired:      false,
					IsSearchable:    false,
					ValidationRules: map[string]any{"rule_key": "rule_value"},
					DisplayOrder:    0,
				}},
			},
			AttributeDefinition: AttributeDefinitionEntityDefinition{
				ID:              "UUID",
				SchemaID:        "UUID",
				AttributeKey:    "TEXT (snake_case)",
				Label:           "TEXT (display)",
				DataType:        "text|number|boolean|date|datetime|select|multi_select|location|money",
				IsRequired:      false,
				IsSearchable:    false,
				ValidationRules: map[string]any{"rule_key": "rule_value"},
				DisplayOrder:    0,
			},
			Variant: VariantEntityDefinition{
				ID:            "UUID",
				CatalogItemID: "UUID",
				Name:          "TEXT",
				Attributes:    map[string]any{"attribute_key": "value_per_definition"},
				Status:        "active|draft|archived",
			},
			Offer: OfferEntityDefinition{
				ID:                      "UUID",
				VariantID:               entityContractStrPtr("UUID?"),
				Name:                    "TEXT",
				PricingMode:             "fixed|starting_from|range|negotiable|on_request|free",
				Amount:                  entityContractStrPtr("NUMERIC(20,4) as string?"),
				Currency:                entityContractStrPtr("ISO4217?"),
				PricingUnit:             entityContractStrPtr("TEXT?"),
				PriceSource:             entityContractStrPtr("merchant_manual|verified_by_provider|...?"),
				PriceVerificationStatus: entityContractStrPtr("verified|unverified|stale|in_progress?"),
				PriceCheckedAt:          entityContractStrPtr("ISO8601?"),
				AvailabilityMode:        entityContractStrPtr("in_stock|limited|pre_order|made_to_order|out_of_stock|discontinued?"),
				AvailabilityStatus:      entityContractStrPtr("available|backordered|reserved|unavailable|discontinued?"),
				AvailabilitySource:      entityContractStrPtr("TEXT?"),
				AvailabilityCheckedAt:   entityContractStrPtr("ISO8601?"),
				AvailabilityValidUntil:  entityContractStrPtr("ISO8601?"),
				AvailabilityEvidenceRef: entityContractStrPtr("TEXT?"),
				FulfillmentMode:         entityContractStrPtr("physical_delivery|digital_delivery|pickup|service_execution|subscription?"),
				ValidityFrom:            entityContractStrPtr("ISO8601?"),
				ValidityUntil:           entityContractStrPtr("ISO8601?"),
				Status:                  "active|draft|expired|archived",
			},
		},
		Descriptor: DefaultCatalogEntityContractDescriptor(),
	}
}

// strPtr and intPtr are intentionally NOT declared here to avoid collisions
// with existing test helpers in the same package. Use entityContractStrPtr
// and entityContractIntPtr instead.

func entityContractStrPtr(s string) *string { return &s }
func entityContractIntPtr(i int) *int       { return &i }
