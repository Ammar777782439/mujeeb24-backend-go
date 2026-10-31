package r2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Client uses the official AWS SDK for both S3-compatible private stores.
// Legacy R2 credentials, the media port, and database object keys are unchanged.
// There is no hand-written HTTP request, signature, or presign implementation.
type Client struct {
	AccountID       string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	HTTP            *http.Client
	Now             func() time.Time // kept for source compatibility; SDK owns signing time
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

func (c *Client) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return "https://" + c.AccountID + ".r2.cloudflarestorage.com"
}

func validateKey(key string) error {
	if !strings.HasPrefix(key, "catalog/") || strings.Contains(key, "..") || strings.ContainsAny(key, "\\?#") {
		return errors.New("unsafe catalog media key")
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("unsafe catalog media key")
		}
	}
	return nil
}

func (c *Client) sdk() (*s3.Client, error) {
	if err := c.valid(); err != nil {
		return nil, err
	}
	cfg := aws.Config{
		Region:           c.region(),
		Credentials:      credentials.NewStaticCredentialsProvider(c.AccessKeyID, c.SecretAccessKey, ""),
		RetryMaxAttempts: 2,
	}
	if c.HTTP != nil {
		cfg.HTTPClient = c.HTTP
	} else {
		cfg.HTTPClient = &http.Client{Timeout: 25 * time.Second}
	}
	return s3.NewFromConfig(cfg, func(opts *s3.Options) {
		opts.BaseEndpoint = aws.String(c.endpoint())
		opts.UsePathStyle = true
	}), nil
}

func safeDiagnosticToken(v string) string {
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

// Return only structured safe S3 diagnostics; never log SDK ErrorMessage,
// Authorization, request URLs, signed URLs, or any object bytes.
func wrapSDKError(op string, err error) error {
	if err == nil {
		return nil
	}
	code := "unavailable"
	if apiErr := (smithy.APIError)(nil); errors.As(err, &apiErr) {
		code = safeDiagnosticToken(apiErr.ErrorCode())
	}
	status := 0
	var responseStatus interface{ HTTPStatusCode() int }
	if errors.As(err, &responseStatus) {
		status = responseStatus.HTTPStatusCode()
	}
	requestID := "unavailable"
	var responseID interface{ ServiceRequestID() string }
	if errors.As(err, &responseID) {
		requestID = safeDiagnosticToken(responseID.ServiceRequestID())
	}
	return fmt.Errorf("S3 %s failed: HTTP %d code=%s request_id=%s", op, status, code, requestID)
}

func (c *Client) Put(ctx context.Context, key, mime string, data []byte) error {
	if len(data) == 0 || len(data) > 10*1024*1024 {
		return errors.New("catalog image must be between 1 and 10 MiB")
	}
	if mime != "image/jpeg" && mime != "image/png" {
		return errors.New("unsupported catalog media content type")
	}
	if err := validateKey(key); err != nil {
		return err
	}
	client, err := c.sdk()
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.Bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(mime),
		ContentLength: aws.Int64(int64(len(data))),
		Body:          bytes.NewReader(data),
	})
	return wrapSDKError("PUT", err)
}

// ProbeBucketLocation is read-only and checks S3 SigV4 independently of PUT.
// Backblaze documents GetBucketLocation as a supported operation.
func (c *Client) ProbeBucketLocation(ctx context.Context) error {
	client, err := c.sdk()
	if err != nil {
		return err
	}
	_, err = client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: aws.String(c.Bucket),
	})
	return wrapSDKError("GET_BUCKET_LOCATION", err)
}

func (c *Client) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	client, err := c.sdk()
	if err != nil {
		return err
	}
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
	})
	return wrapSDKError("DELETE", err)
}

func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > time.Hour {
		return "", errors.New("invalid catalog media URL TTL")
	}
	client, err := c.sdk()
	if err != nil {
		return "", err
	}
	signed, err := s3.NewPresignClient(client).PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = ttl
	})
	if err != nil {
		return "", errors.New("catalog media presign failed")
	}
	return signed.URL, nil
}
