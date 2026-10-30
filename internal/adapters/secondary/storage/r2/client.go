package r2

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client speaks S3-compatible SigV4 for Cloudflare R2 and Backblaze B2.
// Legacy R2 credentials are preserved; Backblaze sets Endpoint and Region.
type Client struct {
	AccountID       string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	HTTP            *http.Client
	Now             func() time.Time
}

func (c *Client) valid() error {
	if c == nil || c.Bucket == "" || c.AccessKeyID == "" || c.SecretAccessKey == "" {
		return errors.New("S3 storage credentials are incomplete")
	}
	for _, r := range c.Bucket {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return errors.New("invalid object storage bucket")
		}
	}
	if c.Endpoint == "" && c.Region == "" {
		if c.AccountID == "" {
			return errors.New("R2 account ID is missing")
		}
		for _, r := range c.AccountID {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return errors.New("invalid R2 account ID")
			}
		}
		return nil
	}
	// Restrict signed URL destinations and API calls to official B2 hosts.
	if c.AccountID != "" || len(c.Region) < 3 {
		return errors.New("invalid Backblaze B2 region")
	}
	for _, r := range c.Region {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return errors.New("invalid Backblaze B2 region")
		}
	}
	if c.Endpoint != "https://s3."+c.Region+".backblazeb2.com" {
		return errors.New("Backblaze B2 endpoint does not match the region")
	}
	return nil
}
func (c *Client) region() string {
	if c.Region != "" {
		return c.Region
	}
	return "auto"
}
func (c *Client) clock() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func (c *Client) host() string {
	if c.Endpoint != "" {
		return strings.TrimPrefix(c.Endpoint, "https://")
	}
	return c.AccountID + ".r2.cloudflarestorage.com"
}
func escapeKey(key string) (string, error) {
	if !strings.HasPrefix(key, "catalog/") || strings.Contains(key, "..") || strings.ContainsAny(key, "\\?#") {
		return "", errors.New("unsafe R2 key")
	}
	parts := strings.Split(key, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", errors.New("unsafe R2 path")
		}
	}
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/"), nil
}
func sha(v []byte) string { s := sha256.Sum256(v); return hex.EncodeToString(s[:]) }
func hmac256(k []byte, s string) []byte {
	m := hmac.New(sha256.New, k)
	m.Write([]byte(s))
	return m.Sum(nil)
}
func (c *Client) signingKey(day string) []byte {
	k := hmac256([]byte("AWS4"+c.SecretAccessKey), day)
	k = hmac256(k, c.region())
	k = hmac256(k, "s3")
	return hmac256(k, "aws4_request")
}

// countedBody tracks bytes actually read by the HTTP transport. It cannot
// prove remote receipt, but distinguishes local body truncation from a remote
// rejection. Never record image bytes, signatures, credentials or signed URLs.
type countedBody struct {
	io.ReadCloser
	readBytes int64
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.readBytes += int64(n)
	return n, err
}

// Keep provider error diagnostics safe: use XML Code and request ID only.
// Never echo the raw S3 XML Message (it may contain signed request material).
func safeProviderToken(v string) string {
	if v == "" || len(v) > 96 {
		return "unavailable"
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "unavailable"
		}
	}
	return v
}

func classifyProviderHTTPError(method string, res *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	var body struct {
		Code      string `xml:"Code"`
		Message   string `xml:"Message"`
		RequestID string `xml:"RequestId"`
	}
	_ = xml.Unmarshal(raw, &body)
	id := res.Header.Get("X-Amz-Request-Id")
	if id == "" {
		id = body.RequestID
	}
	// Classification is deliberately based on controlled strings, not echoed text.
	reason := "unclassified"
	m := strings.ToLower(body.Code + " " + body.Message)
	switch {
	case strings.EqualFold(body.Code, "IncompleteBody"):
		reason = "remote_incomplete_body"
	case strings.EqualFold(body.Code, "AccessDenied"):
		reason = "provider_access_denied"
	case strings.EqualFold(body.Code, "SignatureDoesNotMatch"):
		reason = "signature_mismatch"
	case strings.Contains(m, "unsupported header") && strings.Contains(m, "checksum"):
		reason = "unsupported_checksum_header"
	case strings.Contains(m, "signature") || strings.Contains(m, "authorization"):
		reason = "signature_or_auth"
	case strings.Contains(m, "content-length") || strings.Contains(m, "content length"):
		reason = "content_length"
	case strings.Contains(m, "digest") || strings.Contains(m, "checksum") ||
		strings.Contains(m, "uploaded data") || strings.Contains(m, "payload hash"):
		reason = "content_integrity"
	case strings.Contains(m, "region") || strings.Contains(m, "endpoint"):
		reason = "region_or_endpoint"
	case strings.Contains(m, "bucket"):
		reason = "bucket_reference"
	case strings.Contains(m, "invalidrequest") || strings.Contains(m, "malformed"):
		reason = "malformed_request"
	}
	return fmt.Errorf("S3 %s failed: HTTP %d code=%s reason=%s request_id=%s",
		method, res.StatusCode, safeProviderToken(body.Code), reason, safeProviderToken(id))
}

