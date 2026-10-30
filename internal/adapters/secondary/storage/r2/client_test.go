package r2

import (
	"context"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(presigned)
	if err != nil {
		t.Fatal(err)
	}
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

func TestBackblazeReturnsSafeErrorCodeAndRequestID(t *testing.T) {
	c := &Client{
		Endpoint:        "https://s3.us-east-005.backblazeb2.com",
		Region:          "us-east-005",
		Bucket:          "mujeeb24-images",
		AccessKeyID:     "test-access-id",
		SecretAccessKey: "test-secret",
		HTTP: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 400,
				Header:     http.Header{"X-Amz-Request-Id": []string{"request-abc"}},
				Body: io.NopCloser(strings.NewReader(
					`<Error><Code>InvalidRequest</Code><Message>Unsupported header x-amz-checksum-sha256; sensitive-example-content</Message></Error>`)),
			}, nil
		})},
	}
	err := c.Put(context.Background(), "catalog/test/image.jpg", "image/jpeg", []byte("image"))
	if err == nil {
		t.Fatal("HTTP 400 must not succeed")
	}
	msg := err.Error()
	for _, expected := range []string{"HTTP 400", "code=InvalidRequest", "reason=unsupported_checksum_header", "request_id=request-abc"} {
		if !strings.Contains(msg, expected) {
			t.Fatalf("missing diagnostic %q in %q", expected, msg)
		}
	}
	for _, forbidden := range []string{"test-secret", "sensitive-example-content", "test-access-id"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("provider response leaked sensitive text in %q", msg)
		}
	}
}

func TestBackblazeRejectsUnsafeDiagnosticTokens(t *testing.T) {
	res := &http.Response{
		StatusCode: 400,
		Header:     http.Header{"X-Amz-Request-Id": []string{"not safe\ncontent"}},
		Body: io.NopCloser(strings.NewReader(
			`<Error><Code>InvalidRequest extra</Code><Message>private details</Message></Error>`)),
	}
	err := classifyProviderHTTPError("PUT", res)
	if err == nil || !strings.Contains(err.Error(), "code=unavailable") ||
		!strings.Contains(err.Error(), "request_id=unavailable") ||
		strings.Contains(err.Error(), "private details") {
		t.Fatalf("unsafe diagnostic: %v", err)
	}
}

func TestBackblazeHTTPErrorWithoutXMLStillHasStatus(t *testing.T) {
	res := &http.Response{
		StatusCode: 400,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("not XML")),
	}
	got := classifyProviderHTTPError("PUT", res)
	if got == nil || !strings.Contains(got.Error(), "HTTP 400") ||
		!strings.Contains(got.Error(), "code=unavailable") {
		t.Fatalf("missing HTTP diagnostic: %v", got)
	}
}

func TestBackblazeFullHTTPBodyIsSentWithoutChunking(t *testing.T) {
	data := bytes.Repeat([]byte{0x73, 0x17, 0x44, 0x29}, 117000)
	var received int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("wrong method: %s", r.Method)
		}
		if len(r.TransferEncoding) != 0 || r.ContentLength != int64(len(data)) {
			t.Errorf("incorrect HTTP framing: length=%d encoding=%v", r.ContentLength, r.TransferEncoding)
		}
		all, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read actual HTTP body: %v", err)
		}
		received = int64(len(all))
		if !bytes.Equal(all, data) {
			t.Error("actual HTTP body bytes differ from source")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &Client{
		Endpoint: "https://s3.us-east-005.backblazeb2.com",
		Region: "us-east-005", Bucket: "mujeeb24-images",
		AccessKeyID: "example-key-id", SecretAccessKey: "example-secret",
		HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			// Test-only transport rewrite to an in-process HTTP server. The
			// app still constructs a real SigV4-signed B2-style PUT request.
			copyRequest := r.Clone(r.Context())
			copyRequest.URL.Scheme = "http"
			copyRequest.URL.Host = strings.TrimPrefix(srv.URL, "http://")
			copyRequest.Host = copyRequest.URL.Host
			return http.DefaultTransport.RoundTrip(copyRequest)
		})},
	}
	if err := c.Put(context.Background(), "catalog/test/large.jpeg", "image/jpeg", data); err != nil {
		t.Fatalf("real TCP upload should transmit every byte: %v", err)
	}
	if received != int64(len(data)) {
		t.Fatalf("server received %d bytes, wanted %d", received, len(data))
	}
}

func TestBackblazeIncompleteBodyHasTransferEvidence(t *testing.T) {
	c := &Client{
		Endpoint: "https://s3.us-east-005.backblazeb2.com",
		Region: "us-east-005", Bucket: "mujeeb24-images",
		AccessKeyID: "example-key-id", SecretAccessKey: "example-secret",
		HTTP: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			buf := make([]byte, 4)
			_, _ = req.Body.Read(buf)
			return &http.Response{StatusCode: 400, Proto: "HTTP/1.1",
				Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
				"<Error><Code>IncompleteBody</Code></Error>"))}, nil
		})},
	}
	err := c.Put(context.Background(), "catalog/test/truncated.jpg", "image/jpeg", []byte("full-body"))
	if err == nil {
		t.Fatal("partial stream must fail")
	}
	for _, want := range []string{"HTTP 400", "code=IncompleteBody", "reason=remote_incomplete_body", "body_read=4", "body_expected=9"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing failure evidence %q in %q", want, err.Error())
		}
	}
}

func TestBackblazeAccessDeniedNotAssumedToBeBadSignature(t *testing.T) {
	res := &http.Response{StatusCode: 403, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader("<Error><Code>AccessDenied</Code></Error>"))}
	err := classifyProviderHTTPError("PUT", res)
	if err == nil || !strings.Contains(err.Error(), "reason=provider_access_denied") {
		t.Fatalf("403 must not be mislabelled a signature failure: %v", err)
	}
}
