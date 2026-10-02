package merchantcatalogai

const ProposalSchemaVersion = 3

// DefaultOfferName is the canonical contract-level name used when a merchant
// gives a price/offer but does not provide a separate commercial label.
const DefaultOfferName = "سعر البيع"

type ProposalStatus string

const (
	StatusResolved      ProposalStatus = "resolved"
	StatusAmbiguous     ProposalStatus = "ambiguous"
	StatusNotFound      ProposalStatus = "not_found"
	StatusNeedsMoreData ProposalStatus = "needs_more_data"
)

type Operation string

const (
	OperationCreate      Operation = "create"
	OperationUpdate      Operation = "update"
	OperationDelete      Operation = "delete"
	OperationAskMerchant Operation = "ask_merchant"
)

type MissingField struct {
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	DataType    string `json:"data_type"`
	Reason      string `json:"reason"`
}

type ItemCreate struct {
	Name                 string          `json:"name"`
	AttributeSchemaID    *string         `json:"attribute_schema_id,omitempty"`
	ItemType             string          `json:"item_type"`
	ShortDescription     *string         `json:"short_description,omitempty"`
	LongDescription      *string         `json:"long_description,omitempty"`
	PricingMode          string          `json:"pricing_mode"`
	AvailabilityMode     string          `json:"availability_mode"`
	FulfillmentMode      string          `json:"fulfillment_mode"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	Attributes           map[string]any  `json:"attributes,omitempty"`
	Variants             []VariantCreate `json:"variants,omitempty"`
	Offers               []OfferCreate   `json:"offers,omitempty"`
}

type ItemChanges struct {
	Name                 *string        `json:"name,omitempty"`
	ItemType             *string        `json:"item_type,omitempty"`
	ShortDescription     *string        `json:"short_description,omitempty"`
	LongDescription      *string        `json:"long_description,omitempty"`
	Status               *string        `json:"status,omitempty"`
	PricingMode          *string        `json:"pricing_mode,omitempty"`
	AvailabilityMode     *string        `json:"availability_mode,omitempty"`
	FulfillmentMode      *string        `json:"fulfillment_mode,omitempty"`
	Attributes           map[string]any `json:"attributes,omitempty"`
	RequiresConfirmation *bool          `json:"requires_confirmation,omitempty"`
}

type VariantCreate struct {
	Ref        string         `json:"ref"`
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type VariantUpdate struct {
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Status     *string        `json:"status,omitempty"`
}

const (
	OfferNameSourceSystemDefault   = "system_default"
	OfferNameSourceMerchantStated  = "merchant_stated"
	OfferPriceSourceMerchantStated = "merchant_stated"
	OfferPriceSourceNotStated      = "not_stated"
)

type OfferCreate struct {
	VariantID          *string `json:"variant_id,omitempty"`
	VariantRef         *string `json:"variant_ref,omitempty"`
	Name               string  `json:"name"`
	NameSource         string  `json:"name_source"`
	PricingMode        string  `json:"pricing_mode"`
	Amount             *string `json:"amount,omitempty"`
	PriceSource        string  `json:"price_source"`
	Currency           *string `json:"currency,omitempty"`
	PricingUnit        *string `json:"pricing_unit,omitempty"`
	AvailabilityMode   string  `json:"availability_mode"`
	AvailabilityStatus string  `json:"availability_status"`
	FulfillmentMode    string  `json:"fulfillment_mode"`
	Status             string  `json:"status"`
}

type OfferUpdate struct {
	ID                 string  `json:"id"`
	Name               *string `json:"name,omitempty"`
	Amount             *string `json:"amount,omitempty"`
	AvailabilityStatus *string `json:"availability_status,omitempty"`
	Status             *string `json:"status,omitempty"`
}

type UpdateOperation struct {
	ItemID           string          `json:"item_id"`
	Changes          ItemChanges     `json:"changes"`
	ExistingVariants []VariantUpdate `json:"existing_variants,omitempty"`
	NewVariants      []VariantCreate `json:"new_variants,omitempty"`
	ExistingOffers   []OfferUpdate   `json:"existing_offers,omitempty"`
	NewOffers        []OfferCreate   `json:"new_offers,omitempty"`
}

type DeleteOperation struct {
	ItemID      string `json:"id"`
	ReasonGiven string `json:"reason_given,omitempty"`
}

type Proposal struct {
	SchemaVersion      int              `json:"schema_version"`
	Status             ProposalStatus   `json:"status"`
	Operation          Operation        `json:"operation"`
	ResponseText       string           `json:"response_text"`
	EvidenceReferences []string         `json:"evidence_references,omitempty"`
	MissingInformation []MissingField   `json:"missing_information,omitempty"`
	Create             *ItemCreate      `json:"create,omitempty"`
	Update             *UpdateOperation `json:"update,omitempty"`
	Delete             *DeleteOperation `json:"delete,omitempty"`
}

func (p Proposal) IsMutation() bool {
	return p.Operation == OperationCreate || p.Operation == OperationUpdate || p.Operation == OperationDelete
}
