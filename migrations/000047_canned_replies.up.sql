CREATE TABLE canned_replies (
    id               UUID PRIMARY KEY,
    business_id      UUID NOT NULL,
    title            TEXT NOT NULL,
    shortcut         TEXT NOT NULL,
    body             TEXT NOT NULL,
    status           TEXT NOT NULL,
    resource_version BIGINT NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,

    CONSTRAINT canned_replies_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT canned_replies_title_not_blank_chk
        CHECK (length(btrim(title)) > 0),
    CONSTRAINT canned_replies_shortcut_not_blank_chk
        CHECK (length(btrim(shortcut)) > 0),
    CONSTRAINT canned_replies_shortcut_normalized_chk
        CHECK (shortcut = lower(btrim(shortcut))),
    CONSTRAINT canned_replies_body_not_blank_chk
        CHECK (length(btrim(body)) > 0),
    CONSTRAINT canned_replies_status_chk
        CHECK (status IN ('active', 'archived')),
    CONSTRAINT canned_replies_resource_version_chk
        CHECK (resource_version > 0)
);

CREATE UNIQUE INDEX canned_replies_business_shortcut_uq
    ON canned_replies (business_id, shortcut);

CREATE INDEX idx_canned_replies_business_status
    ON canned_replies (business_id, status, updated_at DESC, id DESC);
