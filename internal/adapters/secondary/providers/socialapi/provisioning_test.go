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
		sawConnect = request.Header.Get("Authorization") == "Bearer sapi_key_test" && strings.Contains(string(body), `"platform":"instagram"`) && strings.Contains(string(body), `"state":"session-1"`) && strings.Contains(string(body), `"brand_id":"brand-1"`)
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"auth_url":"https://social.example/authorize","state":"provider-state"}`))
	}))
	defer server.Close()
	adapter := NewProvisioningAdapter(NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()}))
	started, err := adapter.BeginAuthorization(context.Background(), ports.SocialAuthorizationRequest{ProviderRef: "socialapi", Channel: "instagram", RedirectURI: "https://app.example/oauth/callback", State: "session-1", BrandID: "brand-1"})
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


func TestProvisioningAdapterManagesProviderBrands(t *testing.T) {
	var createBody string
	var deletedID string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/brands":
			_, _ = writer.Write([]byte(`{"data":[{"id":"brand-1","name":"Mujeeb24 — acme","accounts_count":0}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/brands":
			body, _ := io.ReadAll(request.Body)
			createBody = string(body)
			_, _ = writer.Write([]byte(`{"id":"brand-2","name":"Mujeeb24 — beta","accounts_count":0}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/brands/brand-2":
			deletedID = "brand-2"
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	adapter := NewProvisioningAdapter(NewClient(Config{BaseURL: server.URL, APIKey: "sapi_key_test", HTTPClient: server.Client()}))

	brands, err := adapter.ListBrands(context.Background())
	if err != nil || len(brands) != 1 || brands[0].ProviderBrandRef != "brand-1" || brands[0].DisplayName != "Mujeeb24 — acme" {
		t.Fatalf("list brands=%#v err=%v", brands, err)
	}

	created, err := adapter.CreateBrand(context.Background(), "Mujeeb24 — beta")
	if err != nil || created.ProviderBrandRef != "brand-2" || !strings.Contains(createBody, `"name":"Mujeeb24 — beta"`) {
		t.Fatalf("created brand=%#v err=%v body=%s", created, err, createBody)
	}

	if err := adapter.DeleteBrand(context.Background(), "brand-2"); err != nil || deletedID != "brand-2" {
		t.Fatalf("delete brand err=%v deleted=%q", err, deletedID)
	}
}
