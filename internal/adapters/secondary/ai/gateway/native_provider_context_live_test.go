//go:build provider_live

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// TestNativeProviderContextLive calls real provider APIs. Neither these tests
// nor the summary generation path can execute sales actions or send messages.
// Only anonymized, user-provided conversation content is used.
func TestNativeProviderContextLive(t *testing.T) {
	if os.Getenv("MUJEEB_PROVIDER_CONTEXT_LIVE") != "1" {
		t.Skip("explicit opt-in required")
	}
	providers := []struct {
		name, model, key string
		factory          func(string, string) (native.Provider, error)
	}{
		{"gemini", "gemini-3.1-flash-lite", os.Getenv("GEMINI_API_KEY"), func(model, key string) (native.Provider, error) {
			return native.NewGemini(native.GeminiConfig{Model: model, APIKey: key, Timeout: 80 * time.Second})
		}},
		{"deepseek", "deepseek-chat", os.Getenv("DEEPSEEK_API_KEY"), func(model, key string) (native.Provider, error) {
			return native.NewDeepSeek(native.DeepSeekConfig{Model: model, APIKey: key, Timeout: 80 * time.Second})
		}},
	}
	contract, err := json.Marshal(services.BuildCatalogEntityContractPayload())
	if err != nil {
		t.Fatal(err)
	}
	history := subscriptionHistory()
	summary := "العميل يملك متجرًا صغيرًا فيه 20 منتجًا ولا يريد ردودًا كثيرة، وقد طلب مقارنة الباقات ومعلومات الدفع والضمانات. المساعد ذكر باقة Business ووعد بالتواصل مع موظفين سابقًا، ولكن هذا وعد حواري وليس إثبات تنفيذ. المعلومات التجارية السابقة تحتاج أدلة موثوقة."
	cases := []struct {
		name    string
		history []ports.CustomerSalesRecentMessageEvidence
		summary string
	}{
		{"memory_and_current_only", nil, summary},
		{"memory_plus_recent_four", history[len(history)-4:], summary},
		{"entire_conversation", history, ""},
	}
	filter := strings.TrimSpace(os.Getenv("MUJEEB_LIVE_PROVIDER"))
	for _, provider := range providers {
		if filter != "" && provider.name != filter {
			continue
		}
		t.Run(provider.name, func(t *testing.T) {
			if strings.TrimSpace(provider.key) == "" {
				t.Fatalf("required real %s API secret unavailable", provider.name)
			}
			clientProvider, err := provider.factory(provider.model, provider.key)
			if err != nil {
				t.Fatal(err)
			}
			router, err := native.NewRouter(clientProvider, nil, native.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			completion, err := native.NewCompletionClient(router)
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := NewNativeCustomerSalesAdapter(completion, provider.model, 1800, 130000)
			if err != nil {
				t.Fatal(err)
			}
			// Use the SAME production capability schema for both providers.
			offered := ports.CustomerSalesCapabilities(true, false)
			for i, tc := range cases {
				if i > 0 {
					pause := 3 * time.Second
					if provider.name == "gemini" {
						pause = 90 * time.Second
					}
					t.Logf("pacing before next %s real request: %v", provider.name, pause)
					time.Sleep(pause)
				}
				now := time.Now().UTC()
				customerContext := &ports.CustomerSalesContext{
					SchemaVersion: 1, Freshness: "fresh",
					Business:        ports.CustomerSalesContextBusiness{Reference: "isolated-business", Name: "مجيب 24", VerticalType: "software", Locale: "ar", DefaultCurrency: "YER"},
					Conversation:    ports.CustomerSalesContextConversation{Reference: "continued-chat", CustomerReference: "anonymized-customer", State: "open", Ownership: "ai"},
					Customer:        ports.CustomerSalesContextCustomer{Reference: "anonymized-customer", LocalePreference: "ar"},
					CatalogManifest: &ports.CatalogAIManifest{TotalActiveItems: 1, TotalCatalogs: 1},
					CatalogEvidence: []ports.CustomerSalesCatalogEvidence{{Reference: "plan-business", CatalogReference: "plans", Name: "مجيب Business", ItemType: "service", Status: "active", EvidenceState: "fresh", RetrievedAt: now, SchemaVersion: 1}},
					RecentMessages:  tc.history, ConversationSummary: tc.summary, GeneratedAt: now, ExpiresAt: now.Add(5 * time.Minute),
				}
				timeoutCtx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
				out, err := adapter.Decide(timeoutCtx, ports.CustomerSalesDecisionInput{
					EntityContractPayload: contract, Capabilities: offered,
					Request: ports.CustomerSalesDecisionRequest{BusinessID: "isolated-business", ConversationID: "continued-chat", SourceMessageReference: "next-inbound", Text: "طيب اريد الاشتراك ؟", Channel: "facebook", Context: customerContext},
				})
				cancel()
				if err != nil {
					t.Fatalf("provider=%s case=%s real request failed: %v", provider.name, tc.name, err)
				}
				t.Logf("REAL_PROVIDER_RESULT provider=%s model=%s case=%s capability=%s input_tokens=%d output_tokens=%d",
					out.Usage.Provider, out.Usage.Model, tc.name, out.Decision.Capability, out.Usage.InputTokens, out.Usage.OutputTokens)
				if out.Usage.Provider != provider.name || out.Usage.Model != provider.model || out.Usage.ModelRequests != 1 {
					t.Fatalf("provider identity/attempt mismatch: got %s/%s attempts=%d", out.Usage.Provider, out.Usage.Model, out.Usage.ModelRequests)
				}
				if out.Decision.Capability != ports.CustomerSalesCapabilityHandoff {
					t.Errorf("semantic capability mismatch: provider=%s strategy=%s got=%s expected handoff_to_human", provider.name, tc.name, out.Decision.Capability)
				}
			}
			// Verify the provider-neutral memory port has no tools and can generate text.
			if provider.name == "gemini" {
				time.Sleep(90 * time.Second)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
			memory, usage, err := adapter.Summarize(ctx,
				"Summarize an ongoing merchant/customer conversation in concise Arabic. Preserve customer goals and open questions. Distinguish assistant promises from verified operations; no operational execution data is supplied.",
				"العميل: أحتاج موعداً لخدمة صيانة. المساعد: سأخبر الفريق. العميل: هل تم الحجز؟")
			cancel()
			if err != nil {
				t.Fatalf("real independent summary call failed on %s: %v", provider.name, err)
			}
			if strings.TrimSpace(memory) == "" || usage.Provider != provider.name {
				t.Fatalf("invalid real summary result: provider=%s model=%s", usage.Provider, usage.Model)
			}
			t.Logf("REAL_SUMMARY_OK provider=%s model=%s input_tokens=%d output_tokens=%d characters=%d", usage.Provider, usage.Model, usage.InputTokens, usage.OutputTokens, len([]rune(memory)))
		})
	}
	// Keep unused-independent-API contracts from altering production.
	_ = fmt.Sprintf
}
