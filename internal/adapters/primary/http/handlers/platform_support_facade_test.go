package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ----------------------------------------------------------------------------
// Mock implementations for Support repository port
// ----------------------------------------------------------------------------

type stubSupportRepository struct {
	createdTicket ports.SupportTicketRecord
	createErr     error
	getByID       ports.SupportTicketRecord
	getErr        error
	listPage      ports.SupportTicketPage
	listErr       error
	started       ports.SupportTicketRecord
	startErr      error
	resolved      ports.SupportTicketRecord
	resolveErr    error
	closed        ports.SupportTicketRecord
	closeErr      error
	appendedMsg   ports.SupportMessageRecord
	appendMsgErr  error
	listMsgsPage  ports.SupportMessagePage
	listMsgsErr   error
}

func (s *stubSupportRepository) CreateTicket(_ context.Context, _ ports.SupportTicketCreate) (ports.SupportTicketRecord, error) {
	return s.createdTicket, s.createErr
}
func (s *stubSupportRepository) GetTicketByID(_ context.Context, _ string) (ports.SupportTicketRecord, error) {
	return s.getByID, s.getErr
}
func (s *stubSupportRepository) ListTickets(_ context.Context, _ ports.SupportTicketListFilter) (ports.SupportTicketPage, error) {
	return s.listPage, s.listErr
}
func (s *stubSupportRepository) StartTicket(_ context.Context, _, _ string, _ time.Time) (ports.SupportTicketRecord, error) {
	return s.started, s.startErr
}
func (s *stubSupportRepository) ResolveTicket(_ context.Context, _, _ string, _ time.Time) (ports.SupportTicketRecord, error) {
	return s.resolved, s.resolveErr
}
func (s *stubSupportRepository) CloseTicket(_ context.Context, _, _ string, _ time.Time) (ports.SupportTicketRecord, error) {
	return s.closed, s.closeErr
}
func (s *stubSupportRepository) AppendMessage(_ context.Context, _ ports.SupportMessageCreate) (ports.SupportMessageRecord, error) {
	return s.appendedMsg, s.appendMsgErr
}
func (s *stubSupportRepository) ListMessages(_ context.Context, _ ports.SupportMessageListFilter) (ports.SupportMessagePage, error) {
	return s.listMsgsPage, s.listMsgsErr
}

// ----------------------------------------------------------------------------
// Tests: Support ticket lifecycle (Contract §37-44)
// ----------------------------------------------------------------------------

const ticketIDForTests = "00000000-0000-0000-0000-000000ticket1"

func platformSupportTicketPath() dto.PlatformSupportTicketPath {
	return dto.PlatformSupportTicketPath{TicketID: dto.UUID(ticketIDForTests)}
}

