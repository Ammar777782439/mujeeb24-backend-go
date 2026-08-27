package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func main() {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	fmt.Printf("JWT_ED25519_PRIVATE_KEY=%s\nJWT_ED25519_PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(pub))
}
