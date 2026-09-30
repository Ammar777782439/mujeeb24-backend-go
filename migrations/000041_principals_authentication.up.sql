CREATE TABLE principals (
    id            UUID PRIMARY KEY,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    status        TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,

    CONSTRAINT principals_email_not_blank_chk CHECK (length(btrim(email)) > 0),
    CONSTRAINT principals_email_format_chk CHECK (email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'),
    CONSTRAINT principals_display_name_not_blank_chk CHECK (length(btrim(display_name)) > 0),
    CONSTRAINT principals_password_hash_not_blank_chk CHECK (length(btrim(password_hash)) > 0),
    CONSTRAINT principals_status_chk CHECK (status IN ('active', 'suspended'))
);

CREATE UNIQUE INDEX principals_email_ci_uq ON principals (lower(email));

CREATE TABLE business_memberships (
    business_id  UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    principal_id UUID NOT NULL REFERENCES principals(id) ON DELETE RESTRICT,
    role         TEXT NOT NULL,
    permissions  JSONB NOT NULL DEFAULT '[]'::jsonb,
    status       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (business_id, principal_id),
    CONSTRAINT business_memberships_role_chk CHECK (role IN ('owner', 'admin', 'manager', 'agent', 'analyst', 'viewer')),
    CONSTRAINT business_memberships_status_chk CHECK (status IN ('active', 'revoked')),
    CONSTRAINT business_memberships_permissions_array_chk CHECK (jsonb_typeof(permissions) = 'array')
);

CREATE INDEX business_memberships_principal_active_idx
    ON business_memberships (principal_id, business_id)
    WHERE status = 'active';

CREATE TABLE refresh_sessions (
    id           UUID PRIMARY KEY,
    principal_id UUID NOT NULL REFERENCES principals(id) ON DELETE RESTRICT,
    token_hash   CHAR(64) NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,

    CONSTRAINT refresh_sessions_token_hash_uq UNIQUE (token_hash),
    CONSTRAINT refresh_sessions_expiry_after_create_chk CHECK (expires_at > created_at)
);

CREATE INDEX refresh_sessions_principal_active_idx
    ON refresh_sessions (principal_id, expires_at)
    WHERE revoked_at IS NULL;
