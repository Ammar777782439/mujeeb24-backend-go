package socialapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
	"github.com/google/uuid"
)

const defaultBaseURL = "https://api.social-api.ai"

var (
	ErrNotConfigured  = errors.New("socialapi client is not configured")
	ErrInvalidWebhook = errors.New("socialapi webhook signature is invalid")
	ErrReplay         = errors.New("socialapi webhook timestamp is outside tolerance")
	ErrUnsupported    = errors.New("socialapi operation is unsupported by the verified contract")
)

type Client struct {
	baseURL       string
	apiKey        string
	webhookSecret string
	httpClient    *http.Client
	tolerance     time.Duration
}

type Config struct {
	BaseURL       string
	APIKey        string
	WebhookSecret string
	HTTPClient    *http.Client
	HTTPTimeout   time.Duration
	ReplayWindow  time.Duration
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.HTTPTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	tolerance := cfg.ReplayWindow
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	return &Client{baseURL: baseURL, apiKey: strings.TrimSpace(cfg.APIKey), webhookSecret: cfg.WebhookSecret, httpClient: client, tolerance: tolerance}
}

type ConnectedAccount struct {
	ID       string         `json:"id"`
	BrandID  string         `json:"brand_id"`
	Platform string         `json:"platform"`
	Status   string         `json:"status"`
	Username string         `json:"username"`
	Name     string         `json:"name"`
	PageName string         `json:"page_name"`
	Metadata map[string]any `json:"metadata"`
}

type connectedAccountsResponse struct {
	Count int                `json:"count"`
	Data  []ConnectedAccount `json:"data"`
}

func (c *Client) ListConnectedAccounts(ctx context.Context, brandID string) ([]ConnectedAccount, error) {
	path := "/v1/accounts"
	if strings.TrimSpace(brandID) != "" {
		path += "?brand_id=" + url.QueryEscape(brandID)
	}
	var response connectedAccountsResponse
	_, err := c.doJSON(ctx, http.MethodGet, path, nil, &response)
	return response.Data, err
}

type ConnectRequest struct {
	Platform    string         `json:"platform"`
	RedirectURI string         `json:"redirect_uri"`
	State       string         `json:"state,omitempty"`
	BrandID     string         `json:"brand_id,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type ConnectResponse struct {
	AccountID   string `json:"account_id"`
	AuthURL     string `json:"auth_url"`
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	State       string `json:"state"`
	Username    string `json:"username"`
	Message     string `json:"message"`
}

func (c *Client) BeginConnection(ctx context.Context, request ConnectRequest) (ConnectResponse, error) {
	if strings.TrimSpace(request.Platform) == "" {
		return ConnectResponse{}, fmt.Errorf("%w: platform is required", ErrInvalidRequest)
	}
	if request.RedirectURI != "" {
		redirectURI, err := url.Parse(request.RedirectURI)
		if err != nil || redirectURI.Scheme != "https" || redirectURI.Host == "" {
			return ConnectResponse{}, fmt.Errorf("%w: redirect_uri must be a valid https URL", ErrInvalidRequest)
		}
	}
	if utf8.RuneCountInString(request.State) > 512 {
		return ConnectResponse{}, fmt.Errorf("%w: state exceeds 512 characters", ErrInvalidRequest)
	}
	var response ConnectResponse
	_, err := c.doJSON(ctx, http.MethodPost, "/v1/accounts/connect", request, &response)
	return response, err
}

type InboxPagination struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}

type InboxConversation struct {
	ID                 string `json:"id"`
	AccountID          string `json:"account_id"`
	Platform           string `json:"platform"`
	PlatformID         string `json:"platform_id"`
	PageID             string `json:"page_id"`
	ParticipantID      string `json:"participant_id"`
	ParticipantName    string `json:"participant_name"`
	ParticipantPicture string `json:"participant_picture"`
	UserID             string `json:"user_id"`
	LastMessage        string `json:"last_message"`
	Status             string `json:"status"`
	UnreadCount        int    `json:"unread_count"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
	LastMessageAt      string `json:"last_message_at"`
}

