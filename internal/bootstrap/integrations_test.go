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

func TestBuildExternalAdaptersRejectsDisabledNativeAIWhenCustomerSalesEnabled(t *testing.T) {
	adapters := BuildExternalAdapters(config.ProcessConfig{
		CustomerSalesEnabled: true,
	})
	if adapters.LLMConfigError == nil || adapters.NativeCustomerDecision != nil {
		t.Fatal("customer AI must not start when native runtime is disabled")
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

func TestBuildExternalAdaptersDeepSeekOnly(t *testing.T) {
	base := config.ProcessConfig{
		NativeAIEnabled:               true,
		DeepSeekAPIKey:                "test-deepseek-key",
		DeepSeekModel:                 "deepseek-flash",
		NativeHTTPTimeout:             10 * time.Second,
		NativeMaxContextTokens:        32768,
		NativeMaxOutputTokens:         4096,
		NativeMaxInputCharacters:      48000,
		NativeMerchantMaxRequestBytes: 524288,
	}
	adapters := BuildExternalAdapters(base)
	if adapters.LLMConfigError != nil || adapters.NativeCustomerDecision == nil ||
		adapters.NativeCatalogClient == nil || adapters.NativeMerchantAuthoring == nil {
		t.Fatalf("DeepSeek-only native runtime was not constructed: %+v", adapters)
	}
	if adapters.NativeAISettings == nil || !adapters.NativeAISettings.Configured ||
		adapters.NativeAISettings.Provider != "deepseek" ||
		adapters.NativeAISettings.Model != "deepseek-flash" ||
		adapters.ReadinessChecks()["llm_runtime"] != "configured" {
		t.Fatalf("DeepSeek-only readiness mismatch: %+v", adapters)
	}
	base.DeepSeekModel = ""
	invalid := BuildExternalAdapters(base)
	if invalid.LLMConfigError == nil || invalid.NativeCustomerDecision != nil {
		t.Fatal("DeepSeek with missing model must fail closed")
	}
	base.DeepSeekAPIKey = ""
	base.DeepSeekModel = "deepseek-flash"
	missingKey := BuildExternalAdapters(base)
	if missingKey.LLMConfigError == nil || missingKey.NativeCustomerDecision != nil {
		t.Fatal("DeepSeek with missing key must fail closed")
	}
}
