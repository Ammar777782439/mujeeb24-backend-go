package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type fakeScope struct {
	actor commands.ActorContext
	err   error
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