type InboxConversationsResponse struct {
	Data         []InboxConversation `json:"data"`
	LastSyncedAt string              `json:"last_synced_at"`
	Pagination   InboxPagination     `json:"pagination"`
	SyncState    string              `json:"sync_state"`
}

type InboxConversationQuery struct {
	AccountID string
	BrandID   string
	PageID    string
	Platform  string
	Status    string
	Limit     int
	Cursor    string
}

func (c *Client) ListInboxConversations(ctx context.Context, query InboxConversationQuery) (InboxConversationsResponse, error) {
	if query.Limit < 0 || query.Limit > 100 {
		return InboxConversationsResponse{}, fmt.Errorf("%w: conversation limit must be between 1 and 100", ErrInvalidRequest)
	}
	if query.Status != "" && query.Status != "active" && query.Status != "archived" {
		return InboxConversationsResponse{}, fmt.Errorf("%w: conversation status must be active or archived", ErrInvalidRequest)
	}
	values := url.Values{}
	if query.AccountID != "" {
		values.Set("account_id", query.AccountID)
	}
	if query.BrandID != "" {
		values.Set("brand_id", query.BrandID)
	}
	if query.PageID != "" {
		values.Set("page_id", query.PageID)
	}
	if query.Platform != "" {
		values.Set("platform", query.Platform)
	}
	if query.Status != "" {
		values.Set("status", query.Status)
	}
	if query.Limit != 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	if query.Cursor != "" {
		values.Set("cursor", query.Cursor)
	}
	path := "/v1/inbox/conversations"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var response InboxConversationsResponse
	_, err := c.doJSON(ctx, http.MethodGet, path, nil, &response)
	return response, err
}

func (c *Client) GetInboxConversation(ctx context.Context, conversationID string) (InboxConversation, error) {
	if strings.TrimSpace(conversationID) == "" {
		return InboxConversation{}, fmt.Errorf("%w: conversation id is required", ErrInvalidRequest)
	}
	var response struct {
		Data InboxConversation `json:"data"`
	}
	_, err := c.doJSON(ctx, http.MethodGet, "/v1/inbox/conversations/"+url.PathEscape(conversationID), nil, &response)
	return response.Data, err
}

type InboxMessagesQuery struct {
	Limit  int
	Cursor string
}

type InboxMessage struct {
	ID              string `json:"id"`
	ConversationID  string `json:"conversation_id"`
	PlatformID      string `json:"platform_id"`
	SenderID        string `json:"sender_id"`
	SenderName      string `json:"sender_name"`
	Text            string `json:"text"`
	Direction       string `json:"direction"`
	Status          string `json:"status"`
	AttachmentType  string `json:"attachment_type"`
	AttachmentURL   string `json:"attachment_url"`
	CreatedAt       string `json:"created_at"`
	StatusUpdatedAt string `json:"status_updated_at"`
}

type InboxMessagesResponse struct {
	Data         []InboxMessage  `json:"data"`
	LastSyncedAt string          `json:"last_synced_at"`
	Pagination   InboxPagination `json:"pagination"`
	SyncState    string          `json:"sync_state"`
}

func (c *Client) ListConversationMessages(ctx context.Context, conversationID string, query InboxMessagesQuery) (InboxMessagesResponse, error) {
	if strings.TrimSpace(conversationID) == "" {
		return InboxMessagesResponse{}, fmt.Errorf("%w: conversation id is required", ErrInvalidRequest)
	}
	if query.Limit < 0 || query.Limit > 200 {
		return InboxMessagesResponse{}, fmt.Errorf("%w: message limit must be between 1 and 200", ErrInvalidRequest)
	}
	values := url.Values{}
	if query.Limit != 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	if query.Cursor != "" {
		values.Set("cursor", query.Cursor)
	}
	path := "/v1/inbox/conversations/" + url.PathEscape(conversationID) + "/messages"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var response InboxMessagesResponse
	_, err := c.doJSON(ctx, http.MethodGet, path, nil, &response)
	return response, err
}

type RegisterWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type RegisterWebhookResponse struct {
	ID     string   `json:"id"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret"`
}

