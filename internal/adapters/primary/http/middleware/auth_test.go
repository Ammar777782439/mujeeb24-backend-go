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
)

type fakeVerifier struct {
	principal commands.PrincipalID
	err       error
	token     string
}

func (f *fakeVerifier) VerifyAccessToken(_ context.Context, token string) (commands.PrincipalID, error) {
	f.token = token
	return f.principal, f.err
}

func TestRequireAccessTokenRejectsMissingTokenWithEnvelope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	res := httptest.NewRecorder()
	called := false
	RequireAccessToken(&fakeVerifier{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || called {
		t.Fatalf("expected 401 without invoking next, got status=%d called=%v", res.Code, called)
	}
	var envelope contract.ErrorEnvelope
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.Error.Code != "unauthorized" {
		t.Fatalf("unexpected error code: %#v", envelope.Error)
	}
}

func TestRequireAccessTokenRejectsVerifierFailure(t *testing.T) {
	verifier := &fakeVerifier{err: errors.New("bad signature")}
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer bad")
	res := httptest.NewRecorder()
	RequireAccessToken(verifier, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("next must not run") })).ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized || verifier.token != "bad" {
		t.Fatalf("unexpected verifier failure result: status=%d token=%q", res.Code, verifier.token)
	}
}

func TestRequireAccessTokenStoresPrincipalOnly(t *testing.T) {
	verifier := &fakeVerifier{principal: commands.PrincipalID("principal-1")}
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "bearer access-token")
	res := httptest.NewRecorder()
	called := false
	RequireAccessToken(verifier, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		principal, ok := PrincipalID(r.Context())
		if !ok || principal != "principal-1" {
			t.Fatalf("principal not stored: %q %v", principal, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || !called {
		t.Fatalf("expected next handler, got status=%d called=%v", res.Code, called)
	}
}
