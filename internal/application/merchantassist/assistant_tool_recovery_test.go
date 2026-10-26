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

type recoveryConversationList struct{ calls int }

func (l *recoveryConversationList) Handle(_ context.Context, _ queries.ListConversationsQuery) (commands.ListResult[commands.ConversationView], error) {
	l.calls++
	return commands.ListResult[commands.ConversationView]{
		Items: []commands.ConversationView{{
			ID: "conversation-1", BusinessID: "business-1",
			CustomerID: "customer-1", State: "waiting_customer",
		}},
	}, nil
}

func TestAssistantResolvesUnverifiedConversationThroughDiscovery(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil { t.Fatal(err) }
	list := &recoveryConversationList{}
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("guessed", "conversation_history", `{"record_id":"made-up-record"}`),
		nativeCall("discover", "list_conversations", "{}"),
		nativeAnswer("وجدت محادثة في المتجر، لكن أحتاج منك تحديد المحادثة المقصودة قبل تلخيص رسائلها."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "conversations:read"}}
	a := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		History: &ConversationHistoryReader{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Conversations: list}}
	answer, err := a.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "إيش جرى في محادثة هذا العميل؟",
	})
	if err != nil || list.calls != 1 || len(model.seen) != 3 {
		t.Fatalf("unable to recover by reading authorized conversations: answer=%+v err=%v reads=%d", answer, err, list.calls)
	}
	if !strings.Contains(answer.Answer, "تحديد") ||
		!strings.Contains(model.seen[1][2].Content, "unverified_record_reference") ||
		!strings.Contains(model.seen[1][2].Content, "list_conversations") {
		t.Fatalf("missing safe discovery feedback: %+v", model.seen)
	}
	if !strings.Contains(model.seen[2][len(model.seen[2])-1].Content, "conversation-1") {
		t.Fatal("verified conversation list did not reach model")
	}
}

func TestAssistantPassesSelectedScreenRecordWithoutRequiringMerchantUUID(t *testing.T) {
	registry, _ := contracts.NewMerchantAssistRegistry()
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeAnswer("أقدر أراجع الطلب الموجود في الصفحة باستخدام بياناته المصرح بها."),
	}}
	actor := commands.ActorContext{BusinessID:"business-1", PrincipalID:"principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	a := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}}}
	_, err := a.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "هذا الطلب إيش وضعه؟",
		RecordID: "transaction-1", Search: "screen:transactions",
	})
	if err != nil { t.Fatal(err) }
	contextText := model.seen[0][0].Content
	if !strings.Contains(contextText, "transaction-1") ||
		!strings.Contains(contextText, `"kind":"transaction"`) ||
		!strings.Contains(contextText, "validate using a read tool") {
		t.Fatalf("screen selection was hidden from model: %q", contextText)
	}
}

func TestAssistantReturnsInvalidArgumentsAsToolResult(t *testing.T) {
	registry, _ := contracts.NewMerchantAssistRegistry()
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("wrong", "find_customers", `{"search":9}`),
		nativeAnswer("ما قدرت أتحقق من سجل محدد بهذا السؤال. ممكن توضح اسم العميل؟"),
	}}
	actor := commands.ActorContext{BusinessID:"business-1", PrincipalID:"principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	reader := &multiToolCustomerReader{}
	a := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Customers: reader}}
	answer, err := a.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "شوف العميل",
	})
	if err != nil || reader.calls != 0 || answer.Answer == "" {
		t.Fatalf("malformed arguments should not query data or fail the merchant: %+v / %v", answer, err)
	}
	var report struct{
		Ok bool `json:"ok"`
		Error struct{ Code string `json:"code"` } `json:"error"`
	}
	if err := json.Unmarshal([]byte(model.seen[1][2].Content), &report); err != nil {
		t.Fatal(err)
	}
	if report.Ok || report.Error.Code != "invalid_arguments" {
		t.Fatalf("unsafe read was not reported as model tool error: %+v", report)
	}
}
