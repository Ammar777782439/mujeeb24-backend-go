// Package contracts defines provider-neutral, versioned metadata for
// merchant-facing AI evidence. It contains no database or model clients.
//
// These contracts do not replace the closed Catalog Entity Contract,
// CatalogAIProjection, or Customer Sales Context.
package contracts

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const EvidenceSchemaVersion = 1

// EvidenceSourceType is a closed semantic category, not a database table.
// Adding a source requires a reviewed projection contract and tests.
type EvidenceSourceType string

const (
	SourceBusiness       EvidenceSourceType = "business"
	SourceCatalogItem    EvidenceSourceType = "catalog_item"
	SourceCatalogOffer   EvidenceSourceType = "catalog_offer"
	SourceCustomer       EvidenceSourceType = "customer"
	SourceConversation   EvidenceSourceType = "conversation"
	SourceMessage        EvidenceSourceType = "message"
	SourceLead           EvidenceSourceType = "lead"
	SourceTransaction    EvidenceSourceType = "transaction"
	SourceSalesWorkflow  EvidenceSourceType = "sales_workflow"
	SourceBusinessPolicy EvidenceSourceType = "business_policy"
	SourceKnowledge      EvidenceSourceType = "knowledge"
	SourceChannel        EvidenceSourceType = "channel"
	SourceSubscription   EvidenceSourceType = "subscription"
)

func validEvidenceSource(source EvidenceSourceType) bool {
	switch source {
	case SourceBusiness, SourceCatalogItem, SourceCatalogOffer, SourceCustomer,
		SourceConversation, SourceMessage, SourceLead, SourceTransaction,
		SourceSalesWorkflow, SourceBusinessPolicy, SourceKnowledge,
		SourceChannel, SourceSubscription:
		return true
	default:
		return false
	}
}

type EvidenceAuthority string

const (
	AuthorityMerchantRecord    EvidenceAuthority = "merchant_record"
	AuthorityMerchantVerified  EvidenceAuthority = "merchant_verified"
	AuthoritySystemRecord      EvidenceAuthority = "system_record"
	AuthorityCustomerStatement EvidenceAuthority = "customer_statement"
	AuthorityPublishedPolicy   EvidenceAuthority = "published_policy"
	AuthorityDerived           EvidenceAuthority = "derived"
)

type EvidenceCompleteness string

const (
	EvidenceComplete EvidenceCompleteness = "complete"
	EvidencePartial  EvidenceCompleteness = "partial"
	EvidenceMissing  EvidenceCompleteness = "missing"
	EvidenceStale    EvidenceCompleteness = "stale"
)

// EvidenceEnvelopeV1 identifies the origin and limits of one model-bound
// piece of evidence. Business record contents belong to separately approved
// projections, not to this metadata envelope.
type EvidenceEnvelopeV1 struct {
	SchemaVersion   int                  `json:"schema_version"`
	ResourceRef     string               `json:"resource_ref"`
	SourceType      EvidenceSourceType   `json:"source_type"`
	ResourceVersion *string              `json:"resource_version,omitempty"`
	Authority       EvidenceAuthority    `json:"authority"`
	Completeness    EvidenceCompleteness `json:"completeness"`
	RetrievedAt     time.Time            `json:"retrieved_at"`
	ValidUntil      *time.Time           `json:"valid_until,omitempty"`
}

var (
	ErrInvalidEvidence = errors.New("invalid AI evidence")
	ErrExpiredEvidence = errors.New("AI evidence is expired")
)

func (e EvidenceEnvelopeV1) Validate(now time.Time) error {
	if now.IsZero() {
		return fmt.Errorf("%w: validation time is required", ErrInvalidEvidence)
	}
	if e.SchemaVersion != EvidenceSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version", ErrInvalidEvidence)
	}
	if strings.TrimSpace(e.ResourceRef) == "" || !validEvidenceSource(e.SourceType) {
		return fmt.Errorf("%w: missing resource reference or source type", ErrInvalidEvidence)
	}
	if e.ResourceVersion != nil && strings.TrimSpace(*e.ResourceVersion) == "" {
		return fmt.Errorf("%w: empty resource version", ErrInvalidEvidence)
	}
	switch e.Authority {
	case AuthorityMerchantRecord, AuthorityMerchantVerified, AuthoritySystemRecord,
		AuthorityCustomerStatement, AuthorityPublishedPolicy, AuthorityDerived:
	default:
		return fmt.Errorf("%w: unknown authority", ErrInvalidEvidence)
	}
	switch e.Completeness {
	case EvidenceComplete, EvidencePartial, EvidenceMissing, EvidenceStale:
	default:
		return fmt.Errorf("%w: unknown completeness", ErrInvalidEvidence)
	}
	if e.RetrievedAt.IsZero() || e.RetrievedAt.After(now) {
		return fmt.Errorf("%w: invalid retrieval timestamp", ErrInvalidEvidence)
	}
	if e.ValidUntil != nil {
		if e.ValidUntil.Before(e.RetrievedAt) {
			return fmt.Errorf("%w: expiration predates retrieval", ErrInvalidEvidence)
		}
		// An expired record cannot claim to be complete and current.
		if !now.Before(*e.ValidUntil) && e.Completeness == EvidenceComplete {
			return ErrExpiredEvidence
		}
	}
	// Absence of valid_until is NOT proof of perpetual freshness.
	// Domain-specific freshness rules must still run before model use.
	return nil
}

// UsableAsCurrentFact refuses stale, missing or expired data. Partial records
// can still be used for their known fields, but not to assert full coverage.
func (e EvidenceEnvelopeV1) UsableAsCurrentFact(now time.Time) bool {
	if e.Validate(now) != nil {
		return false
	}
	return e.Completeness == EvidenceComplete
}
