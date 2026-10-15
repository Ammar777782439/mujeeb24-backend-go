# One-time live LiteLLM catalog acceptance

Approved by the user after configuring the encrypted GitHub Actions secret `LITELLM_API_KEY` on 2026-10-15.

This file contains no credentials. Its creation triggers the limited authenticated live model smoke test once; the workflow trigger will then be returned to manual-only mode.

Second diagnostic pass: distinguish 403 on model listing from a direct authenticated /v1/chat/completions call. No credentials in source.
