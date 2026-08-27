package chatwoot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestPlatformClientCreatesAccountAndAPIInbox(t *testing.T) {
	var accountBody, accountUserBody, inboxBody map[string]any
	var accountAuth, accountUserAuth, inboxAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if request.URL.Path == "/platform/api/v1/accounts" {
			_ = json.Unmarshal(body, &accountBody)
			accountAuth = request.Header.Get("api_access_token") == "platform-token"
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":42,"name":"Acme"}`))
			return
		}
		if request.URL.Path == "/platform/api/v1/accounts/42/account_users" {
			_ = json.Unmarshal(body, &accountUserBody)
			accountUserAuth = request.Header.Get("api_access_token") == "platform-token"
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"account_id":42,"user_id":7,"role":"administrator"}`))
			return
		}
		if request.URL.Path == "/api/v1/accounts/42/inboxes" {
			_ = json.Unmarshal(body, &inboxBody)
			inboxAuth = request.Header.Get("api_access_token") == "application-token"
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":77,"name":"Acme"}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	client := NewPlatformClient(PlatformConfig{BaseURL: server.URL, PlatformToken: "platform-token", APIToken: "application-token", APIUserID: 7, HTTPClient: server.Client()})
	account, err := client.EnsureAccount(context.Background(), "Acme", "ar")
	if err != nil || account.ID != "42" || !accountAuth || !accountUserAuth {
		t.Fatalf("account=%#v err=%v accountAuth=%v accountUserAuth=%v", account, err, accountAuth, accountUserAuth)
	}
	inbox, err := client.EnsureInbox(context.Background(), account.ID, "Acme", "whatsapp", "https://app.example/webhooks/chatwoot")
	if err != nil || inbox.ID != "77" || !inboxAuth {
		t.Fatalf("inbox=%#v err=%v auth=%v", inbox, err, inboxAuth)
	}
	if accountBody["name"] != "Acme" || accountBody["locale"] != "ar" || accountUserBody["user_id"] != float64(7) || accountUserBody["role"] != "administrator" || inboxBody["name"] != "Acme" {
		t.Fatalf("unexpected bodies account=%#v accountUser=%#v inbox=%#v", accountBody, accountUserBody, inboxBody)
	}
	channel, ok := inboxBody["channel"].(map[string]any)
	if !ok || channel["type"] != "api" || channel["webhook_url"] != "https://app.example/webhooks/chatwoot" || channel["hmac_mandatory"] != true {
		t.Fatalf("unexpected channel body: %#v", channel)
	}
}

func TestPlatformClientRejectsNonHTTPSWebhookAndMissingConfig(t *testing.T) {
	client := NewPlatformClient(PlatformConfig{BaseURL: "http://chatwoot.local", PlatformToken: "token"})
	if _, err := client.EnsureInbox(context.Background(), "42", "Acme", "facebook", "http://app.example/webhook"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https validation error, got %v", err)
	}
	var provisioner ports.WorkspaceProvisioner = NewPlatformClient(PlatformConfig{})
	if _, err := provisioner.EnsureAccount(context.Background(), "Acme", "ar"); err != ErrPlatformNotConfigured {
		t.Fatalf("expected not configured, got %v", err)
	}
}

func TestPlatformClientRejectsMissingApplicationUserConfiguration(t *testing.T) {
	client := NewPlatformClient(PlatformConfig{BaseURL: "https://chatwoot.example", PlatformToken: "platform-token", APIToken: "application-token"})
	if _, err := client.EnsureAccount(context.Background(), "Acme", "ar"); err == nil {
		t.Fatal("EnsureAccount accepted missing application API user id")
	}
}
