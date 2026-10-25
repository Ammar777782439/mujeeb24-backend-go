package contracts

import (
	"errors"
	"fmt"
	"strings"
)

// BusinessContextV1 is a small merchant-facing projection. It is not a SQL
// row, permission decision or provider payload until an authorized reader
// supplies and validates it. Tenant ID, slug, and internal timestamps stay
// outside the model-facing fields.
type BusinessContextV1 struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	VerticalType    string `json:"vertical_type"`
	Timezone        string `json:"timezone"`
	DefaultCurrency string `json:"default_currency"`
	Locale          string `json:"locale"`
}

// BusinessRuntimePolicyContextV1 describes the *recorded* runtime policy.
// These flags are information only: they do not grant the model permission
// to reply, approve, create a lead, or perform any business action.
type BusinessRuntimePolicyContextV1 struct {
	AIMode                    string `json:"ai_mode"`
	DefaultHumanReview        bool   `json:"default_human_review"`
	AllowAutoReply            bool   `json:"allow_auto_reply"`
	AllowAutoLeadCreation     bool   `json:"allow_auto_lead_creation"`
	AllowAutoTransactionDraft bool   `json:"allow_auto_transaction_draft"`
	AllowAutoConfirmation     bool   `json:"allow_auto_confirmation"`
	SalesWorkflowEnabled      bool   `json:"sales_workflow_enabled"`
	AllowSalesInteractions    bool   `json:"allow_sales_interactions"`
}

var ErrInvalidBusinessProjection = errors.New("invalid business AI projection")

// Values are taken from the PostgreSQL 16 baseline generated after
// migrations 000001..000081; do not infer additional enum members.
func (b BusinessContextV1) Validate() error {
	if strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Timezone) == "" ||
		strings.TrimSpace(b.Locale) == "" {
		return fmt.Errorf("%w: missing required business context", ErrInvalidBusinessProjection)
	}
	switch b.Status {
	case "pending_setup", "active", "suspended", "archived":
	default:
		return fmt.Errorf("%w: unknown business status", ErrInvalidBusinessProjection)
	}
	switch b.VerticalType {
	case "retail", "travel", "services", "restaurant", "clinic",
		"hospitality", "education", "real_estate", "other":
	default:
		return fmt.Errorf("%w: unknown business vertical", ErrInvalidBusinessProjection)
	}
	if len(b.DefaultCurrency) != 3 {
		return fmt.Errorf("%w: invalid currency code length", ErrInvalidBusinessProjection)
	}
	for _, ch := range b.DefaultCurrency {
		if ch < 'A' || ch > 'Z' {
			return fmt.Errorf("%w: currency must use three uppercase ASCII letters", ErrInvalidBusinessProjection)
		}
	}
	return nil
}

func (p BusinessRuntimePolicyContextV1) Validate() error {
	switch p.AIMode {
	case "disabled", "assist", "approval", "restricted_auto":
		return nil
	default:
		return fmt.Errorf("%w: unknown AI mode", ErrInvalidBusinessProjection)
	}
}
