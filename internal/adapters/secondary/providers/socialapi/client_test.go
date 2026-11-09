package socialapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	if len(events) != 1 || events[0].ProviderEventID != "sapi_dm_1" || events[0].ProviderMessageID != "sapi_dm_1" || events[0].EventType != "interaction_received" || events[0].Direction != channel.DirectionInbound || events[0].Origin != channel.OriginCustomer || events[0].InteractionKind != channel.InteractionDM || events[0].Text != "مرحبا" || events[0].ProviderConversationID != "conv_1" {
		t.Fatalf("unexpected normalized event: %#v", events)
	}
	if err := client.VerifyWebhook(ctx, headers, body); err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
}

func TestClientNormalizesInboundMediaWithoutPersistingTemporaryURLs(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"sapi_dm_media_1","type":"dm","platform":"whatsapp","account_id":"acc_1","conversation_id":"conv_1","platform_id":"platform-message-1","author":{"id":"customer-1"},"content":{"text":"هذه الصور","media":[{"url":"https://api.social-api.ai/v1/inbox/media/media-1","type":"image"},{"url":"https://api.social-api.ai/v1/inbox/media/media-2","type":"file"}]}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SocialAPI-Timestamp":    timestamp,
		"X-SocialAPI-Delivery":     "delivery-media-1",
	}

	events, err := NewClient(Config{WebhookSecret: "secret"}).NormalizeWebhook(
		httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body,
	)
	if err != nil {
		t.Fatalf("NormalizeWebhook: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one normalized event, got %d", len(events))
	}
	event := events[0]
	if event.ProviderMessageID != "platform-message-1" {
		t.Fatalf("expected the platform message id for later provider lookup, got %q", event.ProviderMessageID)
	}
	if len(event.Attachments) != 2 {
		t.Fatalf("expected two normalized attachments, got %#v", event.Attachments)
	}
	wantKinds := []channel.AttachmentKind{channel.AttachmentImage, channel.AttachmentFile}
	for index, attachment := range event.Attachments {
		if attachment.Kind != wantKinds[index] {
			t.Fatalf("attachment %d kind=%q want=%q", index, attachment.Kind, wantKinds[index])
		}
		if err := attachment.Validate(); err != nil {
			t.Fatalf("attachment %d invalid: %v", index, err)
		}
		if strings.Contains(attachment.ProviderMediaReference, "https://") ||
			strings.Contains(attachment.ProviderMediaReference, "api.social-api.ai") {
			t.Fatalf("temporary provider URL leaked into media reference: %q", attachment.ProviderMediaReference)
		}
	}
	if event.Attachments[0].ProviderMediaReference == event.Attachments[1].ProviderMediaReference {
		t.Fatal("attachments from the same message must retain distinct positions")
	}
}

func TestClientUsesStableSocialAPIInteractionIDWhenPlatformMessageIDIsAbsent(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"sapi_dm_media_2","type":"dm","platform":"instagram","account_id":"acc_1","conversation_id":"conv_1","author":{"id":"customer-1"},"content":{"text":"","media":[{"url":"https://cdn.example.test/image.jpg","type":"image"}]}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SocialAPI-Timestamp":    timestamp,
	}
	events, err := NewClient(Config{WebhookSecret: "secret"}).NormalizeWebhook(
		httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body,
	)
	if err != nil || len(events) != 1 || len(events[0].Attachments) != 1 {
		t.Fatalf("stable SocialAPI interaction id should support media lookup: events=%#v err=%v", events, err)
	}
	interactionID, index, err := decodeInboundMediaReference(events[0].Attachments[0].ProviderMediaReference)
	if err != nil || interactionID != "sapi_dm_media_2" || index != 0 {
		t.Fatalf("unexpected opaque media lookup reference: id=%q index=%d err=%v", interactionID, index, err)
	}
}

