package merchantassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type multiToolCustomerReader struct { calls int }

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
	if err != nil { t.Fatal(err) }
	reader := &multiToolCustomerReader{}
	model := &nativeModelStub{replies: []ToolCompletion{
		{Provider:"test-provider", FinishReason:"tool_calls", Calls:fiveCustomerReadCalls()},
		nativeAnswer("راجعت نتائج البحث الخمسة، ولا توجد تطابقات في القراءات المتاحة."),
	}}
	actor := commands.ActorContext{BusinessID:"business-1",PrincipalID:"principal-1",
		Permissions:[]string{"merchant_ai:use","customers:read"}}
	assistant := &Assistant{Registry:registry,Native:model,Sanitizer:ConservativeSanitizer{},
		Discovery:&DiscoveryReader{Guard:&guardFixture{}, Customers:reader}}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor:actor, Business:"business-1", ProjectionID:"auto.v1",
		Task:contracts.TaskExplain, Question:"ابحث عن هؤلاء العملاء",
	})
	if err != nil {t.Fatalf("five valid model calls must not fail: %v",err)}
	if len(model.seen)!=2 || reader.calls!=5 || response.Answer=="" {
		t.Fatalf("incomplete tool batch: calls=%d rounds=%d answer=%q",reader.calls,len(model.seen),response.Answer)
	}
	replay := model.seen[1]
	if len(replay)!=7 || replay[1].Role!="assistant" || len(replay[1].ToolCalls)!=5 {
		t.Fatalf("must replay one assistant turn with five calls, then five results: %+v", replay)
	}
	for i, call := range fiveCustomerReadCalls() {
		if replay[i+2].Role!="tool" || replay[i+2].ToolCallID!=call.ID ||
			!json.Valid([]byte(replay[i+2].Content)) {
			t.Fatalf("tool %d result not paired with model call: %+v",i,replay[i+2])
		}
	}
}

func TestNativeMerchantAssistantRejectsEntireUnauthorizedBatchBeforeAnyRead(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {t.Fatal(err)}
	reader := &multiToolCustomerReader{}
	model := &nativeModelStub{replies:[]ToolCompletion{{
		Provider:"test-provider", FinishReason:"tool_calls",
		Calls:[]ToolCall{
			{ID:"allowed",Name:"find_customers",Arguments:json.RawMessage(`{"search":"Ali"}`)},
			{ID:"denied",Name:"list_transactions",Arguments:json.RawMessage(`{}`)},
		},
	}}}
	actor:=commands.ActorContext{BusinessID:"business-1",PrincipalID:"principal-1",
		Permissions:[]string{"merchant_ai:use","customers:read"}}
	assistant:=&Assistant{Registry:registry,Native:model,Sanitizer:ConservativeSanitizer{},
		Discovery:&DiscoveryReader{Guard:&guardFixture{},Customers:reader}}
	_, err=assistant.Assist(context.Background(),AssistRequest{
		Actor:actor,Business:"business-1",ProjectionID:"auto.v1",
		Task:contracts.TaskExplain,Question:"اعرض العملاء والطلبات",
	})
	if !errors.Is(err,ErrInvalidModelPlan) || reader.calls!=0 {
		t.Fatalf("batch was partially executed despite unauthorized tool: %v calls=%d",err,reader.calls)
	}
}

func TestNativeMerchantAssistantRejectsOversizedToolBatch(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil { t.Fatal(err) }
	reader := &multiToolCustomerReader{}
	calls := make([]ToolCall, MaxReadToolsPerBatch+1)
	for i:=range calls {
		calls[i]=ToolCall{ID:fmt.Sprint(i), Name:"find_customers",
			Arguments:json.RawMessage(fmt.Sprintf(`{"search":"%d"}`,i))}
	}
	model:=&nativeModelStub{replies:[]ToolCompletion{{
		Provider:"test-provider", FinishReason:"tool_calls",Calls:calls,
	}}}
	actor:=commands.ActorContext{BusinessID:"business-1",PrincipalID:"principal-1",
		Permissions:[]string{"merchant_ai:use","customers:read"}}
	assistant:=&Assistant{Registry:registry,Native:model,Sanitizer:ConservativeSanitizer{},
		Discovery:&DiscoveryReader{Guard:&guardFixture{},Customers:reader}}
	_,err=assistant.Assist(context.Background(),AssistRequest{
		Actor:actor,Business:"business-1",ProjectionID:"auto.v1",
		Task:contracts.TaskExplain,Question:"اعرض العملاء",
	})
	if !errors.Is(err,ErrInvalidModelPlan) || reader.calls!=0 {
		t.Fatalf("excessive model tool batch was not denied before reads: %v count=%d",err,reader.calls)
	}
}
