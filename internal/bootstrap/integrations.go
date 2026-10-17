package bootstrap

import (
	"errors"

	gatewayai "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gateway"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

type ExternalAdapters struct {
	LiteLLMPlatformConfig          *ports.LiteLLMPlatformConfig
	SocialAPI                      ports.ChannelProvider
	SocialWebhook                  ports.WebhookReceiver
	LiteLLMCustomerDecision        ports.CustomerSalesDecisionPort
	LiteLLMCatalogClient           *gatewayai.LiteLLMCatalogAdapter
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
		"litellm_customer":     "disabled",
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
	if a.LiteLLMCustomerDecision != nil && a.LiteLLMCatalogClient != nil && a.LiteLLMMerchantAuthoring != nil {
		checks["llm_runtime"] = "configured"
	}
	if a.LiteLLMCustomerDecision != nil {
		checks["litellm_customer"] = "configured"
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
	if cfg.LiteLLMGatewayOnly {
		adapters.LiteLLMPlatformConfig = &ports.LiteLLMPlatformConfig{
			BaseURL: cfg.LiteLLMBaseURL, APIKey: cfg.LiteLLMAPIKey,
			CustomerModel: cfg.LiteLLMCustomerModel, CatalogModel: cfg.LiteLLMCatalogModel,
			MerchantModel:   cfg.LiteLLMMerchantModel,
			MaxOutputTokens: cfg.LiteLLMMaxOutputTokens, MaxInputCharacters: cfg.LiteLLMMaxInputCharacters,
			USDToYERRate: cfg.LiteLLMUSDToYERRate,
		}
	}
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
	if !cfg.NativeAIEnabled && !cfg.LiteLLMGatewayOnly && (cfg.AutoReplyEnabled || cfg.CustomerSalesEnabled) {
		adapters.LLMConfigError = errors.New("customer AI requires a configured native Gemini and DeepSeek runtime")
	}
	if cfg.LiteLLMGatewayOnly && adapters.LLMConfigError == nil {
		gateway, err := litellm.NewClient(litellm.Config{
			BaseURL: cfg.LiteLLMBaseURL, APIKey: cfg.LiteLLMAPIKey, Timeout: cfg.LiteLLMHTTPTimeout,
		})
		if err != nil {
			adapters.LLMConfigError = err
		} else {
			customer, customerErr := gatewayai.NewLiteLLMCustomerSalesAdapter(gateway, cfg.LiteLLMCustomerModel,
				cfg.LiteLLMMaxOutputTokens, cfg.LiteLLMMaxInputCharacters)
			if customerErr != nil {
				adapters.LLMConfigError = customerErr
			} else {
				merchant, merchantErr := gatewayai.NewLiteLLMMerchantCatalogAuthoringAdapter(gateway, cfg.LiteLLMMerchantModel,
					cfg.LiteLLMMaxOutputTokens, cfg.LiteLLMMerchantMaxRequestBytes)
				if merchantErr != nil {
					adapters.LLMConfigError = merchantErr
				} else {
					catalog, catalogErr := gatewayai.NewLiteLLMCatalogAdapter(gateway, cfg.LiteLLMCatalogModel,
						cfg.LiteLLMMaxOutputTokens, cfg.LiteLLMMaxContextTokens)
					if catalogErr != nil {
						adapters.LLMConfigError = catalogErr
					} else {
						adapters.LiteLLMCustomerDecision = customer
						adapters.LiteLLMMerchantAuthoring = merchant
						adapters.LiteLLMCatalogClient = catalog
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
