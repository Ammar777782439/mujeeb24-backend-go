DROP INDEX IF EXISTS sales_action_jobs_retry_claim_idx;
ALTER TABLE sales_action_jobs
 DROP COLUMN IF EXISTS next_attempt_at,
 DROP COLUMN IF EXISTS phase;
