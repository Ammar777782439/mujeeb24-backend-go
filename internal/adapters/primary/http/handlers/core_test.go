package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/middleware"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type fakeScope struct {
	actor commands.ActorContext
	err   error
}

type fakeWebhookHandler struct {
	command commands.IngestWebhookCommand
}

type errorWebhookHandler struct {
	err error
}

func (f errorWebhookHandler) Handle(context.Context, commands.IngestWebhookCommand) (commands.WebhookAcceptedResult, error) {
	return commands.WebhookAcceptedResult{}, f.err
}

func (f *fakeWebhookHandler) Handle(_ context.Context, command commands.IngestWebhookCommand) (commands.WebhookAcceptedResult, error) {
	f.command = command
	return commands.WebhookAcceptedResult{Accepted: true, RequestID: command.RequestID}, nil
}

func (f fakeScope) Resolve(context.Context, commands.BusinessID) (commands.ActorContext, error) {
	return f.actor, f.err
}

type fakeConversationList struct {
	received queries.ListConversationsQuery
}

func (f *fakeConversationList) Handle(_ context.Context, q queries.ListConversationsQuery) (commands.ListResult[commands.ConversationView], error) {
	f.received = q
	return commands.ListResult[commands.ConversationView]{Items: []commands.ConversationView{{ID: "conversation-1", BusinessID: q.Meta.Actor.BusinessID, CustomerID: "customer-1", State: "open", Ownership: "team", AIMode: "assist", ResourceVersion: "v1"}}, NextCursor: "next", HasMore: true}, nil
}

func TestListConversationsMapsDTOToTypedQueryAndScope(t *testing.T) {
	list := &fakeConversationList{}
	server := NewServer(Dependencies{Scope: fakeScope{actor: commands.ActorContext{PrincipalID: "principal-1", BusinessID: "business-1", Role: "agent"}}, ListConversations: list})
	out, err := server.ListConversations(context.Background(), &contract.ConversationListInput{BusinessPath: contract.BusinessPath{BusinessID: "business-1"}, ListQuery: contract.ListQuery{Limit: 10, Cursor: "cursor-1"}, State: "open", Ownership: "team", Channel: "whatsapp", CustomerID: "customer-1"})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(out.Body.Data) != 1 || out.Body.Data[0].ID != "conversation-1" {
		t.Fatalf("unexpected mapped response: %#v", out.Body.Data)
	}
	if list.received.Meta.Actor.BusinessID != "business-1" || list.received.Limit != 10 || list.received.Cursor != "cursor-1" || list.received.CustomerID == nil || *list.received.CustomerID != "customer-1" {
		t.Fatalf("unexpected typed query: %#v", list.received)
	}
}

func TestRequireScopeRejectsMismatch(t *testing.T) {
	server := NewServer(Dependencies{Scope: fakeScope{actor: commands.ActorContext{PrincipalID: "principal-1", BusinessID: "other-business"}}})
	_, err := server.requireScope(context.Background(), "business-1")
	if err == nil {
		t.Fatal("expected scope mismatch error")
	}
	var typed *appErrors.Error
	if !asApplicationError(err, &typed) || typed.Code != appErrors.CodeForbidden {
		t.Fatalf("expected forbidden scope mismatch, got %v", err)
	}
}

func TestCommandMetaPreservesOpaqueIfMatchAndIdempotency(t *testing.T) {
	actor := commands.ActorContext{PrincipalID: "principal-1", BusinessID: "business-1"}
	meta := commandMeta(context.Background(), actor, contract.CommandHeaders{IfMatch: "\"opaque-v4\"", IdempotencyKey: "idem-1", XRequestID: "req-1", XCorrelationID: "corr-1"})
	if meta.ExpectedVersion == nil || *meta.ExpectedVersion != "opaque-v4" {
		t.Fatalf("expected opaque resource version, got %#v", meta.ExpectedVersion)
	}
	if meta.IdempotencyKey != "idem-1" || meta.RequestID != "req-1" || meta.CorrelationID != "corr-1" {
		t.Fatalf("metadata mapping lost values: %#v", meta)
	}
}

func asApplicationError(err error, target **appErrors.Error) bool {
	if e, ok := err.(*appErrors.Error); ok {
		*target = e
		return true
	}
	return false
}

