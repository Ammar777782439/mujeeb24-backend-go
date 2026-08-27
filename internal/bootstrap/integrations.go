package bootstrap

import (
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/openaicompatible"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/workspaces/chatwoot"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type ExternalAdapters struct {
	SocialAPI                      ports.ChannelProvider
	Chatwoot                       ports.CommunicationWorkspace
	SocialWebhook                  ports.WebhookReceiver
	ChatwootWebhook                ports.WebhookReceiver
	AIRuntime                      ports.AIRuntime
	LLMConfigError                 error
	ChatwootAutoReplyEnabled       bool
	ChatwootMirrorEnabled          bool
	ChannelProvisioningSocial      ports.SocialChannelProvisioner
	ChannelProvisioningWorkspace   ports.WorkspaceProvisioner
	ChannelProvisioningError       error
	ChannelProvisioningEnabled     bool
	ChannelProvisioningRedirectURI string
	ChannelProvisioningWebhookURL  string
}

func (a ExternalAdapters) ReadinessChecks() map[string]string {
	checks := map[string]string{
		"socialapi_webhook":    "disabled",
		"chatwoot_webhook":     "disabled",
		"chatwoot_mirror":      "disabled",
		"chatwoot_autoreply":   "disabled",
		"channel_provisioning": "disabled",
		"llm_runtime":          "disabled",
	}
	if a.SocialWebhook != nil {
		checks["socialapi_webhook"] = "configured"
	}
	if a.ChatwootWebhook != nil {
		checks["chatwoot_webhook"] = "configured"
	}
	if a.ChatwootMirrorEnabled {
		checks["chatwoot_mirror"] = "configured"
	}
	if a.ChatwootAutoReplyEnabled {
		checks["chatwoot_autoreply"] = "configured"
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
	return checks
}

func BuildExternalAdapters(cfg config.ProcessConfig) ExternalAdapters {
	adapters := ExternalAdapters{}
	if cfg.SocialAPIAPIKey != "" || cfg.SocialAPIWebhookSecret != "" {
		client := socialapi.NewClient(socialapi.Config{BaseURL: cfg.SocialAPIBaseURL, APIKey: cfg.SocialAPIAPIKey, WebhookSecret: cfg.SocialAPIWebhookSecret, HTTPTimeout: cfg.SocialAPIHTTPTimeout})
		adapters.SocialAPI = client
		adapters.SocialWebhook = client
		adapters.ChannelProvisioningSocial = socialapi.NewProvisioningAdapter(client)
	}
	if cfg.ChatwootAPIToken != "" || cfg.ChatwootWebhookSecret != "" || cfg.ChatwootPlatformAPIToken != "" {
		client := chatwoot.NewClient(chatwoot.Config{BaseURL: cfg.ChatwootBaseURL, APIToken: cfg.ChatwootAPIToken, WebhookSecret: cfg.ChatwootWebhookSecret, HTTPTimeout: cfg.ChatwootHTTPTimeout})
		adapters.Chatwoot = client
		adapters.ChatwootWebhook = client
		if cfg.ChatwootPlatformAPIToken != "" {
			adapters.ChannelProvisioningWorkspace = chatwoot.NewPlatformClient(chatwoot.PlatformConfig{BaseURL: cfg.ChatwootBaseURL, PlatformToken: cfg.ChatwootPlatformAPIToken, APIToken: cfg.ChatwootAPIToken, APIUserID: int64(cfg.ChatwootProvisioningUserID), HTTPTimeout: cfg.ChatwootHTTPTimeout})
		}
	}
	if cfg.ChatwootProvisioningEnabled {
		adapters.ChannelProvisioningError = cfg.ValidateChannelProvisioning()
		if adapters.ChannelProvisioningError == nil && (adapters.ChannelProvisioningSocial == nil || adapters.ChannelProvisioningWorkspace == nil) {
			adapters.ChannelProvisioningError = errors.New("channel provisioning adapters are not configured")
		}
	}
	if cfg.LLMEnabled {
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
	adapters.ChatwootAutoReplyEnabled = cfg.ChatwootAutoReplyEnabled
	adapters.ChatwootMirrorEnabled = cfg.ChatwootMirrorEnabled
	adapters.ChannelProvisioningEnabled = cfg.ChatwootProvisioningEnabled
	adapters.ChannelProvisioningRedirectURI = cfg.ChannelProvisioningRedirectURI
	adapters.ChannelProvisioningWebhookURL = cfg.ChannelProvisioningWebhookURL
	return adapters
}
