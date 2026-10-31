package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxPrivatePreviewBytes = 10 * 1024 * 1024

// isTrustedCatalogObjectURL prevents the API from becoming an arbitrary URL
// fetcher. The signed URL originates in our own configured storage SDK and is
// never returned to merchants by this endpoint.
func isTrustedCatalogObjectURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil ||
		u.Fragment != "" || u.Port() != "" || u.RawQuery == "" ||
		u.Query().Get("X-Amz-Signature") == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	if parts[1] == "r2" && parts[2] == "cloudflarestorage" && parts[3] == "com" {
		return isSafePreviewLabel(parts[0])
	}
	if parts[0] == "s3" && parts[2] == "backblazeb2" && parts[3] == "com" {
		return isSafePreviewLabel(parts[1])
	}
	return false
}

func isSafePreviewLabel(v string) bool {
	if v == "" {
		return false
	}
	for _, c := range v {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func verifiedPreviewBytes(r io.Reader, mime string) ([]byte, error) {
	if mime != "image/jpeg" && mime != "image/png" {
		return nil, errors.New("unsupported media preview type")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxPrivatePreviewBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxPrivatePreviewBytes {
		return nil, errors.New("media preview is oversized or incomplete")
	}
	if http.DetectContentType(data) != mime {
		return nil, errors.New("media preview bytes do not match recorded type")
	}
	return data, nil
}

func fetchPrivateCatalogPreview(ctx context.Context, signedURL, mime string) ([]byte, error) {
	if !isTrustedCatalogObjectURL(signedURL) {
		return nil, errors.New("untrusted catalog image host")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, errors.New("invalid signed image request")
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("private image fetch failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.ContentLength > maxPrivatePreviewBytes {
		return nil, errors.New("private image storage rejected read")
	}
	return verifiedPreviewBytes(res.Body, mime)
}