func TestWebhookRuntimeForwardsProviderHeadersToApplication(t *testing.T) {
	body := []byte(`{"event":"webhook.test"}`)
	capture := &fakeWebhookHandler{}
	server := NewServer(Dependencies{IngestSocialAPIWebhook: capture})
	_, mux := contract.BuildAPIWithHandlers(server)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/socialapi/callback", bytes.NewReader(body))
	req.Header.Set("X-SocialAPI-Signature-V2", "sha256=signature")
	req.Header.Set("X-SocialAPI-Timestamp", "1787659200")
	req.Header.Set("X-SocialAPI-Delivery", "delivery-1")
	req.Header.Set("X-SocialAPI-Event", "webhook.test")
	req.Header.Set("X-Request-ID", "request-1")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", res.Code, res.Body.String())
	}
	if capture.command.RouteKey != "callback" || capture.command.RequestID != "request-1" || capture.command.DeliveryID != "delivery-1" || capture.command.ProviderEvent != "webhook.test" || capture.command.ProviderHeaders["X-SocialAPI-Signature-V2"] != "sha256=signature" || capture.command.ProviderHeaders["X-SocialAPI-Timestamp"] != "1787659200" || string(capture.command.RawPayload) != string(body) {
		t.Fatalf("provider metadata was not forwarded exactly: %#v", capture.command)
	}
}

func TestWebhookRuntimeMapsApplicationErrorStatus(t *testing.T) {
	server := NewServer(Dependencies{IngestSocialAPIWebhook: errorWebhookHandler{err: &appErrors.Error{Code: appErrors.CodeUnauthenticated, Message: "signature verification failed"}}})
	_, mux := contract.BuildAPIWithHandlers(server)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/socialapi/socialapi-test", bytes.NewReader([]byte(`{"event":"dm.received","id":"9001"}`)))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 from application error, got %d body=%s", res.Code, res.Body.String())
	}
}

func TestChatwootWebhookRouteIsNotRegistered(t *testing.T) {
	server := NewServer(Dependencies{})
	_, mux := contract.BuildAPIWithHandlers(server)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/chatwoot/legacy", bytes.NewReader([]byte(`{}`)))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected Chatwoot route to be absent (404), got %d body=%s", res.Code, res.Body.String())
	}
}

