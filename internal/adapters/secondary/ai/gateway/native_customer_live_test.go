//go:build gemini_live

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

// TestGeminiSubscriptionConversationLive uses the real Google Gemini REST API.
// It is intentionally excluded from standard CI, never executes any business
// capability or sends any channel message, and uses anonymized customer history.
//
// First call: production-shaped context: last four messages + summary.
// Second call: complete prior history to isolate context-window effects.
// One 90-second pause separates the calls to respect low-rate plans.
func TestGeminiSubscriptionConversationLive(t *testing.T) {
	if os.Getenv("MUJEEB_GEMINI_LIVE_EVAL") != "1" {
		t.Skip("requires explicit live-evaluation opt-in")
	}
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Fatal("GEMINI_API_KEY secret is unavailable")
	}
	const model = "gemini-3.1-flash-lite"
	if supplied := strings.TrimSpace(os.Getenv("GEMINI_MODEL")); supplied != "" && supplied != model {
		t.Fatalf("unexpected model %q; this evaluation requires %s", supplied, model)
	}
	gemini, err := native.NewGemini(native.GeminiConfig{
		Model: model, APIKey: apiKey, Timeout: 75 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := native.NewRouter(gemini, nil, native.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCustomerSalesAdapter(client, model, 2048, 130000)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := json.Marshal(services.BuildCatalogEntityContractPayload())
	if err != nil {
		t.Fatal(err)
	}
	// The reported case has no available sales executor. Only the three
	// messaging capabilities and full-catalog retrieval are offered.
	offered := ports.CustomerSalesCapabilities(true, false)
	for _, capability := range offered {
		if capability == ports.CustomerSalesCapabilityRecordLead ||
			capability == ports.CustomerSalesCapabilityCreateDraft ||
			capability == ports.CustomerSalesCapabilityUpdateDraft {
			t.Fatal("sales capabilities unexpectedly offered")
		}
	}

	history := subscriptionHistory()
	cases := []struct {
		name     string
		messages []ports.CustomerSalesRecentMessageEvidence
		summary  string
	}{
		{
			name:     "production_shaped_four_recent_plus_summary",
			messages: history[len(history)-4:],
			summary:  "المحادثة جارية وليست محادثة جديدة. العميل قدّم سابقاً بيانات تواصل لمنشأة، ثم أفاد أن أحداً لم يتواصل معه. سبق للمساعد أن وعد بتسجيل بياناته ورفع طلبه وتذكير الموظفين، لكن هذه الوعود ليست دليلاً على تنفيذ تحويل أو متابعة. طلب العميل تفاصيل الموقع وطرق الدفع والضمانات وفوائد الاشتراك، وبقيت تلك التفاصيل غير مثبتة. ذكر المساعد باقة مجيب Business وأرقاماً متضاربة لعدد الردود الذكية (400 و4000)، فلا تعاملها بوصفها بيانات موثوقة. العميل عنده متجر صغير يضم 20 منتجاً ولا يحتاج ردوداً كثيرة، وسأل عن أنسب باقة، وتلقّى جواباً بأن Business قد تكون أكبر من حاجته وبأن الفريق سيفحص خيارات أصغر. لم يُنفّذ أي اشتراك أو تحويل بشري مثبت.",
		},
		{
			name:     "complete_prior_conversation",
			messages: history,
			summary:  "",
		},
	}
	failures := 0
	for i, tc := range cases {
		if i != 0 {
			t.Log("rate-limit pacing: waiting 90 seconds before the next real Gemini request")
			time.Sleep(90 * time.Second)
		}
		now := time.Now().UTC()
		requestContext := &ports.CustomerSalesContext{
			SchemaVersion: 1,
			Freshness:     "fresh",
			Business: ports.CustomerSalesContextBusiness{
				Reference: "live-eval-business", Name: "مجيب 24", VerticalType: "software",
				Locale: "ar", DefaultCurrency: "YER",
			},
			Conversation: ports.CustomerSalesContextConversation{
				Reference: "ongoing-customer-conversation", CustomerReference: "anonymized-customer",
				State: "open", Ownership: "ai",
			},
			Customer: ports.CustomerSalesContextCustomer{
				Reference: "anonymized-customer", LocalePreference: "ar",
			},
			CatalogManifest: &ports.CatalogAIManifest{TotalActiveItems: 1, TotalCatalogs: 1},
			CatalogEvidence: []ports.CustomerSalesCatalogEvidence{{
				Reference: "plan-business", CatalogReference: "plans",
				Name: "مجيب Business", ItemType: "service", Status: "active",
				EvidenceState: "fresh", RetrievedAt: now, SchemaVersion: 1,
			}},
			RecentMessages:      tc.messages,
			ConversationSummary: tc.summary,
			GeneratedAt:         now,
			ExpiresAt:           now.Add(10 * time.Minute),
		}
		requestCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		result, callErr := adapter.Decide(requestCtx, ports.CustomerSalesDecisionInput{
			EntityContractPayload: contract,
			Capabilities:         offered,
			Request: ports.CustomerSalesDecisionRequest{
				BusinessID: "live-eval-business",
				ConversationID: "ongoing-customer-conversation",
				SourceMessageReference: "new-subscribe-message",
				Text: "طيب اريد الاشتراك ؟", Channel: "facebook",
				Context: requestContext,
			},
		})
		cancel()
		if callErr != nil {
			t.Fatalf("case=%s live Gemini API or strict-contract error: %v; no automatic retry", tc.name, callErr)
		}
		responseText := []rune(result.Decision.ResponseText)
		if len(responseText) > 750 {
			responseText = responseText[:750]
		}
		t.Logf("case=%s provider=%s model=%s capability=%s input_tokens=%d output_tokens=%d response=%q",
			tc.name, result.Usage.Provider, result.Usage.Model, result.Decision.Capability,
			result.Usage.InputTokens, result.Usage.OutputTokens, string(responseText))
		if result.Usage.Provider != "gemini" || result.Usage.Model != model || result.Usage.ModelRequests != 1 {
			t.Fatalf("case=%s did not use direct Gemini-only inference: provider=%s model=%s attempts=%d",
				tc.name, result.Usage.Provider, result.Usage.Model, result.Usage.ModelRequests)
		}
		if result.Decision.Capability != ports.CustomerSalesCapabilityHandoff {
			failures++
			t.Logf("SEMANTIC_MISMATCH case=%s got=%s want=%s: customer is proceeding with an unsupported subscription operation",
				tc.name, result.Decision.Capability, ports.CustomerSalesCapabilityHandoff)
		}
	}
	if failures > 0 {
		t.Errorf("Gemini selected an incorrect capability in %d/%d real continued-conversation scenarios",
			failures, len(cases))
	}
}

func subscriptionHistory() []ports.CustomerSalesRecentMessageEvidence {
	type transcriptTurn struct {
		direction string
		origin    string
		text      string
		hour      int
		minute    int
	}
	turns := []transcriptTurn{
		{direction: "inbound", origin: "customer", text: "اسمي عميل التجربة، اسم المؤسسة مؤسسة تجريبية، ورقم التواصل محفوظ لديكم.", hour: 15, minute: 13},
		{direction: "outbound", origin: "ai", text: "أهلاً بك. شكراً لتزويدنا ببيانات مؤسستك ورقم التواصل. لقد تم تسجيل بياناتكم لدينا، وسيقوم فريقنا بالتواصل معكم في أقرب وقت ممكن لخدمتكم.", hour: 15, minute: 13},
		{direction: "inbound", origin: "customer", text: "للان ما تم التواصل معي", hour: 15, minute: 13},
		{direction: "outbound", origin: "ai", text: "نعتذر عن التأخير في التواصل معك. سأقوم فوراً برفع طلبك إلى فريق المتابعة لدينا لضمان التواصل معك في أسرع وقت ممكن.", hour: 15, minute: 13},
		{direction: "inbound", origin: "customer", text: "السلام عليكم", hour: 18, minute: 43},
		{direction: "inbound", origin: "customer", text: "السلام عليكم", hour: 18, minute: 47},
		{direction: "outbound", origin: "ai", text: "وعليكم السلام ورحمة الله وبركاته. نعتذر مجدداً عن التأخير. أؤكد لك أن طلبك قيد المتابعة، وسأحرص على تذكير الفريق المختص لضمان التواصل معك في أقرب وقت ممكن. شكراً لصبرك.", hour: 18, minute: 47},
		{direction: "inbound", origin: "customer", text: "لا اريد شي ثاني طلب موقعكم وطرق الدفع والضمانات وايش بستفيد من الاشتراك", hour: 18, minute: 48},
		{direction: "inbound", origin: "customer", text: "لا اريد شي ثاني طلب موقعكم وطرق الدفع والضمانات وايش بستفيد من الاشتراك", hour: 18, minute: 50},
		{direction: "outbound", origin: "ai", text: "يسعدني توضيح ذلك لك: بالنسبة لاستفسارك عن مجيب Business، فهو اشتراك شهري، وقد ورد في الرد السابق أنه يقدم 400 رد ذكي و2500 عنصر وربط 5 قنوات وقيمة الاشتراك 30,000 ريال يمني. أما موقعنا وطرق الدفع والضمانات فلا تتوفر لدي تفاصيل مؤكدة عنها، وسأقوم برفع طلبك للزملاء المختصين ليتواصلوا معك. هل هناك استفسار آخر؟", hour: 18, minute: 50},
		{direction: "inbound", origin: "customer", text: "ايش تنصحني من باقه اشترك للعلم معي متجر صغير يوجد لديه ٢٠ منتج فقط وما اريد ردود كثير", hour: 19, minute: 55},
		{direction: "outbound", origin: "ai", text: "بما أن متجرك يحتوي على 20 منتجاً ولا تحتاج لعدد كبير من الردود، فقد تكون باقة مجيب Business أكبر من احتياجك. ورد في الرد السابق أنها تتسع إلى 2500 منتج و4000 رد ذكي. لا تتوفر لدي معلومات مؤكدة عن باقات أصغر، وسأقوم بتسجيل ملاحظتك للتحقق من خطة ملائمة لدى الفريق، وسيتابع الزملاء أسئلتك السابقة. هل هناك أي شيء آخر؟", hour: 19, minute: 56},
	}
	loc := time.FixedZone("Yemen", 3*60*60)
	history := make([]ports.CustomerSalesRecentMessageEvidence, 0, len(turns))
	for i, turn := range turns {
		history = append(history, ports.CustomerSalesRecentMessageEvidence{
			Reference: fmt.Sprintf("historical-message-%02d", i),
			Direction: turn.direction, Origin: turn.origin, Text: turn.text,
			OccurredAt: time.Date(2026, time.October, 18, turn.hour, turn.minute, i, 0, loc),
			EvidenceState: "fresh", SchemaVersion: 1,
		})
	}
	return history
}
