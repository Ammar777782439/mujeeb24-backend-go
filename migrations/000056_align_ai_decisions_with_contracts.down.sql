-- Reverse of 000056_align_ai_decisions_with_contracts.up.sql
ALTER TABLE ai_decisions DROP CONSTRAINT IF EXISTS ai_decisions_effective_action_chk;
ALTER TABLE ai_decisions DROP CONSTRAINT IF EXISTS ai_decisions_ai_run_fk;
ALTER TABLE ai_decisions DROP INDEX IF EXISTS idx_ai_decisions_run;
ALTER TABLE ai_decisions DROP COLUMN IF EXISTS ai_run_id;
ALTER TABLE ai_decisions DROP COLUMN IF EXISTS effective_action;
ALTER TABLE ai_decisions DROP COLUMN IF EXISTS effective_decision_reason;
ALTER TABLE ai_decisions DROP COLUMN IF EXISTS authorized_at;
ALTER TABLE ai_decisions DROP COLUMN IF EXISTS executed_at;

ALTER TABLE ai_decisions DROP CONSTRAINT IF EXISTS ai_decisions_policy_decision_chk;
ALTER TABLE ai_decisions DROP CONSTRAINT IF EXISTS ai_decisions_lifecycle_chk;
ALTER TABLE ai_decisions DROP CONSTRAINT IF EXISTS ai_decisions_action_chk;

-- Restore old constraints (best-effort; data was migrated so old enums would be a subset).
ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_action_chk
        CHECK (requested_action IN (
            'answer', 'ask_clarification', 'create_lead', 'update_lead',
            'create_transaction_draft', 'request_availability_check',
            'request_price_check', 'request_human', 'no_action'
        ));
ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_lifecycle_chk
        CHECK (lifecycle IN ('proposed', 'validated', 'policy_evaluated', 'expired', 'rejected'));
ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_policy_decision_chk
        CHECK (policy_decision IS NULL OR policy_decision IN ('allowed', 'requires_approval', 'denied'));
