package config

import "testing"

func setLiteLLMGatewayConfig(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/mujeeb")
	t.Setenv("LITELLM_GATEWAY_ONLY", "true")
	t.Setenv("LITELLM_BASE_URL", "http://127.0.0.1:4000")
	t.Setenv("LITELLM_API_KEY", "test-only-virtual-key")
	t.Setenv("LITELLM_USD_TO_YER_RATE", "500")
	t.Setenv("LITELLM_CUSTOMER_MODEL", "mujeeb-sales")
	t.Setenv("LITELLM_CATALOG_MODEL", "mujeeb-catalog")
	t.Setenv("LITELLM_MERCHANT_MODEL", "mujeeb-merchant")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("AUTOREPLY_ENABLED", "false")
	t.Setenv("CUSTOMER_SALES_ENABLED", "false")
}
func TestGatewayOnlyConfigNeedsNoGeminiKey(t *testing.T) {
	setLiteLLMGatewayConfig(t)
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("valid gateway-only config: %v", err)
	}
	if !cfg.LiteLLMGatewayOnly || cfg.LiteLLMCustomerModel != "mujeeb-sales" {
		t.Fatal("gateway-only mode still requires Gemini")
	}
}
func TestGatewayOnlyAppliesAgreedModelRouteDefaults(t *testing.T) {
	setLiteLLMGatewayConfig(t)
	t.Setenv("LITELLM_CUSTOMER_MODEL", "")
	t.Setenv("LITELLM_CATALOG_MODEL", "")
	t.Setenv("LITELLM_MERCHANT_MODEL", "")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("gateway-only should use agreed route names: %v", err)
	}
	if cfg.LiteLLMCustomerModel != "mujeeb-sales" ||
		cfg.LiteLLMCatalogModel != "mujeeb-catalog" ||
		cfg.LiteLLMMerchantModel != "mujeeb-merchant" {
		t.Fatalf("wrong LiteLLM route defaults: customer=%q catalog=%q merchant=%q",
			cfg.LiteLLMCustomerModel, cfg.LiteLLMCatalogModel, cfg.LiteLLMMerchantModel)
	}
}

func TestGatewayOnlyRespectsExplicitModelRoutes(t *testing.T) {
	setLiteLLMGatewayConfig(t)
	t.Setenv("LITELLM_CUSTOMER_MODEL", "my-customer")
	t.Setenv("LITELLM_CATALOG_MODEL", "my-catalog")
	t.Setenv("LITELLM_MERCHANT_MODEL", "my-merchant")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("explicit model routes should be accepted: %v", err)
	}
	if cfg.LiteLLMCustomerModel != "my-customer" || cfg.LiteLLMCatalogModel != "my-catalog" || cfg.LiteLLMMerchantModel != "my-merchant" {
		t.Fatal("explicit LiteLLM route names were overwritten by defaults")
	}
}
func TestGatewayOnlyRequiresGatewayKey(t *testing.T) {
	setLiteLLMGatewayConfig(t)
	t.Setenv("LITELLM_API_KEY", "")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("gateway-only accepted missing virtual key")
	}
}
func TestGatewayOnlyCanEnableAutoReplyWithoutGemini(t *testing.T) {
	setLiteLLMGatewayConfig(t)
	t.Setenv("AUTOREPLY_ENABLED", "true")
	t.Setenv("SOCIALAPI_API_KEY", "test-key")
	t.Setenv("SOCIALAPI_WEBHOOK_SECRET", "test-secret")
	cfg, err := LoadFromEnv()
	if err != nil || !cfg.AutoReplyEnabled {
		t.Fatalf("cannot enable gateway-only auto reply: %v", err)
	}
}
