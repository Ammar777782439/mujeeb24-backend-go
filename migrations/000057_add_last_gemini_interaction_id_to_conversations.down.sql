-- Reverse of 000057_add_last_gemini_interaction_id_to_conversations.up.sql
DROP INDEX IF EXISTS idx_conversations_last_gemini_interaction;
ALTER TABLE conversations DROP COLUMN IF EXISTS last_gemini_interaction_id;
