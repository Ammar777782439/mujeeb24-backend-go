package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestB2CredentialCheckValidatesKeyPairAndBucketWithoutLoggingSecrets(t *testing.T) {
	keyID, secret := "synthetic-key-id", "synthetic-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/b2api/v4/b2_authorize_account" {
			t.Errorf("unexpected B2 auth request: %s %s", req.Method, req.URL.Path)
		}
		id, pass, ok := req.BasicAuth()
		if !ok || id != keyID || pass != secret {
			t.Error("B2 native auth must use the same key pair as the S3 client")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorizationToken":"DO-NOT-LOG", "apiInfo":{"storageApi":{
			"s3ApiUrl":"https://s3.us-east-005.backblazeb2.com",
			"allowed":{"buckets":[{"name":"mujeeb24-images"}],
			"capabilities":["readFiles","writeFiles","deleteFiles"]}}}}`))
	}))
	defer server.Close()
	result := checkB2Credentials(context.Background(), server.Client(),
		server.URL+"/b2api/v4/b2_authorize_account", keyID, secret,
		"mujeeb24-images", "https://s3.us-east-005.backblazeb2.com")
	if result.Result != "key_pair_valid" || !result.EndpointMatch ||
		result.BucketScope != "matches" || !result.CanWrite {
		t.Fatalf("expected safe positive credentials result: %+v", result)
	}
	if strings.Contains(result.Result, "DO-NOT-LOG") ||
		strings.Contains(result.Result, secret) {
		t.Fatal("auth token or secret leaked into result")
	}
}

func TestB2CredentialCheckSeparatesInvalidCredentialsFromS3PUT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized", "message":"sensitive-secret"}`))
	}))
	defer server.Close()
	result := checkB2Credentials(context.Background(), server.Client(), server.URL,
		"wrong-key-id", "wrong-secret", "mujeeb24-images",
		"https://s3.us-east-005.backblazeb2.com")
	if result.Result != "invalid_key_pair" || result.CanWrite ||
		strings.Contains(result.Result, "wrong-") {
		t.Fatalf("unexpected result for invalid credentials: %+v", result)
	}
}

func TestB2CredentialCheckDetectsEndpointAndAccessScopeMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(`{"apiInfo":{"storageApi":{
			"s3ApiUrl":"https://s3.us-west-004.backblazeb2.com",
			"allowed":{"buckets":[{"name":"other-bucket"}],
			"capabilities":["readFiles"]}}}}`))
	}))
	defer server.Close()
	result := checkB2Credentials(context.Background(), server.Client(),
		server.URL, "test-id", "test-secret", "mujeeb24-images",
		"https://s3.us-east-005.backblazeb2.com")
	if result.Result != "key_pair_valid" || result.EndpointMatch || result.CanWrite ||
		result.BucketScope != "other_or_unresolved" {
		t.Fatalf("invalid storage constraints were missed: %+v", result)
	}
}

func TestB2CredentialCheckReturnsTransportErrorWithoutCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Timeout: 10 * time.Millisecond}
	result := checkB2Credentials(ctx, client, "https://api.backblazeb2.com/b2api/v4/b2_authorize_account",
		"synthetic-key-id", "synthetic-secret", "mujeeb24-images",
		"https://s3.us-east-005.backblazeb2.com")
	if result.Result != "transport_error" || result.EndpointMatch {
		t.Fatalf("cancelled diagnostic should never authorize: %+v", result)
	}
}

func TestB2CredentialCheckDistinguishes401UnsupportedFromInvalidPair(t *testing.T) {
	for _, tc := range []struct {
		name, response, expected string
	}{
		{"unsupported", `{"status":401,"code":"unsupported","message":"private error"}`, "api_version_unsupported"},
		{"unauthorized", `{"status":401,"code":"unauthorized","message":"private error"}`, "invalid_key_pair"},
		{"unknown", `{"status":401,"code":"unexpected","message":"private error"}`, "auth_401_unclassified"},
		{"invalid_json", `garbage`, "auth_401_unclassified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			result := checkB2Credentials(context.Background(), server.Client(), server.URL,
				"synthetic-id", "synthetic-secret", "mujeeb24-images",
				"https://s3.us-east-005.backblazeb2.com")
			if result.Result != tc.expected {
				t.Fatalf("unexpected 401 diagnosis: result=%q expected=%q", result.Result, tc.expected)
			}
			if strings.Contains(result.Result, "private") ||
				strings.Contains(result.Result, "synthetic-secret") {
				t.Fatal("error response leaked secrets")
			}
		})
	}
}

func TestB2CredentialDiagnosticsNoKeySecretInUnexpected401(t *testing.T) {
	keyID, secret := "synthetic-id-0002", "secret-with-opaque-data"
	for _, tc := range []struct {
		name string
		response string
		kind string
		providerCode string
	}{
		{"json_unknown_code", `{"code":"unexpected_code","message":"secret-with-opaque-data"}`, "json", "unexpected_code"},
		{"html_response", `<html><body>secret-with-opaque-data</body></html>`, "html_or_xml", "unavailable"},
		{"empty", "", "empty", "unavailable"},
		{"malformed", `{"code":"inject dangerous\\ntext"}`, "json", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			got := checkB2Credentials(context.Background(), server.Client(), server.URL, keyID, secret,
				"mujeeb24-images", "https://s3.us-east-005.backblazeb2.com")
			if got.Result != "auth_401_unclassified" || got.ResponseKind != tc.kind ||
				got.ProviderCode != tc.providerCode || got.KeyIDSuffix != "0002" ||
				got.KeyIDWhitespace || got.SecretWhitespace {
				t.Fatalf("unexpected sanitized diagnostics: %+v", got)
			}
			if strings.Contains(got.ProviderCode, secret) || strings.Contains(got.KeyIDSuffix, secret) {
				t.Fatal("secret leaked")
			}
		})
	}
}
func TestB2CredentialDiagnosticsDetectsWhitespaceWithoutLoggingIt(t *testing.T) {
	got := checkB2Credentials(context.Background(), &http.Client{Timeout: time.Millisecond},
		":bad-url", "synthetic-0001 ", " synthetic-secret",
		"mujeeb24-images", "https://s3.us-east-005.backblazeb2.com")
	if got.KeyIDSuffix != "001 " || !got.KeyIDWhitespace || !got.SecretWhitespace {
		t.Fatalf("credential whitespace diagnostics wrong: %+v", got)
	}
}
