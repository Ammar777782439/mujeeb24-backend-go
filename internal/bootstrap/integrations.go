package bootstrap

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/workspaces/chatwoot"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type ExternalAdapters struct {
	SocialAPI       ports.ChannelProvider
	Chatwoot        ports.CommunicationWorkspace
	SocialWebhook   ports.WebhookReceiver
	ChatwootWebhook ports.WebhookReceiver
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
	return adapters
}
