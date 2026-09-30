package middleware

import (
        "context"
        "encoding/json"
        "errors"
        "net/http"
        "net/http/httptest"
        "testing"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
        "github.com/danielgtaylor/huma/v2"
        "github.com/danielgtaylor/huma/v2/adapters/humago"
)

// fakePlatformChecker implements PlatformSuperAdminChecker for tests.
type fakePlatformChecker struct {
        isActive bool
        err      error
        called   bool
}

func (f *fakePlatformChecker) IsActiveSuperAdmin(_ context.Context, _ string) (bool, error) {
        f.called = true
        return f.isActive, f.err
}

// runHumaMiddleware invokes the given Huma middleware against a fake
// request, returning the HTTP status code + whether next() was called.
func runHumaMiddleware(t *testing.T, mw func(huma.Context, func(huma.Context)), method, path string, principalID string) (int, bool) {
        t.Helper()
        req := httptest.NewRequest(method, path, nil)
        if principalID != "" {
                // Simulate that RequireAccessTokenHuma has already run and stashed
                // the principal in the request context.
                ctx := WithPrincipal(req.Context(), commands.PrincipalID(principalID))
                req = req.WithContext(ctx)
        }
        res := httptest.NewRecorder()
        op := &huma.Operation{}
        humaCtx := humago.NewContext(op, req, res)
        nextCalled := false
        mw(humaCtx, func(_ huma.Context) { nextCalled = true })
        return res.Code, nextCalled
}

// Test P0-1: RequirePlatformAdminHuma MUST short-circuit on merchant paths.
// A merchant principal (not a platform super admin) must be able to call
// /api/v1/businesses/* and /api/v1/me WITHOUT being blocked by the platform
// admin middleware. Before the fix, the middleware applied globally and
// returned 403 for any non-platform-super-admin principal, even on merchant
// routes.
func TestRequirePlatformAdminHumaBypassesMerchantPaths(t *testing.T) {
        t.Parallel()
        checker := &fakePlatformChecker{isActive: false, err: nil}

        cases := []struct {
                name string
                path string
        }{
                {"businesses collection", "/api/v1/businesses/abc-123"},
                {"me endpoint", "/api/v1/me"},
                {"me businesses", "/api/v1/me/businesses"},
                {"business conversations", "/api/v1/businesses/abc-123/conversations"},
                {"business customers", "/api/v1/businesses/abc-123/customers/def-456"},
                {"business transactions", "/api/v1/businesses/abc-123/transactions"},
                {"webhook path (public)", "/api/v1/webhooks/socialapi/route-key"},
                {"health (public)", "/api/v1/health/live"},
        }

        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        checker.called = false
                        status, nextCalled := runHumaMiddleware(t, RequirePlatformAdminHuma(checker), http.MethodGet, tc.path, "merchant-principal-1")
                        if status != http.StatusOK {
                                t.Errorf("merchant path %s: expected status=200 (next invoked, no write), got %d", tc.path, status)
                        }
                        if !nextCalled {
                                t.Errorf("merchant path %s: next() must be called (bypass)", tc.path)
                        }
                        if checker.called {
                                t.Errorf("merchant path %s: IsActiveSuperAdmin must NOT be called (checker short-circuited)", tc.path)
                        }
                })
        }
}

// Test P0-1: RequirePlatformAdminHuma MUST enforce on platform paths.
// A merchant principal (not a platform super admin) calling /api/v1/platform/*
// must receive 403.
func TestRequirePlatformAdminHumaEnforcesOnPlatformPaths(t *testing.T) {
        t.Parallel()
        checker := &fakePlatformChecker{isActive: false, err: nil}

        cases := []struct {
                name string
                path string
        }{
                {"platform root", "/api/v1/platform/businesses"},
                {"platform business detail", "/api/v1/platform/businesses/abc-123"},
                {"platform plans", "/api/v1/platform/plans"},
                {"platform subscriptions", "/api/v1/platform/subscriptions"},
                {"platform ai", "/api/v1/platform/ai"},
                {"platform operations ai", "/api/v1/platform/operations/ai/providers"},
                {"platform audit events", "/api/v1/platform/audit-events"},
        }

        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        checker.called = false
                        status, nextCalled := runHumaMiddleware(t, RequirePlatformAdminHuma(checker), http.MethodGet, tc.path, "merchant-principal-1")
                        if status != http.StatusForbidden {
                                t.Errorf("platform path %s: expected 403 for non-super-admin, got %d", tc.path, status)
                        }
                        if nextCalled {
                                t.Errorf("platform path %s: next() must NOT be called when blocked", tc.path)
                        }
                        if !checker.called {
                                t.Errorf("platform path %s: IsActiveSuperAdmin must be called", tc.path)
                        }
                })
        }
}

