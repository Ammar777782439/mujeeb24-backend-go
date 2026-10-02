#!/usr/bin/env bash
set -euo pipefail

OUT=".dockhosting.generated.env"

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required to generate Ed25519 secrets."
  exit 1
fi

umask 077
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

cat > "$tmpdir/keygen.go" <<'GO'
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main() {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	aiKey := make([]byte, 32)
	if _, err := rand.Read(aiKey); err != nil {
		panic(err)
	}

	fmt.Printf("JWT_ED25519_PRIVATE_KEY=%s\n", base64.StdEncoding.EncodeToString(privateKey))
	fmt.Printf("JWT_ED25519_PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(publicKey))
	fmt.Printf("AI_CONFIG_ENCRYPTION_KEY=%s\n", base64.StdEncoding.EncodeToString(aiKey))
}
GO

{
  echo "APP_ENV=production"
  echo "AUTH_ENABLED=true"
  echo "JWT_ISSUER=mujeeb24"
  go run "$tmpdir/keygen.go"
} > "$OUT"

chmod 600 "$OUT"
echo "Generated $OUT"
echo "Do NOT commit or share this file."
