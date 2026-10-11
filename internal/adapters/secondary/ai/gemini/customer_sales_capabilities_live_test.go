//go:build gemini_live

package gemini

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Explicit opt-in only: evaluates semantic capability selection, not exact wording.
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

	now := time.Now().UTC()
	baseContext := func(business string) *ports.CustomerSalesContext {
		return &ports.CustomerSalesContext{
			Business:        ports.CustomerSalesContextBusiness{Name: business},
			CatalogManifest: &ports.CatalogAIManifest{TotalActiveItems: 250, TotalCatalogs: 3},
			GeneratedAt:     now,
		}
	}
	growthContext := func() *ports.CustomerSalesContext {
		ctx := baseContext("منصة اشتراكات")
		ctx.CatalogEvidence = []ports.CustomerSalesCatalogEvidence{{
			Reference: "item-growth", CatalogReference: "catalog-plans", ItemType: "service", Name: "مجيب Growth",
			Status: "active", EvidenceState: "verified", RetrievedAt: now, SchemaVersion: 1,
			PricingMode: "fixed", AvailabilityMode: "available", FulfillmentMode: "manual", RequiresConfirmation: true,
		}}
		ctx.OfferEvidence = []ports.CustomerSalesOfferEvidence{{
			Reference: "offer-growth", CatalogItemReference: "item-growth", Name: "مجيب Growth شهري",
			PricingMode: "fixed", Amount: "10000", Currency: "YER", AvailabilityStatus: "available",
			Status: "active", EvidenceState: "verified", RetrievedAt: now, SchemaVersion: 1,
		}}
		return ctx
	}
	growthConfirmationContext := func() *ports.CustomerSalesContext {
		ctx := growthContext()
		ctx.RecentMessages = []ports.CustomerSalesRecentMessageEvidence{
			{Reference: "m1", Direction: "inbound", Origin: "customer", Text: "طيب اريد باقه مجيب Growth", OccurredAt: now.Add(-2 * time.Minute), EvidenceState: "verified", SchemaVersion: 1},
			{Reference: "m2", Direction: "outbound", Origin: "ai", Text: "هل ترغب في تأكيد طلبك ومتابعة الإجراءات؟", OccurredAt: now.Add(-time.Minute), EvidenceState: "verified", SchemaVersion: 1},
		}
		return ctx
	}

	tests := []struct {
		name    string
		text    string
		context *ports.CustomerSalesContext
		want    ports.CustomerSalesCapability
	}{
		{"greeting", "السلام عليكم", baseContext("متجر عام"), ports.CustomerSalesCapabilityRespondWithoutAction},
		{"dialect greeting", "هلا والله", baseContext("متجر عام"), ports.CustomerSalesCapabilityRespondWithoutAction},
		{"thanks", "شكرا يعطيك العافيه", baseContext("متجر عام"), ports.CustomerSalesCapabilityRespondWithoutAction},
		{"explicit human", "ابغى اكلم موظف", baseContext("متجر عام"), ports.CustomerSalesCapabilityHandoff},
		{"human typo", "اشتي اكلم الموضف", baseContext("متجر عام"), ports.CustomerSalesCapabilityHandoff},
		{"price requires catalog", "بكم باقة Growth؟", baseContext("منصة اشتراكات"), ports.CustomerSalesCapabilityFullCatalog},
		{"grounded price is informational", "كم سعر باقة مجيب Growth؟", growthContext(), ports.CustomerSalesCapabilityRespondWithoutAction},
		{"grounded explicit proceed must handoff", "طيب اريد باقه مجيب Growth", growthContext(), ports.CustomerSalesCapabilityHandoff},
		{"short confirmation inherits unsupported operation", "نعم", growthConfirmationContext(), ports.CustomerSalesCapabilityHandoff},
		{"catalog presence", "هل عندكم ثلاجات؟", baseContext("متجر أجهزة"), ports.CustomerSalesCapabilityFullCatalog},
		{"implicit family need", "معي عايله ٥ اشخاص ايش يناسبنا؟", baseContext("مطعم"), ports.CustomerSalesCapabilityFullCatalog},
		{"comparison", "ايش الفرق بين الباقات عندكم؟", baseContext("منصة اشتراكات"), ports.CustomerSalesCapabilityFullCatalog},
		{"alternatives", "اشتي خيارات أقل من ١٠٠ ألف", baseContext("متجر أجهزة"), ports.CustomerSalesCapabilityFullCatalog},
		{"availability", "متى عندكم خدمات متاحة؟", baseContext("عيادة"), ports.CustomerSalesCapabilityFullCatalog},
		{"absence", "أكيد ما عندكم نوع يناسبني؟ ايش الخيارات عندكم؟", baseContext("متجر عام"), ports.CustomerSalesCapabilityFullCatalog},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := adapter.Decide(ctx, ports.CustomerSalesDecisionInput{
				Capabilities: ports.CustomerSalesCapabilities(true),
				Request:      ports.CustomerSalesDecisionRequest{Text: tc.text, Context: tc.context},
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
