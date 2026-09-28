CREATE TABLE team_invitations (
    id                UUID PRIMARY KEY,
    business_id       UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    email             TEXT NOT NULL,
    role              TEXT NOT NULL,
    permissions       JSONB NOT NULL DEFAULT '[]'::jsonb,
    token_hash        CHAR(64) NOT NULL UNIQUE,
    status            TEXT NOT NULL,
    invited_by        UUID NOT NULL REFERENCES principals(id) ON DELETE RESTRICT,
    accepted_by       UUID REFERENCES principals(id) ON DELETE RESTRICT,
    expires_at        TIMESTAMPTZ NOT NULL,
    accepted_at       TIMESTAMPTZ,
    revoked_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,

    CONSTRAINT team_invitations_email_not_blank_chk CHECK (length(btrim(email)) > 0),
    CONSTRAINT team_invitations_email_format_chk CHECK (email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'),
    CONSTRAINT team_invitations_role_chk CHECK (role IN ('owner', 'admin', 'manager', 'agent', 'analyst', 'viewer')),
    CONSTRAINT team_invitations_permissions_array_chk CHECK (jsonb_typeof(permissions) = 'array'),
    CONSTRAINT team_invitations_status_chk CHECK (status IN ('pending', 'accepted', 'revoked', 'expired')),
    CONSTRAINT team_invitations_expiry_after_create_chk CHECK (expires_at > created_at),
    CONSTRAINT team_invitations_acceptance_chk CHECK ((status = 'accepted') = (accepted_by IS NOT NULL AND accepted_at IS NOT NULL)),
    CONSTRAINT team_invitations_revocation_chk CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE UNIQUE INDEX team_invitations_pending_email_uq
    ON team_invitations (business_id, lower(email))
    WHERE status = 'pending';

CREATE INDEX team_invitations_business_status_idx
    ON team_invitations (business_id, status, created_at DESC);
