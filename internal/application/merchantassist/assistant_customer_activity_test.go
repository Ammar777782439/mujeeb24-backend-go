package merchantassist

import (
	"context"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type linkedCustomerList struct{}

func (linkedCustomerList) Handle(_ context.Context, _ queries.ListCustomersQuery) (commands.ListResult[commands.CustomerView], error) {
	return commands.ListResult[commands.CustomerView]{Items: []commands.CustomerView{{
		ID: "customer-1", BusinessID: "business-1", DisplayName: "أحمد", Status: "active",
	}}}, nil
}

type linkedCustomerGet struct{}

func (linkedCustomerGet) Handle(_ context.Context, _ queries.GetCustomerQuery) (commands.CustomerView, error) {
	return commands.CustomerView{ID: "customer-1", BusinessID: "business-1", DisplayName: "أحمد", Status: "active"}, nil
}

type linkedCustomerConversations struct{}

func (linkedCustomerConversations) Handle(_ context.Context, _ queries.ListCustomerConversationsQuery) (commands.ListResult[commands.ConversationView], error) {
	return commands.ListResult[commands.ConversationView]{Items: []commands.ConversationView{{
		ID: "conversation-1", BusinessID: "business-1", CustomerID: "customer-1",
		State: "open", Ownership: "human",
	}}}, nil
}

type linkedCustomerTransactions struct{}

func (linkedCustomerTransactions) Handle(_ context.Context, _ queries.ListCustomerTransactionsQuery) (commands.ListResult[commands.TransactionView], error) {
	return commands.ListResult[commands.TransactionView]{Items: []commands.TransactionView{{
		ID: "transaction-1", BusinessID: "business-1", CustomerID: "customer-1", State: "draft",
	}}}, nil
}

func TestConversationalAssistantCanFollowCustomerToRelatedRecords(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &adviceModel{answers: []string{
		`{"calls":[{"name":"find_customers","search":"أحمد"}],"inspect_customer_activity":true}`,
		"أحمد عنده محادثة مفتوحة وطلب مسودة. راجع المحادثة أولًا.",
	}}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1", Role: "member",
		Permissions: []string{"merchant_ai:use", "customers:read", "conversations:read", "sales:read"},
	}
	assistant := &Assistant{
		Registry: registry, Generator: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{
			Guard: &guardFixture{}, Customers: linkedCustomerList{}, GetCustomer: linkedCustomerGet{},
			CustomerConversations: linkedCustomerConversations{}, CustomerTransactions: linkedCustomerTransactions{},
		},
	}
	reply, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "إيش وضع أحمد وليش ما كمل طلبه؟",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.seen) != 2 || !strings.Contains(model.seen[1], "conversation-1") ||
		!strings.Contains(model.seen[1], "transaction-1") || !strings.Contains(model.seen[1], "customer-1") {
		t.Fatalf("model did not receive linked authorized identifiers, calls=%d", len(model.seen))
	}
	if !strings.Contains(reply.Answer, "محادثة") || !strings.Contains(string(reply.Evidence), "conversation_id") {
		t.Fatalf("missing merchant-facing navigation: %+v", reply)
	}
}

func TestCustomerActivityRespectsRelatedPermissions(t *testing.T) {
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1", Role: "member",
		Permissions: []string{"merchant_ai:use", "customers:read"},
	}
	reader := &DiscoveryReader{
		Guard: &guardFixture{}, GetCustomer: linkedCustomerGet{},
		CustomerConversations: linkedCustomerConversations{}, CustomerTransactions: linkedCustomerTransactions{},
	}
	response, err := reader.Read(context.Background(), actor, "business-1", DiscoveryCall{
		Name: "customer_activity", RecordID: "customer-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := response.Rows.(map[string]any)
	if !ok {
		t.Fatal("missing customer evidence")
	}
	if _, exists := rows["conversations"]; exists {
		t.Fatal("conversation data leaked to customer-only user")
	}
	if _, exists := rows["transactions"]; exists {
		t.Fatal("sales data leaked to customer-only user")
	}
}
