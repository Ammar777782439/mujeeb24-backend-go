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
	SocialAPI                ports.ChannelProvider
	Chatwoot                 ports.CommunicationWorkspace
	SocialWebhook            ports.WebhookReceiver
	ChatwootWebhook          ports.WebhookReceiver
	AIRuntime                ports.AIRuntime
	LLMConfigError           error
	ChatwootAutoReplyEnabled bool
}

func BuildExternalAdapters(cfg config.ProcessConfig) ExternalAdapters {
	adapters := ExternalAdapters{}
	if cfg.SocialAPIAPIKey != "" || cfg.SocialAPIWebhookSecret != "" {
		client := socialapi.NewClient(socialapi.Config{BaseURL: cfg.SocialAPIBaseURL, APIKey: cfg.SocialAPIAPIKey, WebhookSecret: cfg.SocialAPIWebhookSecret, HTTPTimeout: cfg.SocialAPIHTTPTimeout})
		adapters.SocialAPI = client
		adapters.SocialWebhook = client
	}
	if cfg.ChatwootAPIToken != "" || cfg.ChatwootWebhookSecret != "" {
		client := chatwoot.NewClient(chatwoot.Config{BaseURL: cfg.ChatwootBaseURL, APIToken: cfg.ChatwootAPIToken, WebhookSecret: cfg.ChatwootWebhookSecret, HTTPTimeout: cfg.ChatwootHTTPTimeout})
		adapters.Chatwoot = client
		adapters.ChatwootWebhook = client
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
	return adapters
}
