package socialapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

func TestClientNormalizesAndVerifiesWebhookV2(t *testing.T) {
	secret := "test-webhook-secret"
	body := []byte(`{"event":"dm.received","data":{"id":"sapi_dm_1","type":"dm","platform":"instagram","account_id":"acc_1","conversation_id":"conv_1","author":{"id":"user_1"},"content":{"text":"مرحبا"},"received_at":"2026-08-25T10:00:00Z"}}`)
	timestamp := time.Now().Unix()
	timestampValue := strconv.FormatInt(timestamp, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestampValue + "." + string(body)))
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	client := NewClient(Config{WebhookSecret: secret, APIKey: "sapi_key_test"})
	headers := map[string]string{
		"X-SocialAPI-Signature-V2": signature,
		"X-SocialAPI-Timestamp":    timestampValue,
		"X-SocialAPI-Delivery":     "delivery_1",
	}
	events, err := client.NormalizeWebhook(nil, headers, body)
	if err == nil || events != nil {
		t.Fatalf("nil context unexpectedly accepted: events=%#v err=%v", events, err)
	}
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	events, err = client.NormalizeWebhook(ctx, headers, body)
	if err != nil {
		t.Fatalf("NormalizeWebhook: %v", err)
	}
	if len(events) != 1 || events[0].ProviderEventID != "sapi_dm_1" || events[0].InteractionKind != channel.InteractionDM || events[0].Text != "مرحبا" || events[0].ProviderConversationID != "conv_1" {
		t.Fatalf("unexpected normalized event: %#v", events)
	}
	if err := client.VerifyWebhook(ctx, headers, body); err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
}

