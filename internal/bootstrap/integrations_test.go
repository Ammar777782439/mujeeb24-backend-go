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

func TestBuildExternalAdaptersKeepsChatwootAutoReplyDisabledByDefault(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{ChatwootWebhookSecret: "placeholder"})
	if adapters.ChatwootAutoReplyEnabled {
		t.Fatal("Chatwoot AutoReply must be disabled by default")
	}

	adapters = BuildExternalAdapters(config.ProcessConfig{ChatwootWebhookSecret: "placeholder", ChatwootAutoReplyEnabled: true})
	if !adapters.ChatwootAutoReplyEnabled {
		t.Fatal("Chatwoot AutoReply flag was not propagated")
	}
}

func TestBuildExternalAdaptersConstructsLLMWithoutCallingNetwork(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{
		LLMEnabled:            true,
		LLMBaseURL:            "https://example.invalid/v1",
		LLMAPIKey:             "test-only-key",
		LLMModel:              "test-model",
		LLMHTTPTimeout:        2 * time.Second,
		LLMMaxOutputTokens:    300,
		LLMMaxInputCharacters: 4000,
		LLMOutputTokensField:  "max_completion_tokens",
	})
	if adapters.LLMConfigError != nil || adapters.AIRuntime == nil {
		t.Fatalf("expected configured LLM runtime without network: %#v", adapters)
	}
}

func TestBuildExternalAdaptersReportsInvalidLLMConfiguration(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{LLMEnabled: true, LLMModel: "test-model"})
	if adapters.LLMConfigError == nil || adapters.AIRuntime != nil {
		t.Fatalf("expected invalid LLM configuration: %#v", adapters)
	}
}
