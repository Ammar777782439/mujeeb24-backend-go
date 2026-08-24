package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type ScopeProvider interface {
	Resolve(context.Context, commands.BusinessID) (commands.ActorContext, error)
}

type Dependencies struct {
	Scope                    ScopeProvider
	GetCurrentPrincipal      queries.GetCurrentPrincipalHandler
	ListAccessibleBusinesses queries.ListAccessibleBusinessesHandler
	GetBusiness              queries.GetBusinessHandler
	GetBusinessPolicy        queries.GetBusinessPolicyHandler
	GetDashboardOverview     queries.GetDashboardOverviewHandler
	ListConversations        queries.ListConversationsHandler
	GetConversation          queries.GetConversationHandler
	ListConversationMessages queries.ListConversationMessagesHandler
	CreateOutboundMessage    commands.CreateOutboundMessageHandler
	ListCustomers            queries.ListCustomersHandler
	GetCustomer              queries.GetCustomerHandler
	CreateCustomer           commands.CreateCustomerHandler
	UpdateCustomer           commands.UpdateCustomerHandler
	MergeCustomer            commands.MergeCustomerHandler
}

type Server struct{ deps Dependencies }

func NewServer(deps Dependencies) *Server { return &Server{deps: deps} }

func (s *Server) requireScope(ctx context.Context, businessID contract.UUID) (commands.ActorContext, error) {
	if s.deps.Scope == nil {
		return commands.ActorContext{}, appErrors.NotImplemented()
	}
	if businessID == "" {
		return commands.ActorContext{}, appErrors.New(appErrors.CodeValidation, "business_id is required")
	}
	actor, err := s.deps.Scope.Resolve(ctx, commands.BusinessID(businessID))
	if err != nil {
		return commands.ActorContext{}, err
	}
	if actor.BusinessID != commands.BusinessID(businessID) {
		return commands.ActorContext{}, appErrors.New(appErrors.CodeForbidden, "business scope mismatch")
	}
	return actor, nil
}

func commandMeta(ctx context.Context, actor commands.ActorContext, h contract.CommandHeaders) commands.CommandMeta {
	meta := commands.CommandMeta{Actor: actor, RequestID: h.XRequestID, CorrelationID: h.XCorrelationID, IdempotencyKey: h.IdempotencyKey}
	if version := strings.Trim(h.IfMatch, "\""); version != "" {
		v := commands.ResourceVersion(version)
		meta.ExpectedVersion = &v
	}
	return meta
}
func queryMeta(actor commands.ActorContext, requestID, correlationID string) queries.QueryMeta {
	return queries.QueryMeta{Actor: actor, RequestID: requestID, CorrelationID: correlationID}
}

