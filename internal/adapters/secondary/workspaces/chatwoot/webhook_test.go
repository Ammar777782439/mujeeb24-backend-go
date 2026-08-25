package chatwoot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

func TestClientNormalizesSignedChatwootWebhook(t *testing.T) {
	secret := "chatwoot-secret"
	body := []byte(`{"event":"message_created","id":901,"content":"hello","created_at":1787659200,"message_type":"incoming","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"contact"}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-Chatwoot-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-Chatwoot-Timestamp": timestamp,
		"X-Chatwoot-Delivery":  "delivery-901",
	}
	client := NewClient(Config{APIToken: "chatwoot-token", WebhookSecret: secret})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	events, err := client.NormalizeWebhook(ctx, headers, body)
	if err != nil {
		t.Fatalf("NormalizeWebhook: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	event := events[0]
	if event.Provider != channel.ProviderChatwoot || event.ProviderEventID != "901" || event.ProviderConnectionID != "12:34" || event.ProviderConversationID != "78" || event.ExternalUserID != "56" || event.EventType != "interaction_received" || event.ProviderMessageID != "901" || event.Text != "hello" || event.MessageType != "incoming" || event.Direction != channel.DirectionInbound || event.Origin != channel.OriginCustomer || event.Private {
		t.Fatalf("unexpected event: %#v", event)
	}
	if err := client.VerifyWebhook(ctx, headers, body); err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
}

func TestClientNormalizesSignedChatwootWebhookWithStringCreatedAt(t *testing.T) {
	secret := "chatwoot-secret"
	body := []byte(`{"event":"message_created","id":902,"content":"hello","created_at":"2026-08-25T01:00:00.000Z","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-Chatwoot-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-Chatwoot-Timestamp": timestamp,
	}
	client := NewClient(Config{WebhookSecret: secret})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	events, err := client.NormalizeWebhook(ctx, headers, body)
	if err != nil {
		t.Fatalf("NormalizeWebhook: %v", err)
	}
	if len(events) != 1 || events[0].ProviderEventID != "902" || events[0].ReceivedAt.Unix() != 1787619600 {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestChatwootMessageDirectionIsExplicitAndEchoSafe(t *testing.T) {
	cases := []struct {
		name      string
		message   string
		sender    string
		private   bool
		direction channel.MessageDirection
		origin    channel.MessageOrigin
	}{
		{name: "incoming customer", message: "incoming", sender: "contact", direction: channel.DirectionInbound, origin: channel.OriginCustomer},
		{name: "outgoing human", message: "outgoing", sender: "user", direction: channel.DirectionOutbound, origin: channel.OriginHuman},
		{name: "outgoing agent bot", message: "outgoing", sender: "agentbot", direction: channel.DirectionOutbound, origin: channel.OriginAutomation},
		{name: "private note", message: "incoming", sender: "user", private: true, direction: channel.DirectionOutbound, origin: channel.OriginSystem},
		{name: "unknown is unclassified", message: "", sender: "contact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direction, origin := chatwootMessageDirection(tc.message, tc.sender, tc.private)
			if direction != tc.direction || origin != tc.origin {
				t.Fatalf("direction=%q origin=%q, want direction=%q origin=%q", direction, origin, tc.direction, tc.origin)
			}
		})
	}
}

func TestClientRejectsChatwootReplay(t *testing.T) {
	client := NewClient(Config{APIToken: "chatwoot-token", WebhookSecret: "secret", ReplayWindow: time.Minute})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	timestamp := strconv.FormatInt(time.Now().Add(-2*time.Minute).Unix(), 10)
	body := []byte(`{"event":"message_created","id":901}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-Chatwoot-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-Chatwoot-Timestamp": timestamp,
	}
	if err := client.VerifyWebhook(ctx, headers, body); err != ErrReplay {
		t.Fatalf("VerifyWebhook=%v, want ErrReplay", err)
	}
}

var _ ports.WebhookReceiver = (*Client)(nil)
