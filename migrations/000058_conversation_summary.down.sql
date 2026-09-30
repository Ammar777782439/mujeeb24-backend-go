-- ADR-039: Rollback — drop summary columns from conversation_state.
ALTER TABLE conversation_state
    DROP COLUMN IF EXISTS summary,
    DROP COLUMN IF EXISTS summary_turn_count;
