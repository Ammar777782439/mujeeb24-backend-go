package merchantcatalogai

import (
	"errors"
	"strings"
)

type ProposalStatus string
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
	Path string
	DisplayName string
	DataType string
	Reason string
}

type ItemCreate struct {
	Name string
	ItemType string
	PricingMode string
	AvailabilityMode string
	FulfillmentMode string
	RequiresConfirmation bool
	Attributes map[string]any
	Variants []VariantCreate
	Offers []OfferCreate
}

type ItemChanges struct {
	Name *string
	Status *string
	Attributes map[string]any
	RequiresConfirmation *bool
}

type VariantCreate struct {
	Name string
	Attributes map[string]any
}

type VariantUpdate struct {
	ID string
	Name *string
	Attributes map[string]any
	Status *string
}

type OfferCreate struct {
	VariantID *string
	VariantName *string
	Name string
	PricingMode string
	Amount *string
	Currency *string
	PricingUnit *string
	AvailabilityMode string
	AvailabilityStatus string
	FulfillmentMode string
	Status string
}

type OfferUpdate struct {
	ID string
	Name *string
	Amount *string
	AvailabilityStatus *string
	Status *string
}

type UpdateOperation struct {
	ItemID string
	Changes ItemChanges
	ExistingVariants []VariantUpdate
	NewVariants []VariantCreate
	ExistingOffers []OfferUpdate
	NewOffers []OfferCreate
}

type DeleteOperation struct {
	ItemID string
	ReasonGiven string
}

type Proposal struct {
	SchemaVersion int
	Status ProposalStatus
	Operation Operation
	ResponseText string
	EvidenceReferences []string
	MissingInformation []MissingField
	Create *ItemCreate
	Update *UpdateOperation
	Delete *DeleteOperation
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
