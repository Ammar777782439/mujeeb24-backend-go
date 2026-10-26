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

type verifiedTransactionList struct{}
func (verifiedTransactionList) Handle(_ context.Context, _ queries.ListTransactionsQuery) (commands.ListResult[commands.TransactionView], error) {
	return commands.ListResult[commands.TransactionView]{Items: []commands.TransactionView{{
		ID: "transaction-1", BusinessID: "business-1", CustomerID: "customer-1",
		State: "completed", TransactionType: "order",
	}}}, nil
}
type verifiedTransactionGet struct{}
func (verifiedTransactionGet) Handle(_ context.Context, _ queries.GetTransactionQuery) (commands.TransactionView, error) {
	return commands.TransactionView{
		ID: "transaction-1", BusinessID: "business-1", CustomerID: "customer-1",
		State: "completed", TransactionType: "order",
		Lines: []commands.TransactionLineSnapshotView{{
			ID: "line-1", ItemNameSnapshot: "قهوة عربية", Quantity: "2",
		}},
	}, nil
}

func TestRememberVerifiedCustomerReferenceFromTransactionAndLead(t *testing.T) {
	for _, tool := range []string{"list_transactions", "list_leads", "list_conversations"} {
		known := map[string]map[string]bool{
			"customer": {}, "conversation": {}, "transaction": {}, "lead": {},
		}
		rememberIDs(known, tool, map[string]any{"sample": []map[string]any{
			{"id": "source-1", "customer_id": "customer-1"},
		}})
		if !known["customer"]["customer-1"] {
			t.Fatalf("%s did not retain verified related customer", tool)
		}
	}
}

func TestVerifiedTransactionCustomerDetailsIncludesOrderLineItems(t *testing.T) {
	reader := &DiscoveryReader{
		Guard: &guardFixture{}, GetCustomer: linkedCustomerGet{},
		GetTransaction: verifiedTransactionGet{},
	}
	actor := commands.ActorContext{BusinessID:"business-1", PrincipalID:"principal-1",
		Permissions:[]string{"merchant_ai:use","customers:read","sales:read"}}
	available := reader.Available(actor)
	found := false
	for _, name := range available { if name == "transaction_customer_details" { found = true } }
	if !found { t.Fatal("legitimate merchant cannot discover verified order details") }
	got, err := reader.Read(context.Background(), actor, "business-1",
		DiscoveryCall{Name:"transaction_customer_details",RecordID:"transaction-1"})
	if err!=nil { t.Fatal(err) }
	raw, err := json.Marshal(got.Rows)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(raw),"قهوة عربية") || !strings.Contains(string(raw),"أحمد") ||
		!strings.Contains(string(raw),"\"quantity\":\"2\"") ||
		!strings.Contains(string(raw),"\"line_items_complete\":true") {
		t.Fatalf("missing trusted customer/order snapshot: %s",raw)
	}
	if len(got.Links)!=1 || !strings.Contains(got.Links[0].URL, "transaction-1") {
		t.Fatal("verified order link missing")
	}
}

func TestTransactionCustomerDetailsRequiresBothReadPermissions(t *testing.T) {
	reader := &DiscoveryReader{
		Guard:&guardFixture{},GetCustomer:linkedCustomerGet{},
		GetTransaction:verifiedTransactionGet{},
	}
	for _, permissions := range [][]string{
		{"merchant_ai:use","sales:read"},
		{"merchant_ai:use","customers:read"},
	} {
		actor:=commands.ActorContext{BusinessID:"business-1",PrincipalID:"principal-1", Permissions: permissions}
		for _,name:=range reader.Available(actor) {
			if name=="transaction_customer_details" {t.Fatal("cross-permission tool leaked into discovery")}
		}
		_,err:=reader.Read(context.Background(),actor,"business-1",
			DiscoveryCall{Name:"transaction_customer_details",RecordID:"transaction-1"})
		if !errors.Is(err,ErrReadDenied) && !errors.Is(err,ErrInvalidReadRequest) {
			t.Fatalf("both permissions must be enforced, got %v",err)
		}
	}
}

func TestNativeMerchantCanFollowOrderToVerifiedCustomerAndItems(t *testing.T) {
	registry, err:=contracts.NewMerchantAssistRegistry()
	if err!=nil {t.Fatal(err)}
	model:=&nativeModelStub{replies:[]ToolCompletion{
		nativeCall("list","list_transactions","{}"),
		nativeCall("details","transaction_customer_details",`{"record_id":"transaction-1"}`),
		nativeAnswer("العميل أحمد طلب قطعتين قهوة عربية حسب تفاصيل الطلب المسجلة."),
	}}
	actor:=commands.ActorContext{BusinessID:"business-1",PrincipalID:"principal-1",
		Permissions:[]string{"merchant_ai:use","sales:read","customers:read"}}
	assistant:=&Assistant{Registry:registry,Native:model,Sanitizer:ConservativeSanitizer{},
		Discovery:&DiscoveryReader{Guard:&guardFixture{}, Transactions:verifiedTransactionList{},
			GetTransaction:verifiedTransactionGet{}, GetCustomer:linkedCustomerGet{}}}
	reply,err:=assistant.Assist(context.Background(),AssistRequest{
		Actor:actor,Business:"business-1",ProjectionID:"auto.v1",
		Task:contracts.TaskExplain,Question:"بيانات العميل للطلب الناجح وإيش كان طلبه",
	})
	if err!=nil {t.Fatal(err)}
	if len(model.seen)!=3 || !strings.Contains(reply.Answer,"أحمد") {
		t.Fatalf("failed verified multi-tool continuation: %+v",reply)
	}
	lastHistory:=model.seen[2]
	foundCustomerAndItem:=false
	for _, msg:=range lastHistory {
		if msg.Role=="tool" && strings.Contains(msg.Content,"قهوة عربية") &&
			strings.Contains(msg.Content,"customer-1") {
			foundCustomerAndItem=true
		}
	}
	if !foundCustomerAndItem {t.Fatal("joined order/identity evidence did not reach model")}
}