func TestClientRejectsInboundMediaWithoutStableSocialAPIInteractionID(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"","type":"dm","platform":"instagram","account_id":"acc_1","conversation_id":"conv_1","platform_id":"platform-message-2","author":{"id":"customer-1"},"content":{"media":[{"url":"https://cdn.example.test/image.jpg","type":"image"}]}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SocialAPI-Timestamp":    timestamp,
		"X-SocialAPI-Delivery":     "delivery-without-interaction-id",
	}
	_, err := NewClient(Config{WebhookSecret: "secret"}).NormalizeWebhook(
		httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body,
	)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected a missing stable SocialAPI interaction id to be rejected, got %v", err)
	}
}

func TestClientRejectsUnknownInboundMediaType(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"sapi_dm_media_3","type":"dm","platform":"instagram","account_id":"acc_1","conversation_id":"conv_1","platform_id":"platform-message-3","author":{"id":"customer-1"},"content":{"media":[{"url":"https://cdn.example.test/file.bin","type":"archive"}]}}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	headers := map[string]string{
		"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SocialAPI-Timestamp":    timestamp,
	}
	_, err := NewClient(Config{WebhookSecret: "secret"}).NormalizeWebhook(
		httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body,
	)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected unsupported media kind, got %v", err)
	}
}

func TestClientResolvesInboundMediaThroughConversationHistory(t *testing.T) {
	interactionID := "sapi_dm_stable-interaction"
	attachmentURL := "https://cdn.example.test/private-image.jpg"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/inbox/conversations/conversation-1/messages" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("limit") != "200" {
			t.Errorf("expected bounded 200-message page, got query %q", r.URL.RawQuery)
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("SocialAPI authorization header missing")
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"other-message","platform_id":"platform-1","attachment_type":"image","attachment_url":"https://cdn.example.test/other.jpg"},{"id":"sapi_dm_stable-interaction","platform_id":"platform-2","attachment_type":"image","attachment_url":"https://cdn.example.test/private-image.jpg"}],"pagination":{"has_more":false,"next_cursor":""}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	reference := encodeInboundMediaReference(interactionID, 0)
	source, err := client.ResolveInboundMediaSource(context.Background(), "conversation-1", reference, "image")
	if err != nil {
		t.Fatalf("ResolveInboundMediaSource: %v", err)
	}
	if source.URL != attachmentURL || source.Kind != "image" {
		t.Fatalf("unexpected media source: %#v", source)
	}
}

func TestClientResolvesSocialAPIProxyMediaWithoutReturningTemporaryURL(t *testing.T) {
	mediaID := "7f3c1e2a-9b4d-4e5f-8a6b-1c2d3e4f5a6b"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/inbox/conversations/conversation-1/messages" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"sapi_dm_stable-interaction","attachment_type":"image","attachment_url":"`+server.URL+`/v1/inbox/media/`+mediaID+`"}],"pagination":{"has_more":false,"next_cursor":""}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	source, err := client.ResolveInboundMediaSource(context.Background(), "conversation-1",
		encodeInboundMediaReference("sapi_dm_stable-interaction", 0), "image")
	if err != nil {
		t.Fatalf("ResolveInboundMediaSource: %v", err)
	}
	if source.ProviderMediaID != mediaID || source.URL != "" || source.Kind != "image" {
		t.Fatalf("SocialAPI proxy URL must become an opaque media id, got %#v", source)
	}
}

func TestClientDownloadsSocialAPIMediaWithBearerAndBoundedBody(t *testing.T) {
	const mediaID = "media-id-123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/inbox/media/"+mediaID {
			t.Errorf("unexpected media request: %s %s", r.Method, r.URL.String())
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing SocialAPI bearer token")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	body, mime, err := client.DownloadInboundMedia(context.Background(), mediaID, 32)
	if err != nil {
		t.Fatalf("DownloadInboundMedia: %v", err)
	}
	if string(body) != "image-bytes" || mime != "image/jpeg" {
		t.Fatalf("unexpected downloaded media: body=%q mime=%q", body, mime)
	}
	if _, _, err := client.DownloadInboundMedia(context.Background(), mediaID, 4); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected bounded response rejection, got %v", err)
	}
}

