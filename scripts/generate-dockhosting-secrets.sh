#!/usr/bin/env bash
set -euo pipefail

OUT=".dockhosting.generated.env"

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required"
  exit 1
fi

umask 077

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

openssl genpkey -algorithm Ed25519 -out "$tmpdir/private.pem" >/dev/null 2>&1
openssl pkey -in "$tmpdir/private.pem" -pubout -out "$tmpdir/public.pem" >/dev/null 2>&1

private_b64="$(
  openssl pkey -in "$tmpdir/private.pem" -text -noout 2>/dev/null |
    awk '/priv:/{flag=1;next} /pub:/{flag=0} flag' |
    tr -d '[:space:]:' |
    xxd -r -p |
    base64 -w0
)"

public_b64="$(
  openssl pkey -in "$tmpdir/public.pem" -pubin -text -noout 2>/dev/null |
    awk '/pub:/{flag=1;next} flag' |
    tr -d '[:space:]:' |
    xxd -r -p |
    tail -c 32 |
    base64 -w0
)"

ai_key="$(openssl rand -base64 32 | tr -d '\n')"

cat > "$OUT" <<EOF
APP_ENV=production
AUTH_ENABLED=true
JWT_ISSUER=mujeeb24
JWT_ED25519_PRIVATE_KEY=$private_b64
JWT_ED25519_PUBLIC_KEY=$public_b64
AI_CONFIG_ENCRYPTION_KEY=$ai_key
EOF

chmod 600 "$OUT"
echo "Generated $OUT"
echo "Do NOT commit or share this file."
