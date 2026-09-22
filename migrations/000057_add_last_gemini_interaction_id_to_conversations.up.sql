-- ADR-027 — Add last_gemini_interaction_id to conversations per contract ③ §4.
--
-- Per contract ③ §4, Mujeeb persists the last gemini_interaction_id on the
-- conversation row so the next customer turn can use it as previous_interaction_id
-- (Gemini Interactions API chaining with store=true per contract ③ §9).
--
-- Per contract ③ §5: "Mujeeb retention = canonical; Gemini retention = convenience
-- only." Mujeeb does NOT lose conversation, messages, state, or business context
-- when Gemini is unavailable. This column just enables continuity.
--
-- The column is nullable: NULL means no previous Gemini interaction (first turn
-- or after Gemini history expiry — 1 day free tier, 55 days paid tier per ③ §9).
--
-- We do NOT add a CHECK constraint: the value is opaque to Mujeeb (Gemini
-- generates it), and Mujeeb only stores + passes it back.

ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS last_gemini_interaction_id TEXT;

-- Index for filtering by interaction (used by trace queries per contract ⑧ §17).
CREATE INDEX IF NOT EXISTS idx_conversations_last_gemini_interaction
    ON conversations (last_gemini_interaction_id)
    WHERE last_gemini_interaction_id IS NOT NULL;
