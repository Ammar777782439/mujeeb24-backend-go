-- Preserve optional verified USD cost metadata and historical usage attribution.
-- Legacy provider_cost_yer rows remain valid for historical reporting.
ALTER TABLE ai_usage_records
    ADD COLUMN gateway_cost_usd NUMERIC(24,12),
    ADD COLUMN gateway_fx_yer_per_usd NUMERIC(24,8),
    ADD COLUMN gateway_call_id TEXT,
    ADD COLUMN gateway_deployment_id TEXT,
    ADD CONSTRAINT ai_usage_gateway_cost_nonnegative CHECK (gateway_cost_usd IS NULL OR gateway_cost_usd >= 0),
    ADD CONSTRAINT ai_usage_gateway_fx_positive CHECK (gateway_fx_yer_per_usd IS NULL OR gateway_fx_yer_per_usd > 0),
    ADD CONSTRAINT ai_usage_gateway_cost_fx_pair CHECK ((gateway_cost_usd IS NULL) = (gateway_fx_yer_per_usd IS NULL));
CREATE UNIQUE INDEX ai_usage_gateway_call_id_uq ON ai_usage_records (gateway_call_id) WHERE gateway_call_id IS NOT NULL;
