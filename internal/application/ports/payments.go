package ports

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// Manual Payment Recording — Platform Administration Contract V1 §25-28
// ----------------------------------------------------------------------------
//
// Contract rules (CLOSED):
//
//   §25 — Manual payment model. NO Visa/Stripe/PayPal integration in V1.
//          Platform Admin records the payment after verifying it OUTSIDE Mujeeb.
//
//   §26 — Payment methods: CASH | BANK_TRANSFER | MOBILE_MONEY | OTHER.
//          No provider-specific payment logic lives in the Subscription Domain.
//
//   §27 — Payment Record is APPEND ONLY. The PaymentRepository port has NO
//          Update / Delete method. Corrections go through a new documented
//          adjustment entry — not silent edits.
//
//   §28 — Recording a payment against a PENDING subscription ACTIVATES it
//          atomically in the same transaction. A subscription never becomes
//          ACTIVE without a recorded payment (or a documented admin
//          activation decision — TODO if needed in V1).
// ----------------------------------------------------------------------------

// PaymentRecord is an immutable record of a single manual payment.
//
// Per Contract §27: append-only. No field can be edited after creation.
// A correction is a NEW PaymentRecord with reference="CORRECTION:{original_payment_id}"
// and a negative amount if a refund was issued (TODO: V1 may not need negative;
// the rule is "no edit" — corrections are documented separately).
type PaymentRecord struct {
	ID             string
	SubscriptionID string
	BusinessID     string
	AmountYER      int
	Method         string
	Reference      string
	PaidAt         time.Time
	RecordedBy     string
	CreatedAt      time.Time
}

// PaymentCreate is the input to Append. The repository generates ID + CreatedAt.
type PaymentCreate struct {
	SubscriptionID string
	BusinessID     string
	AmountYER      int
	Method         string
	Reference      string
	PaidAt         time.Time
	RecordedBy     string
	Now            time.Time
}

// PaymentListFilter is the query input to List.
type PaymentListFilter struct {
	SubscriptionID string
	BusinessID     string
	Method         string
	Limit          int
	Cursor         string
}

type PaymentPage struct {
	Items      []PaymentRecord
	NextCursor string
	HasMore    bool
}

// PaymentRepository is the Platform-side payment port.
//
// Per Contract §27: APPEND ONLY. There is intentionally NO Update / Delete method.
type PaymentRepository interface {
	Append(ctx context.Context, create PaymentCreate) (PaymentRecord, error)
	List(ctx context.Context, filter PaymentListFilter) (PaymentPage, error)
	GetByID(ctx context.Context, paymentID string) (PaymentRecord, error)
}
