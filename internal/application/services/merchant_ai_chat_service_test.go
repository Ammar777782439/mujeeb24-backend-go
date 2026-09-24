package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

type mockMerchantAISessionRepo struct {
	sessions map[string]ports.MerchantAISessionRecord
	messages map[string][]ports.MerchantAIMessageRecord
	touchLog []string
}

func newMockMerchantAISessionRepo() *mockMerchantAISessionRepo {
	return &mockMerchantAISessionRepo{
		sessions: make(map[string]ports.MerchantAISessionRecord),
		messages: make(map[string][]ports.MerchantAIMessageRecord),
	}
}

func (m *mockMerchantAISessionRepo) CreateSession(_ context.Context, session ports.MerchantAISessionRecord) (ports.MerchantAISessionRecord, error) {
	m.sessions[session.ID] = session
	return session, nil
}

type errNotFound struct{ msg string }

func (e errNotFound) Error() string     { return e.msg }
func (e errNotFound) ErrorKind() string { return "not_found" }

func (m *mockMerchantAISessionRepo) GetSession(_ context.Context, businessID, principalID, sessionID string) (ports.MerchantAISessionRecord, error) {
	s, ok := m.sessions[sessionID]
	if !ok || s.BusinessID != businessID || s.PrincipalID != principalID {
		return ports.MerchantAISessionRecord{}, errNotFound{msg: "not_found"}
	}
	return s, nil
}

func (m *mockMerchantAISessionRepo) AppendMessage(_ context.Context, draft ports.MerchantAIMessageDraft) (ports.MerchantAIMessageRecord, error) {
	rec := ports.MerchantAIMessageRecord{
		ID:         draft.ID,
		BusinessID: draft.BusinessID,
		SessionID:  draft.SessionID,
		SenderType: draft.SenderType,
		Text:       draft.Text,
		CreatedAt:  draft.CreatedAt,
	}
	m.messages[draft.SessionID] = append(m.messages[draft.SessionID], rec)
	return rec, nil
}

func (m *mockMerchantAISessionRepo) ListRecentMessages(_ context.Context, businessID, sessionID string, limit int) ([]ports.MerchantAIMessageRecord, error) {
	msgs := m.messages[sessionID]
	if len(msgs) <= limit {
		return msgs, nil
	}
	return msgs[len(msgs)-limit:], nil
}

func (m *mockMerchantAISessionRepo) TouchSession(_ context.Context, businessID, sessionID string, updatedAt time.Time) error {
	m.touchLog = append(m.touchLog, sessionID)
	if s, ok := m.sessions[sessionID]; ok {
		s.UpdatedAt = updatedAt
		m.sessions[sessionID] = s
	}
	return nil
}

type mockMerchantAIRuntime struct {
	output ports.MerchantAIChatOutput
	err    error
}

func (m *mockMerchantAIRuntime) Chat(_ context.Context, _ ports.MerchantAIChatInput) (ports.MerchantAIChatOutput, error) {
	if m.err != nil {
		return ports.MerchantAIChatOutput{}, m.err
	}
	return m.output, nil
}

type mockTxManager struct{}

func (m mockTxManager) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestMerchantAIChatService_NewSession(t *testing.T) {
	repo := newMockMerchantAISessionRepo()
	aiRuntime := &mockMerchantAIRuntime{
		output: ports.MerchantAIChatOutput{
			Action:       "answer",
			ResponseText: "أهلاً بك! تم استلام طلبك.",
		},
	}
	svc := services.NewMerchantAIChatService(repo, aiRuntime, mockTxManager{})

	cmd := commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "biz-1",
				PrincipalID: "user-1",
				Role:        "owner",
			},
		},
		Message: "ضيف قميص رجالي بـ 5000",
	}

	res, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if res.SessionID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if res.Message != "أهلاً بك! تم استلام طلبك." {
		t.Errorf("unexpected message: %s", res.Message)
	}
	if len(repo.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(repo.sessions))
	}
	if len(repo.messages[res.SessionID]) != 2 {
		t.Errorf("expected 2 messages (merchant + assistant), got %d", len(repo.messages[res.SessionID]))
	}
}

func TestMerchantAIChatService_Validation(t *testing.T) {
	repo := newMockMerchantAISessionRepo()
	svc := services.NewMerchantAIChatService(repo, nil, mockTxManager{})

	_, err := svc.Handle(context.Background(), commands.MerchantAIChatCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  "",
				PrincipalID: "user-1",
			},
		},
		Message: "مرحبا",
	})
	if err == nil {
		t.Fatal("expected validation error for empty business_id")
	}
}
