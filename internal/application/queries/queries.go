package queries

import (
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

type QueryMeta = commands.QueryMeta

type GetCurrentPrincipalQuery struct{ Meta QueryMeta }
type PrincipalView struct {
	ID          commands.PrincipalID
	DisplayName string
	Email       string
}
type GetCurrentPrincipalHandler = commands.QueryHandler[GetCurrentPrincipalQuery, PrincipalView]

type ListAccessibleBusinessesQuery struct {
	Meta   QueryMeta
	Limit  int
	Cursor string
}
type BusinessMembershipView struct {
	Business    commands.BusinessView
	Role        string
	Permissions []string
}
type ListAccessibleBusinessesHandler = commands.QueryHandler[ListAccessibleBusinessesQuery, commands.ListResult[BusinessMembershipView]]

type GetBusinessQuery struct{ Meta QueryMeta }
type GetBusinessHandler = commands.QueryHandler[GetBusinessQuery, commands.BusinessView]
type GetBusinessPolicyQuery struct{ Meta QueryMeta }
type GetBusinessPolicyHandler = commands.QueryHandler[GetBusinessPolicyQuery, commands.BusinessPolicyView]

type ListChannelConnectionsQuery struct {
	Meta    QueryMeta
	Limit   int
	Cursor  string
	Status  string
	Channel string
}
type GetChannelConnectionQuery struct {
	Meta         QueryMeta
	ConnectionID commands.ConnectionID
}
type GetConnectionCapabilitiesQuery struct {
	Meta         QueryMeta
	ConnectionID commands.ConnectionID
}
type ConnectionCapabilityView struct {
	Name           string
	Enabled        bool
	CheckedAt      time.Time
	EvidenceSource string
}
type ListChannelConnectionsHandler = commands.QueryHandler[ListChannelConnectionsQuery, commands.ListResult[commands.ChannelConnectionView]]
type GetChannelConnectionHandler = commands.QueryHandler[GetChannelConnectionQuery, commands.ChannelConnectionView]
type GetConnectionCapabilitiesHandler = commands.QueryHandler[GetConnectionCapabilitiesQuery, commands.ListResult[ConnectionCapabilityView]]

type GetDashboardOverviewQuery struct{ Meta QueryMeta }
type DashboardOverviewView struct {
	OpenConversations         int
	WaitingHuman              int
	NewCustomers              int
	NewLeads                  int
	TransactionsNeedingReview int
}
type GetDashboardOverviewHandler = commands.QueryHandler[GetDashboardOverviewQuery, DashboardOverviewView]

type ListConversationsQuery struct {
	Meta       QueryMeta
	Limit      int
	Cursor     string
	State      string
	Ownership  string
	Channel    string
	CustomerID *commands.CustomerID
}
type GetConversationQuery struct {
	Meta           QueryMeta
	ConversationID commands.ConversationID
}
type ListConversationMessagesQuery struct {
	Meta           QueryMeta
	ConversationID commands.ConversationID
	Limit          int
	Cursor         string
}
type ListConversationsHandler = commands.QueryHandler[ListConversationsQuery, commands.ListResult[commands.ConversationView]]
type GetConversationHandler = commands.QueryHandler[GetConversationQuery, commands.ConversationView]
type ListConversationMessagesHandler = commands.QueryHandler[ListConversationMessagesQuery, commands.ListResult[commands.MessageView]]

type ListCustomersQuery struct {
	Meta   QueryMeta
	Limit  int
	Cursor string
	Search string
	Status string
}
type GetCustomerQuery struct {
	Meta       QueryMeta
	CustomerID commands.CustomerID
}
type ListCustomerConversationsQuery struct {
	Meta       QueryMeta
	CustomerID commands.CustomerID
	Limit      int
	Cursor     string
}
type ListCustomerTransactionsQuery struct {
	Meta       QueryMeta
	CustomerID commands.CustomerID
	Limit      int
	Cursor     string
}
type ListCustomersHandler = commands.QueryHandler[ListCustomersQuery, commands.ListResult[commands.CustomerView]]
type GetCustomerHandler = commands.QueryHandler[GetCustomerQuery, commands.CustomerView]
type ListCustomerConversationsHandler = commands.QueryHandler[ListCustomerConversationsQuery, commands.ListResult[commands.ConversationView]]
type ListCustomerTransactionsHandler = commands.QueryHandler[ListCustomerTransactionsQuery, commands.ListResult[commands.TransactionView]]

type ListCatalogsQuery struct {
	Meta   QueryMeta
	Limit  int
	Cursor string
	Status string
}
type GetCatalogQuery struct {
	Meta      QueryMeta
	CatalogID commands.CatalogID
}
type ListCatalogItemsQuery struct {
	Meta      QueryMeta
	CatalogID commands.CatalogID
	Limit     int
	Cursor    string
	Search    string
	Status    string
}
type GetCatalogItemQuery struct {
	Meta      QueryMeta
	CatalogID commands.CatalogID
	ItemID    commands.CatalogItemID
}
type ListOffersQuery struct {
	Meta   QueryMeta
	ItemID commands.CatalogItemID
	Limit  int
	Cursor string
	Status string
}
type ListVariantsQuery struct {
	Meta   QueryMeta
	ItemID commands.CatalogItemID
	Limit  int
	Cursor string
	Status string
}
type ListAttributeSchemasQuery struct {
	Meta    QueryMeta
	Limit   int
	Cursor  string
	Name    string
	Version *int
}
type GetAttributeSchemaQuery struct {
	Meta     QueryMeta
	SchemaID commands.AttributeSchemaID
}
type ListCatalogsHandler = commands.QueryHandler[ListCatalogsQuery, commands.ListResult[commands.CatalogView]]
type GetCatalogHandler = commands.QueryHandler[GetCatalogQuery, commands.CatalogView]
type ListCatalogItemsHandler = commands.QueryHandler[ListCatalogItemsQuery, commands.ListResult[commands.CatalogItemView]]
type GetCatalogItemHandler = commands.QueryHandler[GetCatalogItemQuery, commands.CatalogItemView]
type ListOffersHandler = commands.QueryHandler[ListOffersQuery, commands.ListResult[commands.OfferView]]
type ListVariantsHandler = commands.QueryHandler[ListVariantsQuery, commands.ListResult[commands.VariantView]]
type ListAttributeSchemasHandler = commands.QueryHandler[ListAttributeSchemasQuery, commands.ListResult[commands.AttributeSchemaView]]
type GetAttributeSchemaHandler = commands.QueryHandler[GetAttributeSchemaQuery, commands.AttributeSchemaView]

type ListLeadsQuery struct {
	Meta       QueryMeta
	Limit      int
	Cursor     string
	Status     string
	CustomerID *commands.CustomerID
	ScoreBand  string
}
type GetLeadQuery struct {
	Meta   QueryMeta
	LeadID commands.LeadID
}
type ListLeadAttributionsQuery struct {
	Meta   QueryMeta
	LeadID commands.LeadID
	Limit  int
	Cursor string
}
type ListLeadScoresQuery struct {
	Meta   QueryMeta
	LeadID commands.LeadID
	Limit  int
	Cursor string
}
type LeadAttributionView struct {
	ID                   commands.ID
	LeadID               commands.LeadID
	SourceConversationID *commands.ConversationID
	SourceChannel        string
}
type LeadScoreView struct {
	ID     commands.ID
	LeadID commands.LeadID
	Value  float64
	Band   string
}
type ListLeadsHandler = commands.QueryHandler[ListLeadsQuery, commands.ListResult[commands.LeadView]]
type GetLeadHandler = commands.QueryHandler[GetLeadQuery, commands.LeadView]
type ListLeadAttributionsHandler = commands.QueryHandler[ListLeadAttributionsQuery, commands.ListResult[LeadAttributionView]]
type ListLeadScoresHandler = commands.QueryHandler[ListLeadScoresQuery, commands.ListResult[LeadScoreView]]

type ListTransactionsQuery struct {
	Meta            QueryMeta
	Limit           int
	Cursor          string
	State           string
	TransactionType string
	CustomerID      *commands.CustomerID
}
type GetTransactionQuery struct {
	Meta          QueryMeta
	TransactionID commands.TransactionID
}
type GetTransactionReviewQuery struct {
	Meta          QueryMeta
	TransactionID commands.TransactionID
}
type ListTransactionsHandler = commands.QueryHandler[ListTransactionsQuery, commands.ListResult[commands.TransactionView]]
type GetTransactionHandler = commands.QueryHandler[GetTransactionQuery, commands.TransactionView]
type TransactionReviewView struct {
	Required          bool
	Status            string
	ReasonCodes       []string
	ReviewerReference string
}
type GetTransactionReviewHandler = commands.QueryHandler[GetTransactionReviewQuery, TransactionReviewView]

type ListAIDecisionsQuery struct {
	Meta           QueryMeta
	Limit          int
	Cursor         string
	Lifecycle      string
	ConversationID *commands.ConversationID
	RequiresHuman  *bool
}
type GetAIDecisionQuery struct {
	Meta       QueryMeta
	DecisionID commands.AIDecisionID
}
type ListAIDecisionsHandler = commands.QueryHandler[ListAIDecisionsQuery, commands.ListResult[commands.AIDecisionView]]
type GetAIDecisionHandler = commands.QueryHandler[GetAIDecisionQuery, commands.AIDecisionView]

type ListAuditEventsQuery struct {
	Meta         QueryMeta
	Limit        int
	Cursor       string
	ActorType    string
	Action       string
	ResourceType string
	From         string
	Until        string
}
type GetAuditEventQuery struct {
	Meta         QueryMeta
	AuditEventID commands.AuditEventID
}
type ListAuditEventsHandler = commands.QueryHandler[ListAuditEventsQuery, commands.ListResult[commands.AuditEventView]]
type GetAuditEventHandler = commands.QueryHandler[GetAuditEventQuery, commands.AuditEventView]
