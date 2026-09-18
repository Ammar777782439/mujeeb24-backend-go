package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mockMerchantAISessionRepo struct {
	mu       sync.Mutex
	sessions map[string]ports.MerchantAISessionRecord
	messages map[string][]ports.MerchantAIMessageRecord
	touched  map[string]time.Time
	touchErr error
}

func newMockMerchantAISessionRepo() *mockMerchantAISessionRepo {
	return &mockMerchantAISessionRepo{
		sessions: make(map[string]ports.MerchantAISessionRecord),
		messages: make(map[string][]ports.MerchantAIMessageRecord),
		touched:  make(map[string]time.Time),
	}
}

type errNotFound struct{ msg string }

func (e errNotFound) Error() string     { return e.msg }
func (e errNotFound) ErrorKind() string { return "not_found" }

func (m *mockMerchantAISessionRepo) CreateSession(ctx context.Context, session ports.MerchantAISessionRecord) (ports.MerchantAISessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := session.BusinessID + ":" + session.PrincipalID + ":" + session.ID
	m.sessions[key] = session
	return session, nil
}

func (m *mockMerchantAISessionRepo) GetSession(ctx context.Context, businessID, principalID, sessionID string) (ports.MerchantAISessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := businessID + ":" + principalID + ":" + sessionID
	rec, ok := m.sessions[key]
	if !ok {
		return ports.MerchantAISessionRecord{}, errNotFound{msg: "session not found"}
	}
	return rec, nil
}

func (m *mockMerchantAISessionRepo) AppendMessage(ctx context.Context, draft ports.MerchantAIMessageDraft) (ports.MerchantAIMessageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := draft.BusinessID + ":" + draft.SessionID
	rec := ports.MerchantAIMessageRecord{
		ID:         draft.ID,
		BusinessID: draft.BusinessID,
		SessionID:  draft.SessionID,
		SenderType: draft.SenderType,
		Text:       draft.Text,
		CreatedAt:  draft.CreatedAt,
	}
	m.messages[key] = append(m.messages[key], rec)
	return rec, nil
}

func (m *mockMerchantAISessionRepo) ListRecentMessages(ctx context.Context, businessID, sessionID string, limit int) ([]ports.MerchantAIMessageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := businessID + ":" + sessionID
	msgs := m.messages[key]
	if len(msgs) <= limit {
		res := make([]ports.MerchantAIMessageRecord, len(msgs))
		copy(res, msgs)
		return res, nil
	}
	start := len(msgs) - limit
	res := make([]ports.MerchantAIMessageRecord, limit)
	copy(res, msgs[start:])
	return res, nil
}

func (m *mockMerchantAISessionRepo) TouchSession(ctx context.Context, businessID, sessionID string, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.touchErr != nil {
		return m.touchErr
	}
	m.touched[businessID+":"+sessionID] = updatedAt
	return nil
}

// Test 1: New session creation when session_id is absent
func TestMerchantAIChatServiceNewSession(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	decisionRepo := &fakeDecisionRepository{}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	idCounter := 0
	ids := []string{"sess-100", "msg-user-1", "msg-ai-1", "dec-1"}

	aiRuntime := fakeAIRuntime{
		proposal: ports.AIDecisionProposal{
			IntentBase:      "catalog_authoring",
			RequestedAction: "answer",
			ResponseText:    "تمت إضافة المنتج بنجاح.",
			SchemaVersion:   1,
		},
	}

	service := NewMerchantAIChatService(sessionRepo, aiRuntime, decisionRepo, mockTransactionManager{})
	service.Now = func() time.Time { return now }
	service.NewID = func() string {
		if idCounter < len(ids) {
			val := ids[idCounter]
			idCounter++
			return val
		}
		return "gen-id"
	}

	res, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
				Role:        "owner",
			},
		},
		Message: "أضف عطر ديور سوفاج",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.SessionID != "sess-100" {
		t.Fatalf("expected sessionID sess-100, got %s", res.SessionID)
	}
	if res.Message != "تمت إضافة المنتج بنجاح." {
		t.Fatalf("expected assistant message, got %s", res.Message)
	}
	if res.Action != "answer" {
		t.Fatalf("expected action answer, got %s", res.Action)
	}

	// Verify messages saved
	msgs := sessionRepo.messages["biz-1:sess-100"]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (merchant + assistant), got %d", len(msgs))
	}
	if msgs[0].SenderType != "merchant" || msgs[0].Text != "أضف عطر ديور سوفاج" {
		t.Fatalf("unexpected merchant message: %+v", msgs[0])
	}
	if msgs[1].SenderType != "assistant" || msgs[1].Text != "تمت إضافة المنتج بنجاح." {
		t.Fatalf("unexpected assistant message: %+v", msgs[1])
	}

	// Verify decision saved with ConversationID == nil and ExecutionReference == sessionID
	if decisionRepo.record.ID != "dec-1" || decisionRepo.record.BusinessID != "biz-1" {
		t.Fatalf("unexpected decision record: %+v", decisionRepo.record)
	}
	if decisionRepo.record.ConversationID != nil {
		t.Fatalf("expected ConversationID to be nil for Merchant AI decision, got %v", *decisionRepo.record.ConversationID)
	}
	if decisionRepo.record.ExecutionReference == nil || *decisionRepo.record.ExecutionReference != "sess-100" {
		t.Fatalf("expected ExecutionReference to be sess-100, got %v", decisionRepo.record.ExecutionReference)
	}
}

