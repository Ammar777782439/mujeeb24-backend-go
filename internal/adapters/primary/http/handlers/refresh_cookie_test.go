package handlers

import (
	"net/http"
	"testing"
	"time"
)

func TestRefreshCookieTransportModes(t *testing.T) {
	for _, env := range []string{"development", "test", "production", "staging"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)
			cookie := refreshCookie("test-token", time.Now().Add(time.Hour))
			if !cookie.HttpOnly || cookie.Path != "/api/v1/auth" {
				t.Fatal("refresh cookie scope changed")
			}
			if env == "development" || env == "test" {
				if cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
					t.Fatal("local HTTP cookie would be rejected")
				}
			} else {
				if !cookie.Secure || cookie.SameSite != http.SameSiteNoneMode {
					t.Fatal("production cross-site cookie must be secure")
				}
			}
		})
	}
}