func (c *Client) RegisterWebhook(ctx context.Context, request RegisterWebhookRequest) (RegisterWebhookResponse, error) {
	webhookURL, err := url.Parse(request.URL)
	if strings.TrimSpace(request.URL) == "" || err != nil || webhookURL.Scheme != "https" || webhookURL.Host == "" || len(request.Events) == 0 {
		return RegisterWebhookResponse{}, fmt.Errorf("%w: webhook url must be https and events are required", ErrInvalidRequest)
	}
	var response RegisterWebhookResponse
	_, err = c.doJSON(ctx, http.MethodPost, "/v1/webhooks", request, &response)
	return response, err
}

type PendingSelectionRequest struct {
	PageIDs           []string `json:"page_ids,omitempty"`
	PlatformAccountID string   `json:"platform_account_id,omitempty"`
}

type PendingSelectionResponse struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	Platform    string `json:"platform"`
	Username    string `json:"username"`
}

func (c *Client) SelectPendingConnection(ctx context.Context, connectionID string, request PendingSelectionRequest) (PendingSelectionResponse, error) {
	if strings.TrimSpace(connectionID) == "" {
		return PendingSelectionResponse{}, fmt.Errorf("%w: connection id is required", ErrInvalidRequest)
	}
	var response PendingSelectionResponse
	_, err := c.doJSON(ctx, http.MethodPost, "/v1/accounts/pending/"+url.PathEscape(connectionID)+"/select", request, &response)
	return response, err
}

func (c *Client) VerifyWebhook(ctx context.Context, headers map[string]string, rawBody []byte) error {
	if ctx == nil {
		return ErrInvalidWebhook
	}
	if headerValue(headers, "X-SocialAPI-Event") == "webhook.test" {
		return nil
	}
	if strings.TrimSpace(c.webhookSecret) == "" {
		return fmt.Errorf("%w: webhook secret is not configured", ErrInvalidWebhook)
	}
	v2Signature := strings.TrimSpace(headerValue(headers, "X-SocialAPI-Signature-V2"))
	if v2Signature != "" {
		timestamp := strings.TrimSpace(headerValue(headers, "X-SocialAPI-Timestamp"))
		seconds, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil || seconds <= 0 {
			return ErrInvalidWebhook
		}
		age := time.Since(time.Unix(seconds, 0))
		if age < 0 {
			age = -age
		}
		if age > c.tolerance {
			return ErrReplay
		}
		if !secureSignature(v2Signature, c.webhookSecret, timestamp+"."+string(rawBody)) {
			return ErrInvalidWebhook
		}
		return nil
	}
	if !secureSignature(strings.TrimSpace(headerValue(headers, "X-SocialAPI-Signature")), c.webhookSecret, string(rawBody)) {
		return ErrInvalidWebhook
	}
	return nil
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func secureSignature(header, secret, signed string) bool {
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signed))
	return hmac.Equal(provided, mac.Sum(nil))
}

type socialWebhookEnvelope struct {
	Event      string            `json:"event"`
	Data       socialWebhookData `json:"data"`
	RawPayload json.RawMessage   `json:"raw_payload"`
}

type socialWebhookData struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Platform       string `json:"platform"`
	AccountID      string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	PlatformID     string `json:"platform_id"`
	ReceivedAt     string `json:"received_at"`
	CreatedAt      string `json:"created_at"`
	Content        struct {
		Text string `json:"text"`
	} `json:"content"`
	Author struct {
		ID string `json:"id"`
	} `json:"author"`
	Status      string   `json:"status"`
	MIDs        []string `json:"mids"`
	RecipientID string   `json:"recipient_id"`
}

