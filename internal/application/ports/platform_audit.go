package ports

import (
	"context"
	"time"
)

// PlatformAuditEvent is a single entry in the Platform Audit Log.
//
// Per Platform Administration Contract §45-49:
//   - Platform Audit is SEPARATE from Merchant Audit (audit_events table).
//   - APPEND ONLY — no PATCH / DELETE.
//   - NEVER stores: password, JWT, refresh token, provider secret, social
//     token, or customer message content.
//   - Records: actor (platform admin), action, target, business (nullable),
//     result, correlation_id, occurred_at, opaque metadata.
//
// All Platform Commands listed in Contract §46 must write a row:
//   business.created / .suspended / .reactivated / .archived
//   plan.created / .activated / .retired
//   subscription.created / .activated / .cancelled / .expired
//   payment.recorded
//   support.ticket.started / .resolved / .closed / support.message.created
//   ai.disabled / .enabled / provider.health_checked / channel.health_checked
//   platform.channel_action
//   ai.cost_budget_changed / subscription.cost_budget_overridden
//   plan.ai_reply_limit_changed / plan.internal_ai_cost_budget_changed
//   provider.pricing_version_changed
type PlatformAuditEvent struct {
	ID                  string
	ActorPlatformAdminID *string
	Action              string
	TargetType          string
	TargetID            *string
	BusinessID          *string
	Result              string
	FailureCode         *string
	CorrelationID       *string
	OccurredAt          time.Time
	Metadata            []byte // JSONB; defaults to {} on insert
}

// PlatformAuditDraft is the input shape for the Append method.
// ID + OccurredAt + CreatedAt are filled by the repository (server-generated
// UUID + now()); the caller supplies the substantive fields.
type PlatformAuditDraft struct {
	ActorPlatformAdminID *string
	Action              string
	TargetType          string
	TargetID            *string
	BusinessID          *string
	Result              string
	FailureCode         *string
	CorrelationID       *string
	OccurredAt          time.Time
	Metadata            []byte
}

// PlatformAuditFilter is the query input for List.
// All fields optional; nil/empty means no filter on that dimension.
type PlatformAuditFilter struct {
	ActorPlatformAdminID *string
	Action               string
	TargetType           string
	TargetID             string
	BusinessID           string
	Result               string
	OccurredFrom         *time.Time
	OccurredTo           *time.Time
	Limit                int
	Cursor               string
}

// PlatformAuditPage is the paginated List response.
type PlatformAuditPage struct {
	Items      []PlatformAuditEvent
	NextCursor string
	HasMore    bool
}

// PlatformAuditRepository is the Platform-side audit log port.
// Per Contract §48: APPEND ONLY. No Update / Delete method exists on this port.
type PlatformAuditRepository interface {
	Append(ctx context.Context, draft PlatformAuditDraft) (PlatformAuditEvent, error)
	List(ctx context.Context, filter PlatformAuditFilter) (PlatformAuditPage, error)
	GetByID(ctx context.Context, eventID string) (PlatformAuditEvent, error)
}
