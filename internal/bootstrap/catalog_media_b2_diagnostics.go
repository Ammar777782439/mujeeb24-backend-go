package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// b2CredentialCheck uses the read-only Native API to check the actual B2 key
// pair independently of AWS SigV4. It never returns auth tokens or raw bodies.
type b2CredentialCheck struct {
	Result           string
	ResponseKind     string
	ProviderCode     string
	KeyIDSuffix      string
	KeyIDWhitespace  bool
	SecretWhitespace bool
	EndpointMatch    bool
	BucketScope      string
	CanWrite         bool
}

// Only the trailing 4 characters of the non-secret application key ID
// are logged, never any part of the application key secret itself.
func b2KeyIDSuffix(keyID string) string {
	if len(keyID) < 4 {
		return "unavailable"
	}
	return keyID[len(keyID)-4:]
}

func safeB2ErrorCode(code string) string {
	if code == "" || len(code) > 64 {
		return "unavailable"
	}
	for _, r := range code {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return "unavailable"
		}
	}
	return code
}

func b2ResponseKind(raw []byte) string {
	clean := bytes.TrimSpace(raw)
	if len(clean) == 0 {
		return "empty"
	}
	switch clean[0] {
	case '{':
		return "json"
	case '<':
		return "html_or_xml"
	default:
		return "other"
	}
}

func checkB2Credentials(ctx context.Context, client *http.Client, authURL, keyID, secret, bucket, endpoint string) b2CredentialCheck {
	result := b2CredentialCheck{
		Result: "unavailable", ResponseKind: "none",
		ProviderCode: "unavailable", BucketScope: "unknown",
		KeyIDSuffix: b2KeyIDSuffix(keyID),
		KeyIDWhitespace: strings.TrimSpace(keyID) != keyID,
		SecretWhitespace: strings.TrimSpace(secret) != secret,
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return result
	}
	req.SetBasicAuth(keyID, secret)
	resp, err := client.Do(req)
	if err != nil {
		result.Result = "transport_error"
		return result
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		// Backblaze distinguishes 401 unauthorized (invalid key ID / secret)
		// from 401 unsupported (key cannot use this API version). Do not
		// treat every 401 as invalid credentials.
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		result.ResponseKind = b2ResponseKind(raw)
		var failure struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(raw, &failure) != nil {
			result.Result = "auth_401_unclassified"
			return result
		}
		result.ProviderCode = safeB2ErrorCode(failure.Code)
		switch failure.Code {
		case "unauthorized":
			result.Result = "invalid_key_pair"
		case "unsupported":
			result.Result = "api_version_unsupported"
		default:
			result.Result = "auth_401_unclassified"
		}
		return result
	case http.StatusOK:
	default:
		result.Result = "http_error"
		return result
	}
	var body struct {
		APIInfo struct {
			StorageAPI struct {
				S3APIURL string `json:"s3ApiUrl"`
				Allowed  struct {
					Buckets []struct {
						Name *string `json:"name"`
					} `json:"buckets"`
					Capabilities []string `json:"capabilities"`
				} `json:"allowed"`
			} `json:"storageApi"`
		} `json:"apiInfo"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&body); err != nil {
		result.Result = "invalid_authorize_response"
		return result
	}
	result.Result = "key_pair_valid"
	result.ResponseKind = "json"
	result.EndpointMatch = strings.TrimSuffix(body.APIInfo.StorageAPI.S3APIURL, "/") == endpoint
	allowed := body.APIInfo.StorageAPI.Allowed
	if len(allowed.Buckets) > 0 {
		result.BucketScope = "other_or_unresolved"
		for _, b := range allowed.Buckets {
			if b.Name != nil && *b.Name == bucket {
				result.BucketScope = "matches"
				break
			}
		}
	}
	for _, capability := range allowed.Capabilities {
		if capability == "writeFiles" {
			result.CanWrite = true
			break
		}
	}
	return result
}

const b2NativeAuthorizationEndpoint = "https://api.backblazeb2.com/b2api/v4/b2_authorize_account"
