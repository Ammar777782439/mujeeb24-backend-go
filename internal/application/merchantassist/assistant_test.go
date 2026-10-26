package merchantassist

import (
	"context"
	"errors"
	"testing"
	"time"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

type modelStub struct {
	calls   int
	payload string
}

func (m *modelStub) Generate(_ context.Context, _, p string) (string, error) {
	m.calls++
	m.payload = p
	return "البيانات المتاحة محدودة، وتحتاج مراجعتك.", nil
}
func TestAssistantRejectsUnknownProjectionWithoutCallingModel(t *testing.T) {
	r, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	m := &modelStub{}
	agent := &Assistant{Registry: r, Generator: m, Sanitizer: ConservativeSanitizer{}}
	_, e := agent.Assist(context.Background(), AssistRequest{ProjectionID: "unknown", Question: "هل تم الطلب", Task: contracts.TaskExplain, Business: "business-1"})
	if !errors.Is(e, contracts.ErrUnknownProjection) || m.calls != 0 {
		t.Fatalf("unregistered SQL read admitted: %v", e)
	}
}

func TestAssistantNoPublishedPolicyReturnsSourceExplanationWithoutProvider(t *testing.T) {
	registry, err := contracts.NewMerchantAssistRegistry()
	if err != nil {
		t.Fatal(err)
	}
	model := &modelStub{}
	assistant := &Assistant{
		Registry: registry,
		Generator: model,
		Sanitizer: ConservativeSanitizer{},
		Published: &PublishedReader{
			Policies: &policyFixture{},
			AIGuard: &guardFixture{},
			Now: func() time.Time { return readNow },
		},
	}
	response, err := assistant.Assist(context.Background(), AssistRequest{
		Actor: readerActor(), Business: "business-1",
		ProjectionID: "published_policy.v1", Task: contracts.TaskExplain,
		Search: "استرجاع", Question: "اشرح لي",
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 0 || !response.ReviewRequired ||
		!strings.Contains(response.Answer, "لم أجد سياسات منشورة") ||
		!strings.Contains(string(response.Evidence), "no_matching_published_records") {
		t.Fatalf("no evidence must not call provider or invent published policies: %+v", response)
	}
}
