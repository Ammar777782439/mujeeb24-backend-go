CREATE TABLE identity_matches (
    id                   UUID PRIMARY KEY,
    business_id          UUID NOT NULL,
    left_identity_id     UUID NOT NULL,
    right_identity_id    UUID NOT NULL,
    method               TEXT NOT NULL,
    evidence_references  JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence_band      TEXT NOT NULL,
    decision              TEXT NOT NULL,
    decided_by           TEXT NOT NULL,
    decided_at           TIMESTAMPTZ,
    status               TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,

    CONSTRAINT identity_matches_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT identity_matches_left_fk
        FOREIGN KEY (business_id, left_identity_id)
        REFERENCES external_identities (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT identity_matches_right_fk
        FOREIGN KEY (business_id, right_identity_id)
        REFERENCES external_identities (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT identity_matches_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT identity_matches_order_chk
        CHECK (left_identity_id < right_identity_id),
    CONSTRAINT identity_matches_method_chk
        CHECK (method IN (
            'explicit_link', 'verified_phone', 'verified_email',
            'provider_link', 'manual_confirmation', 'candidate_similarity'
        )),
    CONSTRAINT identity_matches_confidence_chk
        CHECK (confidence_band IN ('unknown', 'low', 'medium', 'high')),
    CONSTRAINT identity_matches_decision_chk
        CHECK (decision IN ('match', 'no_match', 'needs_review', 'revoked')),
    CONSTRAINT identity_matches_decided_by_chk
        CHECK (decided_by IN ('system', 'ai', 'human', 'automation')),
    CONSTRAINT identity_matches_status_chk
        CHECK (status IN ('pending', 'decided', 'revoked')),
    CONSTRAINT identity_matches_evidence_array_chk
        CHECK (jsonb_typeof(evidence_references) = 'array'),
    CONSTRAINT identity_matches_lifecycle_chk
        CHECK (
            (status = 'pending' AND decision = 'needs_review' AND decided_at IS NULL)
            OR (status = 'decided' AND decision IN ('match', 'no_match') AND decided_at IS NOT NULL)
            OR (status = 'revoked' AND decision = 'revoked' AND decided_at IS NOT NULL)
        )
);

CREATE UNIQUE INDEX uq_identity_matches_pair
    ON identity_matches (business_id, left_identity_id, right_identity_id);

CREATE INDEX idx_identity_matches_business_status
    ON identity_matches (business_id, status, updated_at DESC);
