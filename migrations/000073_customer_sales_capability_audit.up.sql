-- Preserve the exact B2C model choice through audit without inventing intent.
-- Historical and B2B decisions retain NULL capability fields; no inferred backfill.
ALTER TABLE ai_decisions
 ADD COLUMN requested_capability TEXT,
 ADD COLUMN effective_capability TEXT,
 ADD CONSTRAINT ai_decisions_requested_capability_chk CHECK (requested_capability IS NULL OR length(btrim(requested_capability)) > 0),
 ADD CONSTRAINT ai_decisions_effective_capability_chk CHECK (effective_capability IS NULL OR (requested_capability IS NOT NULL AND effective_capability = requested_capability));

-- Legacy intent remains required for old callers; capability decisions use empty intent.
ALTER TABLE ai_decisions DROP CONSTRAINT ai_decisions_intent_chk;
ALTER TABLE ai_decisions ADD CONSTRAINT ai_decisions_intent_chk
 CHECK (length(btrim(intent_base)) > 0 OR requested_capability IS NOT NULL);

-- Rejected/unimplemented capability names have no truthful legacy action.
ALTER TABLE ai_decisions DROP CONSTRAINT ai_decisions_action_chk;
ALTER TABLE ai_decisions ADD CONSTRAINT ai_decisions_action_chk CHECK (
 requested_action IN ('answer', 'clarification', 'human_request', 'lead_draft', 'order_draft')
 OR (requested_capability IS NOT NULL AND requested_action = '' AND lifecycle = 'expired')
);
