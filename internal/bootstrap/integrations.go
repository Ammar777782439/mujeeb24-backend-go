package bootstrap

import (
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type ExternalAdapters struct {
	SocialAPI                      ports.ChannelProvider
	SocialWebhook                  ports.WebhookReceiver
	GeminiHTTPClient               *gemini.GeminiHTTPClient
	LiteLLMCustomerFallback        ports.CustomerSalesDecisionPort
	LiteLLMCatalogClient           *gemini.LiteLLMCatalogAdapter
	LiteLLMMerchantAuthoring       merchantcatalogai.MerchantCatalogAuthoringPort
	SocialAPIHealthProbe           ports.HealthCheckProbe
	LLMConfigError                 error
	CustomerSalesEnabled           bool
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
		"litellm_fallback":     "disabled",
		"litellm_catalog":      "disabled",
		"litellm_merchant":     "disabled",
	}
	if a.SocialWebhook != nil {
		checks["socialapi_webhook"] = "configured"
	}
	if a.CustomerSalesEnabled {
		checks["customer_sales"] = "configured"
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
	if a.LiteLLMCustomerFallback != nil {
		checks["litellm_fallback"] = "configured"
	}
	if a.LiteLLMCatalogClient != nil {
		checks["litellm_catalog"] = "configured"
	}
	if a.LiteLLMMerchantAuthoring != nil {
		checks["litellm_merchant"] = "configured"
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
		provisioning := socialapi.NewProvisioningAdapter(client)
		adapters.ChannelProvisioningSocial = provisioning
		adapters.ChannelProvisioningBrand = provisioning
	}
	if cfg.ChannelProvisioningEnabled {
		adapters.ChannelProvisioningError = cfg.ValidateChannelProvisioning()
		if adapters.ChannelProvisioningError == nil && (adapters.ChannelProvisioningSocial == nil || adapters.ChannelProvisioningBrand == nil) {
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
	if cfg.LiteLLMFallbackEnabled && adapters.LLMConfigError == nil {
		gateway, err := litellm.NewClient(litellm.Config{
			BaseURL: cfg.LiteLLMBaseURL, APIKey: cfg.LiteLLMAPIKey, Timeout: cfg.LiteLLMHTTPTimeout,
		})
		if err != nil {
			adapters.LLMConfigError = err
		} else {
			adapter, adapterErr := gemini.NewLiteLLMCustomerSalesAdapter(gateway, cfg.LiteLLMCustomerModel,
				cfg.LiteLLMMaxOutputTokens, cfg.LiteLLMMaxInputCharacters)
			if adapterErr != nil {
				adapters.LLMConfigError = adapterErr
			} else {
				adapters.LiteLLMCustomerFallback = adapter
				if cfg.LiteLLMMerchantEnabled {
					merchantAdapter, merchantErr := gemini.NewLiteLLMMerchantCatalogAuthoringAdapter(gateway, cfg.LiteLLMMerchantModel,
						cfg.LiteLLMMaxOutputTokens, cfg.LiteLLMMerchantMaxRequestBytes)
					if merchantErr != nil {
						adapters.LLMConfigError = merchantErr
					} else {
						adapters.LiteLLMMerchantAuthoring = merchantAdapter
					}
				}
				if cfg.LiteLLMCatalogEnabled {
					catalogAdapter, catalogErr := gemini.NewLiteLLMCatalogAdapter(gateway, cfg.LiteLLMCatalogModel,
						cfg.LiteLLMMaxOutputTokens, cfg.LiteLLMMaxContextTokens)
					if catalogErr != nil {
						adapters.LLMConfigError = catalogErr
					} else {
						adapters.LiteLLMCatalogClient = catalogAdapter
					}
				}
			}
		}
	}
	adapters.CustomerSalesEnabled = cfg.CustomerSalesEnabled
	adapters.AutoReplyEnabled = cfg.AutoReplyEnabled
	adapters.ChannelProvisioningEnabled = cfg.ChannelProvisioningEnabled
	adapters.ChannelProvisioningRedirectURI = cfg.ChannelProvisioningRedirectURI
	adapters.FrontendURL = cfg.FrontendURL
	return adapters
}
