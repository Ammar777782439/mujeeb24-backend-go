-- Existing postback job remains the only durable carrier for consent continuation.
ALTER TABLE sales_action_jobs
 ADD COLUMN phase TEXT NOT NULL DEFAULT 'apply_action'
    CHECK (phase IN ('apply_action','continue_ai')),
 ADD COLUMN next_attempt_at TIMESTAMPTZ;
CREATE INDEX sales_action_jobs_retry_claim_idx
 ON sales_action_jobs(state,next_attempt_at,lease_until,created_at)
 WHERE state IN ('pending','processing');
