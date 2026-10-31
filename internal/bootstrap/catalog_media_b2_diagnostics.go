package bootstrap

import (
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
	Result        string
	EndpointMatch bool
	BucketScope   string
	CanWrite      bool
}

func checkB2Credentials(ctx context.Context, client *http.Client, authURL, keyID, secret, bucket, endpoint string) b2CredentialCheck {
	result := b2CredentialCheck{Result: "unavailable", BucketScope: "unknown"}
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
		result.Result = "invalid_key_pair"
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
