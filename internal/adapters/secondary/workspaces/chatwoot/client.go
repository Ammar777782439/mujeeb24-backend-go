package chatwoot

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
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

var (
	ErrNotConfigured   = errors.New("chatwoot client is not configured")
	ErrInvalidWebhook  = errors.New("chatwoot webhook signature is invalid")
	ErrReplay          = errors.New("chatwoot webhook timestamp is outside tolerance")
	ErrInvalidRequest  = errors.New("chatwoot invalid request")
	ErrInvalidResponse = errors.New("chatwoot invalid response")
	ErrTransport       = errors.New("chatwoot transport error")
)

type Config struct {
	BaseURL       string
	APIToken      string
	HTTPClient    *http.Client
	HTTPTimeout   time.Duration
	WebhookSecret string
	ReplayWindow  time.Duration
}

type Client struct {
	baseURL       string
	apiToken      string
	httpClient    *http.Client
	webhookSecret string
	tolerance     time.Duration
}

func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
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
	return &Client{baseURL: baseURL, apiToken: strings.TrimSpace(cfg.APIToken), httpClient: client, webhookSecret: strings.TrimSpace(cfg.WebhookSecret), tolerance: tolerance}
}

func (c *Client) VerifyWebhook(ctx context.Context, headers map[string]string, rawBody []byte) error {
	if ctx == nil || c == nil || c.webhookSecret == "" {
		return ErrInvalidWebhook
	}
	timestamp := strings.TrimSpace(headerValue(headers, "X-Chatwoot-Timestamp"))
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
	provided := strings.TrimSpace(headerValue(headers, "X-Chatwoot-Signature"))
	if !strings.HasPrefix(provided, "sha256=") {
		return ErrInvalidWebhook
	}
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	_, _ = mac.Write([]byte(timestamp + "." + string(rawBody)))
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(provided)) {
		return ErrInvalidWebhook
	}
	return nil
}

func (c *Client) NormalizeWebhook(ctx context.Context, headers map[string]string, rawBody []byte) ([]channel.InboundEvent, error) {
	if err := c.VerifyWebhook(ctx, headers, rawBody); err != nil {
		return nil, err
	}
	var payload chatwootWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON webhook", ErrInvalidRequest)
	}
	payload.normalizeReferences()
	if payload.Event == "" || string(payload.ID) == "" {
		return nil, fmt.Errorf("%w: event and message id are required", ErrInvalidRequest)
	}
	receivedAt := time.Now().UTC()
	if payload.CreatedAt > 0 {
		receivedAt = time.Unix(payload.CreatedAt, 0).UTC()
	}
	messageID := string(payload.ID)
	return []channel.InboundEvent{{ID: messageID, Provider: channel.ProviderChatwoot, ProviderConnectionID: string(payload.AccountID), ProviderEventID: messageID, EventType: payload.Event, InteractionKind: channel.InteractionDM, ProviderMessageID: messageID, ProviderConversationID: string(payload.ConversationID), ExternalUserID: string(payload.SenderID), Text: payload.Content, ReceivedAt: receivedAt, RawPayloadReference: "chatwoot://webhook/" + messageID, ExternalCreatedAt: &receivedAt}}, nil
}

type chatwootWebhookPayload struct {
	Event          string         `json:"event"`
	ID             flexibleString `json:"id"`
	Content        string         `json:"content"`
	CreatedAt      int64          `json:"created_at"`
	AccountID      flexibleString `json:"account_id"`
	ConversationID flexibleString `json:"conversation_id"`
	SenderID       flexibleString `json:"sender_id"`
	Conversation   struct {
		ID flexibleString `json:"id"`
	} `json:"conversation"`
	Sender struct {
		ID flexibleString `json:"id"`
	} `json:"sender"`
	Account struct {
		ID flexibleString `json:"id"`
	} `json:"account"`
}

type flexibleString string

