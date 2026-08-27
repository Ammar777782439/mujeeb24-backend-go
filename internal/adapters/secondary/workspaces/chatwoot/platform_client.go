package chatwoot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

var ErrPlatformNotConfigured = errors.New("chatwoot platform client is not configured")
var ErrPlatformUnsupported = errors.New("chatwoot platform provisioning requires self-hosted platform api")

type PlatformConfig struct {
	BaseURL       string
	PlatformToken string
	APIToken      string
	APIUserID     int64
	HTTPClient    *http.Client
	HTTPTimeout   time.Duration
}

type PlatformClient struct {
	baseURL   string
	token     string
	apiToken  string
	apiUserID int64
	client    *http.Client
}

func NewPlatformClient(cfg PlatformConfig) *PlatformClient {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.HTTPTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &PlatformClient{baseURL: baseURL, token: strings.TrimSpace(cfg.PlatformToken), apiToken: strings.TrimSpace(cfg.APIToken), apiUserID: cfg.APIUserID, client: client}
}

func (c *PlatformClient) EnsureAccount(ctx context.Context, name, locale string) (ports.WorkspaceAccount, error) {
	if err := c.validate(); err != nil {
		return ports.WorkspaceAccount{}, err
	}
	if strings.TrimSpace(name) == "" {
		return ports.WorkspaceAccount{}, fmt.Errorf("%w: account name is required", ErrInvalidRequest)
	}
	var response struct {
		ID int64 `json:"id"`
	}
	_, err := c.do(ctx, http.MethodPost, "/platform/api/v1/accounts", map[string]any{
		"name":   strings.TrimSpace(name),
		"locale": strings.TrimSpace(locale),
		"status": "active",
	}, &response)
	if err != nil {
		return ports.WorkspaceAccount{}, err
	}
	if response.ID <= 0 {
		return ports.WorkspaceAccount{}, fmt.Errorf("%w: account id missing", ErrInvalidResponse)
	}
	if c.apiUserID <= 0 {
		return ports.WorkspaceAccount{}, fmt.Errorf("%w: application API user id is required to provision an inbox", ErrPlatformNotConfigured)
	}
	if _, err := c.do(ctx, http.MethodPost, "/platform/api/v1/accounts/"+strconv.FormatInt(response.ID, 10)+"/account_users", map[string]any{
		"user_id": c.apiUserID,
		"role":    "administrator",
	}, nil); err != nil {
		return ports.WorkspaceAccount{}, fmt.Errorf("%w: attach application API user to account", err)
	}

	return ports.WorkspaceAccount{ID: strconv.FormatInt(response.ID, 10)}, nil
}

func (c *PlatformClient) EnsureInbox(ctx context.Context, accountID, name, channel, webhookURL string) (ports.WorkspaceInbox, error) {
	if err := c.validate(); err != nil {
		return ports.WorkspaceInbox{}, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(webhookURL) == "" {
		return ports.WorkspaceInbox{}, fmt.Errorf("%w: account, name, and webhook url are required", ErrInvalidRequest)
	}
	parsed, err := url.Parse(webhookURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ports.WorkspaceInbox{}, fmt.Errorf("%w: webhook url must be https", ErrInvalidRequest)
	}
	var response struct {
		ID int64 `json:"id"`
	}
	_, err = c.doAsUser(ctx, http.MethodPost, "/api/v1/accounts/"+url.PathEscape(accountID)+"/inboxes", map[string]any{
		"name":                   strings.TrimSpace(name),
		"enable_auto_assignment": false,
		"working_hours_enabled":  false,
		"channel": map[string]any{
			"type":           "api",
			"webhook_url":    webhookURL,
			"hmac_mandatory": true,
		},
	}, &response)
	if err != nil {
		return ports.WorkspaceInbox{}, err
	}
	if response.ID <= 0 {
		return ports.WorkspaceInbox{}, fmt.Errorf("%w: inbox id missing", ErrInvalidResponse)
	}
	return ports.WorkspaceInbox{ID: strconv.FormatInt(response.ID, 10)}, nil
}

func (c *PlatformClient) validate() error {
	if c == nil || c.client == nil || c.token == "" || c.baseURL == "" {
		return ErrPlatformNotConfigured
	}
	return nil
}

func (c *PlatformClient) do(ctx context.Context, method, path string, body any, response any) (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%w: encode request", ErrInvalidRequest)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, strings.NewReader(string(encoded)))
	if err != nil {
		return "", fmt.Errorf("%w: create request", ErrInvalidRequest)
	}
	req.Header.Set("api_access_token", c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer res.Body.Close()
	requestID := res.Header.Get("X-Request-ID")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return requestID, fmt.Errorf("%w: platform http %d", ErrInvalidResponse, res.StatusCode)
	}
	if response != nil {
		if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(response); err != nil {
			return requestID, fmt.Errorf("%w: decode response", ErrInvalidResponse)
		}
	}
	return requestID, nil
}

func (c *PlatformClient) doAsUser(ctx context.Context, method, path string, body any, response any) (string, error) {
	if err := c.validateUserAPI(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%w: encode request", ErrInvalidRequest)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, strings.NewReader(string(encoded)))
	if err != nil {
		return "", fmt.Errorf("%w: create request", ErrInvalidRequest)
	}
	req.Header.Set("api_access_token", c.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer res.Body.Close()
	requestID := res.Header.Get("X-Request-ID")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return requestID, fmt.Errorf("%w: platform http %d", ErrInvalidResponse, res.StatusCode)
	}
	if response != nil {
		if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(response); err != nil {
			return requestID, fmt.Errorf("%w: decode response", ErrInvalidResponse)
		}
	}
	return requestID, nil
}

func (c *PlatformClient) validateUserAPI() error {
	if err := c.validate(); err != nil {
		return err
	}
	if c.apiToken == "" || c.apiUserID <= 0 {
		return ErrPlatformNotConfigured
	}
	return nil
}

var _ ports.WorkspaceProvisioner = (*PlatformClient)(nil)
