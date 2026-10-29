package contracts

import (
	"fmt"
	"math/big"
	"time"
)

// No raw qualification_context, qualification_evidence or scoring factors
// enter these V1 projections. Evidence-specific sanitized fields require
// an independent reviewed contract before the model can see them.
type LeadAssistV1 struct {
	LeadRef            string     `json:"lead_ref"`
	CustomerRef        string     `json:"customer_ref"`
	Status             string     `json:"status"`
	QualificationState string     `json:"qualification_state"`
	CurrentScoreBand   *string    `json:"current_score_band,omitempty"`
	CurrentScoreValue  *string    `json:"current_score_value,omitempty"`
	NextActionAt       *time.Time `json:"next_action_at,omitempty"`
}

func (l LeadAssistV1) Validate() error {
	if err := requireText("lead_ref", l.LeadRef); err != nil {
		return err
	}
	if err := requireText("customer_ref", l.CustomerRef); err != nil {
		return err
	}
	if err := requireEnum("lead status", l.Status, "new", "interested", "qualified", "won", "lost"); err != nil {
		return err
	}
	if err := requireEnum("qualification_state", l.QualificationState, "new", "qualified", "working", "converted", "lost", "disqualified"); err != nil {
		return err
	}
	if l.CurrentScoreBand != nil {
		if err := requireEnum("score_band", *l.CurrentScoreBand, "unknown", "low", "medium", "high"); err != nil {
			return err
		}
	}
	if l.CurrentScoreValue != nil {
		value := *l.CurrentScoreValue
		score, ok := new(big.Rat).SetString(value)
		if !fixedDecimal2.MatchString(value) || !ok || score.Cmp(big.NewRat(100, 1)) > 0 {
			return fmt.Errorf("%w: invalid score decimal", ErrInvalidReviewedProjection)
		}
	}
	if l.NextActionAt != nil && l.NextActionAt.IsZero() {
		return fmt.Errorf("%w: invalid next action timestamp", ErrInvalidReviewedProjection)
	}
	return nil
}

type TransactionAssistV1 struct {
	TransactionRef      string  `json:"transaction_ref"`
	CustomerRef         string  `json:"customer_ref"`
	TransactionType     string  `json:"transaction_type"`
	State               string  `json:"state"`
	RequiresHumanReview bool    `json:"requires_human_review"`
	Currency            *string `json:"currency,omitempty"`
	TotalAmount         *string `json:"total_amount,omitempty"`
}

func (t TransactionAssistV1) Validate() error {
	if err := requireText("transaction_ref", t.TransactionRef); err != nil {
		return err
	}
	if err := requireText("customer_ref", t.CustomerRef); err != nil {
		return err
	}
	if err := validateTransactionType(t.TransactionType); err != nil {
		return err
	}
	if err := requireEnum("transaction state", t.State, "draft", "needs_information", "awaiting_confirmation", "confirmed", "in_fulfillment", "completed", "cancelled", "expired", "rejected"); err != nil {
		return err
	}
	if t.Currency != nil {
		if err := validateCurrency(*t.Currency); err != nil {
			return err
		}
	}
	if t.TotalAmount != nil {
		if t.Currency == nil || !fixedDecimal4.MatchString(*t.TotalAmount) {
			return fmt.Errorf("%w: invalid amount or missing currency", ErrInvalidReviewedProjection)
		}
	}
	return nil
}

func validateTransactionType(v string) error {
	return requireEnum("transaction_type", v, "order", "booking", "appointment", "service_request", "reservation", "quote", "subscription")
}

// Values are recorded commercial snapshots and do NOT represent present-day
// catalog pricing/availability. Dynamic selected attributes and other JSONB
// snapshots stay excluded pending their independent contracts.
type TransactionLineAssistV1 struct {
	ItemRef           string  `json:"item_ref"`
	NameSnapshot      string  `json:"name_snapshot"`
	Quantity          string  `json:"quantity"`
	UnitPriceSnapshot *string `json:"unit_price_snapshot,omitempty"`
	LineTotalSnapshot *string `json:"line_total_snapshot,omitempty"`
	Currency          *string `json:"currency,omitempty"`
}

