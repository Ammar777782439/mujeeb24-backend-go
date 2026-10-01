package merchantcatalogai

import (
	"errors"
	"strings"
)

type ProposalStatus string `json:"status"`
const (
	StatusResolved ProposalStatus = "resolved"
	StatusAmbiguous ProposalStatus = "ambiguous"
	StatusNotFound ProposalStatus = "not_found"
	StatusNeedsMoreData ProposalStatus = "needs_more_data"
)

type Operation string
const (
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
	OperationAskMerchant Operation = "ask_merchant"
)

type MissingField struct {
	Path string `json:"path"`
	DisplayName string `json:"display_name"`
	DataType string `json:"data_type"`
	Reason string `json:"reason"`
}

type ItemCreate struct {
	Name string `json:"name"`
	ItemType string `json:"item_type"`
	PricingMode string `json:"pricing_mode"`
	AvailabilityMode string `json:"availability_mode"`
	FulfillmentMode string `json:"fulfillment_mode"`
	RequiresConfirmation bool `json:"requires_confirmation"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Variants []VariantCreate `json:"variants,omitempty"`
	Offers []OfferCreate `json:"offers,omitempty"`
}

type ItemChanges struct {
	Name *string `json:"name,omitempty"`
	Status *string `json:"status,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	RequiresConfirmation *bool `json:"requires_confirmation,omitempty"`
}

type VariantCreate struct {
	Name string `json:"name"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type VariantUpdate struct {
	ID string `json:"id"`
	Name *string `json:"name,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Status *string `json:"status,omitempty"`
}

type OfferCreate struct {
	VariantID *string `json:"variant_id,omitempty"`
	VariantName *string `json:"variant_name,omitempty"`
	Name string `json:"name"`
	PricingMode string `json:"pricing_mode"`
	Amount *string `json:"amount,omitempty"`
	Currency *string `json:"currency,omitempty"`
	PricingUnit *string `json:"pricing_unit,omitempty"`
	AvailabilityMode string `json:"availability_mode"`
	AvailabilityStatus string `json:"availability_status"`
	FulfillmentMode string `json:"fulfillment_mode"`
	Status string
}

type OfferUpdate struct {
	ID string `json:"id"`
	Name *string `json:"name,omitempty"`
	Amount *string `json:"amount,omitempty"`
	AvailabilityStatus *string `json:"availability_status,omitempty"`
	Status *string `json:"status,omitempty"`
}

type UpdateOperation struct {
	ItemID string `json:"item_id"`
	Changes ItemChanges `json:"changes"`
	ExistingVariants []VariantUpdate `json:"existing_variants,omitempty"`
	NewVariants []VariantCreate `json:"new_variants,omitempty"`
	ExistingOffers []OfferUpdate `json:"existing_offers,omitempty"`
	NewOffers []OfferCreate `json:"new_offers,omitempty"`
}

type DeleteOperation struct {
	ItemID string `json:"id"`
	ReasonGiven string `json:"reason_given,omitempty"`
}

type Proposal struct {
	SchemaVersion int `json:"schema_version"`
	Status ProposalStatus `json:"status"`
	Operation Operation `json:"operation"`
	ResponseText string `json:"response_text"`
	EvidenceReferences []string `json:"evidence_references,omitempty"`
	MissingInformation []MissingField `json:"missing_information,omitempty"`
	Create *ItemCreate `json:"create,omitempty"`
	Update *UpdateOperation `json:"update,omitempty"`
	Delete *DeleteOperation `json:"delete,omitempty"`
}

func (p Proposal) IsMutation() bool {
	return p.Operation == OperationCreate || p.Operation == OperationUpdate || p.Operation == OperationDelete
}

func (p Proposal) Validate() error {
	if strings.TrimSpace(p.ResponseText) == "" {
		return errors.New("merchant catalog proposal response_text is required")
	}
	switch p.Status {
	case StatusResolved, StatusAmbiguous, StatusNotFound, StatusNeedsMoreData:
	default:
		return errors.New("merchant catalog proposal status is invalid")
	}
	switch p.Operation {
	case OperationCreate, OperationUpdate, OperationDelete, OperationAskMerchant:
	default:
		return errors.New("merchant catalog proposal operation is invalid")
	}
	if p.Status != StatusResolved {
		if p.IsMutation() {
			return errors.New("non-resolved proposal cannot contain a mutation operation")
		}
		return nil
	}
	switch p.Operation {
	case OperationCreate:
		if p.Create == nil || strings.TrimSpace(p.Create.Name) == "" {
			return errors.New("create proposal requires item data")
		}
		if p.Update != nil || p.Delete != nil {
			return errors.New("create proposal cannot contain update/delete data")
		}
	case OperationUpdate:
		if p.Update == nil || strings.TrimSpace(p.Update.ItemID) == "" {
			return errors.New("update proposal requires item_id")
		}
		if p.Create != nil || p.Delete != nil {
			return errors.New("update proposal cannot contain create/delete data")
		}
	case OperationDelete:
		if p.Delete == nil || strings.TrimSpace(p.Delete.ItemID) == "" {
			return errors.New("delete proposal requires item_id")
		}
		if p.Create != nil || p.Update != nil {
			return errors.New("delete proposal cannot contain create/update data")
		}
	case OperationAskMerchant:
		if p.Create != nil || p.Update != nil || p.Delete != nil {
			return errors.New("ask_merchant proposal cannot contain mutation data")
		}
	}
	return nil
}
