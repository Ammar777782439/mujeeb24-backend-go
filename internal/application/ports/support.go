package ports

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// Support Management — Platform Administration Contract V1 §37-44
// ----------------------------------------------------------------------------
//
// Contract rules (CLOSED):
//
//   §37 — Support is a Platform domain. NO Chatwoot — that's permanently
//         cancelled per Contract §97.
//
//   §38 — Ticket is linked to a Business:
//         Business → Support Ticket → Support Messages.
//         The business_id is for cross-reference only — Platform Admin
//         reads/writes support data; they do NOT get merchant membership.
//
//   §39 — Ticket states (forward-only transitions):
//         OPEN → IN_PROGRESS → WAITING_MERCHANT → IN_PROGRESS (loop)
//                                       → RESOLVED → CLOSED
//         Per Contract §43: the merchant/admin ping-pong can move a ticket
//         between IN_PROGRESS and WAITING_MERCHANT multiple times.
//         RESOLVED is reached when support fixes the issue; CLOSED is the
//         terminal state (final archival).
//
//   §40 — Priority: NORMAL | HIGH | CRITICAL.
//
//   §41 — Category: ACCOUNT | CHANNEL | AI | CATALOG | SALES | BILLING | OTHER.
//
//   §42 — Message author_type: BUSINESS_USER | PLATFORM_ADMIN. No file
//         attachments in V1 (Media Upload API is not part of this contract).
//
//   §44 — APIs (registered as platformListSupportTickets / platformGetSupportTicket
//          / platformCreateSupportMessage / platformStartSupportTicket /
//          platformResolveSupportTicket / platformCloseSupportTicket in
//          contract/platform_operations.go).
// ----------------------------------------------------------------------------

// SupportTicketRecord is a single support ticket.
type SupportTicketRecord struct {
	ID            string
	BusinessID    string
	Subject       string
	Status        string
	Priority      string
	Category      string
	CreatedBy     string
	CreatedByType string
	ResolvedAt    *time.Time
	ResolvedBy    *string
	ClosedAt      *time.Time
	ClosedBy      *string
	LastMessageAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SupportTicketCreate is the input to CreateTicket. Note: the platform
// contract §44 only exposes Platform-side lifecycle commands (start/resolve/
// close) and message creation. The "Create Ticket" flow (§43) is initiated
// by the merchant — that lives on the merchant-side API surface (TODO when
// wiring merchant support). For now, the Platform Support port exposes a
// Create method so the platform admin can create a ticket on behalf of a
// business (e.g., to log an inbound phone complaint).
type SupportTicketCreate struct {
	ID            string
	BusinessID    string
	Subject       string
	Priority      string
	Category      string
	CreatedBy     string
	CreatedByType string
	Now           time.Time
}

// SupportTicketListFilter is the query input to List. Per Contract §44:
// supports status, priority, category, business, date range filters.
type SupportTicketListFilter struct {
	BusinessID  string
	Status      string
	Priority    string
	Category    string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
	Cursor      string
}

type SupportTicketPage struct {
	Items      []SupportTicketRecord
	NextCursor string
	HasMore    bool
}

// SupportMessageRecord is a single message in a support ticket.
type SupportMessageRecord struct {
	ID         string
	TicketID   string
	BusinessID string
	AuthorType string
	AuthorID   string
	Body       string
	CreatedAt  time.Time
}

// SupportMessageCreate is the input to AppendMessage. Per Contract §42:
// author_type is BUSINESS_USER or PLATFORM_ADMIN. The repository enforces
// the CHECK constraint at the SQL layer.
type SupportMessageCreate struct {
	ID         string
	TicketID   string
	BusinessID string
	AuthorType string
	AuthorID   string
	Body       string
	Now        time.Time
}

// SupportMessageListFilter is the query input to ListMessages.
type SupportMessageListFilter struct {
	TicketID string
	Limit    int
	Cursor   string
}

type SupportMessagePage struct {
	Items      []SupportMessageRecord
	NextCursor string
	HasMore    bool
}

// SupportRepository is the Platform-side support port.
//
// Per Contract §39: ticket transitions are forward-only with explicit
// commands (Start / Resolve / Close). There is NO "reopen" command — once
// CLOSED, the ticket is terminal (a new issue gets a new ticket).
//
// Per Contract §42: support messages are APPEND ONLY. There is NO Update /
// Delete method on SupportMessageRepository.
type SupportRepository interface {
	// Ticket lifecycle
	CreateTicket(ctx context.Context, create SupportTicketCreate) (SupportTicketRecord, error)
	GetTicketByID(ctx context.Context, ticketID string) (SupportTicketRecord, error)
	ListTickets(ctx context.Context, filter SupportTicketListFilter) (SupportTicketPage, error)
	StartTicket(ctx context.Context, ticketID, startedBy string, now time.Time) (SupportTicketRecord, error)
	ResolveTicket(ctx context.Context, ticketID, resolvedBy string, now time.Time) (SupportTicketRecord, error)
	CloseTicket(ctx context.Context, ticketID, closedBy string, now time.Time) (SupportTicketRecord, error)

	// Message lifecycle (append-only)
	AppendMessage(ctx context.Context, create SupportMessageCreate) (SupportMessageRecord, error)
	ListMessages(ctx context.Context, filter SupportMessageListFilter) (SupportMessagePage, error)
}