func TestClientDoesNotFollowSocialAPIMediaRedirects(t *testing.T) {
	var targetRequests int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests++
		if r.Header.Get("Authorization") != "" {
			t.Errorf("SocialAPI bearer token leaked to redirect target")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusFound)
	}))
	defer origin.Close()

	client := NewClient(Config{BaseURL: origin.URL, APIKey: "test-key", HTTPClient: origin.Client()})
	_, _, err := client.DownloadInboundMedia(context.Background(), "media-id", 1024)
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect to be surfaced, got %v", err)
	}
	if targetRequests != 0 {
		t.Fatalf("redirect target must not be requested, got %d requests", targetRequests)
	}
}

func TestClientTreatsSocialAPIMedia504AsRetryableAnd410AsTerminal(t *testing.T) {
	for _, tc := range []struct {
		status         int
		retryable      bool
		retryAfter     string
		wantRetryAfter time.Duration
	}{
		{status: http.StatusServiceUnavailable, retryable: true, retryAfter: "1", wantRetryAfter: time.Second},
		{status: http.StatusGatewayTimeout, retryable: true},
		{status: http.StatusGone, retryable: false},
	} {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"code":"media_not_ready","message":"not ready"}}`)
			}))
			defer server.Close()
			client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
			_, _, err := client.DownloadInboundMedia(context.Background(), "media-id", 1024)
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Retryable != tc.retryable || apiErr.RetryAfter != tc.wantRetryAfter {
				var actual time.Duration
				if apiErr != nil {
					actual = apiErr.RetryAfter
				}
				t.Fatalf("status %d retryable=%v retry_after=%s actual_retry_after=%s, err=%v", tc.status, tc.retryable, tc.wantRetryAfter, actual, err)
			}
		})
	}
}

func TestClientResolvesInboundMediaAcrossHistoryPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("limit") != "200" {
			t.Errorf("expected 200-message page, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		switch requests {
		case 1:
			if r.URL.Query().Get("cursor") != "" {
				t.Errorf("first page must not have a cursor")
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"recent-message"}],"pagination":{"has_more":true,"next_cursor":"page-2"}}`)
		case 2:
			if r.URL.Query().Get("cursor") != "page-2" {
				t.Errorf("expected second-page cursor, got %q", r.URL.Query().Get("cursor"))
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"sapi_dm_stable-interaction","attachment_type":"image","attachment_url":"https://cdn.example.test/older.jpg"}],"pagination":{"has_more":false,"next_cursor":""}}`)
		default:
			t.Errorf("unexpected history request %d", requests)
			http.Error(w, "too many requests", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	source, err := client.ResolveInboundMediaSource(context.Background(), "conversation-1",
		encodeInboundMediaReference("sapi_dm_stable-interaction", 0), "image")
	if err != nil {
		t.Fatalf("ResolveInboundMediaSource: %v", err)
	}
	if source.URL != "https://cdn.example.test/older.jpg" || requests != 2 {
		t.Fatalf("unexpected source=%#v requests=%d", source, requests)
	}
}

func TestClientRejectsRepeatedInboundMediaHistoryCursor(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"other-message"}],"pagination":{"has_more":true,"next_cursor":"same-cursor"}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	_, err := client.ResolveInboundMediaSource(context.Background(), "conversation-1",
		encodeInboundMediaReference("sapi_dm_stable-interaction", 0), "image")
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected repeated cursor response error, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected cursor repetition to stop after second page, requests=%d", requests)
	}
}

func TestClientRejectsUnsupportedInboundMediaPositionBeforeCallingSocialAPI(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	_, err := client.ResolveInboundMediaSource(
		context.Background(),
		"conversation-1",
		encodeInboundMediaReference("sapi_dm_stable-interaction", 1),
		"image",
	)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected unsupported media position, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("unsupported attachment position must not call SocialAPI; requests=%d", requests)
	}
}

func TestClientRejectsInboundMediaKindMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"sapi_dm_stable-interaction","platform_id":"platform-1","attachment_type":"video","attachment_url":"https://cdn.example.test/clip.mp4"}],"pagination":{"has_more":false,"next_cursor":""}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	_, err := client.ResolveInboundMediaSource(
		context.Background(),
		"conversation-1",
		encodeInboundMediaReference("sapi_dm_stable-interaction", 0),
		"image",
	)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected provider kind mismatch, got %v", err)
	}
}

func TestClientRejectsNonHTTPSInboundMediaSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"sapi_dm_stable-interaction","attachment_type":"image","attachment_url":"http://cdn.example.test/photo.jpg"}],"pagination":{"has_more":false,"next_cursor":""}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	_, err := client.ResolveInboundMediaSource(
		context.Background(),
		"conversation-1",
		encodeInboundMediaReference("sapi_dm_stable-interaction", 0),
		"image",
	)
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected non-HTTPS provider URL to be rejected, got %v", err)
	}
}

func TestClientRejectsInboundMediaWithoutHistoryMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"different-message","attachment_type":"image","attachment_url":"https://cdn.example.test/photo.jpg"}],"pagination":{"has_more":false,"next_cursor":""}}`)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	_, err := client.ResolveInboundMediaSource(
		context.Background(),
		"conversation-1",
		encodeInboundMediaReference("sapi_dm_not-present", 0),
		"image",
	)
	if !errors.Is(err, ErrInboundMediaNotFound) {
		t.Fatalf("expected stable message not found, got %v", err)
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
	// SECURITY audit C1: a real SocialAPI verification ping sends the
	// header `X-SocialAPI-Event: webhook.test` AND a body whose `event`
	// field also equals `webhook.test`. Both must match for the bypass
	// to be granted. The previous broken behavior accepted the header
	// alone — that allowed an attacker to forge any `dm.received`
	// payload by setting the header, bypassing all signature checks.
	verificationHeaders := map[string]string{"X-SocialAPI-Event": "webhook.test"}
	verificationBody := []byte(`{"event":"webhook.test","data":{"verification":"ping"}}`)
	if err := client.VerifyWebhook(ctx, verificationHeaders, verificationBody); err != nil {
		t.Fatalf("verification ping: %v", err)
	}
	// Negative test: header set to webhook.test but body is NOT webhook.test
	// → bypass must NOT be granted.
	forgedHeaders := map[string]string{"X-SocialAPI-Event": "webhook.test"}
	forgedBody := []byte(`{"event":"dm.received","data":{"id":"forged","platform":"facebook"}}`)
	if err := client.VerifyWebhook(ctx, forgedHeaders, forgedBody); err == nil {
		t.Fatalf("forged webhook bypass: header-only test bypass must be rejected (audit C1)")
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
	if err != nil || len(events) != 1 || events[0].EventType != "delivery_status_changed" || events[0].InteractionKind != channel.InteractionOther || events[0].ProviderMessageID != "message-1" || events[0].DeliveryStatus != "delivered" || events[0].Text != "" {
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
			body, _ := io.ReadAll(request.Body)
			sawConnectAuth = request.Header.Get("Authorization") == "Bearer sapi_key_test" && strings.Contains(string(body), `"brand_id":"brand-1"`)
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

func TestClientDisconnectsSocialAccount(t *testing.T) {
	var gotMethod, gotPath, gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotMethod = request.Method
		gotPath = request.URL.Path
		gotAuthorization = request.Header.Get("Authorization")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
	if err := client.DisconnectAccount(httptest.NewRequest(http.MethodDelete, "/", nil).Context(), "acc_123"); err != nil {
		t.Fatalf("DisconnectAccount: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/v1/accounts/acc_123" || gotAuthorization != "Bearer sapi_key_test" {
		t.Fatalf("unexpected disconnect request method=%q path=%q authorization=%q", gotMethod, gotPath, gotAuthorization)
	}
}

func TestClientDisconnectTreatsMissingProviderAccountAsAlreadyDisconnected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":{"code":"resource.not_found","message":"Account not found"}}`))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
	if err := client.DisconnectAccount(httptest.NewRequest(http.MethodDelete, "/", nil).Context(), "acc_gone"); err != nil {
		t.Fatalf("DisconnectAccount 404 should converge to disconnected state, got %v", err)
	}
}

func TestClientSendsMessageAndMapsDeliveryStatus(t *testing.T) {
	var sawAuth, sawProviderIdempotencyHeader bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/inbox/conversations/conv_1/messages" && request.Method == http.MethodPost {
			body, _ := io.ReadAll(request.Body)
			sawAuth = request.Header.Get("Authorization") == "Bearer sapi_key_test"
			sawProviderIdempotencyHeader = request.Header.Get("Idempotency-Key") != ""
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
	if result.ProviderRequestID != "request_1" || result.ProviderMessageID != "m_1" || result.Status != channel.DeliveryAccepted || !sawAuth || sawProviderIdempotencyHeader {
		t.Fatalf("unexpected send result=%#v auth=%v provider_idem_header=%v", result, sawAuth, sawProviderIdempotencyHeader)
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

func TestClientMapsOfficialHTTPErrorSemantics(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		code      string
		retryable bool
	}{
		{name: "auth", status: http.StatusUnauthorized, code: "auth.invalid_key", retryable: false},
		{name: "permission", status: http.StatusForbidden, code: "account.reconnection_required", retryable: false},
		{name: "not_found", status: http.StatusNotFound, code: "conversation.not_found", retryable: false},
		{name: "rate_limit", status: http.StatusTooManyRequests, code: "platform.facebook.rate_limit", retryable: true},
		{name: "unsupported", status: http.StatusNotImplemented, code: "resource.not_supported", retryable: false},
		{name: "upstream", status: http.StatusBadGateway, code: "platform.facebook.api_error", retryable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-ID", "header-request")
				writer.WriteHeader(tc.status)
				_, _ = writer.Write([]byte(`{"error":{"code":"` + tc.code + `","message":"official message"},"request_id":"body-request"}`))
			}))
			defer server.Close()
			client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
			_, err := client.ListConnectedAccounts(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "")
			providerErr, ok := err.(*Error)
			if !ok || providerErr.StatusCode != tc.status || providerErr.Code != tc.code || providerErr.Message != "official message" || providerErr.RequestID != "body-request" || providerErr.Retryable != tc.retryable {
				t.Fatalf("unexpected provider error: %#v", err)
			}
		})
	}
}

func TestClientReadsInboxContractAndRegistersWebhook(t *testing.T) {
	var sawConversationQuery, sawMessageQuery bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/inbox/conversations" && request.Method == http.MethodGet {
			query := request.URL.Query()
			sawConversationQuery = request.Header.Get("Authorization") == "Bearer sapi_key_test" && query.Get("account_id") == "acc_1" && query.Get("platform") == "facebook" && query.Get("status") == "active" && query.Get("limit") == "25" && query.Get("cursor") == "cursor_1"
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"data":[{"id":"sapi_dm_1","account_id":"acc_1","platform":"facebook","platform_id":"fb-thread-1","participant_id":"person-1","participant_name":"Test Person","last_message":"hello","status":"active","unread_count":1,"created_at":"2026-08-25T10:00:00Z","updated_at":"2026-08-25T10:01:00Z","last_message_at":"2026-08-25T10:01:00Z"}],"pagination":{"has_more":true,"next_cursor":"cursor_2"},"sync_state":"idle","last_synced_at":"2026-08-25T10:01:00Z"}`))
			return
		}
		if request.URL.Path == "/v1/inbox/conversations/sapi_dm_1/messages" && request.Method == http.MethodGet {
			query := request.URL.Query()
			sawMessageQuery = request.Header.Get("Authorization") == "Bearer sapi_key_test" && query.Get("limit") == "50" && query.Get("cursor") == "cursor_2"
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"data":[{"id":"sapi_msg_1","conversation_id":"sapi_dm_1","platform_id":"fb-msg-1","sender_id":"person-1","sender_name":"Test Person","text":"hello","direction":"inbound","status":"received","created_at":"2026-08-25T10:01:00Z"}],"pagination":{"has_more":false},"sync_state":"idle"}`))
			return
		}
		if request.URL.Path == "/v1/accounts/connect" && request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusCreated)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"account_id":"acc_fb_1","display_name":"Test Page","platform":"facebook","username":"test-page"}`))
			return
		}
		if request.URL.Path == "/v1/webhooks" && request.Method == http.MethodPost {
			writer.WriteHeader(http.StatusCreated)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":"wh_1","url":"https://example.test/webhook","events":["dm.received"],"secret":"one-time-secret"}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	conversations, err := client.ListInboxConversations(ctx, InboxConversationQuery{AccountID: "acc_1", Platform: "facebook", Status: "active", Limit: 25, Cursor: "cursor_1"})
	if err != nil || len(conversations.Data) != 1 || conversations.Data[0].ID != "sapi_dm_1" || !conversations.Pagination.HasMore || conversations.Pagination.NextCursor != "cursor_2" || !sawConversationQuery {
		t.Fatalf("conversations=%#v err=%v query=%v", conversations, err, sawConversationQuery)
	}
	messages, err := client.ListConversationMessages(ctx, "sapi_dm_1", InboxMessagesQuery{Limit: 50, Cursor: "cursor_2"})
	if err != nil || len(messages.Data) != 1 || messages.Data[0].PlatformID != "fb-msg-1" || messages.Data[0].Direction != "inbound" || !sawMessageQuery {
		t.Fatalf("messages=%#v err=%v query=%v", messages, err, sawMessageQuery)
	}
	connection, err := client.BeginConnection(ctx, ConnectRequest{Platform: "facebook"})
	if err != nil || connection.AccountID != "acc_fb_1" || connection.Platform != "facebook" || connection.Username != "test-page" {
		t.Fatalf("direct connection=%#v err=%v", connection, err)
	}
	webhook, err := client.RegisterWebhook(ctx, RegisterWebhookRequest{URL: "https://example.test/webhook", Events: []string{"dm.received"}})
	if err != nil || webhook.ID != "wh_1" || webhook.Secret != "one-time-secret" || len(webhook.Events) != 1 {
		t.Fatalf("webhook=%#v err=%v", webhook, err)
	}
}

func TestClientValidatesSocialAPIRequestInputs(t *testing.T) {
	client := NewClient(Config{APIKey: "sapi_key_test"})
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	if _, err := client.ListConversationMessages(ctx, "", InboxMessagesQuery{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty conversation id error=%v", err)
	}
	if _, err := client.RegisterWebhook(ctx, RegisterWebhookRequest{URL: "https://example.test/webhook"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty webhook events error=%v", err)
	}
	if _, err := client.BeginConnection(ctx, ConnectRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty platform error=%v", err)
	}
}

func TestClientVerifiesSocialAPIV1RawBodySignature(t *testing.T) {
	secret := "test-webhook-secret"
	body := []byte(`{"event":"comment.received","data":{"id":"sapi_cmt_1","type":"comment","platform":"facebook","account_id":"acc_1","content":{"text":"hello"}}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	headers := map[string]string{"X-SocialAPI-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil))}
	client := NewClient(Config{WebhookSecret: secret})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	if err := client.VerifyWebhook(ctx, headers, body); err != nil {
		t.Fatalf("VerifyWebhook v1: %v", err)
	}
	body[0] = '{'
	body[1] = 'x'
	if err := client.VerifyWebhook(ctx, headers, body); err == nil {
		t.Fatal("modified raw body unexpectedly verified")
	}
}

func TestClientManagesSocialAPIBrands(t *testing.T) {
	var createBody string
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/brands":
			if request.Header.Get("Authorization") != "Bearer sapi_key_test" {
				t.Fatal("missing brands list authorization")
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"data":[{"id":"brand-1","name":"Mujeeb24 — acme","accounts_count":0}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/brands":
			body, _ := io.ReadAll(request.Body)
			createBody = string(body)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":"brand-2","name":"Mujeeb24 — beta","accounts_count":0}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/brands/brand-2":
			deletedPath = request.URL.Path
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})

	brands, err := client.ListBrands(httptest.NewRequest(http.MethodGet, "/", nil).Context())
	if err != nil || len(brands) != 1 || brands[0].ID != "brand-1" || brands[0].Name != "Mujeeb24 — acme" {
		t.Fatalf("brands=%#v err=%v", brands, err)
	}

	created, err := client.CreateBrand(httptest.NewRequest(http.MethodPost, "/", nil).Context(), "Mujeeb24 — beta")
	if err != nil || created.ID != "brand-2" || !strings.Contains(createBody, `"name":"Mujeeb24 — beta"`) {
		t.Fatalf("created=%#v err=%v body=%s", created, err, createBody)
	}

	if err := client.DeleteBrand(httptest.NewRequest(http.MethodDelete, "/", nil).Context(), "brand-2"); err != nil || deletedPath != "/v1/brands/brand-2" {
		t.Fatalf("delete err=%v path=%q", err, deletedPath)
	}
}

func TestClientOnlyNormalizesButtonsFromPostbackEvents(t *testing.T) {
	for _, kind := range []string{"dm.received", "dm.postback"} {
		t.Run(kind, func(t *testing.T) {
			body := []byte(`{"event":"` + kind + `","data":{"id":"event-1","platform":"instagram","account_id":"account-1","conversation_id":"conversation-1","author":{"id":"user-1"},"content":{"text":"hello"},"metadata":{"payload":"m24.sales:request:accept","title":"نعم"}}}`)
			stamp := strconv.FormatInt(time.Now().Unix(), 10)
			mac := hmac.New(sha256.New, []byte("secret"))
			_, _ = mac.Write([]byte(stamp + "." + string(body)))
			headers := map[string]string{"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-SocialAPI-Timestamp": stamp}
			events, err := NewClient(Config{WebhookSecret: "secret"}).NormalizeWebhook(httptest.NewRequest(http.MethodPost, "/", nil).Context(), headers, body)
			if err != nil || len(events) != 1 {
				t.Fatalf("normalize: %+v %v", events, err)
			}
			if kind == "dm.postback" {
				if events[0].ActionPayload != "m24.sales:request:accept" || events[0].Text != "نعم" {
					t.Fatalf("postback lost: %+v", events[0])
				}
			} else if events[0].ActionPayload != "" {
				t.Fatal("ordinary message metadata became a button action")
			}
		})
	}
}

func TestClientSendsInteractiveButtonsOnSupportedChannels(t *testing.T) {
	for _, kind := range []channel.Channel{channel.ChannelInstagram, channel.ChannelFacebook, channel.ChannelWhatsApp} {
		t.Run(string(kind), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(data), `"type":"postback"`) || !strings.Contains(string(data), `"payload":"m24.sales:request:accept"`) {
					t.Errorf("interactive payload lost: %s", data)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"message_id":"message-1","success":true}`))
			}))
			defer server.Close()
			client := NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()})
			_, err := client.SendMessage(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.SendMessageCommand{ProviderAccountID: "account-1", ProviderConversationID: "conversation-1", Channel: string(kind), IdempotencyKey: "interactive-1", Text: "هل تؤكد؟", Buttons: []ports.InteractiveButton{{Title: "نعم", Payload: "m24.sales:request:accept"}, {Title: "لا", Payload: "m24.sales:request:decline"}}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
