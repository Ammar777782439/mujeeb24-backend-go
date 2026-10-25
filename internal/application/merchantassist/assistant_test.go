package merchantassist

import (
	"context"
	"errors"
	"testing"

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
