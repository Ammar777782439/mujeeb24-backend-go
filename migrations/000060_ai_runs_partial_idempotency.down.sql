-- ADR-043 rollback — restore the full UNIQUE index on idempotency_key.
DROP INDEX IF EXISTS uq_ai_runs_idempotency;

CREATE UNIQUE INDEX uq_ai_runs_idempotency
    ON ai_runs (business_id, idempotency_key);
