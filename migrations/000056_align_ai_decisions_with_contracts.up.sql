-- ADR-033 — Align ai_decisions.requested_action and lifecycle with contracts ④ ⑥ ⑨
-- Breaking change: no backward compatibility with old enum values.
-- Contract ④ §5: Allowed actions = answer, clarification, human_request, lead_draft, order_draft.
-- Contract ⑥ §17: Effective Decision separated from AI Proposal; statuses for AI layer only.
-- Contract ⑨ §2: AI Run lifecycle is in ai_runs.status (separate table). ai_decisions.lifecycle
--   now means the decision's lifecycle (proposed→validated→authorized→executed/expired),
--   not the operational run state.

-- Step 1: drop old check constraints so we can re-write the enum.
ALTER TABLE ai_decisions
    DROP CONSTRAINT IF EXISTS ai_decisions_action_chk;
ALTER TABLE ai_decisions
    DROP CONSTRAINT IF EXISTS ai_decisions_lifecycle_chk;
ALTER TABLE ai_decisions
    DROP CONSTRAINT IF EXISTS ai_decisions_policy_decision_chk;

-- Step 2: re-map old action values to the contract ④ set.
-- Mapping rationale:
--   answer                  -> answer
--   ask_clarification       -> clarification
--   request_human           -> human_request
--   create_lead             -> lead_draft
--   update_lead             -> lead_draft  (merged: lead update is still a draft proposal)
--   create_transaction_draft -> order_draft
--   request_availability_check -> clarification (asking for missing availability info)
--   request_price_check       -> clarification (asking for missing price info)
--   no_action                 -> answer (treat as informational answer with no side effects)
UPDATE ai_decisions SET requested_action = 'answer'             WHERE requested_action = 'answer';
UPDATE ai_decisions SET requested_action = 'clarification'      WHERE requested_action = 'ask_clarification';
UPDATE ai_decisions SET requested_action = 'clarification'      WHERE requested_action = 'request_availability_check';
UPDATE ai_decisions SET requested_action = 'clarification'      WHERE requested_action = 'request_price_check';
UPDATE ai_decisions SET requested_action = 'human_request'      WHERE requested_action = 'request_human';
UPDATE ai_decisions SET requested_action = 'lead_draft'         WHERE requested_action = 'create_lead';
UPDATE ai_decisions SET requested_action = 'lead_draft'         WHERE requested_action = 'update_lead';
UPDATE ai_decisions SET requested_action = 'order_draft'        WHERE requested_action = 'create_transaction_draft';
UPDATE ai_decisions SET requested_action = 'answer'             WHERE requested_action = 'no_action';

-- Step 3: re-map old lifecycle values to the new contract-aligned set.
-- Mapping:
--   proposed            -> proposed       (AI Proposal produced, awaiting validation)
--   validated           -> validated      (Structural+Reference+Tenant validation passed)
--   policy_evaluated    -> authorized     (Policy + Authorization finished; ready for execution)
--   expired             -> expired
--   rejected            -> expired        (rejected decision is effectively expired; failure is in ai_runs)
UPDATE ai_decisions SET lifecycle = 'proposed'   WHERE lifecycle = 'proposed';
UPDATE ai_decisions SET lifecycle = 'validated'   WHERE lifecycle = 'validated';
UPDATE ai_decisions SET lifecycle = 'authorized'  WHERE lifecycle = 'policy_evaluated';
UPDATE ai_decisions SET lifecycle = 'expired'     WHERE lifecycle = 'expired';
UPDATE ai_decisions SET lifecycle = 'expired'     WHERE lifecycle = 'rejected';

-- Add the new "executed" lifecycle for decisions whose Effective Decision was successfully executed.
-- (No data migration needed; only new rows will use this.)

-- Step 4: re-introduce check constraints with the contract-aligned enums.
ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_action_chk
        CHECK (requested_action IN (
            'answer', 'clarification', 'human_request', 'lead_draft', 'order_draft'
        ));

ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_lifecycle_chk
        CHECK (lifecycle IN ('proposed', 'validated', 'authorized', 'executed', 'expired'));

-- Contract ⑥: PolicyDecision values per PolicyEvaluator
ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_policy_decision_chk
        CHECK (policy_decision IS NULL OR policy_decision IN ('allowed', 'requires_approval', 'denied'));

-- Step 5: Add column to link ai_decisions to the operational ai_run_id.
-- This is the bridge between the business decision row and the operational trace.
ALTER TABLE ai_decisions
    ADD COLUMN IF NOT EXISTS ai_run_id UUID;

ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_ai_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_ai_decisions_run
    ON ai_decisions (ai_run_id)
    WHERE ai_run_id IS NOT NULL;

-- Step 6: Add Effective Decision columns to ai_decisions per contract ⑥ §17.
ALTER TABLE ai_decisions
    ADD COLUMN IF NOT EXISTS effective_action TEXT,
    ADD COLUMN IF NOT EXISTS effective_decision_reason TEXT,
    ADD COLUMN IF NOT EXISTS authorized_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS executed_at TIMESTAMPTZ;

ALTER TABLE ai_decisions
    ADD CONSTRAINT ai_decisions_effective_action_chk
        CHECK (effective_action IS NULL OR effective_action IN (
            'answer', 'clarification', 'human_request', 'lead_draft',
            'order_draft', 'no_action', 'blocked'
        ));
