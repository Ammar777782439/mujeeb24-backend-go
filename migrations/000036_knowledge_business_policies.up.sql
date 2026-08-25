CREATE TABLE knowledge_documents (
    id                UUID PRIMARY KEY,
    business_id       UUID NOT NULL,
    knowledge_key     TEXT NOT NULL,
    title             TEXT NOT NULL,
    content           TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    source_reference  TEXT NOT NULL,
    authority         TEXT NOT NULL,
    status            TEXT NOT NULL,
    version           INTEGER NOT NULL,
    valid_from        TIMESTAMPTZ NOT NULL,
    valid_until       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,

    CONSTRAINT knowledge_documents_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT knowledge_documents_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT knowledge_documents_key_chk
        CHECK (length(btrim(knowledge_key)) > 0),
    CONSTRAINT knowledge_documents_title_chk
        CHECK (length(btrim(title)) > 0),
    CONSTRAINT knowledge_documents_content_chk
        CHECK (length(btrim(content)) > 0),
    CONSTRAINT knowledge_documents_content_type_chk
        CHECK (content_type IN ('faq', 'hours', 'location', 'service', 'general')),
    CONSTRAINT knowledge_documents_source_chk
        CHECK (length(btrim(source_reference)) > 0),
    CONSTRAINT knowledge_documents_authority_chk
        CHECK (authority IN ('merchant', 'system')),
    CONSTRAINT knowledge_documents_status_chk
        CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT knowledge_documents_version_chk
        CHECK (version > 0),
    CONSTRAINT knowledge_documents_validity_chk
        CHECK (valid_until IS NULL OR valid_until > valid_from),
    CONSTRAINT knowledge_documents_key_version_uq
        UNIQUE (business_id, knowledge_key, version)
);

CREATE UNIQUE INDEX uq_knowledge_documents_published_key
    ON knowledge_documents (business_id, knowledge_key)
    WHERE status = 'published';

CREATE INDEX idx_knowledge_documents_published
    ON knowledge_documents (business_id, status, valid_from DESC, id DESC)
    WHERE status = 'published';

CREATE TABLE business_policy_versions (
    id                UUID PRIMARY KEY,
    business_id       UUID NOT NULL,
    policy_key        TEXT NOT NULL,
    category          TEXT NOT NULL,
    title             TEXT NOT NULL,
    summary           TEXT NOT NULL,
    rules             JSONB NOT NULL,
    authority         TEXT NOT NULL,
    status            TEXT NOT NULL,
    version           INTEGER NOT NULL,
    valid_from        TIMESTAMPTZ NOT NULL,
    valid_until       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,

    CONSTRAINT business_policy_versions_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT business_policy_versions_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT business_policy_versions_key_chk
        CHECK (length(btrim(policy_key)) > 0),
    CONSTRAINT business_policy_versions_category_chk
        CHECK (category IN ('hours', 'returns', 'delivery', 'pricing', 'availability', 'ai_mode', 'channel', 'general')),
    CONSTRAINT business_policy_versions_title_chk
        CHECK (length(btrim(title)) > 0),
    CONSTRAINT business_policy_versions_summary_chk
        CHECK (length(btrim(summary)) > 0),
    CONSTRAINT business_policy_versions_rules_object_chk
        CHECK (jsonb_typeof(rules) = 'object'),
    CONSTRAINT business_policy_versions_authority_chk
        CHECK (authority = 'merchant'),
    CONSTRAINT business_policy_versions_status_chk
        CHECK (status IN ('draft', 'published', 'archived')),
    CONSTRAINT business_policy_versions_version_chk
        CHECK (version > 0),
    CONSTRAINT business_policy_versions_validity_chk
        CHECK (valid_until IS NULL OR valid_until > valid_from),
    CONSTRAINT business_policy_versions_key_version_uq
        UNIQUE (business_id, policy_key, version)
);

CREATE UNIQUE INDEX uq_business_policy_versions_published_key
    ON business_policy_versions (business_id, policy_key)
    WHERE status = 'published';

CREATE INDEX idx_business_policy_versions_published_category
    ON business_policy_versions (business_id, category, valid_from DESC, id DESC)
    WHERE status = 'published';
