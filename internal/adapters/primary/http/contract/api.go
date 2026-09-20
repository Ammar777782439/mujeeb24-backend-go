package contract

import (
	"context"
	"net/http"
	"reflect"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// DashboardOperationHandler is the HTTP-facing runtime boundary. The callback
// receives a typed Huma input; the concrete adapter dispatches by operation ID
// into typed Application Commands/Queries.
type DashboardOperationHandler interface {
	Dispatch(context.Context, string, any) (any, error)
}

// BuildAPI creates the DTO-first API contract with skeleton callbacks.
func BuildAPI() (huma.API, *http.ServeMux) {
	return BuildAPIWithHandlers(nil)
}

// BuildAPIWithHandlers uses the same operation declarations as BuildAPI while
// allowing runtime wiring of the typed HTTP handler boundary.
func BuildAPIWithHandlers(handlers DashboardOperationHandler) (huma.API, *http.ServeMux) {
	return BuildAPIWithHandlersAndMiddleware(handlers, nil)
}

func BuildAPIWithHandlersAndMiddleware(handlers DashboardOperationHandler, middlewares huma.Middlewares) (huma.API, *http.ServeMux) {
	mux := http.NewServeMux()
	config := huma.DefaultConfig("Mujeeb 24 Dashboard API", "1.0.0")
	config.OpenAPI.Servers = []*huma.Server{{URL: "/api/v1", Description: "Mujeeb 24 API V1"}}
	config.OpenAPI.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth":    {Type: "http", Scheme: "bearer", BearerFormat: "JWT", Description: "EdDSA/Ed25519 JWT access token"},
		"refreshCookie": {Type: "apiKey", In: "cookie", Name: "mujeeb_refresh", Description: "HttpOnly opaque refresh session cookie"},
	}
	api := humago.NewWithPrefix(mux, "/api/v1", config)
	if len(middlewares) > 0 {
		api.UseMiddleware(middlewares...)
	}
	registerCoreOperations(api, handlers)
	registerExtendedOperations(api, handlers)
	registerSystemOperations(api, handlers)
	registerRemainingDashboardOperations(api, handlers)
	replaceFrameworkErrorResponses(api)
	return api, mux
}

var dashboardSecurity = []map[string][]string{{"bearerAuth": {}}}

