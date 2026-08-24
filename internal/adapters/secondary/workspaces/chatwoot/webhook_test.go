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
	body := []byte(`{"event":"message_created","id":901,"content":"hello","created_at":1787659200,"account":{"id":12},"conversation":{"id":78},"sender":{"id":56}}`)
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
	if event.Provider != channel.ProviderChatwoot || event.ProviderEventID != "901" || event.ProviderConnectionID != "12" || event.ProviderConversationID != "78" || event.ExternalUserID != "56" || event.Text != "hello" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if err := client.VerifyWebhook(ctx, headers, body); err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
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
