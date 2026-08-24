package bootstrap

import (
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

func TestBuildExternalAdaptersLeavesOptionalIntegrationsDisabled(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{SocialAPIBaseURL: "https://example.invalid", ChatwootBaseURL: "http://localhost:3000"})
	if adapters.SocialAPI != nil || adapters.Chatwoot != nil || adapters.SocialWebhook != nil || adapters.ChatwootWebhook != nil {
		t.Fatalf("expected no external adapters for empty credentials: %#v", adapters)
	}
}

func TestBuildExternalAdaptersConstructsConfiguredClientsWithoutCallingNetwork(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{
		SocialAPIBaseURL:       "https://example.invalid",
		SocialAPIAPIKey:        "test-only-placeholder",
		SocialAPIWebhookSecret: "test-webhook-secret",
		SocialAPIHTTPTimeout:   2 * time.Second,
		ChatwootBaseURL:        "http://example.invalid",
		ChatwootAPIToken:       "test-only-placeholder",
		ChatwootWebhookSecret:  "test-webhook-secret",
		ChatwootHTTPTimeout:    2 * time.Second,
	})
	if adapters.SocialAPI == nil || adapters.SocialWebhook == nil || adapters.Chatwoot == nil || adapters.ChatwootWebhook == nil {
		t.Fatalf("expected configured adapters: %#v", adapters)
	}
}
