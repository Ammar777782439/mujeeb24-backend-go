package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestCreateCannedReplyNormalizesShortcutAndCreatesTenantScopedRecord(t *testing.T) {
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	repository := &fakeCannedReplyRepository{}
	service := CreateCannedReplyCommandService{CannedReplyService: CannedReplyService{Repository: repository, Transactions: passthroughTransactionManager{}, Now: func() time.Time { return now }, NewID: func() string { return "00000000-0000-0000-0000-000000000901" }}}
	result, err := service.Handle(context.Background(), commands.CreateCannedReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "00000000-0000-0000-0000-000000000001"}}, Title: "  ساعات العمل ", Shortcut: " /Hours ", Body: " نفتح من السبت إلى الخميس "})
	if err != nil {
		t.Fatalf("create canned reply: %v", err)
	}
	if repository.created.Shortcut != "hours" || repository.created.Title != "ساعات العمل" || repository.created.Body != "نفتح من السبت إلى الخميس" || result.CannedReply.Status != "active" || result.CannedReply.ResourceVersion != "1" {
		t.Fatalf("unexpected created reply=%#v result=%#v", repository.created, result)
	}
}

func TestSendCannedReplyUsesDedicatedIdempotencyNamespace(t *testing.T) {
	repository := &fakeCannedReplyRepository{record: ports.CannedReplyRecord{ID: "00000000-0000-0000-0000-000000000902", BusinessID: "00000000-0000-0000-0000-000000000001", Body: "أهلًا، كيف أساعدك؟", Status: "active"}}
	outbound := &fakeOutboundMessageHandler{result: commands.MessageResult{Message: commands.MessageView{ID: "00000000-0000-0000-0000-000000000903", Text: "أهلًا، كيف أساعدك؟"}}}
	service := SendCannedReplyCommandService{Repository: repository, Outbound: outbound}
	_, err := service.Handle(context.Background(), commands.SendCannedReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(repository.record.BusinessID)}, IdempotencyKey: "click-7"}, ConversationID: "00000000-0000-0000-0000-000000000904", CannedReplyID: commands.CannedReplyID(repository.record.ID)})
	if err != nil {
		t.Fatalf("send canned reply: %v", err)
	}
	if outbound.command.Text != repository.record.Body || outbound.command.Meta.IdempotencyKey != "canned-reply:"+repository.record.ID+":click-7" {
		t.Fatalf("unexpected delegated outbound=%#v", outbound.command)
	}
}

func TestSendCannedReplyRejectsArchivedReplyWithoutOutbound(t *testing.T) {
	repository := &fakeCannedReplyRepository{record: ports.CannedReplyRecord{ID: "00000000-0000-0000-0000-000000000905", BusinessID: "00000000-0000-0000-0000-000000000001", Body: "قديم", Status: "archived"}}
	outbound := &fakeOutboundMessageHandler{}
	service := SendCannedReplyCommandService{Repository: repository, Outbound: outbound}
	_, err := service.Handle(context.Background(), commands.SendCannedReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(repository.record.BusinessID)}, IdempotencyKey: "click-8"}, ConversationID: "00000000-0000-0000-0000-000000000906", CannedReplyID: commands.CannedReplyID(repository.record.ID)})
	if err == nil || !hasApplicationCode(err, appErrors.CodeConflict) || outbound.called {
		t.Fatalf("expected archived reply rejection without outbound, err=%v called=%v", err, outbound.called)
	}
}

func TestMarkConversationReadReturnsCurrentMessageCursor(t *testing.T) {
	messageID := "00000000-0000-0000-0000-000000000908"
	readAt := time.Date(2026, 8, 28, 10, 5, 0, 0, time.UTC)
	repository := &fakeReadCursorRepository{cursor: ports.ConversationReadCursor{BusinessID: "00000000-0000-0000-0000-000000000001", ConversationID: "00000000-0000-0000-0000-000000000907", PrincipalID: "00000000-0000-0000-0000-000000000009", LastReadMessageID: &messageID}}
	service := MarkConversationReadService{Repository: repository, Now: func() time.Time { return readAt }}
	result, err := service.Handle(context.Background(), commands.MarkConversationReadCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(repository.cursor.BusinessID), PrincipalID: commands.PrincipalID(repository.cursor.PrincipalID)}}, ConversationID: commands.ConversationID(repository.cursor.ConversationID)})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if result.Status != "read" || result.LastReadMessageID == nil || string(*result.LastReadMessageID) != messageID || !repository.readAt.Equal(readAt) {
		t.Fatalf("unexpected result=%#v cursor=%#v", result, repository)
	}
}

type passthroughTransactionManager struct{}

func (passthroughTransactionManager) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeReadCursorRepository struct {
	cursor ports.ConversationReadCursor
	readAt time.Time
}

func (r *fakeReadCursorRepository) MarkRead(_ context.Context, businessID, conversationID, principalID string, readAt time.Time) (ports.ConversationReadCursor, error) {
	r.readAt = readAt
	if businessID != r.cursor.BusinessID || conversationID != r.cursor.ConversationID || principalID != r.cursor.PrincipalID {
		return ports.ConversationReadCursor{}, errors.New("unexpected read scope")
	}
	return r.cursor, nil
}

type fakeCannedReplyRepository struct {
	record  ports.CannedReplyRecord
	created ports.CannedReplyCreate
}

func (r *fakeCannedReplyRepository) List(context.Context, string, string, int, string) (ports.CannedReplyPage, error) {
	return ports.CannedReplyPage{Items: []ports.CannedReplyRecord{r.record}}, nil
}
func (r *fakeCannedReplyRepository) GetByID(_ context.Context, businessID, cannedReplyID string) (ports.CannedReplyRecord, error) {
	if businessID != r.record.BusinessID || cannedReplyID != r.record.ID {
		return ports.CannedReplyRecord{}, errors.New("canned reply not found")
	}
	return r.record, nil
}
func (r *fakeCannedReplyRepository) Create(_ context.Context, create ports.CannedReplyCreate) (ports.CannedReplyRecord, error) {
	r.created = create
	r.record = ports.CannedReplyRecord{ID: create.ID, BusinessID: create.BusinessID, Title: create.Title, Shortcut: create.Shortcut, Body: create.Body, Status: create.Status, ResourceVersion: 1, CreatedAt: create.CreatedAt, UpdatedAt: create.UpdatedAt}
	return r.record, nil
}
func (r *fakeCannedReplyRepository) Update(context.Context, ports.CannedReplyUpdate) (ports.CannedReplyRecord, error) {
	return r.record, nil
}

type fakeOutboundMessageHandler struct {
	command commands.CreateOutboundMessageCommand
	result  commands.MessageResult
	called  bool
}

func (h *fakeOutboundMessageHandler) Handle(_ context.Context, command commands.CreateOutboundMessageCommand) (commands.MessageResult, error) {
	h.called = true
	h.command = command
	return h.result, nil
}

func hasApplicationCode(err error, expected appErrors.Code) bool {
	var typed *appErrors.Error
	return errors.As(err, &typed) && typed.Code == expected
}
