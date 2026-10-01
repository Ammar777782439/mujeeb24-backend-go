-- Raise the default merchant AI output budget for structured catalog proposals.
-- Keep the provider/model limit authoritative when it is lower than the Mujeeb cap.
UPDATE ai_configuration_versions AS cfg
SET
    mujeeb_max_output_tokens = 4096,
    effective_max_output_tokens = LEAST(
        4096,
        COALESCE(model.output_token_limit, 4096)
    ),
    reason = 'Raise merchant catalog AI output budget for structured proposals'
FROM ai_provider_models AS model
WHERE cfg.status = 'ACTIVE'
  AND cfg.provider = 'google_gemini'
  AND model.provider = cfg.provider
  AND model.model_name = cfg.model;
