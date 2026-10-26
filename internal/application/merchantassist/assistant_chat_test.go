package merchantassist

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type nativeStub struct {
	replies []ToolCompletion
	seen    [][]ToolMessage
	tools   [][]ToolDefinition
}

func (m *nativeStub) CompleteTools(_ context.Context, messages []ToolMessage, tools []ToolDefinition, pinned string) (ToolCompletion, error) {
	m.seen = append(m.seen, append([]ToolMessage(nil), messages...))
	m.tools = append(m.tools, append([]ToolDefinition(nil), tools...))
	if len(m.replies) == 0 {
		return ToolCompletion{}, errors.New("unexpected tool round")
	}
	out := m.replies[0]
	m.replies = m.replies[1:]
	if out.Provider == "" {
		out.Provider = "test-provider"
	}
	if pinned != "" && pinned != out.Provider {
		return ToolCompletion{}, errors.New("provider switched")
	}
	return out, nil
}

func requestedTool(id, name, args string) ToolCompletion {
	return ToolCompletion{FinishReason: "tool_calls", Calls: []ToolCall{
		{ID: id, Name: name, Arguments: json.RawMessage(args)},
	}}
}
func finalText(s string) ToolCompletion {
	return ToolCompletion{FinishReason: "stop", Content: s}
}

type overviewStub struct{}

func (overviewStub) Handle(_ context.Context, _ queries.GetDashboardOverviewQuery) (queries.DashboardOverviewView, error) {
	return queries.DashboardOverviewView{OpenConversations: 8, WaitingHuman: 3, TransactionsNeedingReview: 2}, nil
}

func TestGuidedAdviceBasedOnAuthorizedOverview(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &nativeStub{replies: []ToolCompletion{
		requestedTool("call-1", "dashboard_overview", "{}"),
		finalText("عندك ثلاث محادثات تنتظر الموظف وطلبان يحتاجان المراجعة. ابدأ بالمحادثات."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}},
	}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "ما عرفت ايش ابدأ، إيش تنصحني؟",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Answer, "ابدأ") || len(model.seen) != 2 {
		t.Fatalf("wrong reply: %+v", response)
	}
	if len(model.seen[1]) != 3 || model.seen[1][2].Role != "tool" ||
		!strings.Contains(model.seen[1][2].Content, "waiting_human") {
		t.Fatal("native tool round did not replay real dashboard data")
	}
	if strings.Contains(string(response.Evidence), "principal-1") {
		t.Fatal("actor leaked")
	}
	if !strings.Contains(string(response.Evidence), "dashboard") {
		t.Fatal("missing dashboard link")
	}
}

func TestGuidedAdviceRejectsUnauthorizedTool(t *testing.T) {
	registry, _ := contracts.NewMerchantAssistRegistry()
	model := &nativeStub{replies: []ToolCompletion{requestedTool("bad", "list_transactions", "{}")}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}},
	}
	_, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "اطلع على الطلبات",
	})
	if !errors.Is(err, ErrInvalidModelPlan) {
		t.Fatalf("unauthorized tool accepted: %v", err)
	}
	if len(model.seen) != 1 {
		t.Fatal("executed after invalid tool")
	}
}

func TestGuidanceWithoutRecordUsesNativeFinalAnswer(t *testing.T) {
	registry, _ := contracts.NewMerchantAssistRegistry()
	model := &nativeStub{replies: []ToolCompletion{
		finalText("أقدر أساعدك تعرف وين تبدأ: اسأل عن وضع المحادثات أو الطلبات."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}},
	}
	v, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "السلام عليكم",
	})
	if err != nil || !strings.Contains(v.Answer, "أساعدك") || len(model.seen) != 1 {
		t.Fatalf("missing native answer: %+v / %v", v, err)
	}
	if len(model.seen[0]) != 1 || model.seen[0][0].Role != "user" {
		t.Fatal("unexpected planning prompt injected")
	}
}

func TestNativeArgumentsContractRejectsUnknownKeysAndNonStrings(t *testing.T) {
	def := toolDefinitions([]string{"find_customers"})[0]
	for _, raw := range []string{`{"search":"","business_id":"other"}`, `{"search":123}`, `{}`, `null`, `{"search":"x","search":"y","cursor":"oops"}`} {
		_, err := decodeReadArgs(def, json.RawMessage(raw))
		if !errors.Is(err, ErrInvalidModelPlan) {
			t.Fatalf("accepted %q: %v", raw, err)
		}
	}
	if _, err := decodeReadArgs(def, json.RawMessage(`{"search":""}`)); err != nil {
		t.Fatal(err)
	}
}

func TestNativeToolCatalogNeverExposesCatalogMutation(t *testing.T) {
	for _, def := range toolDefinitions([]string{"list_catalogs", "list_transactions", "find_customers"}) {
		if strings.Contains(def.Name, "create") || strings.Contains(def.Name, "delete") || strings.Contains(def.Name, "update") {
			t.Fatal("write capability available through merchant assist")
		}
	}
}