func (c *Client) NormalizeWebhook(ctx context.Context, headers map[string]string, rawBody []byte) ([]channel.InboundEvent, error) {
	if err := c.VerifyWebhook(ctx, headers, rawBody); err != nil {
		return nil, err
	}
	var envelope socialWebhookEnvelope
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON webhook", ErrInvalidRequest)
	}
	if envelope.Event == "webhook.test" {
		return nil, nil
	}
	providerEventID := strings.TrimSpace(envelope.Data.ID)
	deliveryID := strings.TrimSpace(headerValue(headers, "X-SocialAPI-Delivery"))
	dedupeStrategy := "provider_event_id"
	if providerEventID == "" {
		providerEventID = deliveryID
		dedupeStrategy = "provider_specific_fallback"
	}
	if providerEventID == "" || envelope.Event == "" {
		return nil, fmt.Errorf("%w: event id and event type are required", ErrInvalidRequest)
	}
	receivedAt := time.Now().UTC()
	for _, candidate := range []string{envelope.Data.ReceivedAt, envelope.Data.CreatedAt} {
		if candidate == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, candidate)
		if err == nil {
			receivedAt = parsed.UTC()
			break
		}
	}
	providerMessageID := envelope.Data.PlatformID
	if providerMessageID == "" && len(envelope.Data.MIDs) > 0 {
		providerMessageID = envelope.Data.MIDs[0]
	}
	if providerMessageID == "" && envelope.Data.Type == "dm" {
		providerMessageID = envelope.Data.ID
	}
	interaction := interactionKind(envelope.Event)
	providerChannel, err := platformChannel(envelope.Data.Platform)
	if err != nil {
		return nil, err
	}
	providerConversationID := envelope.Data.ConversationID
	event := channel.InboundEvent{ID: uuid.NewString(), Provider: channel.ProviderSocialAPI, Channel: providerChannel, ProviderConnectionID: envelope.Data.AccountID, ProviderEventID: providerEventID, DeliveryID: deliveryID, DedupeStrategy: dedupeStrategy, EventType: envelope.Event, InteractionKind: interaction, ProviderMessageID: providerMessageID, ProviderConversationID: providerConversationID, ExternalUserID: envelope.Data.Author.ID, Text: envelope.Data.Content.Text, ReceivedAt: receivedAt}
	if envelope.Data.ReceivedAt != "" || envelope.Data.CreatedAt != "" {
		timestamp := receivedAt
		event.ExternalCreatedAt = &timestamp
	}
	if event.ExternalUserID == "" {
		event.ExternalUserID = envelope.Data.RecipientID
	}
	if event.ProviderMessageID == "" && len(envelope.Data.MIDs) > 0 {
		event.ProviderMessageID = envelope.Data.MIDs[0]
	}
	if event.InteractionKind == channel.InteractionOther && event.ProviderMessageID == "" && envelope.Event != "account.connected" && envelope.Event != "account.disconnected" && envelope.Event != "page.removed" {
		return nil, fmt.Errorf("%w: unsupported webhook payload for %s", ErrInvalidRequest, envelope.Event)
	}
	return []channel.InboundEvent{event}, nil
}

func interactionKind(event string) channel.InteractionKind {
	switch event {
	case "dm.received", "dm.referral", "dm.postback":
		return channel.InteractionDM
	case "comment.received":
		return channel.InteractionComment
	case "mention.received":
		return channel.InteractionMention
	case "review.received":
		return channel.InteractionOther
	default:
		return channel.InteractionOther
	}
}

func platformChannel(platform string) (channel.Channel, error) {
	switch strings.ToLower(platform) {
	case "facebook":
		return channel.ChannelFacebook, nil
	case "instagram":
		return channel.ChannelInstagram, nil
	case "whatsapp":
		return channel.ChannelWhatsApp, nil
	case "":
		return "", nil
	default:
		return "", fmt.Errorf("%w: unsupported platform %q", ErrInvalidRequest, platform)
	}
}

func (c *Client) SendMessage(ctx context.Context, command ports.SendMessageCommand) (ports.ProviderSendResult, error) {
	if command.ProviderAccountID == "" || command.ProviderConversationID == "" || command.Text == "" || command.IdempotencyKey == "" {
		return ports.ProviderSendResult{}, fmt.Errorf("%w: account, conversation, text, and idempotency key are required", ErrInvalidRequest)
	}
	request := struct {
		AccountID string `json:"account_id"`
		Text      string `json:"text"`
	}{AccountID: command.ProviderAccountID, Text: command.Text}
	var response struct {
		MessageID  string   `json:"message_id"`
		MessageIDs []string `json:"message_ids"`
		Success    bool     `json:"success"`
	}
	requestID, err := c.doJSON(ctx, http.MethodPost, "/v1/inbox/conversations/"+url.PathEscape(command.ProviderConversationID)+"/messages", request, &response)
	if err != nil {
		return ports.ProviderSendResult{}, err
	}
	if !response.Success || (response.MessageID == "" && len(response.MessageIDs) == 0) {
		return ports.ProviderSendResult{}, fmt.Errorf("%w: SocialAPI returned unsuccessful send", ErrInvalidResponse)
	}
	messageID := response.MessageID
	if messageID == "" {
		messageID = response.MessageIDs[0]
	}
	return ports.ProviderSendResult{ProviderRequestID: requestID, ProviderMessageID: messageID, Status: channel.DeliveryAccepted}, nil
}

