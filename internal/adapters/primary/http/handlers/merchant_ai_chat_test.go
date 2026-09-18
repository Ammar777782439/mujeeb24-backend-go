package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/google/uuid"
)

type fakeMerchantAIChatHandler struct {
	command commands.MerchantAIChatCommand
	result  commands.MerchantAIChatResult
	err     error
}

func (f *fakeMerchantAIChatHandler) Handle(_ context.Context, cmd commands.MerchantAIChatCommand) (commands.MerchantAIChatResult, error) {
	f.command = cmd
	if f.err != nil {
		return commands.MerchantAIChatResult{}, f.err
	}
	return f.result, nil
}

func TestChatWithMerchantAI_Success(t *testing.T) {
	sessionID := uuid.New()
	handler := &fakeMerchantAIChatHandler{
		result: commands.MerchantAIChatResult{
			Message:   "تمت إضافة المنتج بنجاح",
			Action:    "answer",
			SessionID: sessionID.String(),
		},
	}
	server := NewServer(Dependencies{
		Scope: fakeScope{actor: commands.ActorContext{
			BusinessID:  "00000000-0000-0000-0000-000000000001",
			PrincipalID: "00000000-0000-0000-0000-000000000100",
			Role:        "owner",
		}},
		ChatWithMerchantAI: handler,
	})

	_, mux := contract.BuildAPIWithHandlers(server)

	reqBody := `{"message":"أضف قهوة كولومبية بسعر 45 ريال","session_id":"` + sessionID.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/ai/chat", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	mux.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", res.Code, res.Body.String())
	}

	var resp struct {
		Data struct {
			Message   string  `json:"message"`
			Action    string  `json:"action"`
			SessionID *string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Data.Message != "تمت إضافة المنتج بنجاح" {
		t.Errorf("expected message 'تمت إضافة المنتج بنجاح', got %q", resp.Data.Message)
	}
	if resp.Data.Action != "answer" {
		t.Errorf("expected action 'answer', got %q", resp.Data.Action)
	}
	if resp.Data.SessionID == nil || *resp.Data.SessionID != sessionID.String() {
		t.Errorf("expected session_id %s, got %#v", sessionID.String(), resp.Data.SessionID)
	}

	// Verify command received
	if handler.command.Meta.Actor.BusinessID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected business_id %s, got %s", "00000000-0000-0000-0000-000000000001", handler.command.Meta.Actor.BusinessID)
	}
	if handler.command.Meta.Actor.PrincipalID != "00000000-0000-0000-0000-000000000100" {
		t.Errorf("expected principal_id %s, got %s", "00000000-0000-0000-0000-000000000100", handler.command.Meta.Actor.PrincipalID)
	}
	if handler.command.Message != "أضف قهوة كولومبية بسعر 45 ريال" {
		t.Errorf("expected message 'أضف قهوة كولومبية بسعر 45 ريال', got %q", handler.command.Message)
	}
	if handler.command.SessionID == nil || *handler.command.SessionID != sessionID.String() {
		t.Errorf("expected session_id %v, got %v", sessionID.String(), handler.command.SessionID)
	}
}

func TestChatWithMerchantAI_ValidationAndScope(t *testing.T) {
	t.Run("scope mismatch", func(t *testing.T) {
		server := NewServer(Dependencies{
			Scope: fakeScope{actor: commands.ActorContext{
				BusinessID:  "00000000-0000-0000-0000-000000000002",
				PrincipalID: "00000000-0000-0000-0000-000000000100",
				Role:        "owner",
			}},
			ChatWithMerchantAI: &fakeMerchantAIChatHandler{},
		})

		_, mux := contract.BuildAPIWithHandlers(server)

		reqBody := `{"message":"مرحبا"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/ai/chat", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()

		mux.ServeHTTP(res, req)

		if res.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden on scope mismatch, got %d: %s", res.Code, res.Body.String())
		}
	})

	t.Run("empty message rejected", func(t *testing.T) {
		server := NewServer(Dependencies{
			Scope: fakeScope{actor: commands.ActorContext{
				BusinessID:  "00000000-0000-0000-0000-000000000001",
				PrincipalID: "00000000-0000-0000-0000-000000000100",
				Role:        "owner",
			}},
			ChatWithMerchantAI: &fakeMerchantAIChatHandler{},
		})

		_, mux := contract.BuildAPIWithHandlers(server)

		reqBody := `{"message":""}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/ai/chat", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()

		mux.ServeHTTP(res, req)

		if res.Code != http.StatusBadRequest && res.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 400 or 422 for empty message, got %d: %s", res.Code, res.Body.String())
		}
	})

	t.Run("session not found mapped to 404", func(t *testing.T) {
		handler := &fakeMerchantAIChatHandler{
			err: appErrors.New(appErrors.CodeNotFound, "merchant ai session not found"),
		}
		server := NewServer(Dependencies{
			Scope: fakeScope{actor: commands.ActorContext{
				BusinessID:  "00000000-0000-0000-0000-000000000001",
				PrincipalID: "00000000-0000-0000-0000-000000000100",
				Role:        "owner",
			}},
			ChatWithMerchantAI: handler,
		})

		_, mux := contract.BuildAPIWithHandlers(server)

		sessionID := uuid.New().String()
		reqBody := `{"message":"تعديل المنتج","session_id":"` + sessionID + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/ai/chat", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()

		mux.ServeHTTP(res, req)

		if res.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got %d: %s", res.Code, res.Body.String())
		}
	})
}
