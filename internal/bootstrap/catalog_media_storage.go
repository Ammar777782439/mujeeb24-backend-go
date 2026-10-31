package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/storage/r2"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// buildCatalogMediaStorageFromEnv keeps the historical R2 environment intact.
// B2 is an explicit selection, never an implicit failover of existing photos.
func buildCatalogMediaStorageFromEnv() (ports.MediaObjectStorage, string, error) {
	if os.Getenv("CATALOG_MEDIA_ENABLED") != "true" {
		return nil, "", nil
	}
	selected := strings.TrimSpace(os.Getenv("S3_PROVIDER"))
	switch selected {
	case "", "r2", "cloudflare":
		selected = "r2"
	case "backblaze":
	default:
		return nil, "", errors.New("unsupported S3_PROVIDER; expected r2 or backblaze")
	}

	backends := make(map[string]ports.MediaObjectStorage)
	if os.Getenv("R2_ACCOUNT_ID") != "" && os.Getenv("R2_BUCKET") != "" &&
		os.Getenv("R2_ACCESS_KEY_ID") != "" && os.Getenv("R2_SECRET_ACCESS_KEY") != "" {
		client := &r2.Client{
			AccountID: os.Getenv("R2_ACCOUNT_ID"), Bucket: os.Getenv("R2_BUCKET"),
			AccessKeyID: os.Getenv("R2_ACCESS_KEY_ID"), SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		}
		if _, err := client.PresignGet(context.Background(), "catalog/config-check", time.Minute); err != nil {
			return nil, "", fmt.Errorf("invalid R2 storage settings: %w", err)
		}
		backends["r2"] = client
	}
	if selected == "backblaze" || (os.Getenv("S3_ENDPOINT") != "" && os.Getenv("S3_REGION") != "" &&
		os.Getenv("S3_BUCKET") != "" && os.Getenv("S3_ACCESS_KEY_ID") != "" && os.Getenv("S3_SECRET_ACCESS_KEY") != "") {
		client := &r2.Client{
			Endpoint: os.Getenv("S3_ENDPOINT"), Region: os.Getenv("S3_REGION"),
			Bucket: os.Getenv("S3_BUCKET"), AccessKeyID: os.Getenv("S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
		}
		if _, err := client.PresignGet(context.Background(), "catalog/config-check", time.Minute); err != nil {
			return nil, "", fmt.Errorf("invalid Backblaze B2 storage settings: %w", err)
		}
		backends["backblaze"] = client
		if selected == "backblaze" && os.Getenv("CATALOG_MEDIA_B2_DIAGNOSTICS") == "true" {
			// Explicit, read-only operational diagnosis. It neither writes
			// objects nor logs credentials, auth tokens, or response bodies.
			probeCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			keyCheck := checkB2Credentials(probeCtx, nil, b2NativeAuthorizationEndpoint,
				client.AccessKeyID, client.SecretAccessKey, client.Bucket, client.Endpoint)
			log.Printf("[CatalogMedia] stage=b2_native_key_check result=%s response_kind=%s provider_code=%s key_id_suffix=%s key_id_outer_whitespace=%t secret_outer_whitespace=%t endpoint_match=%t bucket_scope=%s write_files=%t",
				keyCheck.Result, keyCheck.ResponseKind, keyCheck.ProviderCode, keyCheck.KeyIDSuffix,
				keyCheck.KeyIDWhitespace, keyCheck.SecretWhitespace, keyCheck.EndpointMatch,
				keyCheck.BucketScope, keyCheck.CanWrite)
			if keyCheck.Result == "key_pair_valid" {
				if probeErr := client.ProbeBucketLocation(probeCtx); probeErr != nil {
					log.Printf("[CatalogMedia] stage=b2_s3_readonly_check result=failed cause=%v", probeErr)
				} else {
					log.Printf("[CatalogMedia] stage=b2_s3_readonly_check result=success")
				}
			} else {
				log.Printf("[CatalogMedia] stage=b2_s3_readonly_check result=skipped reason=key_check_unverified")
			}
			cancel()
		}
	}
	if backends[selected] == nil {
		return nil, "", fmt.Errorf("catalog media storage provider %s is not configured", selected)
	}
	log.Printf("[CatalogMedia] stage=storage_configured write_provider=%s r2_available=%t backblaze_available=%t", selected, backends["r2"] != nil, backends["backblaze"] != nil)
	return &services.CatalogMediaObjectStore{WriteProvider: selected, Providers: backends}, selected, nil
}
