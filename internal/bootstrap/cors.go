package bootstrap

import (
	"net/http"
	"net/url"
	"strings"
)

const (
	allowedCORSMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	allowedCORSHeaders = "Accept, Content-Type, Authorization, X-Request-ID, X-Correlation-ID, Idempotency-Key, If-Match"
)

func frontendOrigin(frontendURL string) string {
	if strings.TrimSpace(frontendURL) == "" {
		return ""
	}
	parsed, err := url.Parse(frontendURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func newCORSMiddleware(next http.Handler, frontendURL string) http.Handler {
	origin := frontendOrigin(frontendURL)

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestOrigin := strings.TrimSpace(request.Header.Get("Origin"))
		if requestOrigin != "" && origin != "" && requestOrigin == origin {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
			writer.Header().Set("Access-Control-Allow-Methods", allowedCORSMethods)
			writer.Header().Set("Access-Control-Allow-Headers", allowedCORSHeaders)
			writer.Header().Add("Vary", "Origin")
		}

		if request.Method == http.MethodOptions {
			if requestOrigin == "" || requestOrigin != origin {
				writer.WriteHeader(http.StatusForbidden)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(writer, request)
	})
}
