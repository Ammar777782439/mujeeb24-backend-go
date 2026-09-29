-- Platform Administration Contract V1 — Support Tickets + Messages
-- Per docs/architecture/PlatformAdministrationContractV1.md §37-44

CREATE TABLE support_tickets (
    id              UUID PRIMARY KEY,
    business_id    UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    subject         TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'OPEN',
    priority        TEXT NOT NULL DEFAULT 'NORMAL',
    category        TEXT NOT NULL DEFAULT 'OTHER',
    created_by      UUID NOT NULL,
    created_by_type TEXT NOT NULL,
    resolved_at     TIMESTAMPTZ,
    resolved_by     UUID,
    closed_at       TIMESTAMPTZ,
    closed_by       UUID,
    last_message_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT support_tickets_subject_not_blank_chk CHECK (length(btrim(subject)) > 0),
    CONSTRAINT support_tickets_status_chk CHECK (status IN ('OPEN', 'IN_PROGRESS', 'WAITING_MERCHANT', 'RESOLVED', 'CLOSED')),
    CONSTRAINT support_tickets_priority_chk CHECK (priority IN ('NORMAL', 'HIGH', 'CRITICAL')),
    CONSTRAINT support_tickets_category_chk CHECK (category IN ('ACCOUNT', 'CHANNEL', 'AI', 'CATALOG', 'SALES', 'BILLING', 'OTHER')),
    CONSTRAINT support_tickets_created_by_type_chk CHECK (created_by_type IN ('BUSINESS_USER', 'PLATFORM_ADMIN')),
    CONSTRAINT support_tickets_resolved_chk CHECK ((status IN ('RESOLVED', 'CLOSED')) = (resolved_at IS NOT NULL)),
    CONSTRAINT support_tickets_closed_chk CHECK ((status = 'CLOSED') = (closed_at IS NOT NULL))
);

CREATE INDEX idx_support_tickets_business ON support_tickets (business_id);
CREATE INDEX idx_support_tickets_status ON support_tickets (status);
CREATE INDEX idx_support_tickets_priority ON support_tickets (priority);
CREATE INDEX idx_support_tickets_category ON support_tickets (category);
CREATE INDEX idx_support_tickets_created_at ON support_tickets (created_at);

CREATE TABLE support_messages (
    id              UUID PRIMARY KEY,
    ticket_id      UUID NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    business_id    UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    author_type    TEXT NOT NULL,
    author_id      UUID NOT NULL,
    body           TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL,

    CONSTRAINT support_messages_author_type_chk CHECK (author_type IN ('BUSINESS_USER', 'PLATFORM_ADMIN')),
    CONSTRAINT support_messages_body_not_blank_chk CHECK (length(btrim(body)) > 0)
);

CREATE INDEX idx_support_messages_ticket ON support_messages (ticket_id);
CREATE INDEX idx_support_messages_business ON support_messages (business_id);
CREATE INDEX idx_support_messages_created_at ON support_messages (created_at);
