#!/bin/sh
# Render Blueprint's generateValue emits a stable 256-bit base64 secret with
# no prefix. LiteLLM documents sk- keys; derive them at startup, not in Git.
set -eu
umask 077

# Preserve explicitly configured keys, especially an existing SALT key.
# Replacing a deployed salt would make provider credentials in Neon unreadable.
if [ -z "${LITELLM_MASTER_KEY:-}" ]; then
  : "${LITELLM_MASTER_KEY_SEED:?Render must generate LITELLM_MASTER_KEY_SEED}"
  export LITELLM_MASTER_KEY="sk-${LITELLM_MASTER_KEY_SEED}"
fi
if [ -z "${LITELLM_SALT_KEY:-}" ]; then
  : "${LITELLM_SALT_KEY_SEED:?Render must generate LITELLM_SALT_KEY_SEED}"
  export LITELLM_SALT_KEY="sk-${LITELLM_SALT_KEY_SEED}"
fi
: "${UI_PASSWORD:?Render must generate UI_PASSWORD}"
: "${DATABASE_URL:?Set DATABASE_URL to a dedicated Neon LiteLLM database}"

unset LITELLM_MASTER_KEY_SEED LITELLM_SALT_KEY_SEED
exec /app/docker/prod_entrypoint.sh "$@"