// Test P0-1: a platform super admin calling a platform path must be allowed.
func TestRequirePlatformAdminHumaAllowsPlatformAdminOnPlatformPaths(t *testing.T) {
        t.Parallel()
        checker := &fakePlatformChecker{isActive: true, err: nil}
        status, nextCalled := runHumaMiddleware(t, RequirePlatformAdminHuma(checker), http.MethodGet, "/api/v1/platform/businesses", "platform-admin-1")
        if status != http.StatusOK {
                t.Errorf("expected 200 (next invoked), got %d", status)
        }
        if !nextCalled {
                t.Errorf("next() must be called for platform admin on platform path")
        }
}

// Test P0-1: when the platform checker errors out, the middleware must
// fail closed (403) — never allow access when the checker is unavailable.
func TestRequirePlatformAdminHumaFailsClosedOnCheckerError(t *testing.T) {
        t.Parallel()
        checker := &fakePlatformChecker{isActive: false, err: errors.New("db unavailable")}
        status, nextCalled := runHumaMiddleware(t, RequirePlatformAdminHuma(checker), http.MethodGet, "/api/v1/platform/businesses", "merchant-principal-1")
        if status != http.StatusForbidden {
                t.Errorf("expected 403 on checker error, got %d", status)
        }
        if nextCalled {
                t.Errorf("next() must NOT be called on checker error")
        }
}

// Test P0-1: when no principal is in context (i.e. RequireAccessTokenHuma
// didn't run first), platform middleware must reject — defense-in-depth.
func TestRequirePlatformAdminHumaRejectsMissingPrincipal(t *testing.T) {
        t.Parallel()
        checker := &fakePlatformChecker{isActive: true, err: nil}
        // No principal ID set in context
        req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/businesses", nil)
        res := httptest.NewRecorder()
        op := &huma.Operation{}
        humaCtx := humago.NewContext(op, req, res)
        nextCalled := false
        RequirePlatformAdminHuma(checker)(humaCtx, func(_ huma.Context) { nextCalled = true })
        if res.Code != http.StatusForbidden {
                t.Errorf("expected 403 when principal is missing, got %d", res.Code)
        }
        if nextCalled {
                t.Errorf("next() must NOT be called when principal is missing")
        }
}

// Test P0-1: when the checker is nil (platform admin guard not wired),
// platform middleware must reject — fail closed.
func TestRequirePlatformAdminHumaRejectsNilChecker(t *testing.T) {
        t.Parallel()
        status, nextCalled := runHumaMiddleware(t, RequirePlatformAdminHuma(nil), http.MethodGet, "/api/v1/platform/businesses", "merchant-principal-1")
        if status != http.StatusForbidden {
                t.Errorf("expected 403 when checker is nil, got %d", status)
        }
        if nextCalled {
                t.Errorf("next() must NOT be called when checker is nil")
        }
}

// Test P0-1: merchant paths must still bypass even when checker is nil
// (so the bootstrap flow doesn't break — when platform admin guard isn't
// wired, merchant APIs still work).
func TestRequirePlatformAdminHumaBypassesMerchantPathEvenWithNilChecker(t *testing.T) {
        t.Parallel()
        status, nextCalled := runHumaMiddleware(t, RequirePlatformAdminHuma(nil), http.MethodGet, "/api/v1/businesses/abc-123", "merchant-principal-1")
        if status != http.StatusOK {
                t.Errorf("expected 200 (bypass) on merchant path even with nil checker, got %d", status)
        }
        if !nextCalled {
                t.Errorf("next() must be called on merchant path")
        }
}

// Test P0-1: helper to confirm the error response shape on 403 — must
// match the contract's ErrorEnvelope so the frontend can decode it.
func TestRequirePlatformAdminHumaReturnsContractErrorEnvelope(t *testing.T) {
        t.Parallel()
        checker := &fakePlatformChecker{isActive: false, err: nil}
        req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/businesses", nil)
        req = req.WithContext(WithPrincipal(req.Context(), commands.PrincipalID("p-1")))
        res := httptest.NewRecorder()
        op := &huma.Operation{}
        humaCtx := humago.NewContext(op, req, res)
        RequirePlatformAdminHuma(checker)(humaCtx, func(_ huma.Context) {})

        var envelope contract.ErrorEnvelope
        if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
                t.Fatalf("decode envelope: %v", err)
        }
        if envelope.Error.Code != "forbidden" {
                t.Errorf("expected error.code=forbidden, got %s", envelope.Error.Code)
        }
        if envelope.Error.Message == "" {
                t.Errorf("expected non-empty error.message")
        }
}
