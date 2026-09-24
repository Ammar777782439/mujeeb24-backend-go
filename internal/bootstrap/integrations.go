package bootstrap

import (
	"errors"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/merchantgemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/openaicompatible"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type ExternalAdapters struct {
	SocialAPI                      ports.ChannelProvider
	SocialWebhook                  ports.WebhookReceiver
	AIRuntime                      ports.AIRuntime
	MerchantAIRuntime              ports.MerchantAIRuntime
	LLMConfigError                 error
	AutoReplyEnabled               bool
	ChannelProvisioningSocial      ports.SocialChannelProvisioner
	ChannelProvisioningError       error
	ChannelProvisioningEnabled     bool
	ChannelProvisioningRedirectURI string
	FrontendURL                    string
}

func (a ExternalAdapters) ReadinessChecks() map[string]string {
	checks := map[string]string{
		"socialapi_webhook":    "disabled",
		"auto_reply":           "disabled",
		"channel_provisioning": "disabled",
		"llm_runtime":          "disabled",
	}
	if a.SocialWebhook != nil {
		checks["socialapi_webhook"] = "configured"
	}
	if a.AutoReplyEnabled {
		checks["auto_reply"] = "configured"
	}
	if a.ChannelProvisioningEnabled {
		if a.ChannelProvisioningError != nil {
			checks["channel_provisioning"] = "misconfigured"
		} else {
			checks["channel_provisioning"] = "configured"
		}
	}
	if a.AIRuntime != nil {
		checks["llm_runtime"] = "configured"
	}
	if a.MerchantAIRuntime != nil {
		checks["merchant_ai_runtime"] = "configured"
	}
	return checks
}

func BuildExternalAdapters(cfg config.ProcessConfig, capabilities ...ports.AICapabilityDispatcher) ExternalAdapters {
	var customerCaps ports.AICapabilityDispatcher
	var merchantCaps ports.AICapabilityDispatcher
	if len(capabilities) > 0 {
		customerCaps = capabilities[0]
	}
	if len(capabilities) > 1 {
		merchantCaps = capabilities[1]
	} else {
		merchantCaps = customerCaps
	}
	return BuildExternalAdaptersWithBothCapabilities(cfg, customerCaps, merchantCaps)
}

func BuildExternalAdaptersWithCapabilities(cfg config.ProcessConfig, capabilities ports.AICapabilityDispatcher) ExternalAdapters {
	return BuildExternalAdaptersWithBothCapabilities(cfg, capabilities, capabilities)
}

func BuildExternalAdaptersWithBothCapabilities(cfg config.ProcessConfig, customerCaps ports.AICapabilityDispatcher, merchantCaps ports.AICapabilityDispatcher) ExternalAdapters {
	adapters := ExternalAdapters{}
	if cfg.SocialAPIAPIKey != "" || cfg.SocialAPIWebhookSecret != "" {
		client := socialapi.NewClient(socialapi.Config{BaseURL: cfg.SocialAPIBaseURL, APIKey: cfg.SocialAPIAPIKey, WebhookSecret: cfg.SocialAPIWebhookSecret, HTTPTimeout: cfg.SocialAPIHTTPTimeout})
		adapters.SocialAPI = client
		adapters.SocialWebhook = client
		adapters.ChannelProvisioningSocial = socialapi.NewProvisioningAdapter(client)
	}
	if cfg.ChannelProvisioningEnabled {
		adapters.ChannelProvisioningError = cfg.ValidateChannelProvisioning()
		if adapters.ChannelProvisioningError == nil && adapters.ChannelProvisioningSocial == nil {
			adapters.ChannelProvisioningError = errors.New("channel provisioning adapters are not configured")
		}
	}
	if cfg.GeminiAPIKey != "" {
		client, err := gemini.NewClient(gemini.Config{
			BaseURL:            cfg.GeminiBaseURL,
			APIKey:             cfg.GeminiAPIKey,
			Model:              cfg.GeminiModel,
			RequestTimeout:     cfg.GeminiHTTPTimeout,
			MaxOutputTokens:    cfg.LLMMaxOutputTokens,
			MaxInputCharacters: cfg.LLMMaxInputCharacters,
			Capabilities:       customerCaps,
		})
		if err != nil {
			adapters.LLMConfigError = errors.New("Gemini adapter configuration: " + err.Error())
		} else {
			adapters.AIRuntime = client
		}
	} else if cfg.LLMEnabled {
		client, err := openaicompatible.NewClient(openaicompatible.Config{
			BaseURL:            cfg.LLMBaseURL,
			APIKey:             cfg.LLMAPIKey,
			Model:              cfg.LLMModel,
			RequestTimeout:     cfg.LLMHTTPTimeout,
			MaxOutputTokens:    cfg.LLMMaxOutputTokens,
			MaxInputCharacters: cfg.LLMMaxInputCharacters,
			OutputTokensField:  cfg.LLMOutputTokensField,
		})
		if err != nil {
			adapters.LLMConfigError = errors.New("LLM adapter configuration: " + err.Error())
		} else {
			adapters.AIRuntime = client
		}
	}
	if cfg.MerchantAIEnabled || cfg.MerchantAIGeminiAPIKey != "" {
		merchantRuntime, err := BuildMerchantAIRuntime(cfg, merchantCaps)
		if err == nil {
			adapters.MerchantAIRuntime = merchantRuntime
		}
	}
	adapters.AutoReplyEnabled = cfg.AutoReplyEnabled
	adapters.ChannelProvisioningEnabled = cfg.ChannelProvisioningEnabled
	adapters.ChannelProvisioningRedirectURI = cfg.ChannelProvisioningRedirectURI
	adapters.FrontendURL = cfg.FrontendURL
	return adapters
}

// BuildMerchantAIRuntime builds a dedicated, independent Gemini AIRuntime instance for the Merchant Copilot.
func BuildMerchantAIRuntime(cfg config.ProcessConfig, capabilities ports.AICapabilityDispatcher) (ports.MerchantAIRuntime, error) {
	apiKey := cfg.MerchantAIGeminiAPIKey
	if apiKey == "" {
		apiKey = cfg.GeminiAPIKey
	}
	if apiKey == "" {
		return nil, errors.New("MERCHANT_AI_GEMINI_API_KEY (or GEMINI_API_KEY) is not configured")
	}

	model := cfg.MerchantAIGeminiModel
	if model == "" {
		model = "gemini-2.5-flash"
	}
	timeout := cfg.MerchantAIGeminiTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return merchantgemini.NewClient(merchantgemini.Config{
		BaseURL:            cfg.MerchantAIGeminiBaseURL,
		APIKey:             apiKey,
		Model:              model,
		RequestTimeout:     timeout,
		MaxOutputTokens:    2048,
		MaxInputCharacters: 15000,
		SystemPrompt:       services.MerchantAISystemPrompt,
		Capabilities:       capabilities,
	})
}
