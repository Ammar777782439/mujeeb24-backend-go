package r2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func b2TestClient(transport http.RoundTripper) *Client {
	return &Client{
		Endpoint: "https://s3.us-east-005.backblazeb2.com",
		Region:   "us-east-005", Bucket: "mujeeb24-images",
		AccessKeyID: "test-access-id", SecretAccessKey: "test-secret",
		HTTP: &http.Client{Transport: transport},
	}
}

func TestSDKBackblazePutObjectUsesPathStyleAndExactBytes(t *testing.T) {
	data := bytes.Repeat([]byte{0x73, 0x17, 0x44, 0x29}, 21000)
	var observed bool
	c := b2TestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		observed = true
		if req.Method != http.MethodPut ||
			req.URL.Host != "s3.us-east-005.backblazeb2.com" ||
			req.URL.EscapedPath() != "/mujeeb24-images/catalog/tenant/blue.jpg" {
			t.Errorf("incorrect B2 S3 PUT target: %s", req.URL)
		}
		if req.ContentLength != int64(len(data)) || len(req.TransferEncoding) != 0 {
			t.Errorf("wrong upload framing length=%d transfer=%v", req.ContentLength, req.TransferEncoding)
		}
		if req.Header.Get("Content-Type") != "image/jpeg" {
			t.Errorf("missing media type: %q", req.Header.Get("Content-Type"))
		}
		if !strings.Contains(req.Header.Get("Authorization"), "/us-east-005/s3/aws4_request") {
			t.Error("official SDK didn't sign using configured B2 region")
		}
		if req.Header.Get("X-Amz-Sdk-Checksum-Algorithm") != "" ||
			req.Header.Get("X-Amz-Trailer") != "" {
			t.Error("unexpected SDK checksum/streaming headers incompatible with B2")
		}
		payload, err := io.ReadAll(req.Body)
		if err != nil || !bytes.Equal(payload, data) {
			t.Errorf("SDK upload data mismatch len=%d err=%v", len(payload), err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Etag": []string{`"sample"`}},
			Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err := c.Put(context.Background(), "catalog/tenant/blue.jpg", "image/jpeg", data); err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatal("SDK never dispatched PUT")
	}
}

func TestSDKBackblazeFullHTTPBodyReachesLocalServer(t *testing.T) {
	data := bytes.Repeat([]byte{0x31, 0x42, 0x23}, 140000)
	var got int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut || req.ContentLength != int64(len(data)) || len(req.TransferEncoding) != 0 {
			t.Errorf("unexpected framing: %s len=%d", req.Method, req.ContentLength)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil || !bytes.Equal(body, data) {
			t.Errorf("server received incomplete upload: len=%d err=%v", len(body), err)
		}
		got++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := b2TestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		r := req.Clone(req.Context())
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(srv.URL, "http://")
		r.Host = r.URL.Host
		return http.DefaultTransport.RoundTrip(r)
	}))
	if err := c.Put(context.Background(), "catalog/tenant/photo.png", "image/png", data); err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("expected one complete HTTP upload, got %d", got)
	}
}

func TestSDKPresignGetSupportsR2AndB2(t *testing.T) {
	cases := []struct {
		name   string
		client *Client
		host   string
		region string
	}{
		{name: "backblaze", client: b2TestClient(http.DefaultTransport), host: "s3.us-east-005.backblazeb2.com", region: "us-east-005"},
		{name: "r2", client: &Client{AccountID: "exampleaccount", Bucket: "mujeeb24",
			AccessKeyID: "test-id", SecretAccessKey: "test-secret"},
			host: "exampleaccount.r2.cloudflarestorage.com", region: "auto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			signed, err := tc.client.PresignGet(context.Background(), "catalog/tenant/a b.jpg", 45*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(signed)
			if err != nil {
				t.Fatal(err)
			}
			if u.Scheme != "https" || u.Host != tc.host ||
				!strings.Contains(u.EscapedPath(), "catalog/tenant/a%20b.jpg") {
				t.Fatalf("unexpected presign target host=%s path=%s", u.Host, u.EscapedPath())
			}
			if !strings.Contains(u.Query().Get("X-Amz-Credential"), "/"+tc.region+"/s3/aws4_request") ||
				u.Query().Get("X-Amz-Expires") != "2700" ||
				u.Query().Get("X-Amz-Signature") == "" {
				t.Fatal("presign missing region, TTL or AWS signature")
			}
		})
	}
}

