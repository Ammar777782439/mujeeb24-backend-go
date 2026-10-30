// Package merchantcatalogtrace prints opt-in full-fidelity merchant catalog
// debugging data in Render Logs. Do not enable this flag on production.
package merchantcatalogtrace

import (
	"context"
	"log"
	"os"
	"strings"
)

const EnvironmentVariable = "MUJEEB_MERCHANT_CATALOG_DEBUG"
const logChunkRunes = 2048

type sessionKey struct{}

// WithSession marks only merchant catalog authoring requests and their
// downstream provider calls, keeping other AI workflows out of these logs.
func WithSession(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionKey{}, strings.TrimSpace(sessionID))
}

func session(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(sessionKey{}).(string)
	return value
}

// Enabled requires explicit debug opt-in and a marked catalog session.
func Enabled(ctx context.Context) bool {
	return session(ctx) != "" && strings.EqualFold(strings.TrimSpace(os.Getenv(EnvironmentVariable)), "true")
}

// Event records diagnostic metadata or a parser/validation error.
func Event(ctx context.Context, stage, details string) {
	if !Enabled(ctx) {
		return
	}
	log.Printf("[MerchantCatalogTrace] session=%q stage=%s %s", session(ctx), stage, details)
}

// Dump prints the complete, unredacted payload in numbered Unicode-safe
// chunks so a large JSON message can be reassembled without log truncation.
// HTTP Authorization headers are not part of the JSON request body.
func Dump(ctx context.Context, stage string, payload []byte) {
	if !Enabled(ctx) {
		return
	}
	runes := []rune(string(payload))
	count := (len(runes) + logChunkRunes - 1) / logChunkRunes
	if count == 0 {
		Event(ctx, stage, "part=0/0 bytes=0 payload=")
		return
	}
	for part := 0; part < count; part++ {
		start := part * logChunkRunes
		end := start + logChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		log.Printf("[MerchantCatalogTrace] session=%q stage=%s part=%d/%d bytes=%d payload=%s",
			session(ctx), stage, part+1, count, len(payload), string(runes[start:end]))
	}
}
