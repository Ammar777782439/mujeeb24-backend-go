-- ADR-034 follow-up — allow ai_gemini_interactions.attempt_id to be NULL.
--
-- Per discovery D2-B (Task ID D2-B in /home/z/my-project/worklog.md):
-- ai_gemini_interactions.attempt_id is UUID NOT NULL with FK to
-- ai_run_attempts(id). However, CreateAttempt is never called anywhere in
-- the codebase, so no valid attempt_id exists today. This blocks the
-- centralized telemetry wiring in ContractClient.sendContractRequest from
-- creating ai_gemini_interactions rows for non-tool-loop Gemini calls.
--
-- ai_usage_telemetry.attempt_id is already nullable (per migration 000055).
-- This migration makes ai_gemini_interactions.attempt_id nullable too — so
-- both tables share the same nullability contract and the centralized
-- telemetry boundary can write rows without requiring an attempt row first.
--
-- This is a forward-only schema relaxation (no data migration needed — the
-- constraint is loosened, not tightened). Existing rows (if any) are
-- unaffected because they already had non-NULL attempt_id values.

ALTER TABLE ai_gemini_interactions
    ALTER COLUMN attempt_id DROP NOT NULL;
