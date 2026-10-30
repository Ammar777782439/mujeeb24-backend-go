package r2

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPrivateR2PutUsesSignedRequest(t *testing.T) {
	var called bool
	c := &Client{AccountID: "abc123", Bucket: "catalog", AccessKeyID: "key1",
		SecretAccessKey: "secret", Now: func() time.Time { return time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) },
		HTTP: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			if req.Method != "PUT" || req.URL.Host != "abc123.r2.cloudflarestorage.com" {
				t.Fatalf("unexpected request %s %s", req.Method, req.URL)
			}
			if req.Header.Get("Content-Type") != "image/png" {
				t.Fatal("incorrect stored object media type")
			}
			if req.URL.EscapedPath() != "/catalog/catalog/business1/asset1" {
				t.Fatal(req.URL.String())
			}
			if !strings.Contains(req.Header.Get("Authorization"), "Credential=key1/20261030/auto/s3/aws4_request") ||
				req.Header.Get("X-Amz-Content-Sha256") == "" {
				t.Fatal("missing SigV4 signed headers")
			}
			body, _ := io.ReadAll(req.Body)
			if string(body) != "image-data" {
				t.Fatalf("body %q", body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})},
	}
	if err := c.Put(context.Background(), "catalog/business1/asset1", "image/png", []byte("image-data")); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("R2 PUT not called")
	}
}
func TestR2SignedReadURLIsScoped(t *testing.T) {
	c := &Client{AccountID: "abc123", Bucket: "catalog", AccessKeyID: "key1",
		SecretAccessKey: "secret", Now: func() time.Time { return time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) }}
	raw, err := c.PresignGet(context.Background(), "catalog/biz/image1", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "abc123.r2.cloudflarestorage.com" ||
		u.Query().Get("X-Amz-Expires") != "900" || u.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("malformed presigned URL %s", u)
	}
	if _, err = c.PresignGet(context.Background(), "catalog/../secret", time.Hour); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err = c.PresignGet(context.Background(), "catalog/biz/image1", 2*time.Hour); err == nil {
		t.Fatal("unbounded TTL accepted")
	}
}
func TestR2RejectsMissingCredentials(t *testing.T) {
	c := &Client{AccountID: "abc", Bucket: "catalog"}
	if err := c.Put(context.Background(), "catalog/biz/asset", "image/png", []byte("data")); err == nil {
		t.Fatal("missing credentials")
	}
}

func TestBackblazeB2SignsItsRegionAndEndpoint(t *testing.T) {
	c := &Client{
		Endpoint: "https://s3.us-west-004.backblazeb2.com", Region: "us-west-004",
		Bucket: "mujeeb24", AccessKeyID: "app-key-id", SecretAccessKey: "app-secret",
		Now: func() time.Time { return time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC) },
		HTTP: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != "s3.us-west-004.backblazeb2.com" ||
				req.URL.EscapedPath() != "/mujeeb24/catalog/business-id/asset.png" {
				t.Fatalf("B2 request wrong endpoint: %s", req.URL)
			}
			if !strings.Contains(req.Header.Get("Authorization"), "/20261030/us-west-004/s3/aws4_request") {
				t.Fatal("B2 signing scope must match the actual region")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})},
	}
	if err := c.Put(context.Background(), "catalog/business-id/asset.png", "image/png", []byte("sample")); err != nil {
		t.Fatal(err)
	}
	presigned, err := c.PresignGet(context.Background(), "catalog/business-id/asset.png", 45*time.Minute)
	if err != nil { t.Fatal(err) }
	u, err := url.Parse(presigned)
	if err != nil { t.Fatal(err) }
	if u.Host != "s3.us-west-004.backblazeb2.com" ||
		!strings.Contains(u.Query().Get("X-Amz-Credential"), "/us-west-004/s3/aws4_request") ||
		u.Query().Get("X-Amz-Expires") != "2700" || u.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("invalid B2 signed URL endpoint/credential: %s", u.Host)
	}
}
func TestBackblazeRejectsUnsafeOrMismatchedEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://s3.us-west-004.backblazeb2.com",
		"https://127.0.0.1",
		"https://s3.us-east-005.backblazeb2.com",
		"https://s3.us-west-004.backblazeb2.com.attacker.test",
		"https://s3.us-west-004.backblazeb2.com/extra",
		"https://s3.us-west-004.backblazeb2.com?x=1",
	} {
		c := &Client{Endpoint: endpoint, Region: "us-west-004", Bucket: "catalog", AccessKeyID: "key", SecretAccessKey: "secret"}
		if _, err := c.PresignGet(context.Background(), "catalog/a/b", time.Minute); err == nil {
			t.Fatalf("accepted unsafe endpoint %s", endpoint)
		}
	}
}
