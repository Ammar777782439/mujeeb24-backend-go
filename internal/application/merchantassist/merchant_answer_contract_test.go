package merchantassist

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

func TestMerchantStateSemanticsSeparateLeadTransactionAndConversation(t *testing.T) {
	lead := decorateMerchantToolEvidence("list_leads", map[string]any{
		"rows": map[string]any{"total": 6, "status_counts": map[string]int{"new": 1, "won": 4, "lost": 1}},
	}).(map[string]any)
	terms := lead["status_meanings_ar"].(map[string]map[string]string)
	if !strings.Contains(terms["lead"]["won"], "مكسوبة") ||
		!strings.Contains(terms["lead"]["won"], "ليس دليلًا على دفع") ||
		!strings.Contains(terms["lead"]["lost"], "خاسرة") ||
		!strings.Contains(terms["lead"]["new"], "جديدة") {
		t.Fatalf("lead contract has missing or misleading Arabic meanings: %+v", terms)
	}
	if _, leak := terms["lead"]["waiting_customer"]; leak {
		t.Fatal("conversation status leaked into lead semantics")
	}
	trans := decorateMerchantToolEvidence("list_transactions", map[string]any{}).(map[string]any)
	tr := trans["status_meanings_ar"].(map[string]map[string]string)
	if _, leak := tr["transaction"]["won"]; leak {
		t.Fatal("won must not be reported as a completed transaction")
	}
	if !strings.Contains(tr["transaction"]["completed"], "ليس إثباتًا") {
		t.Fatal("completed transaction must not imply paid/delivered")
	}
	conv := decorateMerchantToolEvidence("list_conversations", map[string]any{}).(map[string]any)
	cl := conv["status_meanings_ar"].(map[string]map[string]string)
	if cl["conversation"]["waiting_customer"] != "بانتظار رد العميل" ||
		cl["conversation"]["waiting_human"] != "بانتظار تدخل موظف" {
		t.Fatalf("conversation status contract is incorrect: %+v", cl)
	}
}

func TestMerchantToolEvidencePreservesOriginalAndDoesNotInventStatuses(t *testing.T) {
	original := map[string]any{"rows": map[string]any{"total": 6, "status_counts": map[string]int{"won": 4, "new": 1, "lost": 1}}}
	extended := decorateMerchantToolEvidence("list_leads", original).(map[string]any)
	if _, exists := original["status_meanings_ar"]; exists {
		t.Fatal("annotation mutated raw source evidence")
	}
	data := extended["rows"].(map[string]any)
	counts := data["status_counts"].(map[string]int)
	if counts["won"] != 4 || counts["new"] != 1 || counts["lost"] != 1 ||
		data["total"] != 6 {
		t.Fatalf("annotation changed verified numeric counts: %+v", data)
	}
	if _, ok := decorateMerchantToolEvidence("platform_guide", original).(map[string]any)["status_meanings_ar"]; ok {
		t.Fatal("unrelated tool should not gain arbitrary state mappings")
	}
	serialized, err := json.Marshal(extended)
	if err != nil || !strings.Contains(string(serialized), "مكسوبة") {
		t.Fatalf("Arabic semantic evidence not valid JSON: %v / %s", err, serialized)
	}
}

type answerLeadList struct{}

func (answerLeadList) Handle(_ context.Context, _ queries.ListLeadsQuery) (commands.ListResult[commands.LeadView], error) {
	return commands.ListResult[commands.LeadView]{Items: []commands.LeadView{
		{ID: "lead-1", BusinessID: "business-1", Status: "new"},
		{ID: "lead-2", BusinessID: "business-1", Status: "won"},
		{ID: "lead-3", BusinessID: "business-1", Status: "lost"},
	}}, nil
}

func TestDeepSeekReceivesArabicStatusContractAndConciseAnswerRules(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("list-1", "list_leads", "{}"),
		nativeAnswer("عندك 6 فرص بيع: 4 مكسوبة، وفرصة جديدة، وواحدة خاسرة. ابدأ بمراجعة الفرصة الجديدة."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "sales:read"}}
	assistant := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Leads: answerLeadList{}}}
	_, err = assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "كيف وضع فرص البيع؟",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.seen) != 2 || len(model.seen[1]) < 3 {
		t.Fatal("expected native tool round")
	}
	if !strings.Contains(model.seen[0][0].Content, "الرد الافتراضي قصير") ||
		!strings.Contains(model.seen[0][0].Content, "إذا طلب التاجر شرحًا") {
		t.Fatalf("merchant answer contract was not delivered: %+v", model.seen[0])
	}
	evidence := model.seen[1][len(model.seen[1])-1].Content
	if !strings.Contains(evidence, "status_meanings_ar") ||
		!strings.Contains(evidence, "مكسوبة") ||
		!strings.Contains(evidence, "خاسرة") {
		t.Fatalf("DeepSeek did not receive Arabic status contract: %s", evidence)
	}
	found := false
	for _, def := range model.offered[0] {
		if def.Name == "list_leads" {
			found = strings.Contains(def.Description, "status_meanings_ar")
		}
	}
	if !found {
		t.Fatal("DeepSeek tool description omitted status semantics contract")
	}
}