func TestSDKBackblazePutObjectReturnsSafeS3ErrorDetails(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{400, "IncompleteBody"}, {403, "AccessDenied"}, {400, "InvalidRequest"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			c := b2TestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
				_, _ = io.Copy(io.Discard, req.Body)
				return &http.Response{
					StatusCode: tc.status,
					Header:     http.Header{"X-Amz-Request-Id": []string{"request-123"}},
					Body:       io.NopCloser(strings.NewReader("<Error><Code>" + tc.code + "</Code><Message>private-test-secret</Message><RequestId>request-123</RequestId></Error>")),
				}, nil
			}))
			err := c.Put(context.Background(), "catalog/tenant/photo.png", "image/png", []byte("png"))
			if err == nil {
				t.Fatal("provider error must reject PUT")
			}
			for _, expected := range []string{fmt.Sprintf("HTTP %d", tc.status), "code=" + tc.code} {
				if !strings.Contains(err.Error(), expected) {
					t.Errorf("missing safe diagnostic %q: %s", expected, err)
				}
			}
			if strings.Contains(err.Error(), "private-test-secret") || strings.Contains(err.Error(), "test-access-id") {
				t.Fatal("S3 credentials or error message leaked into logs")
			}
		})
	}
}

func TestSDKRejectsUnsafeEndpointsKeysAndMissingConfig(t *testing.T) {
	for _, endpoint := range []string{"http://s3.us-east-005.backblazeb2.com",
		"https://127.0.0.1", "https://s3.us-west-004.backblazeb2.com",
		"https://s3.us-east-005.backblazeb2.com.attacker.test"} {
		c := b2TestClient(http.DefaultTransport)
		c.Endpoint = endpoint
		if _, err := c.PresignGet(context.Background(), "catalog/tenant/x.jpg", time.Minute); err == nil {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	c := b2TestClient(http.DefaultTransport)
	for _, key := range []string{"other/key", "catalog/../secret", "catalog/foo?sig=x", "catalog//file.jpg"} {
		if _, err := c.PresignGet(context.Background(), key, time.Minute); err == nil {
			t.Errorf("unsafe key accepted: %q", key)
		}
	}
	if err := c.Put(context.Background(), "catalog/tenant/x.webp", "image/webp", []byte("x")); err == nil {
		t.Fatal("unsupported media content type allowed")
	}
}

func TestSDKBackblazeDeleteUsesSignedSDKRequest(t *testing.T) {
	var sent bool
	c := b2TestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sent = true
		if req.Method != http.MethodDelete ||
			req.URL.Host != "s3.us-east-005.backblazeb2.com" ||
			req.URL.EscapedPath() != "/mujeeb24-images/catalog/tenant/deleted.jpg" {
			t.Errorf("unexpected DELETE target: %s %s", req.Method, req.URL)
		}
		if !strings.Contains(req.Header.Get("Authorization"), "/us-east-005/s3/aws4_request") {
			t.Error("SDK did not sign B2 DELETE")
		}
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err := c.Delete(context.Background(), "catalog/tenant/deleted.jpg"); err != nil {
		t.Fatal(err)
	}
	if !sent {
		t.Fatal("delete was not dispatched")
	}
}

func TestSDKBackblazeReadonlyS3SignatureProbe(t *testing.T) {
	visited := false
	c := b2TestClient(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		visited = true
		if req.Method != http.MethodGet ||
			req.URL.Host != "s3.us-east-005.backblazeb2.com" ||
			req.URL.EscapedPath() != "/mujeeb24-images" ||
			req.URL.RawQuery != "location=" {
			t.Errorf("unexpected read-only probe request: %s %s", req.Method, req.URL)
		}
		if !strings.Contains(req.Header.Get("Authorization"), "/us-east-005/s3/aws4_request") {
			t.Error("read-only S3 probe does not use signed regional credentials")
		}
		body := `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint>us-east-005</LocationConstraint>`
		return &http.Response{
			StatusCode: 200, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(body)),
		}, nil
	}))
	if err := c.ProbeBucketLocation(context.Background()); err != nil {
		t.Fatalf("read-only S3 authorization probe failed: %v", err)
	}
	if !visited {
		t.Fatal("S3 signature probe never called storage provider")
	}
}
