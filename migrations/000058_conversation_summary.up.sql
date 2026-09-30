-- ADR-039: Conversation Summary + Sliding Window Hybrid
-- Per Microsoft Learn, getmaxim.ai, IrisAgent research: best-practice pattern
-- for managing long conversation context in LLM agents.
--
-- Instead of sending last N messages verbatim, we maintain a running summary
-- of older turns + a small sliding window of recent messages. This:
--   1. Reduces token consumption (~50% for long chats)
--   2. Preserves long-term context that falls out of the sliding window
--   3. Improves intent understanding (Gemini sees compressed history + fresh context)
--
-- Schema:
--   summary           TEXT    — the running LLM-generated summary of older turns
--   summary_turn_count INTEGER — the conversation turn count at the time the
--                               summary was last generated. We use this to
--                               decide when to regenerate (every 4 turns).

ALTER TABLE conversation_state
    ADD COLUMN IF NOT EXISTS summary            TEXT,
    ADD COLUMN IF NOT EXISTS summary_turn_count INTEGER NOT NULL DEFAULT 0
        CHECK (summary_turn_count >= 0);

-- Backfill existing rows: empty summary, 0 turn count (no summary yet)
UPDATE conversation_state
SET summary = NULL,
    summary_turn_count = 0
WHERE summary_turn_count IS NULL;