func TestClientRejectsReplayAndAcceptsVerificationPing(t *testing.T) {
	client := NewClient(Config{WebhookSecret: "secret", APIKey: "sapi_key_test", ReplayWindow: time.Minute})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	oldTimestamp := time.Now().Add(-2 * time.Minute).Unix()
	oldTimestampValue := strconv.FormatInt(oldTimestamp, 10)
	body := []byte(`{"event":"dm.received","data":{"id":"id","platform":"facebook"}}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(oldTimestampValue + "." + string(body)))
	headers := map[string]string{
		"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SocialAPI-Timestamp":    oldTimestampValue,
	}
	if err := client.VerifyWebhook(ctx, headers, body); err != ErrReplay {
		t.Fatalf("VerifyWebhook old timestamp=%v, want ErrReplay", err)
	}
	verificationHeaders := map[string]string{"X-SocialAPI-Event": "webhook.test"}
	if err := client.VerifyWebhook(ctx, verificationHeaders, body); err != nil {
		t.Fatalf("verification ping: %v", err)
	}
}

func TestClientUsesDeliveryAsExplicitFallbackDedupeKey(t *testing.T) {
	secret := "secret"
	body := []byte(`{"event":"dm.received","data":{"type":"dm","platform":"whatsapp","account_id":"account-1","conversation_id":"conversation-1","content":{"text":"hello"}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-SocialAPI-Timestamp": timestamp, "X-SocialAPI-Delivery": "delivery-fallback"}
	events, err := NewClient(Config{WebhookSecret: secret}).NormalizeWebhook(httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body)
	if err != nil || len(events) != 1 {
		t.Fatalf("NormalizeWebhook events=%#v err=%v", events, err)
	}
	if events[0].ProviderEventID != "delivery-fallback" || events[0].DeliveryID != "delivery-fallback" || events[0].DedupeStrategy != "provider_specific_fallback" || events[0].Channel != channel.ChannelWhatsApp {
		t.Fatalf("unexpected fallback identity: %#v", events[0])
	}
}

func TestClientNormalizesStatusEventWithoutTreatingItAsInboundText(t *testing.T) {
	secret := "secret"
	body := []byte(`{"event":"dm.status.delivered","data":{"id":"status-1","type":"dm_status","platform":"facebook","account_id":"account-1","conversation_id":"conversation-1","mids":["message-1"],"status":"delivered"}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-SocialAPI-Timestamp": timestamp}
	events, err := NewClient(Config{WebhookSecret: secret}).NormalizeWebhook(httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body)
	if err != nil || len(events) != 1 || events[0].InteractionKind != channel.InteractionOther || events[0].ProviderMessageID != "message-1" || events[0].Text != "" {
		t.Fatalf("unexpected status event=%#v err=%v", events, err)
	}
}

func TestClientRejectsUnknownPlatform(t *testing.T) {
	secret := "secret"
	body := []byte(`{"event":"dm.received","data":{"id":"event-1","type":"dm","platform":"telegram","account_id":"account-1","conversation_id":"conversation-1"}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-SocialAPI-Timestamp": timestamp}
	if _, err := NewClient(Config{WebhookSecret: secret}).NormalizeWebhook(httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body); err == nil || !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("unexpected unknown platform error: %v", err)
	}
}

func TestClientListsAccountsAndBeginsConnection(t *testing.T) {
	var sawListAuth, sawConnectAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/accounts" && request.Method == http.MethodGet {
			sawListAuth = request.Header.Get("Authorization") == "Bearer sapi_key_test"
			_, _ = writer.Write([]byte(`{"count":1,"data":[{"id":"account-1","brand_id":"brand-1","platform":"facebook","status":"active"}]}`))
			return
		}
		if request.URL.Path == "/v1/accounts/connect" && request.Method == http.MethodPost {
			sawConnectAuth = request.Header.Get("Authorization") == "Bearer sapi_key_test"
			_, _ = writer.Write([]byte(`{"auth_url":"https://example.test/oauth","state":"state-1","message":"pending"}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
	accounts, err := client.ListConnectedAccounts(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "brand-1")
	if err != nil || len(accounts) != 1 || accounts[0].ID != "account-1" || !sawListAuth {
		t.Fatalf("accounts=%#v err=%v auth=%v", accounts, err, sawListAuth)
	}
	connection, err := client.BeginConnection(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ConnectRequest{Platform: "facebook", RedirectURI: "https://example.test/callback", State: "state-1", BrandID: "brand-1"})
	if err != nil || connection.AuthURL == "" || connection.State != "state-1" || !sawConnectAuth {
		t.Fatalf("connection=%#v err=%v auth=%v", connection, err, sawConnectAuth)
	}
}

func TestClientSendsMessageAndMapsDeliveryStatus(t *testing.T) {
	var sawAuth, sawIdempotency bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/inbox/conversations/conv_1/messages" && request.Method == http.MethodPost {
			body, _ := io.ReadAll(request.Body)
			sawAuth = request.Header.Get("Authorization") == "Bearer sapi_key_test"
			sawIdempotency = request.Header.Get("Idempotency-Key") == "outbound_1"
			if !strings.Contains(string(body), `"account_id":"acc_1"`) || !strings.Contains(string(body), `"text":"hello"`) {
				t.Errorf("unexpected send body: %s", body)
			}
			writer.Header().Set("X-Request-ID", "request_1")
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"message_id":"m_1","message_ids":["m_1"],"success":true}`))
			return
		}
		if request.URL.Path == "/v1/inbox/conversations/conv_1/messages" && request.Method == http.MethodGet {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"data":[{"id":"m_1","status":"delivered"}]}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
	result, err := client.SendMessage(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.SendMessageCommand{ProviderAccountID: "acc_1", ProviderConversationID: "conv_1", Text: "hello", IdempotencyKey: "outbound_1"})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if result.ProviderRequestID != "request_1" || result.ProviderMessageID != "m_1" || result.Status != channel.DeliveryAccepted || !sawAuth || !sawIdempotency {
		t.Fatalf("unexpected send result=%#v auth=%v idem=%v", result, sawAuth, sawIdempotency)
	}
	status, err := client.GetDeliveryStatus(httptest.NewRequest(http.MethodGet, "/", nil).Context(), ports.DeliveryReference{ProviderConversationID: "conv_1", ProviderMessageID: "m_1"})
	if err != nil || status != channel.DeliveryDelivered {
		t.Fatalf("GetDeliveryStatus status=%v err=%v", status, err)
	}
}

func TestClientClassifiesRetryableAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"code":"rate_limited"},"request_id":"request_429"}`))
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
	_, err := client.ListConnectedAccounts(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "")
	providerErr, ok := err.(*Error)
	if !ok || !providerErr.Retryable || providerErr.StatusCode != http.StatusTooManyRequests || providerErr.RequestID != "request_429" {
		t.Fatalf("unexpected provider error: %#v", err)
	}
}
