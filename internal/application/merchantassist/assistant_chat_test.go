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

type nativeModelStub struct {
	replies []ToolCompletion
	seen    [][]ToolMessage
	offered [][]ToolDefinition
	pins    []string
}

func (m *nativeModelStub) CompleteTools(_ context.Context, history []ToolMessage, defs []ToolDefinition, pinned string) (ToolCompletion, error) {
	m.seen = append(m.seen, append([]ToolMessage(nil), history...))
	m.offered = append(m.offered, append([]ToolDefinition(nil), defs...))
	m.pins = append(m.pins, pinned)
	if len(m.replies) == 0 {
		return ToolCompletion{}, errors.New("unexpected inference round")
	}
	result := m.replies[0]
	m.replies = m.replies[1:]
	return result, nil
}
func nativeCall(id, name, args string) ToolCompletion {
	return ToolCompletion{Provider: "test-provider", FinishReason: "tool_calls", Calls: []ToolCall{{ID: id, Name: name, Arguments: json.RawMessage(args)}}}
}
func nativeAnswer(answer string) ToolCompletion {
	return ToolCompletion{Provider: "test-provider", FinishReason: "stop", Content: answer}
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
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("call-1", "dashboard_overview", "{}"),
		nativeAnswer("عندك ثلاث محادثات تنتظر موظفًا وطلبان للمراجعة. ابدأ بالمحادثات."),
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
	if !strings.Contains(response.Answer, "ابدأ") || len(model.seen) != 2 || model.pins[0] != "" || model.pins[1] != "test-provider" {
		t.Fatalf("unexpected native dialogue: %+v, rounds=%d, pins=%v", response, len(model.seen), model.pins)
	}
	if len(model.seen[1]) != 3 || model.seen[1][1].Role != "assistant" || model.seen[1][2].Role != "tool" {
		t.Fatalf("tool result must be replayed as a genuine tool round: %+v", model.seen)
	}
	if strings.Contains(string(response.Evidence), "principal-1") {
		t.Fatal("internal actor leaked")
	}
	if !strings.Contains(string(response.Evidence), "dashboard") {
		t.Fatal("expected verified dashboard link")
	}
}

func TestGuidedAdviceRejectsUnauthorizedTool(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("call-1", "list_transactions", "{}"),
		nativeAnswer("لا أملك صلاحية قراءة الطلبات؛ أقدر أساعدك في المعلومات المتاحة."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}},
	}
	reply, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "اطلع على الطلبات",
	})
	if err != nil || !strings.Contains(reply.Answer, "صلاحية") {
		t.Fatalf("unauthorized read must be denied but answered safely: %+v / %v", reply, err)
	}
	if len(model.seen) != 2 || len(model.seen[1]) != 3 ||
		!strings.Contains(model.seen[1][2].Content, "tool_unavailable") {
		t.Fatal("unauthorized tool did not receive structured denial feedback")
	}
	for _, tool := range model.offered[0] {
		if tool.Name == "list_transactions" {
			t.Fatal("unauthorized tool advertised to model")
		}
	}
}

func TestNativeAssistantAnswersWithoutReadingOnGreeting(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &nativeModelStub{replies: []ToolCompletion{nativeAnswer("وعليكم السلام، أقدر أساعدك في متابعة المحادثات والطلبات.")}}
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
	if err != nil || !strings.Contains(v.Answer, "السلام") || len(model.seen) != 1 {
		t.Fatalf("unexpected native answer: %+v / %v", v, err)
	}
}

func TestRejectsInvalidNativeArguments(t *testing.T) {
	specs := toolDefinitions([]string{"find_customers", "list_leads", "customer_activity"})
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"unknown", "{\"search\":\"\",\"business_id\":\"another\"}"},
		{"missing", "{}"},
		{"wrongtype", "{\"search\":5}"},
		{"extra", "{\"record_id\":\"x\",\"customer_id\":\"y\"}"},
	} {
		spec := specs[0]
		if test.name == "extra" {
			spec = specs[2]
		}
		if _, err := decodeReadArgs(spec, json.RawMessage(test.raw)); !errors.Is(err, ErrInvalidModelPlan) {
			t.Fatalf("native schema failed to reject %s", test.name)
		}
	}
}
