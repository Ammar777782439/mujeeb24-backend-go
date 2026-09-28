package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

type principalContextKey struct{}

// AccessTokenVerifier is intentionally smaller than JWT storage. It verifies
// the access token and returns only the authenticated principal identity;
// business membership and permissions remain a separate ScopeProvider concern.
type AccessTokenVerifier interface {
	VerifyAccessToken(context.Context, string) (commands.PrincipalID, error)
}

func WithPrincipal(ctx context.Context, principalID commands.PrincipalID) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principalID)
}

func PrincipalID(ctx context.Context) (commands.PrincipalID, bool) {
	principalID, ok := ctx.Value(principalContextKey{}).(commands.PrincipalID)
	return principalID, ok && principalID != ""
}

func RequireAccessToken(verifier AccessTokenVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if verifier == nil {
			writeUnauthorized(w, "authentication is not configured")
			return
		}
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
			writeUnauthorized(w, "missing bearer access token")
			return
		}
		token := strings.TrimSpace(header[len("Bearer "):])
		if token == "" {
			writeUnauthorized(w, "missing bearer access token")
			return
		}
		principalID, err := verifier.VerifyAccessToken(r.Context(), token)
		if err != nil || principalID == "" {
			writeUnauthorized(w, "invalid access token")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principalID)))
	})
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(contract.ErrorEnvelope{Error: contract.ErrorBody{Code: "unauthorized", Message: message}, RequestID: ""})
}
