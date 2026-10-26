package merchantassist

import (
	"context"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type adviceModel struct {
	answers []string
	seen    []string
}

func (m *adviceModel) Generate(_ context.Context, _, prompt string) (string, error) {
	m.seen = append(m.seen, prompt)
	if len(m.answers) == 0 {
		return "", nil
	}
	v := m.answers[0]
	m.answers = m.answers[1:]
	return v, nil
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
	model := &adviceModel{answers: []string{
		`{"calls":[{"name":"dashboard_overview"}]}`,
		"عندك ثلاث محادثات تنتظر الموظف وطلبان يحتاجان المراجعة. ابدأ بمتابعة المحادثات ثم راجع الطلبات.",
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Generator: model, Sanitizer: ConservativeSanitizer{},
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
		t.Fatalf("unexpected response: %+v", response)
	}
	if strings.Contains(string(response.Evidence), "principal-1") {
		t.Fatal("internal actor leaked")
	}
	if !strings.Contains(string(response.Evidence), "dashboard") {
		t.Fatal("expected safe dashboard navigation link")
	}
}

func TestGuidedAdviceRejectsUnauthorizedTool(t *testing.T) {
	registry, _ := contracts.NewMerchantAssistRegistry()
	model := &adviceModel{answers: []string{`{"calls":[{"name":"list_transactions"}]}`}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Generator: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}},
	}
	_, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "اطلع على الطلبات",
	})
	if err != ErrInvalidModelPlan {
		t.Fatalf("expected rejected tool plan, got %v", err)
	}
	if len(model.seen) != 1 {
		t.Fatal("answer should not run after untrusted tool request")
	}
}

func TestGuidanceWithoutRecordDoesNotPretendToKnowBusiness(t *testing.T) {
	registry, _ := contracts.NewMerchantAssistRegistry()
	model := &adviceModel{answers: []string{
		`{"calls":[]}`,
		"أقدر أساعدك تعرف وين تبدأ: اسأل عن وضع المحادثات أو الطلبات.",
	}}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1",
		Permissions: []string{"merchant_ai:use", "dashboard:read"}}
	assistant := &Assistant{
		Registry: registry, Generator: model, Sanitizer: ConservativeSanitizer{},
		Discovery: &DiscoveryReader{Guard: &guardFixture{}, Overview: overviewStub{}},
	}
	v, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: actor, Business: "business-1", ProjectionID: "auto.v1", Task: contracts.TaskExplain,
		Question: "السلام عليكم",
	})
	if err != nil || !strings.Contains(v.Answer, "أساعدك") || len(model.seen) != 2 {
		t.Fatalf("missing useful follow-up: %+v / %v", v, err)
	}
}
