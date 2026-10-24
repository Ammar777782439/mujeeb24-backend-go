-- Maintain the exact last message summarized, independent of turn counts.
-- No existing customer summary is deleted, rewritten or backfilled.
ALTER TABLE conversation_state ADD COLUMN IF NOT EXISTS summary_last_message_id UUID;
