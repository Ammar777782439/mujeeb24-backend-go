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