func registerCoreOperations(api huma.API, dispatcher DashboardOperationHandler) {
	register(api, dispatcher, huma.Operation{OperationID: "getCurrentPrincipal", Method: http.MethodGet, Path: "/me", Tags: []string{"Me"}, Summary: "Get the authenticated principal", Security: dashboardSecurity}, EmptyInput{}, Single[Principal]{})
	register(api, dispatcher, huma.Operation{OperationID: "listAccessibleBusinesses", Method: http.MethodGet, Path: "/me/businesses", Tags: []string{"Me"}, Summary: "List businesses accessible to the principal", Security: dashboardSecurity}, MeBusinessListInput{}, List[BusinessMembership]{})
	register(api, dispatcher, huma.Operation{OperationID: "getBusiness", Method: http.MethodGet, Path: "/businesses/{business_id}", Tags: []string{"Business"}, Summary: "Get a business projection", Security: dashboardSecurity}, BusinessPath{}, Single[Business]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateBusinessProfile", Method: http.MethodPatch, Path: "/businesses/{business_id}", Tags: []string{"Business"}, Summary: "Update editable business profile fields", Security: dashboardSecurity, DefaultStatus: http.StatusOK}, BusinessUpdateInput{}, Single[Business]{})
	register(api, dispatcher, huma.Operation{OperationID: "getDashboardOverview", Method: http.MethodGet, Path: "/businesses/{business_id}/dashboard/overview", Tags: []string{"Dashboard"}, Summary: "Get the merchant dashboard overview", Security: dashboardSecurity}, BusinessPath{}, Single[DashboardOverview]{})
	register(api, dispatcher, huma.Operation{OperationID: "listConversations", Method: http.MethodGet, Path: "/businesses/{business_id}/conversations", Tags: []string{"Conversations"}, Summary: "List merchant conversations", Security: dashboardSecurity}, ConversationListInput{}, List[Conversation]{})
	register(api, dispatcher, huma.Operation{OperationID: "getConversation", Method: http.MethodGet, Path: "/businesses/{business_id}/conversations/{conversation_id}", Tags: []string{"Conversations"}, Summary: "Get a conversation projection", Security: dashboardSecurity}, ConversationInput{}, Single[Conversation]{})
	register(api, dispatcher, huma.Operation{OperationID: "listConversationMessages", Method: http.MethodGet, Path: "/businesses/{business_id}/conversations/{conversation_id}/messages", Tags: []string{"Conversations"}, Summary: "List conversation messages", Security: dashboardSecurity}, ConversationMessageListInput{}, List[Message]{})
	register(api, dispatcher, huma.Operation{OperationID: "createOutboundMessage", Method: http.MethodPost, Path: "/businesses/{business_id}/conversations/{conversation_id}/messages", Tags: []string{"Conversations"}, Summary: "Create an outbound message intent", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted, Errors: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusServiceUnavailable}}, ConversationMessageInput{}, Single[Message]{})
	register(api, dispatcher, huma.Operation{OperationID: "listCustomers", Method: http.MethodGet, Path: "/businesses/{business_id}/customers", Tags: []string{"Customers"}, Summary: "List customers", Security: dashboardSecurity}, CustomerListInput{}, List[Customer]{})
	register(api, dispatcher, huma.Operation{OperationID: "getCustomer", Method: http.MethodGet, Path: "/businesses/{business_id}/customers/{customer_id}", Tags: []string{"Customers"}, Summary: "Get a customer projection", Security: dashboardSecurity}, CustomerInput{}, Single[Customer]{})
	register(api, dispatcher, huma.Operation{OperationID: "createCustomer", Method: http.MethodPost, Path: "/businesses/{business_id}/customers", Tags: []string{"Customers"}, Summary: "Create a customer", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, CreateCustomerInput{}, Single[Customer]{})
}

func replaceFrameworkErrorResponses(api huma.API) {
	registry := api.OpenAPI().Components.Schemas
	errorSchema := registry.Schema(reflect.TypeOf(ErrorEnvelope{}), true, "ErrorEnvelope")
	for _, item := range api.OpenAPI().Paths {
		operations := []*huma.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace}
		for _, operation := range operations {
			if operation == nil {
				continue
			}
			for status, response := range operation.Responses {
				code, err := strconv.Atoi(status)
				if (err != nil && status != "default") || (err == nil && code < http.StatusBadRequest) {
					continue
				}
				response.Content = map[string]*huma.MediaType{
					"application/json": {Schema: errorSchema},
				}
			}
		}
	}
	delete(registry.Map(), "ErrorModel")
}

func register[I any, O any](api huma.API, dispatcher DashboardOperationHandler, operation huma.Operation, input I, output O) {
	registerTyped(api, operation, input, output, func(ctx context.Context, in *I) (*O, error) {
		if dispatcher == nil {
			return nil, huma.Error501NotImplemented("HTTP handler skeleton is not implemented")
		}
		value, err := dispatcher.Dispatch(ctx, operation.OperationID, in)
		if err != nil {
			return nil, err
		}
		if value == nil {
			return nil, nil
		}
		result, ok := value.(*O)
		if !ok {
			return nil, huma.Error500InternalServerError("invalid dashboard handler output")
		}
		return result, nil
	})
}

func registerTyped[I any, O any](api huma.API, operation huma.Operation, input I, output O, handler func(context.Context, *I) (*O, error)) {
	if operation.Errors == nil {
		operation.Errors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity}
	}
	huma.Register(api, operation, handler)
}