func (s *flexibleString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err == nil {
		*s = flexibleString(value)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*s = flexibleString(number.String())
	return nil
}

func (p *chatwootWebhookPayload) normalizeReferences() {
	if string(p.ConversationID) == "" {
		p.ConversationID = p.Conversation.ID
	}
	if string(p.SenderID) == "" {
		p.SenderID = p.Sender.ID
	}
	if string(p.AccountID) == "" {
		p.AccountID = p.Account.ID
	}
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func (c *Client) CreateContact(ctx context.Context, draft ports.WorkspaceContactDraft) (ports.WorkspaceContact, error) {
	if draft.AccountID <= 0 || draft.InboxID <= 0 || strings.TrimSpace(draft.Name) == "" || strings.TrimSpace(draft.Identifier) == "" {
		return ports.WorkspaceContact{}, fmt.Errorf("%w: account, inbox, name, and identifier are required", ErrInvalidRequest)
	}
	attributes, err := decodeObject(draft.AdditionalAttrs)
	if err != nil {
		return ports.WorkspaceContact{}, err
	}
	request := struct {
		InboxID         int64          `json:"inbox_id"`
		Name            string         `json:"name"`
		Identifier      string         `json:"identifier"`
		PhoneNumber     string         `json:"phone_number,omitempty"`
		AvatarURL       string         `json:"avatar_url,omitempty"`
		AdditionalAttrs map[string]any `json:"additional_attributes,omitempty"`
	}{InboxID: draft.InboxID, Name: draft.Name, Identifier: draft.Identifier, PhoneNumber: draft.PhoneNumber, AvatarURL: draft.AvatarURL, AdditionalAttrs: attributes}
	var response contactResponse
	_, err = c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(draft.AccountID, 10)+"/contacts", request, &response)
	if err != nil {
		return ports.WorkspaceContact{}, err
	}
	if response.ID == 0 {
		response.ID = response.contactID()
	}
	if response.ID == 0 {
		return ports.WorkspaceContact{}, fmt.Errorf("%w: contact id missing", ErrInvalidResponse)
	}
	return ports.WorkspaceContact{ID: response.ID, AccountID: draft.AccountID, InboxID: draft.InboxID, Identifier: draft.Identifier}, nil
}

type contactResponse struct {
	ID      int64           `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

func (r contactResponse) contactID() int64 {
	var object struct {
		Contact struct {
			ID int64 `json:"id"`
		} `json:"contact"`
	}
	if len(r.Payload) > 0 && json.Unmarshal(r.Payload, &object) == nil && object.Contact.ID > 0 {
		return object.Contact.ID
	}
	var list []struct {
		ID int64 `json:"id"`
	}
	if len(r.Payload) > 0 && json.Unmarshal(r.Payload, &list) == nil && len(list) > 0 {
		return list[0].ID
	}
	return 0
}

func (c *Client) CreateConversation(ctx context.Context, draft ports.WorkspaceConversationDraft) (ports.WorkspaceConversation, error) {
	if draft.AccountID <= 0 || draft.InboxID <= 0 || draft.ContactID <= 0 || strings.TrimSpace(draft.SourceID) == "" {
		return ports.WorkspaceConversation{}, fmt.Errorf("%w: account, inbox, contact, and source are required", ErrInvalidRequest)
	}
	additional, err := decodeObject(draft.AdditionalAttrs)
	if err != nil {
		return ports.WorkspaceConversation{}, err
	}
	custom, err := decodeObject(draft.CustomAttrs)
	if err != nil {
		return ports.WorkspaceConversation{}, err
	}
	request := struct {
		SourceID        string          `json:"source_id"`
		InboxID         int64           `json:"inbox_id"`
		ContactID       int64           `json:"contact_id"`
		AdditionalAttrs map[string]any  `json:"additional_attributes,omitempty"`
		CustomAttrs     map[string]any  `json:"custom_attributes,omitempty"`
		Status          string          `json:"status,omitempty"`
		Message         *initialMessage `json:"message,omitempty"`
	}{SourceID: draft.SourceID, InboxID: draft.InboxID, ContactID: draft.ContactID, AdditionalAttrs: additional, CustomAttrs: custom, Status: draft.Status}
	if strings.TrimSpace(draft.InitialText) != "" {
		request.Message = &initialMessage{Content: draft.InitialText}
	}
	var response conversationResponse
	_, err = c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(draft.AccountID, 10)+"/conversations", request, &response)
	if err != nil {
		return ports.WorkspaceConversation{}, err
	}
	if response.ID == 0 {
		return ports.WorkspaceConversation{}, fmt.Errorf("%w: conversation id missing", ErrInvalidResponse)
	}
	return ports.WorkspaceConversation{ID: response.ID, AccountID: draft.AccountID, InboxID: response.InboxID}, nil
}

type initialMessage struct {
	Content string `json:"content"`
}

type conversationResponse struct {
	ID        int64 `json:"id"`
	AccountID int64 `json:"account_id"`
	InboxID   int64 `json:"inbox_id"`
}

func (c *Client) CreateMessage(ctx context.Context, draft ports.WorkspaceMessageDraft) (ports.WorkspaceMessage, error) {
	if draft.AccountID <= 0 || draft.ConversationID <= 0 || strings.TrimSpace(draft.Text) == "" {
		return ports.WorkspaceMessage{}, fmt.Errorf("%w: account, conversation, and text are required", ErrInvalidRequest)
	}
	messageType := draft.MessageType
	if messageType == "" {
		messageType = "outgoing"
	}
	request := struct {
		Content     string `json:"content"`
		MessageType string `json:"message_type"`
		Private     bool   `json:"private"`
		ContentType string `json:"content_type"`
	}{Content: draft.Text, MessageType: messageType, Private: draft.Private, ContentType: "text"}
	var response messageResponse
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/accounts/"+strconv.FormatInt(draft.AccountID, 10)+"/conversations/"+strconv.FormatInt(draft.ConversationID, 10)+"/messages", request, &response)
	if err != nil {
		return ports.WorkspaceMessage{}, err
	}
	if response.ID == 0 {
		return ports.WorkspaceMessage{}, fmt.Errorf("%w: message id missing", ErrInvalidResponse)
	}
	createdAt := time.Time{}
	if response.CreatedAt > 0 {
		createdAt = time.Unix(response.CreatedAt, 0).UTC()
	}
	return ports.WorkspaceMessage{ID: response.ID, AccountID: draft.AccountID, ConversationID: draft.ConversationID, Text: response.Content, MessageType: normalizeMessageType(response.MessageType), Status: response.Status, CreatedAt: createdAt, Private: response.Private}, nil
}

type messageResponse struct {
	ID          int64          `json:"id"`
	Content     string         `json:"content"`
	MessageType flexibleString `json:"message_type"`
	CreatedAt   int64          `json:"created_at"`
	Private     bool           `json:"private"`
	Status      string         `json:"status"`
}

func normalizeMessageType(value flexibleString) string {
	switch string(value) {
	case "0":
		return "incoming"
	case "1":
		return "outgoing"
	case "2":
		return "activity"
	case "3":
		return "template"
	default:
		return string(value)
	}
}

func decodeObject(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%w: attributes must be a JSON object", ErrInvalidRequest)
	}
	return object, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, request any, response any) (string, error) {
	if c == nil || c.httpClient == nil || c.apiToken == "" {
		return "", ErrNotConfigured
	}
	encoded, err := json.Marshal(request)
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
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer res.Body.Close()
	requestID := res.Header.Get("X-Request-ID")
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return requestID, parseError(res)
	}
	if response == nil {
		return requestID, nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(response); err != nil {
		return requestID, fmt.Errorf("%w: decode response", ErrInvalidResponse)
	}
	return requestID, nil
}

type Error struct {
	StatusCode int
	Code       string
	Retryable  bool
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("chatwoot http %d: %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("chatwoot http %d", e.StatusCode)
}

func parseError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var decoded struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &decoded)
	code := decoded.Error
	if code == "" {
		code = decoded.Message
	}
	return &Error{StatusCode: res.StatusCode, Code: code, Retryable: res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500}
}

var _ ports.CommunicationWorkspace = (*Client)(nil)
