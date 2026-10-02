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

// RequirePlatformAdminHuma enforces Platform Scope on /api/v1/platform/*
// routes. It must run AFTER RequireAccessTokenHuma so that the principal is
// already authenticated.
//
// Per Platform Administration Contract §4, §55, §63-64:
//   - Platform super admin is a SEPARATE identity from merchant owner/admin.
//   - Platform token ≠ merchant token; platform super admin is NEVER a member
//     of any business.
//   - A merchant owner token used against /api/v1/platform/* returns 403.
//   - A platform admin token used against /api/v1/businesses/{id}/... merchant
//     routes is rejected at the merchant requireScope boundary (which checks
//     business_memberships).
//
// On success, the context is tagged via WithPlatformAdmin so downstream
// handler facades can retrieve the actor via middleware.PlatformAdminID().
//
// P0 FIX: this middleware is registered globally via api.UseMiddleware(...),
// so it MUST short-circuit on non-platform paths. Without this guard, every
// merchant route (e.g. /api/v1/businesses/*, /api/v1/me) would also be
// subject to the platform-super-admin check and return 403 for merchant
// principals. The path check is intentionally prefix-based so any future
// /api/v1/platform/<new-sub-path> is automatically covered.
func RequirePlatformAdminHuma(checker PlatformSuperAdminChecker) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		// P0-1: only enforce platform scope on /api/v1/platform/* paths.
		// Merchant routes (/api/v1/businesses/*, /api/v1/me, etc.) bypass
		// this middleware entirely — they are protected by the merchant
		// requireScope boundary in the dispatcher.
		if !isPlatformAPIPath(ctx.URL().Path) {
			next(ctx)
			return
		}
		if checker == nil {
			writeHumaForbidden(ctx, "platform authorization is not configured")
			return
		}
		principalID, ok := PrincipalID(ctx.Context())
		if !ok {
			writeHumaForbidden(ctx, "missing authenticated principal")
			return
		}
		isActive, err := checker.IsActiveSuperAdmin(ctx.Context(), string(principalID))
		if err != nil || !isActive {
			writeHumaForbidden(ctx, "platform super admin role required")
			return
		}
		next(huma.WithContext(ctx, WithPlatformAdmin(ctx.Context(), principalID)))
	}
}

// isPlatformAPIPath returns true for paths that require platform super admin
// scope. Per Contract §56: every platform API lives under /api/v1/platform/*.
// The OpenAPI prefix is /api/v1 (see contract.BuildAPIWithHandlersAndMiddleware
// line 39), so the runtime URL always carries the full /api/v1/platform/ prefix.
func isPlatformAPIPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/platform/")
}

func isPublicAPIPath(path string) bool {
	// SECURITY audit M-5: `/api/v1/metrics` was previously public. It exposes
	// Postgres pool utilization (acquired/idle/total connections) — useful to
	// an attacker planning a DoS or performing infrastructure reconnaissance.
	// Remove it from the public allowlist. Operators who want public metrics
	// should expose them via a separate /metrics route behind a reverse-proxy
	// with IP allowlisting or basic auth — NOT via this application layer.
	return path == "/api/v1/auth/login" || path == "/api/v1/auth/refresh" || path == "/api/v1/health/live" || path == "/api/v1/health/ready" || strings.HasPrefix(path, "/api/v1/webhooks/")
}

func writeHumaUnauthorized(ctx huma.Context, message string) {
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(http.StatusUnauthorized)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(contract.ErrorEnvelope{Error: contract.ErrorBody{Code: "unauthorized", Message: message}})
}

func writeHumaForbidden(ctx huma.Context, message string) {
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(http.StatusForbidden)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(contract.ErrorEnvelope{Error: contract.ErrorBody{Code: "forbidden", Message: message}})
}