// Test 2: Continuing existing session with multi-turn context
func TestMerchantAIChatServiceExistingSessionMultiTurn(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	decisionRepo := &fakeDecisionRepository{}
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)

	// Pre-create session and initial turn
	_, _ = sessionRepo.CreateSession(context.Background(), ports.MerchantAISessionRecord{
		ID:          "sess-100",
		BusinessID:  "biz-1",
		PrincipalID: "user-1",
		CreatedAt:   now.Add(-time.Minute),
		UpdatedAt:   now.Add(-time.Minute),
	})
	_, _ = sessionRepo.AppendMessage(context.Background(), ports.MerchantAIMessageDraft{
		ID:         "msg-1",
		BusinessID: "biz-1",
		SessionID:  "sess-100",
		SenderType: "merchant",
		Text:       "أضف عطر ديور سوفاج",
		CreatedAt:  now.Add(-time.Minute),
	})
	_, _ = sessionRepo.AppendMessage(context.Background(), ports.MerchantAIMessageDraft{
		ID:         "msg-2",
		BusinessID: "biz-1",
		SessionID:  "sess-100",
		SenderType: "assistant",
		Text:       "كم السعر؟",
		CreatedAt:  now.Add(-30 * time.Second),
	})

	var receivedContext *ports.AIContext
	customAIRuntime := mockAuthoringAIRuntime{
		decideFn: func(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
			receivedContext = input.Context
			return ports.AIDecisionProposal{
				IntentBase:      "catalog_authoring",
				RequestedAction: "answer",
				ResponseText:    "تمت إضافة عطر ديور سوفاج بسعر 25000 ريال.",
				SchemaVersion:   1,
			}, nil
		},
	}

	service := NewMerchantAIChatService(sessionRepo, customAIRuntime, decisionRepo, mockTransactionManager{})
	service.Now = func() time.Time { return now }

	sessID := "sess-100"
	res, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
				Role:        "owner",
			},
		},
		SessionID: &sessID,
		Message:   "25000 ريال",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.SessionID != "sess-100" {
		t.Fatalf("expected sessionID sess-100, got %s", res.SessionID)
	}
	if receivedContext == nil || len(receivedContext.RecentMessages) != 3 {
		t.Fatalf("expected 3 recent messages in context, got %+v", receivedContext)
	}
	if receivedContext.RecentMessages[0].Text != "أضف عطر ديور سوفاج" ||
		receivedContext.RecentMessages[1].Text != "كم السعر؟" ||
		receivedContext.RecentMessages[2].Text != "25000 ريال" {
		t.Fatalf("unexpected recent messages: %+v", receivedContext.RecentMessages)
	}

	// Verify total 4 messages in session
	msgs := sessionRepo.messages["biz-1:sess-100"]
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages in session, got %d", len(msgs))
	}
}

// Test 3: Session ownership defense (different principal -> not found)
func TestMerchantAIChatServiceCrossPrincipalDefense(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	_, _ = sessionRepo.CreateSession(context.Background(), ports.MerchantAISessionRecord{
		ID:          "sess-user-1",
		BusinessID:  "biz-1",
		PrincipalID: "user-1",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})

	service := NewMerchantAIChatService(sessionRepo, fakeAIRuntime{}, &fakeDecisionRepository{}, mockTransactionManager{})

	sessID := "sess-user-1"
	_, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-2", // Different principal in same business
				Role:        "admin",
			},
		},
		SessionID: &sessID,
		Message:   "hello",
	})
	if err == nil {
		t.Fatal("expected error for cross-principal session access, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

// Test 4: Tenant isolation defense (different business -> not found)
func TestMerchantAIChatServiceCrossTenantDefense(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	_, _ = sessionRepo.CreateSession(context.Background(), ports.MerchantAISessionRecord{
		ID:          "sess-biz-1",
		BusinessID:  "biz-1",
		PrincipalID: "user-1",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})

	service := NewMerchantAIChatService(sessionRepo, fakeAIRuntime{}, &fakeDecisionRepository{}, mockTransactionManager{})

	sessID := "sess-biz-1"
	_, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-2", // Different business
				PrincipalID: "user-1",
				Role:        "owner",
			},
		},
		SessionID: &sessID,
		Message:   "hello",
	})
	if err == nil {
		t.Fatal("expected error for cross-tenant session access, got nil")
	}
}