func TestRuntimeRouteUsesTypedHandlerAndErrorEnvelope(t *testing.T) {
	_, mux := contract.BuildAPIWithHandlers(NewServer(Dependencies{}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/conversations", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNotImplemented {
		t.Fatalf("expected typed skeleton status 501, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.HasPrefix(res.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected application/json, got %q", res.Header().Get("Content-Type"))
	}
	var envelope contract.ErrorEnvelope
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v; body=%s", err, res.Body.String())
	}
	if envelope.Error.Code != string(appErrors.CodeNotImplemented) || envelope.Error.Message == "" {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
}

func TestMapApplicationErrorKeepsHTTPOutsideApplication(t *testing.T) {
	cases := []struct {
		code   appErrors.Code
		status int
	}{
		{appErrors.CodeValidation, 422},
		{appErrors.CodeIdempotencyConflict, 409},
		{appErrors.CodeStaleResource, 409},
		{appErrors.CodeExternalDependency, 503},
	}
	for _, tc := range cases {
		err := mapApplicationError(appErrors.New(tc.code, "safe message"))
		statusErr, ok := err.(interface{ GetStatus() int })
		if !ok || statusErr.GetStatus() != tc.status {
			t.Fatalf("%s mapped to unexpected status: %v", tc.code, err)
		}
	}
}

func TestRuntimeNonCoreRouteUsesApplicationBoundary(t *testing.T) {
	_, mux := contract.BuildAPIWithHandlers(NewServer(Dependencies{}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/catalogs", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusNotImplemented {
		t.Fatalf("expected application boundary status 501, got %d body=%s", res.Code, res.Body.String())
	}
	var envelope contract.ErrorEnvelope
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != string(appErrors.CodeNotImplemented) {
		t.Fatalf("expected not_implemented application code, got %#v", envelope.Error)
	}
}

func TestAcceptTeamInvitationUsesAuthenticatedPrincipalWithoutBusinessScope(t *testing.T) {
	handler := &fakeTeamInvitationAcceptance{}
	server := NewServer(Dependencies{AcceptTeamInvitation: handler})
	ctx := middleware.WithPrincipal(context.Background(), "00000000-0000-0000-0000-000000000101")

	value, err := server.Dispatch(ctx, "acceptTeamInvitation", &contract.TeamInvitationAcceptInput{
		Body: contract.AcceptTeamInvitationRequest{AcceptanceToken: "one-time-token"},
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	result, ok := value.(*contract.Single[contract.TeamInvitation])
	if !ok {
		t.Fatalf("unexpected output type: %T", value)
	}
	if handler.command.Meta.Actor.PrincipalID != "00000000-0000-0000-0000-000000000101" || handler.command.AcceptanceToken != "one-time-token" {
		t.Fatalf("acceptance command did not preserve authenticated principal: %#v", handler.command)
	}
	if result.Body.Data.Status != "accepted" || result.Body.Data.Email != "staff@example.test" {
		t.Fatalf("unexpected response: %#v", result.Body.Data)
	}
}

func TestAssignConversationHTTPUsesAssigneePrincipalID(t *testing.T) {
	handler := &fakeConversationAssignment{result: commands.ConversationResult{Conversation: commands.ConversationView{
		ID:              "00000000-0000-0000-0000-000000000201",
		BusinessID:      "00000000-0000-0000-0000-000000000001",
		CustomerID:      "00000000-0000-0000-0000-000000000301",
		State:           "open",
		Ownership:       "human",
		AIMode:          "assist",
		ResourceVersion: "2",
	}}}
	server := NewServer(Dependencies{
		Scope:              fakeScope{actor: commands.ActorContext{BusinessID: "00000000-0000-0000-0000-000000000001", PrincipalID: "00000000-0000-0000-0000-000000000100", Role: "manager"}},
		AssignConversation: handler,
	})
	_, mux := contract.BuildAPIWithHandlers(server)
	body := []byte(`{"assignee_principal_id":"00000000-0000-0000-0000-000000000102"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/conversations/00000000-0000-0000-0000-000000000201/assign", bytes.NewReader(body))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if handler.command.AssigneePrincipalID != "00000000-0000-0000-0000-000000000102" {
		t.Fatalf("expected assignee principal UUID, got %#v", handler.command)
	}
}

type fakeTeamInvitationAcceptance struct {
	command commands.AcceptTeamInvitationCommand
}

func (h *fakeTeamInvitationAcceptance) Handle(_ context.Context, command commands.AcceptTeamInvitationCommand) (commands.TeamInvitationView, error) {
	h.command = command
	return commands.TeamInvitationView{
		ID:        "00000000-0000-0000-0000-000000000401",
		Email:     "staff@example.test",
		Role:      "agent",
		Status:    "accepted",
		ExpiresAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}, nil
}

type fakeConversationAssignment struct {
	command commands.AssignConversationCommand
	result  commands.ConversationResult
}

func (h *fakeConversationAssignment) Handle(_ context.Context, command commands.AssignConversationCommand) (commands.ConversationResult, error) {
	h.command = command
	return h.result, nil
}

type fakeConversationUpdate struct {
	command commands.UpdateConversationCommand
	result  commands.ConversationResult
}

func (h *fakeConversationUpdate) Handle(_ context.Context, command commands.UpdateConversationCommand) (commands.ConversationResult, error) {
	h.command = command
	return h.result, nil
}

func TestUpdateConversationHTTPDispatchesCorrectly(t *testing.T) {
	handler := &fakeConversationUpdate{result: commands.ConversationResult{Conversation: commands.ConversationView{
		ID:              "00000000-0000-0000-0000-000000000201",
		BusinessID:      "00000000-0000-0000-0000-000000000001",
		CustomerID:      "00000000-0000-0000-0000-000000000301",
		State:           "human_handling",
		Ownership:       "human",
		AIMode:          "disabled",
		ResourceVersion: "4",
	}}}
	server := NewServer(Dependencies{
		Scope:              fakeScope{actor: commands.ActorContext{BusinessID: "00000000-0000-0000-0000-000000000001", PrincipalID: "00000000-0000-0000-0000-000000000100", Role: "agent"}},
		UpdateConversation: handler,
	})
	_, mux := contract.BuildAPIWithHandlers(server)
	body := []byte(`{"state":"human_handling","ownership":"human","ai_mode_override":"disabled"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/businesses/00000000-0000-0000-0000-000000000001/conversations/00000000-0000-0000-0000-000000000201", bytes.NewReader(body))
	req.Header.Set("If-Match", "3")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", res.Code, res.Body.String())
	}
	if handler.command.State == nil || *handler.command.State != "human_handling" {
		t.Fatalf("expected state human_handling, got %#v", handler.command)
	}
	if handler.command.Ownership == nil || *handler.command.Ownership != "human" {
		t.Fatalf("expected ownership human, got %#v", handler.command)
	}
	if handler.command.AIModeOverride == nil || *handler.command.AIModeOverride != "disabled" {
		t.Fatalf("expected ai_mode_override disabled, got %#v", handler.command)
	}
}

