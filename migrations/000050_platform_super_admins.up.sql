CREATE TABLE platform_super_admins (
    principal_id UUID PRIMARY KEY REFERENCES principals(id) ON DELETE RESTRICT,
    status       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,

    CONSTRAINT platform_super_admins_status_chk CHECK (status IN ('active', 'revoked')),
    CONSTRAINT platform_super_admins_revocation_chk CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE INDEX platform_super_admins_active_idx
    ON platform_super_admins (principal_id)
    WHERE status = 'active';