func (c *Client) GetDeliveryStatus(ctx context.Context, reference ports.DeliveryReference) (channel.DeliveryStatus, error) {
	if reference.ProviderConversationID == "" || reference.ProviderMessageID == "" {
		return channel.DeliveryUnknown, fmt.Errorf("%w: provider conversation and message references are required", ErrInvalidRequest)
	}
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			PlatformID string `json:"platform_id"`
			Status     string `json:"status"`
		} `json:"data"`
	}

	_, err := c.doJSON(ctx, http.MethodGet, "/v1/inbox/conversations/"+url.PathEscape(reference.ProviderConversationID)+"/messages", nil, &response)
	if err != nil {
		return channel.DeliveryUnknown, err
	}
	for _, item := range response.Data {
		if item.ID == reference.ProviderMessageID || item.PlatformID == reference.ProviderMessageID {
			return deliveryStatus(item.Status), nil
		}
	}

	return channel.DeliveryUnknown, nil
}

func deliveryStatus(value string) channel.DeliveryStatus {
	switch strings.ToLower(value) {
	case "sent":
		return channel.DeliverySent
	case "delivered":
		return channel.DeliveryDelivered
	case "read":
		return channel.DeliveryRead
	case "failed":
		return channel.DeliveryFailed
	default:
		return channel.DeliveryUnknown
	}
}

var (
	ErrInvalidRequest  = errors.New("socialapi invalid request")
	ErrInvalidResponse = errors.New("socialapi invalid response")
)

func (c *Client) doJSON(ctx context.Context, method, path string, request any, response any) (string, error) {
	return c.doJSONWithHeaders(ctx, method, path, request, response, nil)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, method, path string, request any, response any, extra http.Header) (string, error) {
	if c == nil || c.httpClient == nil || strings.TrimSpace(c.apiKey) == "" {
		return "", ErrNotConfigured
	}
	var body io.Reader
	if request != nil {
		encoded, err := json.Marshal(request)
		if err != nil {
			return "", fmt.Errorf("%w: encode request", ErrInvalidRequest)
		}
		body = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return "", fmt.Errorf("%w: create request", ErrInvalidRequest)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range extra {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer res.Body.Close()
	requestID := res.Header.Get("X-Request-ID")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return requestID, parseAPIError(res)
	}
	if response == nil {
		return requestID, nil
	}
	limited := io.LimitReader(res.Body, 2<<20)
	if err := json.NewDecoder(limited).Decode(response); err != nil {
		return requestID, fmt.Errorf("%w: decode response", ErrInvalidResponse)
	}
	return requestID, nil
}

type apiErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

func parseAPIError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var decoded apiErrorResponse
	_ = json.Unmarshal(body, &decoded)
	if decoded.RequestID == "" {
		decoded.RequestID = res.Header.Get("X-Request-ID")
	}
	return &Error{StatusCode: res.StatusCode, Code: decoded.Error.Code, Message: decoded.Error.Message, RequestID: decoded.RequestID, Retryable: retryableStatus(res.StatusCode)}
}

type Error struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
}

func (e *Error) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("socialapi http %d: %s: %s", e.StatusCode, e.Code, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("socialapi http %d: %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("socialapi http %d", e.StatusCode)
}

func retryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway:
		return true
	default:
		return false
	}
}

var ErrTransport = errors.New("socialapi transport error")

var _ ports.ChannelProvider = (*Client)(nil)