func registerExtendedOperations(api huma.API, dispatcher DashboardOperationHandler) {
	register(api, dispatcher, huma.Operation{OperationID: "getBusinessPolicy", Method: http.MethodGet, Path: "/businesses/{business_id}/policy", Tags: []string{"Business"}, Summary: "Get business policy", Security: dashboardSecurity}, BusinessPath{}, Single[BusinessPolicy]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateBusinessPolicy", Method: http.MethodPatch, Path: "/businesses/{business_id}/policy", Tags: []string{"Business"}, Summary: "Update business policy", Security: dashboardSecurity}, BusinessPolicyInput{}, Single[BusinessPolicy]{})
	register(api, dispatcher, huma.Operation{OperationID: "getConnectionCapabilities", Method: http.MethodGet, Path: "/businesses/{business_id}/channel-connections/{connection_id}/capabilities", Tags: []string{"Connections"}, Summary: "List connection capabilities", Security: dashboardSecurity}, ConnectionPath{}, List[Capability]{})

	register(api, dispatcher, huma.Operation{OperationID: "listCatalogs", Method: http.MethodGet, Path: "/businesses/{business_id}/catalogs", Tags: []string{"Catalog"}, Summary: "List catalogs", Security: dashboardSecurity}, CatalogListInput{}, List[Catalog]{})
	register(api, dispatcher, huma.Operation{OperationID: "createCatalog", Method: http.MethodPost, Path: "/businesses/{business_id}/catalogs", Tags: []string{"Catalog"}, Summary: "Create catalog", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, CatalogCreateInput{}, Single[Catalog]{})
	register(api, dispatcher, huma.Operation{OperationID: "getCatalog", Method: http.MethodGet, Path: "/businesses/{business_id}/catalogs/{catalog_id}", Tags: []string{"Catalog"}, Summary: "Get catalog", Security: dashboardSecurity}, CatalogPath{}, Single[Catalog]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateCatalog", Method: http.MethodPatch, Path: "/businesses/{business_id}/catalogs/{catalog_id}", Tags: []string{"Catalog"}, Summary: "Update catalog", Security: dashboardSecurity}, CatalogUpdateInput{}, Single[Catalog]{})
	register(api, dispatcher, huma.Operation{OperationID: "listCatalogItems", Method: http.MethodGet, Path: "/businesses/{business_id}/catalogs/{catalog_id}/items", Tags: []string{"Catalog"}, Summary: "List catalog items", Security: dashboardSecurity}, CatalogItemsInput{}, List[CatalogItem]{})
	register(api, dispatcher, huma.Operation{OperationID: "createCatalogItem", Method: http.MethodPost, Path: "/businesses/{business_id}/catalogs/{catalog_id}/items", Tags: []string{"Catalog"}, Summary: "Create catalog item", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, CatalogItemCreateInput{}, Single[CatalogItem]{})
	register(api, dispatcher, huma.Operation{OperationID: "getCatalogItem", Method: http.MethodGet, Path: "/businesses/{business_id}/catalogs/{catalog_id}/items/{item_id}", Tags: []string{"Catalog"}, Summary: "Get catalog item", Security: dashboardSecurity}, CatalogItemPath{}, Single[CatalogItem]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateCatalogItem", Method: http.MethodPatch, Path: "/businesses/{business_id}/catalogs/{catalog_id}/items/{item_id}", Tags: []string{"Catalog"}, Summary: "Update catalog item", Security: dashboardSecurity}, CatalogItemUpdateInput{}, Single[CatalogItem]{})
	register(api, dispatcher, huma.Operation{OperationID: "listOffers", Method: http.MethodGet, Path: "/businesses/{business_id}/catalog-items/{item_id}/offers", Tags: []string{"Catalog"}, Summary: "List offers", Security: dashboardSecurity}, ItemOffersInput{}, List[Offer]{})
	register(api, dispatcher, huma.Operation{OperationID: "createOffer", Method: http.MethodPost, Path: "/businesses/{business_id}/catalog-items/{item_id}/offers", Tags: []string{"Catalog"}, Summary: "Create offer", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, OfferCreateInput{}, Single[Offer]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateOffer", Method: http.MethodPatch, Path: "/businesses/{business_id}/offers/{offer_id}", Tags: []string{"Catalog"}, Summary: "Update offer", Security: dashboardSecurity}, OfferUpdateInput{}, Single[Offer]{})
	register(api, dispatcher, huma.Operation{OperationID: "listVariants", Method: http.MethodGet, Path: "/businesses/{business_id}/catalog-items/{item_id}/variants", Tags: []string{"Catalog"}, Summary: "List variants", Security: dashboardSecurity}, ItemVariantsInput{}, List[Variant]{})
	register(api, dispatcher, huma.Operation{OperationID: "createVariant", Method: http.MethodPost, Path: "/businesses/{business_id}/catalog-items/{item_id}/variants", Tags: []string{"Catalog"}, Summary: "Create variant", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, VariantCreateInput{}, Single[Variant]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateVariant", Method: http.MethodPatch, Path: "/businesses/{business_id}/variants/{variant_id}", Tags: []string{"Catalog"}, Summary: "Update variant", Security: dashboardSecurity}, VariantUpdateInput{}, Single[Variant]{})
	register(api, dispatcher, huma.Operation{OperationID: "listAttributeSchemas", Method: http.MethodGet, Path: "/businesses/{business_id}/attribute-schemas", Tags: []string{"Catalog"}, Summary: "List attribute schemas", Security: dashboardSecurity}, AttributeSchemasInput{}, List[AttributeSchema]{})
	register(api, dispatcher, huma.Operation{OperationID: "createAttributeSchemaVersion", Method: http.MethodPost, Path: "/businesses/{business_id}/attribute-schemas", Tags: []string{"Catalog"}, Summary: "Create attribute schema", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, AttributeSchemaCreateInput{}, Single[AttributeSchema]{})
	register(api, dispatcher, huma.Operation{OperationID: "getAttributeSchema", Method: http.MethodGet, Path: "/businesses/{business_id}/attribute-schemas/{schema_id}", Tags: []string{"Catalog"}, Summary: "Get attribute schema", Security: dashboardSecurity}, SchemaPath{}, Single[AttributeSchema]{})

	register(api, dispatcher, huma.Operation{OperationID: "listLeads", Method: http.MethodGet, Path: "/businesses/{business_id}/leads", Tags: []string{"Leads"}, Summary: "List leads", Security: dashboardSecurity}, LeadListInput{}, List[Lead]{})
	register(api, dispatcher, huma.Operation{OperationID: "createLead", Method: http.MethodPost, Path: "/businesses/{business_id}/leads", Tags: []string{"Leads"}, Summary: "Create lead", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, LeadCreateInput{}, Single[Lead]{})
	register(api, dispatcher, huma.Operation{OperationID: "getLead", Method: http.MethodGet, Path: "/businesses/{business_id}/leads/{lead_id}", Tags: []string{"Leads"}, Summary: "Get lead", Security: dashboardSecurity}, LeadPath{}, Single[Lead]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateLead", Method: http.MethodPatch, Path: "/businesses/{business_id}/leads/{lead_id}", Tags: []string{"Leads"}, Summary: "Update lead", Security: dashboardSecurity}, LeadUpdateInput{}, Single[Lead]{})
	register(api, dispatcher, huma.Operation{OperationID: "qualifyLead", Method: http.MethodPost, Path: "/businesses/{business_id}/leads/{lead_id}/qualify", Tags: []string{"Leads"}, Summary: "Qualify lead", Security: dashboardSecurity}, LeadQualifyInput{}, Single[Lead]{})
	register(api, dispatcher, huma.Operation{OperationID: "markLeadLost", Method: http.MethodPost, Path: "/businesses/{business_id}/leads/{lead_id}/mark-lost", Tags: []string{"Leads"}, Summary: "Mark lead lost", Security: dashboardSecurity}, LeadLostInput{}, Single[Lead]{})
	register(api, dispatcher, huma.Operation{OperationID: "listLeadAttributions", Method: http.MethodGet, Path: "/businesses/{business_id}/leads/{lead_id}/attributions", Tags: []string{"Leads"}, Summary: "List lead attributions", Security: dashboardSecurity}, LeadAttributionsInput{}, List[LeadAttribution]{})
	register(api, dispatcher, huma.Operation{OperationID: "listLeadScores", Method: http.MethodGet, Path: "/businesses/{business_id}/leads/{lead_id}/scores", Tags: []string{"Leads"}, Summary: "List lead scores", Security: dashboardSecurity}, LeadScoresInput{}, List[LeadScore]{})

	register(api, dispatcher, huma.Operation{OperationID: "listTransactions", Method: http.MethodGet, Path: "/businesses/{business_id}/transactions", Tags: []string{"Transactions"}, Summary: "List transactions", Security: dashboardSecurity}, TransactionListInput{}, List[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "createTransactionDraft", Method: http.MethodPost, Path: "/businesses/{business_id}/transactions", Tags: []string{"Transactions"}, Summary: "Create transaction draft", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, TransactionCreateInput{}, Single[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "getTransaction", Method: http.MethodGet, Path: "/businesses/{business_id}/transactions/{transaction_id}", Tags: []string{"Transactions"}, Summary: "Get transaction", Security: dashboardSecurity}, TransactionPath{}, Single[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateTransactionDraft", Method: http.MethodPatch, Path: "/businesses/{business_id}/transactions/{transaction_id}", Tags: []string{"Transactions"}, Summary: "Update transaction draft", Security: dashboardSecurity}, TransactionUpdateInput{}, Single[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "confirmTransaction", Method: http.MethodPost, Path: "/businesses/{business_id}/transactions/{transaction_id}/confirm", Tags: []string{"Transactions"}, Summary: "Confirm transaction", Security: dashboardSecurity}, TransactionConfirmInput{}, Single[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "cancelTransaction", Method: http.MethodPost, Path: "/businesses/{business_id}/transactions/{transaction_id}/cancel", Tags: []string{"Transactions"}, Summary: "Cancel transaction", Security: dashboardSecurity}, TransactionCancelInput{}, Single[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "submitTransactionReview", Method: http.MethodPost, Path: "/businesses/{business_id}/transactions/{transaction_id}/submit-review", Tags: []string{"Transactions"}, Summary: "Submit transaction review", Security: dashboardSecurity}, TransactionReviewSubmitInput{}, Single[TransactionReview]{})
	register(api, dispatcher, huma.Operation{OperationID: "getTransactionReview", Method: http.MethodGet, Path: "/businesses/{business_id}/transactions/{transaction_id}/reviews", Tags: []string{"Transactions"}, Summary: "Get transaction review", Security: dashboardSecurity}, TransactionPath{}, Single[TransactionReview]{})
	register(api, dispatcher, huma.Operation{OperationID: "approveTransactionReview", Method: http.MethodPost, Path: "/businesses/{business_id}/transactions/{transaction_id}/reviews/approve", Tags: []string{"Transactions"}, Summary: "Approve transaction review", Security: dashboardSecurity}, TransactionReviewDecisionInput{}, Single[TransactionReview]{})
	register(api, dispatcher, huma.Operation{OperationID: "rejectTransactionReview", Method: http.MethodPost, Path: "/businesses/{business_id}/transactions/{transaction_id}/reviews/reject", Tags: []string{"Transactions"}, Summary: "Reject transaction review", Security: dashboardSecurity}, TransactionReviewDecisionInput{}, Single[TransactionReview]{})

	register(api, dispatcher, huma.Operation{OperationID: "listAIDecisions", Method: http.MethodGet, Path: "/businesses/{business_id}/ai-decisions", Tags: []string{"AI"}, Summary: "List structured AI decisions", Security: dashboardSecurity}, AIDecisionListInput{}, List[AIDecision]{})
	register(api, dispatcher, huma.Operation{OperationID: "getAIDecision", Method: http.MethodGet, Path: "/businesses/{business_id}/ai-decisions/{decision_id}", Tags: []string{"AI"}, Summary: "Get structured AI decision", Security: dashboardSecurity}, DecisionPath{}, Single[AIDecision]{})
	register(api, dispatcher, huma.Operation{OperationID: "requestHumanReview", Method: http.MethodPost, Path: "/businesses/{business_id}/ai-decisions/{decision_id}/request-human", Tags: []string{"AI"}, Summary: "Request human review", Security: dashboardSecurity}, AIHumanInput{}, Single[AIDecision]{})
	register(api, dispatcher, huma.Operation{OperationID: "listAuditEvents", Method: http.MethodGet, Path: "/businesses/{business_id}/audit-events", Tags: []string{"Audit"}, Summary: "List audit events", Security: dashboardSecurity}, AuditListInput{}, List[AuditEvent]{})
	register(api, dispatcher, huma.Operation{OperationID: "getAuditEvent", Method: http.MethodGet, Path: "/businesses/{business_id}/audit-events/{audit_event_id}", Tags: []string{"Audit"}, Summary: "Get audit event", Security: dashboardSecurity}, AuditPath{}, Single[AuditEvent]{})
}

func init() { _ = registerExtendedOperations }

func registerSystemOperations(api huma.API, dispatcher DashboardOperationHandler) {
	register(api, dispatcher, huma.Operation{OperationID: "authenticatePrincipal", Method: http.MethodPost, Path: "/auth/login", Tags: []string{"Auth"}, Summary: "Authenticate and issue JWT access token", Security: []map[string][]string{}, DefaultStatus: http.StatusOK, Errors: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests}}, LoginInput{}, AuthOutput{})
	register(api, dispatcher, huma.Operation{OperationID: "rotateRefreshSession", Method: http.MethodPost, Path: "/auth/refresh", Tags: []string{"Auth"}, Summary: "Rotate refresh session", Security: []map[string][]string{{"refreshCookie": {}}}, DefaultStatus: http.StatusOK, Errors: []int{http.StatusUnauthorized, http.StatusTooManyRequests}}, RefreshInput{}, AuthOutput{})
	register(api, dispatcher, huma.Operation{OperationID: "revokeRefreshSession", Method: http.MethodPost, Path: "/auth/logout", Tags: []string{"Auth"}, Summary: "Revoke refresh session", Security: dashboardSecurity, DefaultStatus: http.StatusNoContent, Errors: []int{http.StatusUnauthorized}}, LogoutInput{}, NoContentOutput{})
	register(api, dispatcher, huma.Operation{OperationID: "getLiveness", Method: http.MethodGet, Path: "/health/live", Tags: []string{"Operational"}, Summary: "Process liveness", Security: []map[string][]string{}, Errors: nil}, EmptyInput{}, Single[Health]{})
	register(api, dispatcher, huma.Operation{OperationID: "getReadiness", Method: http.MethodGet, Path: "/health/ready", Tags: []string{"Operational"}, Summary: "Dependency readiness", Security: []map[string][]string{}, Errors: []int{http.StatusServiceUnavailable}}, EmptyInput{}, Single[Health]{})
	register(api, dispatcher, huma.Operation{OperationID: "getMetrics", Method: http.MethodGet, Path: "/metrics", Tags: []string{"Operational"}, Summary: "Internal metrics", Security: []map[string][]string{}, Errors: []int{http.StatusForbidden}}, EmptyInput{}, MetricsOutput{})
	register(api, dispatcher, huma.Operation{OperationID: "ingestSocialAPIWebhook", Method: http.MethodPost, Path: "/webhooks/socialapi/{route_key}", Tags: []string{"Webhooks"}, Summary: "Ingest SocialAPI webhook", Security: []map[string][]string{}, DefaultStatus: http.StatusAccepted, Errors: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusRequestEntityTooLarge}}, SocialWebhookInput{}, WebhookAcceptedOutput{})
}

func init() { _ = registerSystemOperations }

func registerRemainingDashboardOperations(api huma.API, dispatcher DashboardOperationHandler) {
	register(api, dispatcher, huma.Operation{OperationID: "listTeamMembers", Method: http.MethodGet, Path: "/businesses/{business_id}/team/members", Tags: []string{"Team"}, Summary: "List active and revoked team memberships", Security: dashboardSecurity}, TeamMemberListInput{}, List[TeamMember]{})
	register(api, dispatcher, huma.Operation{OperationID: "createTeamInvitation", Method: http.MethodPost, Path: "/businesses/{business_id}/team/invitations", Tags: []string{"Team"}, Summary: "Create a one-time team invitation", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, TeamInvitationCreateInput{}, Single[TeamInvitationCreated]{})
	register(api, dispatcher, huma.Operation{OperationID: "acceptTeamInvitation", Method: http.MethodPost, Path: "/team/invitations/accept", Tags: []string{"Team"}, Summary: "Accept an invitation using the authenticated principal", Security: dashboardSecurity}, TeamInvitationAcceptInput{}, Single[TeamInvitation]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateTeamMemberRole", Method: http.MethodPatch, Path: "/businesses/{business_id}/team/members/{principal_id}/role", Tags: []string{"Team"}, Summary: "Update a non-owner team member role", Security: dashboardSecurity}, TeamMemberRoleUpdateInput{}, Single[TeamMember]{})
	register(api, dispatcher, huma.Operation{OperationID: "revokeTeamMember", Method: http.MethodPost, Path: "/businesses/{business_id}/team/members/{principal_id}/revoke", Tags: []string{"Team"}, Summary: "Revoke a membership while preserving one active owner", Security: dashboardSecurity}, TeamMemberRevokeInput{}, Single[TeamMember]{})
	register(api, dispatcher, huma.Operation{OperationID: "listChannelConnections", Method: http.MethodGet, Path: "/businesses/{business_id}/channel-connections", Tags: []string{"Connections"}, Summary: "List channel connections", Security: dashboardSecurity}, ConnectionListInput{}, List[ChannelConnection]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateConversation", Method: http.MethodPatch, Path: "/businesses/{business_id}/conversations/{conversation_id}", Tags: []string{"Conversations"}, Summary: "Update conversation", Security: dashboardSecurity}, ConversationUpdateInput{}, Single[Conversation]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateCustomer", Method: http.MethodPatch, Path: "/businesses/{business_id}/customers/{customer_id}", Tags: []string{"Customers"}, Summary: "Update customer", Security: dashboardSecurity}, CustomerUpdateInput{}, Single[Customer]{})
	register(api, dispatcher, huma.Operation{OperationID: "beginChannelConnection", Method: http.MethodPost, Path: "/businesses/{business_id}/channel-connections", Tags: []string{"Connections"}, Summary: "Begin channel provisioning", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted}, ConnectionCreateInput{}, Single[ChannelProvisioning]{})
	register(api, dispatcher, huma.Operation{OperationID: "getChannelConnection", Method: http.MethodGet, Path: "/businesses/{business_id}/channel-connections/{connection_id}", Tags: []string{"Connections"}, Summary: "Get channel connection", Security: dashboardSecurity}, ConnectionPath{}, Single[ChannelConnection]{})
	register(api, dispatcher, huma.Operation{OperationID: "reconnectChannel", Method: http.MethodPost, Path: "/businesses/{business_id}/channel-connections/{connection_id}/reconnect", Tags: []string{"Connections"}, Summary: "Reconnect channel", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted}, ConnectionActionInput{}, Single[ChannelConnection]{})
	register(api, dispatcher, huma.Operation{OperationID: "disconnectChannel", Method: http.MethodPost, Path: "/businesses/{business_id}/channel-connections/{connection_id}/disconnect", Tags: []string{"Connections"}, Summary: "Disconnect channel", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted}, ConnectionActionInput{}, Single[ChannelConnection]{})
	register(api, dispatcher, huma.Operation{OperationID: "assignConversation", Method: http.MethodPost, Path: "/businesses/{business_id}/conversations/{conversation_id}/assign", Tags: []string{"Conversations"}, Summary: "Assign conversation", Security: dashboardSecurity}, ConversationAssignInput{}, Single[Conversation]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateConversationLabels", Method: http.MethodPost, Path: "/businesses/{business_id}/conversations/{conversation_id}/labels", Tags: []string{"Conversations"}, Summary: "Update conversation labels", Security: dashboardSecurity}, ConversationLabelsInput{}, Single[Conversation]{})
	register(api, dispatcher, huma.Operation{OperationID: "addPrivateNote", Method: http.MethodPost, Path: "/businesses/{business_id}/conversations/{conversation_id}/notes", Tags: []string{"Conversations"}, Summary: "Add private note", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted}, ConversationNoteInput{}, Single[Message]{})
	register(api, dispatcher, huma.Operation{OperationID: "markConversationRead", Method: http.MethodPost, Path: "/businesses/{business_id}/conversations/{conversation_id}/read", Tags: []string{"Conversations"}, Summary: "Mark conversation read for current principal", Security: dashboardSecurity}, ConversationReadInput{}, Single[ConversationRead]{})
	register(api, dispatcher, huma.Operation{OperationID: "listCannedReplies", Method: http.MethodGet, Path: "/businesses/{business_id}/canned-replies", Tags: []string{"Inbox"}, Summary: "List reusable merchant replies", Security: dashboardSecurity}, CannedReplyListInput{}, List[CannedReply]{})
	register(api, dispatcher, huma.Operation{OperationID: "createCannedReply", Method: http.MethodPost, Path: "/businesses/{business_id}/canned-replies", Tags: []string{"Inbox"}, Summary: "Create reusable merchant reply", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, CannedReplyCreateInput{}, Single[CannedReply]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateCannedReply", Method: http.MethodPatch, Path: "/businesses/{business_id}/canned-replies/{canned_reply_id}", Tags: []string{"Inbox"}, Summary: "Update reusable merchant reply", Security: dashboardSecurity}, CannedReplyUpdateInput{}, Single[CannedReply]{})
	register(api, dispatcher, huma.Operation{OperationID: "sendCannedReply", Method: http.MethodPost, Path: "/businesses/{business_id}/conversations/{conversation_id}/canned-replies/{canned_reply_id}/send", Tags: []string{"Inbox"}, Summary: "Queue a reusable reply for delivery", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted, Errors: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusServiceUnavailable}}, SendCannedReplyInput{}, Single[Message]{})
	register(api, dispatcher, huma.Operation{OperationID: "listAutomationRules", Method: http.MethodGet, Path: "/businesses/{business_id}/automation-rules", Tags: []string{"Inbox"}, Summary: "List constrained inbound automation rules", Security: dashboardSecurity}, AutomationRuleListInput{}, List[AutomationRule]{})
	register(api, dispatcher, huma.Operation{OperationID: "createAutomationRule", Method: http.MethodPost, Path: "/businesses/{business_id}/automation-rules", Tags: []string{"Inbox"}, Summary: "Create constrained inbound automation rule", Security: dashboardSecurity, DefaultStatus: http.StatusCreated}, AutomationRuleCreateInput{}, Single[AutomationRule]{})
	register(api, dispatcher, huma.Operation{OperationID: "updateAutomationRule", Method: http.MethodPatch, Path: "/businesses/{business_id}/automation-rules/{automation_rule_id}", Tags: []string{"Inbox"}, Summary: "Update constrained inbound automation rule", Security: dashboardSecurity}, AutomationRuleUpdateInput{}, Single[AutomationRule]{})
	register(api, dispatcher, huma.Operation{OperationID: "listCustomerConversations", Method: http.MethodGet, Path: "/businesses/{business_id}/customers/{customer_id}/conversations", Tags: []string{"Customers"}, Summary: "List customer conversations", Security: dashboardSecurity}, CustomerConversationsInput{}, List[Conversation]{})
	register(api, dispatcher, huma.Operation{OperationID: "listCustomerTransactions", Method: http.MethodGet, Path: "/businesses/{business_id}/customers/{customer_id}/transactions", Tags: []string{"Customers"}, Summary: "List customer transactions", Security: dashboardSecurity}, CustomerTransactionsInput{}, List[CommercialTransaction]{})
	register(api, dispatcher, huma.Operation{OperationID: "mergeCustomer", Method: http.MethodPost, Path: "/businesses/{business_id}/customers/{customer_id}/merge", Tags: []string{"Customers"}, Summary: "Merge customer", Security: dashboardSecurity, DefaultStatus: http.StatusAccepted}, CustomerMergeInput{}, Single[Customer]{})
}

func init() { _ = registerRemainingDashboardOperations }
