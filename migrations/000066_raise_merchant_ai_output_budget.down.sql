-- Restore the previous 700-token merchant AI budget for the active Gemini configuration.
UPDATE ai_configuration_versions AS cfg
SET
    mujeeb_max_output_tokens = 700,
    effective_max_output_tokens = LEAST(
        700,
        COALESCE(model.output_token_limit, 700)
    ),
    reason = 'Rollback merchant catalog AI output budget change'
FROM ai_provider_models AS model
WHERE cfg.status = 'ACTIVE'
  AND cfg.provider = 'google_gemini'
  AND model.provider = cfg.provider
  AND model.model_name = cfg.model;
