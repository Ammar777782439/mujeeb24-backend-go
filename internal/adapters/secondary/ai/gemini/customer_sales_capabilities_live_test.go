//go:build gemini_live

package gemini

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Explicit opt-in only: evaluates tool selection, not exact Arabic wording.
// Run with -tags=gemini_live -run TestCustomerSalesCapabilityLiveEvals after
// authorizing live requests and providing GEMINI_API_KEY/GEMINI_MODEL in env.
func TestCustomerSalesCapabilityLiveEvals(t *testing.T) {
	if os.Getenv("MUJEEB_CAPABILITY_LIVE_EVALS") != "1" {
		t.Skip("live capability evaluations are not enabled")
	}
	key, model := os.Getenv("GEMINI_API_KEY"), os.Getenv("GEMINI_MODEL")
	if key == "" || model == "" {
		t.Fatal("explicit live eval requires GEMINI_API_KEY and GEMINI_MODEL")
	}
	client, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{BaseURL: "https://generativelanguage.googleapis.com", APIKey: key, Model: model, RequestTimeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewGeminiCustomerSalesAdapter(client)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, text, business string
		want                 ports.CustomerSalesCapability
	}{
		{"greeting", "السلام عليكم", "متجر عام", ports.CustomerSalesCapabilityReply},
		{"dialect greeting", "هلا والله", "متجر عام", ports.CustomerSalesCapabilityReply},
		{"thanks", "شكرا يعطيك العافيه", "متجر عام", ports.CustomerSalesCapabilityReply},
		{"explicit human", "ابغى اكلم موظف", "متجر عام", ports.CustomerSalesCapabilityHandoff},
		{"human typo", "اشتي اكلم الموضف", "متجر عام", ports.CustomerSalesCapabilityHandoff},
		{"price", "بكم باقة Growth؟", "منصة اشتراكات", ports.CustomerSalesCapabilityFullCatalog},
		{"catalog presence", "هل عندكم ثلاجات؟", "متجر أجهزة", ports.CustomerSalesCapabilityFullCatalog},
		{"implicit family need", "معي عايله ٥ اشخاص ايش يناسبنا؟", "مطعم", ports.CustomerSalesCapabilityFullCatalog},
		{"comparison", "ايش الفرق بين الباقات عندكم؟", "منصة اشتراكات", ports.CustomerSalesCapabilityFullCatalog},
		{"alternatives", "اشتي خيارات أقل من ١٠٠ ألف", "متجر أجهزة", ports.CustomerSalesCapabilityFullCatalog},
		{"availability", "متى عندكم خدمات متاحة؟", "عيادة", ports.CustomerSalesCapabilityFullCatalog},
		{"absence", "أكيد ما عندكم نوع يناسبني؟ ايش الخيارات عندكم؟", "متجر عام", ports.CustomerSalesCapabilityFullCatalog},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := adapter.Decide(ctx, ports.CustomerSalesDecisionInput{
				Capabilities: ports.CustomerSalesCapabilities(true),
				Request:      ports.CustomerSalesDecisionRequest{Text: tc.text, Context: &ports.CustomerSalesContext{Business: ports.CustomerSalesContextBusiness{Name: tc.business}, CatalogManifest: &ports.CatalogAIManifest{TotalActiveItems: 250, TotalCatalogs: 3}, GeneratedAt: time.Now().UTC()}},
			})
			if err != nil {
				t.Fatalf("live decision failed: %v", err)
			}
			if out.Decision.Capability != tc.want {
				t.Fatalf("selected %s, want %s", out.Decision.Capability, tc.want)
			}
		})
	}
}
