DROP INDEX IF EXISTS ai_usage_gateway_call_id_uq;
ALTER TABLE ai_usage_records
    DROP CONSTRAINT IF EXISTS ai_usage_gateway_cost_fx_pair,
    DROP CONSTRAINT IF EXISTS ai_usage_gateway_fx_positive,
    DROP CONSTRAINT IF EXISTS ai_usage_gateway_cost_nonnegative,
    DROP COLUMN IF EXISTS gateway_deployment_id,
    DROP COLUMN IF EXISTS gateway_call_id,
    DROP COLUMN IF EXISTS gateway_fx_yer_per_usd,
    DROP COLUMN IF EXISTS gateway_cost_usd;
