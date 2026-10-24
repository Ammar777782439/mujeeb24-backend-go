package merchantcatalogai

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type denyMerchantBusinessAI struct{}

func (denyMerchantBusinessAI) Check(context.Context, string) error {
	return ports.ErrBusinessAIDisabled
}

func TestMerchantAgentRefusesDisabledBusinessBeforeSessionWrite(t *testing.T) {
	agent := &Agent{
		Sessions:       &struct{ SessionStore }{},
		Selector:       &struct{ CatalogSelector }{},
		Business:       &struct{ ports.BusinessRepository }{},
		EntityContract: &struct{ EntityContractProvider }{},
		Authoring:      &struct{ MerchantCatalogAuthoringPort }{},
		AIGuard:        denyMerchantBusinessAI{},
	}
	_, err := agent.HandleTurn(context.Background(), TurnInput{
		BusinessID: "business-1", PrincipalID: "owner-1", Message: "أضف منتجاً",
	})
	if !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("merchant opt-out did not prevent session writing: %v", err)
	}
}

func TestMerchantAIProposalCannotBeExecutedWhileDisabled(t *testing.T) {
	svc := ExecutionService{AIGuard: denyMerchantBusinessAI{}}
	if _, err := svc.Prepare(context.Background(), ExecutionInput{BusinessID: "business-1"}); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("merchant AI proposal preparation was allowed: %v", err)
	}
	if _, err := svc.ExecuteApproved(context.Background(), "business-1", "owner-1", "proposal-1"); !errors.Is(err, ports.ErrBusinessAIDisabled) {
		t.Fatalf("disabled merchant AI proposal was executed: %v", err)
	}
}
