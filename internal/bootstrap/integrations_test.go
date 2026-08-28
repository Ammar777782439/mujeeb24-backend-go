package bootstrap

import (
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

func TestBuildExternalAdaptersLeavesOptionalIntegrationsDisabled(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{SocialAPIBaseURL: "https://example.invalid"})
	if adapters.SocialAPI != nil || adapters.SocialWebhook != nil || adapters.AutoReplyEnabled || adapters.ChannelProvisioningEnabled {
		t.Fatalf("expected no external adapters for empty credentials: %#v", adapters)
	}
}

func TestBuildExternalAdaptersConstructsConfiguredClientsWithoutCallingNetwork(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{
		SocialAPIBaseURL:       "https://example.invalid",
		SocialAPIAPIKey:        "test-only-placeholder",
		SocialAPIWebhookSecret: "test-webhook-secret",
		SocialAPIHTTPTimeout:   2 * time.Second,
	})
	if adapters.SocialAPI == nil || adapters.SocialWebhook == nil || adapters.ChannelProvisioningSocial == nil {
		t.Fatalf("expected configured adapters: %#v", adapters)
	}
}

func TestBuildExternalAdaptersKeepsAutoReplyDisabledByDefault(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{SocialAPIWebhookSecret: "placeholder"})
	if adapters.AutoReplyEnabled {
		t.Fatal("AutoReply must be disabled by default")
	}

	adapters = BuildExternalAdapters(config.ProcessConfig{SocialAPIWebhookSecret: "placeholder", AutoReplyEnabled: true})
	if !adapters.AutoReplyEnabled {
		t.Fatal("AutoReply flag was not propagated")
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

func TestBuildExternalAdaptersMarksIncompleteProvisioningWithoutBlockingOtherRuntime(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{
		SocialAPIBaseURL:           "https://example.invalid",
		ChannelProvisioningEnabled: true,
	})
	if !adapters.ChannelProvisioningEnabled || adapters.ChannelProvisioningError == nil {
		t.Fatalf("expected a visible provisioning configuration error: %#v", adapters)
	}
	if adapters.SocialAPI != nil || adapters.SocialWebhook != nil {
		t.Fatal("a provision failure must not construct SocialAPI clients without credentials")
	}
	if adapters.ReadinessChecks()["channel_provisioning"] != "misconfigured" {
		t.Fatalf("readiness=%#v", adapters.ReadinessChecks())
	}
}
