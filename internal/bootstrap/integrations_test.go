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
	t.Setenv("GEMINI_API_KEY", "test-primary-only-key")
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

func TestBuildExternalAdaptersGeminiOnlyAndOptionalStandby(t *testing.T) {
	base := config.ProcessConfig{
		NativeAIEnabled: true,
		GeminiAPIKey: "test-gemini-key",
		GeminiModel: "gemini-2.5-flash",
		NativeHTTPTimeout: 10 * time.Second,
		NativeMaxContextTokens: 32768,
		NativeMaxOutputTokens: 4096,
		NativeMaxInputCharacters: 48000,
		NativeMerchantMaxRequestBytes: 524288,
	}
	geminiOnly := BuildExternalAdapters(base)
	if geminiOnly.LLMConfigError != nil || geminiOnly.NativeCustomerDecision == nil ||
		geminiOnly.NativeCatalogClient == nil || geminiOnly.NativeMerchantAuthoring == nil {
		t.Fatalf("Gemini-only native runtime was not constructed: %+v", geminiOnly)
	}
	if geminiOnly.NativeAISettings == nil || !geminiOnly.NativeAISettings.GeminiConfigured ||
		geminiOnly.NativeAISettings.DeepSeekConfigured ||
		geminiOnly.ReadinessChecks()["llm_runtime"] != "configured" {
		t.Fatalf("Gemini-only readiness or provider settings mismatch: %+v", geminiOnly)
	}
	base.DeepSeekAPIKey = "test-backup-key"
	base.DeepSeekModel = "deepseek-chat"
	withBackup := BuildExternalAdapters(base)
	if withBackup.LLMConfigError != nil || withBackup.NativeCustomerDecision == nil ||
		withBackup.NativeCatalogClient == nil || withBackup.NativeMerchantAuthoring == nil ||
		withBackup.NativeAISettings == nil || !withBackup.NativeAISettings.DeepSeekConfigured {
		t.Fatalf("optional DeepSeek standby failed to configure: %+v", withBackup)
	}
	base.DeepSeekModel = ""
	invalidBackup := BuildExternalAdapters(base)
	if invalidBackup.LLMConfigError == nil || invalidBackup.NativeCustomerDecision != nil {
		t.Fatal("provided DeepSeek key with invalid model must fail closed")
	}
}
