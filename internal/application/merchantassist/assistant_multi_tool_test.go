package merchantassist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type multiToolCustomerReader struct{ calls int }

func (m *multiToolCustomerReader) Handle(_ context.Context, _ queries.ListCustomersQuery) (commands.ListResult[commands.CustomerView], error) {
	m.calls++
	return commands.ListResult[commands.CustomerView]{Items: []commands.CustomerView{}}, nil
}

func fiveCustomerReadCalls() []ToolCall {
	calls := make([]ToolCall, 0, 5)
	for i := 1; i <= 5; i++ {
		calls = append(calls, ToolCall{
			ID: fmt.Sprintf("tool-%d", i), Name: "find_customers",
			Arguments: json.RawMessage(fmt.Sprintf(`{"search":"customer-%d"}`, i)),
		})
	}
	return calls
}

func TestNativeMerchantAssistantProcessesFiveCallsInOneRound(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reader := &multiToolCustomerReader{}
	model := &nativeModelStub{replies: []ToolCompletion{
		{Provider: "test-provider", FinishReason: "tool_calls", Calls: fiveCustomerReadCalls()},
		nativeAnswer("راجعت نتائج البحث الخمسة، ولا توجد تطابقات في القراءات المتاحة."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	assistant := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Customers: reader}}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "ابحث عن هؤلاء العملاء",
	})
	if err != nil {
		t.Fatalf("five valid model calls must not fail: %v", err)
	}
	if len(model.seen) != 2 || reader.calls != 5 || response.Answer == "" {
		t.Fatalf("incomplete tool batch: calls=%d rounds=%d answer=%q", reader.calls, len(model.seen), response.Answer)
	}
	replay := model.seen[1]
	if len(replay) != 7 || replay[1].Role != "assistant" || len(replay[1].ToolCalls) != 5 {
		t.Fatalf("must replay one assistant turn with five calls, then five results: %+v", replay)
	}
	for i, call := range fiveCustomerReadCalls() {
		if replay[i+2].Role != "tool" || replay[i+2].ToolCallID != call.ID ||
			!json.Valid([]byte(replay[i+2].Content)) {
			t.Fatalf("tool %d result not paired with model call: %+v", i, replay[i+2])
		}
	}
}

func TestNativeMerchantAssistantRejectsEntireUnauthorizedBatchBeforeAnyRead(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reader := &multiToolCustomerReader{}
	model := &nativeModelStub{replies: []ToolCompletion{{
		Provider: "test-provider", FinishReason: "tool_calls",
		Calls: []ToolCall{
			{ID: "allowed", Name: "find_customers", Arguments: json.RawMessage(`{"search":"Ali"}`)},
			{ID: "denied", Name: "list_transactions", Arguments: json.RawMessage(`{}`)},
		},
	}, nativeAnswer("لا تتوفر لي صلاحية الوصول إلى الطلبات. أقدر أساعدك في العملاء.")}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	assistant := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Customers: reader}}
	result, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "اعرض العملاء والطلبات",
	})
	if err != nil || reader.calls != 0 || len(model.seen) != 2 || result.Answer == "" {
		t.Fatalf("unauthorized batch was executed or merchant reply failed: %v calls=%d", err, reader.calls)
	}
	history := model.seen[1]
	if len(history) != 4 || !strings.Contains(history[2].Content, "batch_rejected") ||
		!strings.Contains(history[3].Content, "tool_unavailable") {
		t.Fatalf("all invalid batch calls must be safely rejected: %+v", history)
	}
}

func TestNativeMerchantAssistantAcceptsMoreThanEightToolsInOneBatch(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reader := &multiToolCustomerReader{}
	calls := make([]ToolCall, 11)
	for i := range calls {
		calls[i] = ToolCall{ID: fmt.Sprint(i), Name: "find_customers",
			Arguments: json.RawMessage(fmt.Sprintf(`{"search":"%d"}`, i))}
	}
	model := &nativeModelStub{replies: []ToolCompletion{{
		Provider: "test-provider", FinishReason: "tool_calls", Calls: calls,
	}, nativeAnswer("راجعت نتائج الاستعلامات الإحدى عشرة.")}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	assistant := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Customers: reader}}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "ابحث عن العملاء",
	})
	if err != nil || reader.calls != 11 || len(model.seen) != 2 || response.Answer == "" {
		t.Fatalf("model batch was prematurely capped: err=%v reads=%d rounds=%d", err, reader.calls, len(model.seen))
	}
	if len(model.seen[1]) != 13 {
		t.Fatalf("all tool results must be replayed: %d messages", len(model.seen[1]))
	}
}

func TestMerchantAssistantContinuesAcrossMoreThanEightToolRounds(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reader := &multiToolCustomerReader{}
	replies := make([]ToolCompletion, 0, 14)
	for i := 0; i < 13; i++ {
		replies = append(replies, nativeCall(fmt.Sprintf("step-%d", i), "find_customers",
			fmt.Sprintf(`{"search":"merchant-%d"}`, i)))
	}
	replies = append(replies, nativeAnswer("تم التحقق من السجلات المطلوبة بالقراءات المتاحة."))
	model := &nativeModelStub{replies: replies}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	assistant := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Customers: reader}}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "راجع الأسماء واحدًا واحدًا",
	})
	if err != nil || reader.calls != 13 || len(model.seen) != 14 || response.Answer == "" {
		t.Fatalf("follow-up round budget broke legitimate reads: err=%v reads=%d rounds=%d", err, reader.calls, len(model.seen))
	}
}

func TestMerchantAssistantCachesRepeatedVerifiedReads(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reader := &multiToolCustomerReader{}
	model := &nativeModelStub{replies: []ToolCompletion{
		nativeCall("first", "find_customers", `{"search":"أحمد"}`),
		nativeCall("again", "find_customers", `{"search":"أحمد"}`),
		nativeAnswer("راجعت البحث عن أحمد مرة واحدة ووجدت نتيجته المسجلة."),
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "customers:read"}}
	assistant := &Assistant{Registry: registry, Native: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Customers: reader}}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1",
		Task: contracts.TaskExplain, Question: "ابحث عن أحمد وتحقق من النتيجة",
	})
	if err != nil || reader.calls != 1 || len(model.seen) != 3 || response.Answer == "" {
		t.Fatalf("repeated tool was rejected or re-read: err=%v calls=%d", err, reader.calls)
	}
	history := model.seen[2]
	last := history[len(history)-1]
	if last.Role != "tool" || last.ToolCallID != "again" || !strings.Contains(last.Content, `"cached":true`) ||
		!strings.Contains(last.Content, `"previous_tool_call_id":"first"`) {
		t.Fatalf("repeat tool result did not reference verified original: %+v", last)
	}
}
