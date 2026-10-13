package config

import "testing"

func setLiteLLMGatewayConfig(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/mujeeb")
	t.Setenv("LITELLM_GATEWAY_ONLY", "true")
	t.Setenv("LITELLM_BASE_URL", "http://127.0.0.1:4000")
	t.Setenv("LITELLM_API_KEY", "test-only-virtual-key")
	t.Setenv("LITELLM_CUSTOMER_MODEL", "mujeeb-customer")
	t.Setenv("LITELLM_CATALOG_MODEL", "mujeeb-catalog")
	t.Setenv("LITELLM_MERCHANT_MODEL", "mujeeb-merchant")
	t.Setenv("LLM_ENABLED", "false")
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
	if !cfg.LiteLLMGatewayOnly || cfg.GeminiAPIKey != "" {
		t.Fatal("gateway-only mode still requires Gemini")
	}
}
func TestGatewayOnlyRequiresCompleteRoutes(t *testing.T) {
	setLiteLLMGatewayConfig(t)
	t.Setenv("LITELLM_CATALOG_MODEL", "")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("gateway-only accepted missing catalog route")
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
