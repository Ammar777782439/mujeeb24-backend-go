# One-time live LiteLLM catalog acceptance

Approved by the user after configuring the encrypted GitHub Actions secret `LITELLM_API_KEY` on 2026-10-15.

This file contains no credentials. Its creation triggers the limited authenticated live model smoke test once; the workflow trigger will then be returned to manual-only mode.

Second diagnostic pass: distinguish 403 on model listing from a direct authenticated /v1/chat/completions call. No credentials in source.

Final diagnostic: classify HTTP 403 issuer as JSON API, HTML proxy, or Cloudflare challenge without recording response bodies or secrets.

User-requested rerun after confirming GitHub Actions repository secret LITELLM_API_KEY was set. Execute one authenticated catalog model test and inspect results. No key value is stored here.

Architecture acceptance after removing overview/hierarchy/detail/compare presentation modes: run the production Go catalog adapter against LiteLLM once and require supplier_check + requires_check to end in handoff_to_human after complete catalog evidence. No credentials or merchant data are stored here.