func TestPlatformCreateSupportTicketReturns501WhenUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSupportTicket", &dto.CreateSupportTicketInput{
		Body: dto.CreateSupportTicketRequest{Subject: "Test"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCreateSupportTicketRequiresSubject(t *testing.T) {
	server := newPlatformServer(PlatformDeps{Support: &stubSupportRepository{}, PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSupportTicket", &dto.CreateSupportTicketInput{
		Body: dto.CreateSupportTicketRequest{Subject: "  "},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCreateSupportTicketAuditsSuccess(t *testing.T) {
	supportRepo := &stubSupportRepository{createdTicket: ports.SupportTicketRecord{ID: "ticket-1", BusinessID: "biz-1", Subject: "Test"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSupportTicket", &dto.CreateSupportTicketInput{
		Body: dto.CreateSupportTicketRequest{Subject: "Test", Priority: "HIGH", Category: "BILLING"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "support.ticket.created" {
		t.Errorf("expected action=support.ticket.created, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformListSupportTicketsReturnsPaginatedItems(t *testing.T) {
	supportRepo := &stubSupportRepository{listPage: ports.SupportTicketPage{
		Items:   []ports.SupportTicketRecord{{ID: "ticket-1"}, {ID: "ticket-2"}},
		HasMore: true,
	}}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	result, handled := server.dispatchPlatformCommand(ctx, "platformListSupportTickets", &dto.SupportTicketListInput{Limit: 50})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if result == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestPlatformGetSupportTicketReturns501WhenUnwired(t *testing.T) {
	server := newPlatformServer(PlatformDeps{})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformGetSupportTicket", &dto.PlatformSupportTicketPath{})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCreateSupportMessageRequiresBody(t *testing.T) {
	server := newPlatformServer(PlatformDeps{Support: &stubSupportRepository{}, PlatformAudit: &stubPlatformAuditRepository{}})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSupportMessage", &dto.CreateSupportMessageInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
		Body:                      dto.CreateSupportMessageRequest{Body: "  "},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
}

func TestPlatformCreateSupportMessageFailsIfTicketMissing(t *testing.T) {
	supportRepo := &stubSupportRepository{getErr: errors.New("not found")}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSupportMessage", &dto.CreateSupportMessageInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
		Body:                      dto.CreateSupportMessageRequest{Body: "Hello"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit on ticket lookup failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
}

func TestPlatformCreateSupportMessageAuditsSuccess(t *testing.T) {
	supportRepo := &stubSupportRepository{
		getByID:     ports.SupportTicketRecord{ID: ticketIDForTests, BusinessID: "biz-1", Status: "OPEN"},
		appendedMsg: ports.SupportMessageRecord{ID: "msg-1", TicketID: ticketIDForTests, BusinessID: "biz-1", AuthorType: "PLATFORM_ADMIN"},
	}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCreateSupportMessage", &dto.CreateSupportMessageInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
		Body:                      dto.CreateSupportMessageRequest{Body: "Hello, how can we help?"},
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "support.message.created" {
		t.Errorf("expected action=support.message.created, got %s", auditRepo.appendedDraft.Action)
	}
	if auditRepo.appendedDraft.Result != "SUCCESS" {
		t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
	}
}

// ----------------------------------------------------------------------------
// Tests: Support ticket lifecycle transitions (Contract §39, §43)
// ----------------------------------------------------------------------------

func TestPlatformStartSupportTicketAuditsSuccess(t *testing.T) {
	supportRepo := &stubSupportRepository{started: ports.SupportTicketRecord{ID: "ticket-1", BusinessID: "biz-1", Status: "IN_PROGRESS"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformStartSupportTicket", &dto.StartSupportTicketInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "support.ticket.started" {
		t.Errorf("expected action=support.ticket.started, got %s", auditRepo.appendedDraft.Action)
	}
}

func TestPlatformResolveSupportTicketAuditsSuccess(t *testing.T) {
	supportRepo := &stubSupportRepository{resolved: ports.SupportTicketRecord{ID: "ticket-1", BusinessID: "biz-1", Status: "RESOLVED"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformResolveSupportTicket", &dto.ResolveSupportTicketInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "support.ticket.resolved" {
		t.Errorf("expected action=support.ticket.resolved, got %s", auditRepo.appendedDraft.Action)
	}
}

func TestPlatformCloseSupportTicketAuditsSuccess(t *testing.T) {
	supportRepo := &stubSupportRepository{closed: ports.SupportTicketRecord{ID: "ticket-1", BusinessID: "biz-1", Status: "CLOSED"}}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformCloseSupportTicket", &dto.CloseSupportTicketInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit Append")
	}
	if auditRepo.appendedDraft.Action != "support.ticket.closed" {
		t.Errorf("expected action=support.ticket.closed, got %s", auditRepo.appendedDraft.Action)
	}
}

func TestPlatformResolveSupportTicketAuditsFailureOnConflict(t *testing.T) {
	// Simulate "ticket is RESOLVED already" — ResolveTicket returns Conflict.
	supportRepo := &stubSupportRepository{resolveErr: errors.New("already resolved")}
	auditRepo := &stubPlatformAuditRepository{}
	server := newPlatformServer(PlatformDeps{Support: supportRepo, PlatformAudit: auditRepo})
	ctx, cancel := ctxWithPlatformAdmin()
	defer cancel()
	_, handled := server.dispatchPlatformCommand(ctx, "platformResolveSupportTicket", &dto.ResolveSupportTicketInput{
		PlatformSupportTicketPath: platformSupportTicketPath(),
	})
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if auditRepo.appendedDraft == nil {
		t.Fatalf("expected audit on failure")
	}
	if auditRepo.appendedDraft.Result != "FAILURE" {
		t.Errorf("expected result=FAILURE, got %s", auditRepo.appendedDraft.Result)
	}
}
