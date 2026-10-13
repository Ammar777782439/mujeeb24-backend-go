package bootstrap

import (
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/config"
)

func TestBuildExternalAdaptersGatewayOnlyUsesOneProxyWithoutGemini(t *testing.T) {
	cfg:=config.ProcessConfig{
		LiteLLMGatewayOnly: true,
		GeminiAPIKey:"key-must-not-leak-into-direct-client",
		LiteLLMBaseURL:"http://127.0.0.1:4000",
		LiteLLMAPIKey:"virtual-key",
		LiteLLMCustomerModel:"mujeeb-customer",
		LiteLLMCatalogModel:"mujeeb-catalog",
		LiteLLMMerchantModel:"mujeeb-merchant",
		LiteLLMMaxOutputTokens:4096,
		LiteLLMMaxInputCharacters:48000,
		LiteLLMMaxContextTokens:32768,
		LiteLLMMerchantMaxRequestBytes:524288,
		LiteLLMHTTPTimeout:20*time.Second,
	}
	out:=BuildExternalAdapters(cfg)
	if out.LLMConfigError!=nil {t.Fatalf("gateway-only setup failed: %v",out.LLMConfigError)}
	if out.GeminiHTTPClient!=nil {t.Fatal("direct Gemini client exists in gateway-only mode")}
	if out.LiteLLMCustomerDecision==nil || out.LiteLLMCatalogClient==nil || out.LiteLLMMerchantAuthoring==nil {
		t.Fatal("gateway-only mode did not configure all B2C, catalog and B2B paths")
	}
	if out.ReadinessChecks()["litellm_customer"]!="configured" || out.ReadinessChecks()["litellm_catalog"]!="configured" || out.ReadinessChecks()["litellm_merchant"]!="configured" {
		t.Fatal("gateway routes not reported")
	}
}