// Test 5: AI Failure after merchant message persistence (message is preserved, error returned)
func TestMerchantAIChatServiceAIRuntimeFailurePreservesMerchantMessage(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	failingAI := mockAuthoringAIRuntime{
		decideFn: func(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
			return ports.AIDecisionProposal{}, errors.New("external provider timeout")
		},
	}

	service := NewMerchantAIChatService(sessionRepo, failingAI, &fakeDecisionRepository{}, mockTransactionManager{})
	service.NewID = func() string { return "sess-fail" }

	_, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
			},
		},
		Message: "أريد إضافة منتج",
	})
	if err == nil {
		t.Fatal("expected error on AI failure, got nil")
	}

	// Verify merchant message was persisted in Phase 1
	msgs := sessionRepo.messages["biz-1:sess-fail"]
	if len(msgs) != 1 {
		t.Fatalf("expected merchant message to be preserved, got %d messages", len(msgs))
	}
	if msgs[0].SenderType != "merchant" || msgs[0].Text != "أريد إضافة منتج" {
		t.Fatalf("unexpected message: %+v", msgs[0])
	}
}

// Test 6: AI Clarification Action
func TestMerchantAIChatServiceClarificationAction(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	clarificationAI := fakeAIRuntime{
		proposal: ports.AIDecisionProposal{
			IntentBase:      "catalog_authoring",
			RequestedAction: "ask_clarification",
			ResponseText:    "ما هو سعر العطر؟",
			SchemaVersion:   1,
		},
	}

	service := NewMerchantAIChatService(sessionRepo, clarificationAI, &fakeDecisionRepository{}, mockTransactionManager{})
	res, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
			},
		},
		Message: "أضف عطر سوفاج",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Action != "ask_clarification" {
		t.Fatalf("expected action ask_clarification, got %s", res.Action)
	}
	if res.Message != "ما هو سعر العطر؟" {
		t.Fatalf("expected clarification question, got %s", res.Message)
	}
}

// Test 7: Failure in AIDecisionRepository.CreateProposed fails Tx2 and returns error
func TestMerchantAIChatService_AuditDecisionFailure_FailsTransaction(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	decisionRepo := &fakeDecisionRepository{
		err: errors.New("ai decision persistence failed"),
	}

	aiRuntime := fakeAIRuntime{
		proposal: ports.AIDecisionProposal{
			IntentBase:      "catalog_authoring",
			RequestedAction: "answer",
			ResponseText:    "تمت العملية",
			SchemaVersion:   1,
		},
	}

	service := NewMerchantAIChatService(sessionRepo, aiRuntime, decisionRepo, mockTransactionManager{})
	_, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
			},
		},
		Message: "أضف منتج جديد",
	})
	if err == nil {
		t.Fatal("expected error when AIDecisionRepository.CreateProposed fails, got nil")
	}
	if !strings.Contains(err.Error(), "ai decision persistence failed") {
		t.Fatalf("expected 'ai decision persistence failed' error, got %v", err)
	}
}

// Test 8: Failure in TouchSession fails Tx2 and returns error
func TestMerchantAIChatService_TouchSessionFailure_FailsTransaction(t *testing.T) {
	sessionRepo := newMockMerchantAISessionRepo()
	sessionRepo.touchErr = errors.New("touch session db error")

	aiRuntime := fakeAIRuntime{
		proposal: ports.AIDecisionProposal{
			IntentBase:      "catalog_authoring",
			RequestedAction: "answer",
			ResponseText:    "تمت العملية",
			SchemaVersion:   1,
		},
	}

	service := NewMerchantAIChatService(sessionRepo, aiRuntime, &fakeDecisionRepository{}, mockTransactionManager{})
	_, err := service.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
			},
		},
		Message: "أضف منتج جديد",
	})
	if err == nil {
		t.Fatal("expected error when TouchSession fails, got nil")
	}
	if !strings.Contains(err.Error(), "touch session db error") {
		t.Fatalf("expected 'touch session db error' error, got %v", err)
	}
}
