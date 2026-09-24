package handlers

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

type fakeMerchantAIChatService struct {
	result commands.MerchantAIChatResult
	err    error
	last   commands.MerchantAIChatCommand
}

func (f *fakeMerchantAIChatService) Handle(_ context.Context, cmd commands.MerchantAIChatCommand) (commands.MerchantAIChatResult, error) {
	f.last = cmd
	if f.err != nil {
		return commands.MerchantAIChatResult{}, f.err
	}
	return f.result, nil
}

func TestChatWithMerchantAI_HandlerSuccess(t *testing.T) {
	fakeSvc := &fakeMerchantAIChatService{
		result: commands.MerchantAIChatResult{
			Message:   "تم إضافة الصنف بنجاح.",
			Action:    "answer",
			SessionID: "session-abc",
		},
	}
	deps := Dependencies{
		Scope: fakeScope{
			actor: commands.ActorContext{
				PrincipalID: "user-123",
				BusinessID:  "biz-123",
				Role:        "owner",
			},
		},
		ChatWithMerchantAI: fakeSvc,
	}
	server := NewServer(deps)

	in := &contract.MerchantAIChatInput{
		BusinessPath: contract.BusinessPath{
			BusinessID: "biz-123",
		},
		Body: contract.MerchantAIChatRequest{
			Message: "ضيف قميص رجالي بـ 5000",
		},
	}

	out, err := server.ChatWithMerchantAI(context.Background(), in)
	if err != nil {
		t.Fatalf("ChatWithMerchantAI error = %v", err)
	}
	if out.Body.Data.Message != "تم إضافة الصنف بنجاح." {
		t.Errorf("unexpected response message: %s", out.Body.Data.Message)
	}
	if out.Body.Data.SessionID == nil || *out.Body.Data.SessionID != "session-abc" {
		t.Errorf("unexpected session ID: %v", out.Body.Data.SessionID)
	}
	if fakeSvc.last.Message != "ضيف قميص رجالي بـ 5000" {
		t.Errorf("unexpected command message: %s", fakeSvc.last.Message)
	}
}
