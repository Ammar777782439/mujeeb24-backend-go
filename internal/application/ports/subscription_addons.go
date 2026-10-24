package ports

import (
	"context"
	"time"
)

// SubscriptionAddonRepository owns period-bound, manually paid commercial
// capacity. Add-ons are immutable after activation and never roll into a new
// subscription period.
type SubscriptionAddonRepository interface {
	CreatePending(context.Context, SubscriptionAddonCreate) (SubscriptionAddonRecord, error)
	List(context.Context, SubscriptionAddonListFilter) (SubscriptionAddonPage, error)
	RecordPaymentAndActivate(context.Context, SubscriptionAddonPayment) (SubscriptionAddonRecord, error)
	Cancel(context.Context, SubscriptionAddonCancel) (SubscriptionAddonRecord, error)
	ApproveChannelPassCapacity(context.Context, ChannelPassCapacityApproval) (ChannelPassCapacityApprovalRecord, error)
}

type SubscriptionAddonRecord struct {
	ID, SubscriptionID, BusinessID, Kind, Status                          string
	PriceYERSnapshot, AIReplyDelta, CatalogDelta, SeatDelta, ChannelDelta int
	ActivatedAt                                                           *time.Time
	CancelledAt                                                           *time.Time
	CancelledReason                                                       *string
	CreatedAt                                                             time.Time
}
type SubscriptionAddonCreate struct {
	SubscriptionID, BusinessID, Kind, CreatedBy, IdempotencyKey string
	Now                                                         time.Time
}
type SubscriptionAddonPayment struct {
	AddonID, RecordedBy, IdempotencyKey, Method, Reference string
	AmountYER                                              int
	PaidAt, Now                                            time.Time
}

type SubscriptionAddonCancel struct {
	AddonID, CancelledBy, Reason string
	Now                          time.Time
}

type SubscriptionAddonListFilter struct {
	SubscriptionID string
	Status         string
	Limit          int
	Cursor         string
}

type SubscriptionAddonPage struct {
	Items      []SubscriptionAddonRecord
	NextCursor string
	HasMore    bool
}

// ChannelPassCapacityApproval records the manual, external confirmation that
// SocialAPI capacity has been reserved for this subscription period. It is a
// local audit record only; no provider call is made from a database transaction.
type ChannelPassCapacityApproval struct {
	SubscriptionID, BusinessID, ApprovedBy, Reason string
	MaxChannelPasses                               int
	Now                                            time.Time
}

type ChannelPassCapacityApprovalRecord struct {
	SubscriptionID, BusinessID, ApprovedBy, Reason string
	MaxChannelPasses                               int
	ApprovedAt, UpdatedAt                          time.Time
}