func (l TransactionLineAssistV1) Validate() error {
	if err := requireText("item_ref", l.ItemRef); err != nil {
		return err
	}
	if err := requireText("name_snapshot", l.NameSnapshot); err != nil {
		return err
	}
	if !fixedDecimal4.MatchString(l.Quantity) || zeroDecimal(l.Quantity) {
		return fmt.Errorf("%w: invalid quantity", ErrInvalidReviewedProjection)
	}
	if l.Currency != nil {
		if err := validateCurrency(*l.Currency); err != nil {
			return err
		}
	}
	for _, amount := range []*string{l.UnitPriceSnapshot, l.LineTotalSnapshot} {
		if amount != nil && (l.Currency == nil || !fixedDecimal4.MatchString(*amount)) {
			return fmt.Errorf("%w: invalid price snapshot or missing currency", ErrInvalidReviewedProjection)
		}
	}
	return nil
}

func zeroDecimal(s string) bool {
	for _, c := range s {
		if c >= '1' && c <= '9' {
			return false
		}
	}
	return true
}

type TransactionReviewAssistV1 struct {
	Required bool   `json:"required"`
	Status   string `json:"status"`
}

func (r TransactionReviewAssistV1) Validate() error {
	if err := requireEnum("review status", r.Status, "pending", "approved", "rejected", "bypassed"); err != nil {
		return err
	}
	if (r.Required && r.Status == "bypassed") || (!r.Required && r.Status != "bypassed") {
		return fmt.Errorf("%w: inconsistent review lifecycle", ErrInvalidReviewedProjection)
	}
	return nil
}

type TransactionConfirmationAssistV1 struct {
	Status      string     `json:"status"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

func (c TransactionConfirmationAssistV1) Validate() error {
	if err := requireEnum("confirmation status", c.Status, "not_required", "pending", "confirmed", "rejected"); err != nil {
		return err
	}
	if (c.Status == "confirmed" || c.Status == "rejected") && (c.ConfirmedAt == nil || c.ConfirmedAt.IsZero()) {
		return fmt.Errorf("%w: decided confirmation missing timestamp", ErrInvalidReviewedProjection)
	}
	if (c.Status == "pending" || c.Status == "not_required") && c.ConfirmedAt != nil {
		return fmt.Errorf("%w: pending confirmation has decision timestamp", ErrInvalidReviewedProjection)
	}
	return nil
}

// Does not expose the raw proposal JSON or token-based confirmation requests.
// ResourceVersion is placed in EvidenceEnvelopeV1 by the authorized reader.
type SalesWorkflowAssistV1 struct {
	WorkflowRef    string  `json:"workflow_ref"`
	CustomerRef    string  `json:"customer_ref"`
	State          string  `json:"state"`
	TransactionRef *string `json:"transaction_ref,omitempty"`
	LeadRef        *string `json:"lead_ref,omitempty"`
}

func (w SalesWorkflowAssistV1) Validate() error {
	if err := requireText("workflow_ref", w.WorkflowRef); err != nil {
		return err
	}
	if err := requireText("customer_ref", w.CustomerRef); err != nil {
		return err
	}
	if w.TransactionRef != nil {
		if err := requireText("transaction_ref", *w.TransactionRef); err != nil {
			return err
		}
	}
	if w.LeadRef != nil {
		if err := requireText("lead_ref", *w.LeadRef); err != nil {
			return err
		}
	}
	return requireEnum("workflow state", w.State, "gathering", "draft", "awaiting_confirmation", "awaiting_review", "confirmed", "declined", "cancelled")
}

// Requirements are metadata-only until JSONB definitions have been
// validated against a separately reviewed schema/allowlist.
type SalesRequirementHeaderAssistV1 struct {
	TransactionType string `json:"transaction_type"`
	SchemaVersion   int    `json:"schema_version"`
}

func (s SalesRequirementHeaderAssistV1) Validate() error {
	if s.SchemaVersion <= 0 {
		return fmt.Errorf("%w: invalid requirements version", ErrInvalidReviewedProjection)
	}
	return validateTransactionType(s.TransactionType)
}