func (c *Client) request(ctx context.Context, method, key, mime string, body []byte) (*http.Response, error) {
	if err := c.valid(); err != nil {
		return nil, err
	}
	escaped, err := escapeKey(key)
	if err != nil {
		return nil, err
	}
	uri := "/" + c.Bucket + "/" + escaped
	now := c.clock()
	date := now.Format("20060102")
	timestamp := now.Format("20060102T150405Z")
	hash := sha(body)
	canonicalHeaders := "host:" + c.host() + "\nx-amz-content-sha256:" + hash + "\nx-amz-date:" + timestamp + "\n"
	headers := "host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{method, uri, "", canonicalHeaders, headers, hash}, "\n")
	scope := date + "/" + c.region() + "/s3/aws4_request"
	st := strings.Join([]string{"AWS4-HMAC-SHA256", timestamp, scope, sha([]byte(canonical))}, "\n")
	sign := hex.EncodeToString(hmac256(c.signingKey(date), st))
	auth := "AWS4-HMAC-SHA256 Credential=" + c.AccessKeyID + "/" + scope + ", SignedHeaders=" + headers + ", Signature=" + sign
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+c.host()+uri, reader)
	if err != nil {
		return nil, err
	}
	// Go usually infers the size from bytes.Reader, but require an exact
	// Content-Length and a non-chunked upload before any network I/O.
	var observed *countedBody
	if method == http.MethodPut {
		expected := int64(len(body))
		if expected == 0 || req.ContentLength != expected || req.Body == nil ||
			len(req.TransferEncoding) != 0 {
			return nil, fmt.Errorf("invalid S3 upload body framing expected=%d declared=%d",
				expected, req.ContentLength)
		}
		observed = &countedBody{ReadCloser: req.Body}
		req.Body = observed
	}
	req.Header.Set("X-Amz-Date", timestamp)
	req.Header.Set("X-Amz-Content-Sha256", hash)
	req.Header.Set("Authorization", auth)
	if method == http.MethodPut {
		req.Header.Set("Content-Type", mime)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		if observed != nil {
			return nil, fmt.Errorf("S3 PUT transport failure body_read=%d body_expected=%d: %w",
				observed.readBytes, req.ContentLength, err)
		}
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		providerErr := classifyProviderHTTPError(method, res)
		if observed != nil {
			return nil, fmt.Errorf("%w http_protocol=%s body_read=%d body_expected=%d",
				providerErr, res.Proto, observed.readBytes, req.ContentLength)
		}
		return nil, providerErr
	}
	if observed != nil && observed.readBytes != req.ContentLength {
		res.Body.Close()
		return nil, fmt.Errorf("S3 PUT returned success with incomplete local read body_read=%d body_expected=%d",
			observed.readBytes, req.ContentLength)
	}
	return res, nil
}
func (c *Client) Put(ctx context.Context, key, mime string, data []byte) error {
	if len(data) == 0 || len(data) > 10*1024*1024 {
		return errors.New("image size exceeds limit")
	}
	if mime != "image/jpeg" && mime != "image/png" {
		return errors.New("unsupported R2 media content type")
	}
	res, err := c.request(ctx, http.MethodPut, key, mime, data)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}
func (c *Client) Delete(ctx context.Context, key string) error {
	res, err := c.request(ctx, http.MethodDelete, key, "", nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

// PresignGet is created when the outbox is dispatched, never persisted
// in PostgreSQL. The provider fetches private bytes over a scoped URL.
func (c *Client) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	if err := c.valid(); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > time.Hour {
		return "", errors.New("invalid R2 URL TTL")
	}
	escaped, err := escapeKey(key)
	if err != nil {
		return "", err
	}
	uri := "/" + c.Bucket + "/" + escaped
	now := c.clock()
	date := now.Format("20060102")
	timestamp := now.Format("20060102T150405Z")
	scope := date + "/" + c.region() + "/s3/aws4_request"
	params := url.Values{}
	params.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	params.Set("X-Amz-Credential", c.AccessKeyID+"/"+scope)
	params.Set("X-Amz-Date", timestamp)
	params.Set("X-Amz-Expires", fmt.Sprintf("%d", int(ttl.Seconds())))
	params.Set("X-Amz-SignedHeaders", "host")
	// url.Values.Encode sorts keys, and SigV4 requires RFC3986 %20 encoding.
	query := strings.ReplaceAll(params.Encode(), "+", "%20")
	headers := "host:" + c.host() + "\n"
	canonical := strings.Join([]string{"GET", uri, query, headers, "host", "UNSIGNED-PAYLOAD"}, "\n")
	sign := hex.EncodeToString(hmac256(c.signingKey(date), strings.Join([]string{"AWS4-HMAC-SHA256", timestamp, scope, sha([]byte(canonical))}, "\n")))
	params.Set("X-Amz-Signature", sign)
	// Defense in depth: no user-controlled query is included.
	return "https://" + c.host() + uri + "?" + strings.ReplaceAll(params.Encode(), "+", "%20"), nil
}
