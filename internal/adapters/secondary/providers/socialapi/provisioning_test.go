package socialapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestProvisioningAdapterBeginsAndResolvesSuccess(t *testing.T) {
	var sawConnect bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/accounts/connect" || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		body, _ := io.ReadAll(request.Body)
		sawConnect = request.Header.Get("Authorization") == "Bearer sapi_key_test" && strings.Contains(string(body), `"platform":"instagram"`) && strings.Contains(string(body), `"state":"session-1"`)
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"auth_url":"https://social.example/authorize","state":"provider-state"}`))
	}))
	defer server.Close()
	adapter := NewProvisioningAdapter(NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()}))
	started, err := adapter.BeginAuthorization(context.Background(), ports.SocialAuthorizationRequest{ProviderRef: "socialapi", Channel: "instagram", RedirectURI: "https://app.example/oauth/callback", State: "session-1"})
	if err != nil || started.AuthorizationURL == "" || started.State != "provider-state" || !sawConnect {
		t.Fatalf("started=%#v err=%v saw_connect=%v", started, err, sawConnect)
	}
	resolved, err := adapter.ResolveAuthorization(context.Background(), ports.SocialAuthorizationCallback{Status: "success", State: "session-1", AccountID: "acc-1"})
	if err != nil || resolved.ProviderAccountRef != "acc-1" || resolved.ProviderConnectionRef != "acc-1" {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
}

func TestProvisioningAdapterResolvesFacebookSelection(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/accounts/pending/pc-1/select" || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		data, _ := io.ReadAll(request.Body)
		body = string(data)
		_, _ = writer.Write([]byte(`{"account_id":"acc-facebook"}`))
	}))
	defer server.Close()
	adapter := NewProvisioningAdapter(NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()}))
	resolved, err := adapter.ResolveAuthorization(context.Background(), ports.SocialAuthorizationCallback{Status: "selection_required", State: "session-1", ConnectionID: "pc-1", PageIDs: []string{"page-1"}})
	if err != nil || resolved.ProviderAccountRef != "acc-facebook" || !strings.Contains(body, `"page_ids":["page-1"]`) {
		t.Fatalf("resolved=%#v err=%v body=%s", resolved, err, body)
	}
}

func TestProvisioningAdapterRejectsInvalidCallback(t *testing.T) {
	adapter := NewProvisioningAdapter(NewClient(Config{BaseURL: "https://example.test", APIKey: "sapi_key_test"}))
	if _, err := adapter.ResolveAuthorization(context.Background(), ports.SocialAuthorizationCallback{Status: "success"}); err == nil {
		t.Fatal("expected invalid callback error")
	}
}
