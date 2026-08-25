package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/danielgtaylor/huma/v2"
)

func RequireAccessTokenHuma(verifier AccessTokenVerifier) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if isPublicAPIPath(ctx.URL().Path) {
			next(ctx)
			return
		}
		if verifier == nil {
			writeHumaUnauthorized(ctx, "authentication is not configured")
			return
		}
		header := strings.TrimSpace(ctx.Header("Authorization"))
		if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
			writeHumaUnauthorized(ctx, "missing bearer access token")
			return
		}
		principalID, err := verifier.VerifyAccessToken(ctx.Context(), strings.TrimSpace(header[len("Bearer "):]))
		if err != nil || principalID == "" {
			writeHumaUnauthorized(ctx, "invalid access token")
			return
		}
		next(huma.WithContext(ctx, WithPrincipal(ctx.Context(), principalID)))
	}
}

func isPublicAPIPath(path string) bool {
	return path == "/api/v1/auth/login" || path == "/api/v1/auth/refresh" || path == "/api/v1/health/live" || path == "/api/v1/health/ready" || path == "/api/v1/metrics" || strings.HasPrefix(path, "/api/v1/webhooks/")
}

func writeHumaUnauthorized(ctx huma.Context, message string) {
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(http.StatusUnauthorized)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(contract.ErrorEnvelope{Error: contract.ErrorBody{Code: "unauthorized", Message: message}})
}
