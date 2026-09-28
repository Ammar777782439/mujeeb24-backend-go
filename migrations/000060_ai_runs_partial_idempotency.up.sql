-- ADR-043: Make ai_runs.idempotency_key unique only for non-terminal runs.
--
-- Per contract ⑨ §16, idempotency_key prevents duplicate AI Runs — but
-- only for in-flight (non-terminal) runs. A past completed/failed/cancelled
-- run is the answer for the past turn, not for the current turn. When the
-- merchant sends the same message again in a new turn (or in a new session),
-- we create a fresh Run — and that requires the idempotency_key constraint
-- to allow duplicates when the previous run is terminal.
--
-- Before this migration: uq_ai_runs_idempotency was a full UNIQUE index,
-- which caused INSERT failures when the agent tried to create a new Run
-- with the same idempotency_key after a previous run completed. The agent
-- then returned errIdempotencyViolation, which manifested as:
--   illegal AI Run transition: completed → context_built
--
-- After this migration: the UNIQUE constraint only applies when status is
-- NOT IN ('completed', 'failed', 'cancelled'). This allows:
--   - Concurrent double-clicks within the same turn → second INSERT fails
--     with unique violation → agent returns the first run (dedupe).
--   - Sequential turns with the same idempotency_key after a terminal run
--     → both INSERTs succeed (different rows, different run IDs).
--
-- Per Postgres docs: a partial unique index is the idiomatic way to
-- express "unique within a subset of rows".

DROP INDEX IF EXISTS uq_ai_runs_idempotency;

CREATE UNIQUE INDEX uq_ai_runs_idempotency
    ON ai_runs (business_id, idempotency_key)
    WHERE status NOT IN ('completed', 'failed', 'cancelled');