func (s *Server) ListConversations(ctx context.Context, in *contract.ConversationListInput) (*contract.List[contract.Conversation], error) {
	if s.deps.ListConversations == nil {
		return nil, mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	result, err := s.deps.ListConversations.Handle(ctx, queries.ListConversationsQuery{Meta: queryMeta(actor, "", ""), Limit: in.Limit, Cursor: in.Cursor, State: in.State, Ownership: in.Ownership, Channel: in.Channel, CustomerID: optionalCustomerID(in.CustomerID)})
	if err != nil {
		return nil, mapApplicationError(err)
	}
	out := &contract.List[contract.Conversation]{}
	out.Body.RequestID = ""
	out.Body.Data = make([]contract.Conversation, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Data = append(out.Body.Data, conversationProjection(item))
	}
	out.Body.Pagination = contract.Page{NextCursor: optionalString(result.NextCursor), HasMore: result.HasMore}
	return out, nil
}

func (s *Server) GetConversation(ctx context.Context, in *contract.ConversationInput) (*contract.Single[contract.Conversation], error) {
	if s.deps.GetConversation == nil {
		return nil, mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	item, err := s.deps.GetConversation.Handle(ctx, queries.GetConversationQuery{Meta: queryMeta(actor, "", ""), ConversationID: commands.ConversationID(in.ConversationID)})
	if err != nil {
		return nil, mapApplicationError(err)
	}
	return singleConversation(item), nil
}

func (s *Server) CreateOutboundMessage(ctx context.Context, in *contract.ConversationMessageInput) (*contract.Single[contract.Message], error) {
	if s.deps.CreateOutboundMessage == nil {
		return nil, mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	result, err := s.deps.CreateOutboundMessage.Handle(ctx, commands.CreateOutboundMessageCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), ConversationID: commands.ConversationID(in.ConversationID), Text: in.Body.Text})
	if err != nil {
		return nil, mapApplicationError(err)
	}
	return singleMessage(result.Message), nil
}

func (s *Server) ListCustomers(ctx context.Context, in *contract.CustomerListInput) (*contract.List[contract.Customer], error) {
	if s.deps.ListCustomers == nil {
		return nil, mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	result, err := s.deps.ListCustomers.Handle(ctx, queries.ListCustomersQuery{Meta: queryMeta(actor, "", ""), Limit: in.Limit, Cursor: in.Cursor, Search: in.Search, Status: in.Status})
	if err != nil {
		return nil, mapApplicationError(err)
	}
	out := &contract.List[contract.Customer]{}
	out.Body.Data = make([]contract.Customer, 0, len(result.Items))
	for _, item := range result.Items {
		out.Body.Data = append(out.Body.Data, customerProjection(item))
	}
	out.Body.Pagination = contract.Page{NextCursor: optionalString(result.NextCursor), HasMore: result.HasMore}
	return out, nil
}

type dashboardHTTPError struct {
	contract.ErrorEnvelope
	status int
}

func (e *dashboardHTTPError) Error() string                         { return e.ErrorEnvelope.Error.Message }
func (e *dashboardHTTPError) GetStatus() int                        { return e.status }
func (e *dashboardHTTPError) ContentType(contentType string) string { return contentType }

func mapApplicationError(err error) error {
	if err == nil {
		return nil
	}
	var typed *appErrors.Error
	if !errors.As(err, &typed) {
		return &dashboardHTTPError{ErrorEnvelope: contract.ErrorEnvelope{Error: contract.ErrorBody{Code: "internal_error", Message: "internal error"}}, status: 500}
	}
	status := 500
	switch typed.Code {
	case appErrors.CodeValidation, appErrors.CodeInvalidState:
		status = 422
	case appErrors.CodeNotFound:
		status = 404
	case appErrors.CodeForbidden:
		status = 403
	case appErrors.CodeConflict, appErrors.CodeIdempotencyConflict, appErrors.CodeStaleResource:
		status = 409
	case appErrors.CodeExternalDependency:
		status = 503
	case appErrors.CodeNotImplemented:
		status = 501
	}
	message := typed.Message
	if message == "" {
		message = string(typed.Code)
	}
	fields := map[string]string(nil)
	if typed.Field != "" {
		fields = map[string]string{"field": typed.Field}
	}
	return &dashboardHTTPError{ErrorEnvelope: contract.ErrorEnvelope{Error: contract.ErrorBody{Code: string(typed.Code), Message: message, Fields: fields, Retryable: typed.Retryable}}, status: status}
}

func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func optionalCustomerID(v contract.UUID) *commands.CustomerID {
	if v == "" {
		return nil
	}
	id := commands.CustomerID(v)
	return &id
}
func conversationProjection(v commands.ConversationView) contract.Conversation {
	return contract.Conversation{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), Customer: contract.CustomerSummary{ID: contract.UUID(v.CustomerID)}, State: v.State, Ownership: v.Ownership, AIMode: v.AIMode, ResourceVersion: string(v.ResourceVersion)}
}
func customerProjection(v commands.CustomerView) contract.Customer {
	return contract.Customer{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), DisplayName: optionalString(v.DisplayName), Status: v.Status, ResourceVersion: string(v.ResourceVersion)}
}
func singleConversation(v commands.ConversationView) *contract.Single[contract.Conversation] {
	out := &contract.Single[contract.Conversation]{}
	out.Body.Data = conversationProjection(v)
	return out
}
func singleMessage(v commands.MessageView) *contract.Single[contract.Message] {
	out := &contract.Single[contract.Message]{}
	out.Body.Data = contract.Message{ID: contract.UUID(v.ID), ConversationID: contract.UUID(v.ConversationID), Direction: v.Direction, Origin: v.Origin, Status: v.Status, Text: v.Text, CreatedAt: v.CreatedAt}
	return out
}
