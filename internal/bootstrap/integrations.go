package bootstrap

import (
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type ExternalAdapters struct {
	SocialAPI                      ports.ChannelProvider
	SocialWebhook                  ports.WebhookReceiver
	GeminiHTTPClient               *gemini.GeminiHTTPClient
	SocialAPIHealthProbe           ports.HealthCheckProbe
	LLMConfigError                 error
	AutoReplyEnabled               bool
	ChannelProvisioningSocial      ports.SocialChannelProvisioner
	ChannelProvisioningBrand       ports.ProviderBrandProvisioner
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
	if a.GeminiHTTPClient != nil {
		checks["llm_runtime"] = "configured"
	}
	return checks
}

func BuildExternalAdapters(cfg config.ProcessConfig) ExternalAdapters {
	adapters := ExternalAdapters{}
	if cfg.SocialAPIAPIKey != "" || cfg.SocialAPIWebhookSecret != "" {
		client := socialapi.NewClient(socialapi.Config{BaseURL: cfg.SocialAPIBaseURL, APIKey: cfg.SocialAPIAPIKey, WebhookSecret: cfg.SocialAPIWebhookSecret, HTTPTimeout: cfg.SocialAPIHTTPTimeout})
		adapters.SocialAPI = client
		adapters.SocialWebhook = client
		adapters.SocialAPIHealthProbe = socialapi.NewHealthCheckProbe(client)
		adapters.ChannelProvisioningSocial = socialapi.NewProvisioningAdapter(client)
	}
	if cfg.ChannelProvisioningEnabled {
		adapters.ChannelProvisioningError = cfg.ValidateChannelProvisioning()
		if adapters.ChannelProvisioningError == nil && adapters.ChannelProvisioningSocial == nil {
			adapters.ChannelProvisioningError = errors.New("channel provisioning adapters are not configured")
		}
	}
	if cfg.GeminiAPIKey != "" {
		client, err := gemini.NewGeminiHTTPClient(gemini.GeminiHTTPClientConfig{
			BaseURL:            cfg.GeminiBaseURL,
			APIKey:             cfg.GeminiAPIKey,
			Model:              cfg.GeminiModel,
			RequestTimeout:     cfg.GeminiHTTPTimeout,
			MaxOutputTokens:    cfg.LLMMaxOutputTokens,
			MaxInputCharacters: cfg.LLMMaxInputCharacters,
		})
		if err != nil {
			adapters.LLMConfigError = errors.New("Gemini adapter configuration: " + err.Error())
		} else {
			adapters.GeminiHTTPClient = client
		}
	} else if cfg.LLMEnabled {
		adapters.LLMConfigError = errors.New("the configured non-Gemini AI provider is not wired to a customer AI capability")

	}
	adapters.AutoReplyEnabled = cfg.AutoReplyEnabled
	adapters.ChannelProvisioningEnabled = cfg.ChannelProvisioningEnabled
	adapters.ChannelProvisioningRedirectURI = cfg.ChannelProvisioningRedirectURI
	adapters.FrontendURL = cfg.FrontendURL
	return adapters
}
