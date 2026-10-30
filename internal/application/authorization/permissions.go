package authorization

import (
	"sort"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

const (
	PermissionAll                 = "*"
	PermissionBusinessRead        = "business:read"
	PermissionDashboardRead       = "dashboard:read"
	PermissionTeamRead            = "team:read"
	PermissionTeamManage          = "team:manage"
	PermissionConversationsRead   = "conversations:read"
	PermissionConversationsReply  = "conversations:reply"
	PermissionConversationsAssign = "conversations:assign"
	PermissionCustomersRead       = "customers:read"
	PermissionCustomersWrite      = "customers:write"
	PermissionCatalogRead         = "catalog:read"
	PermissionCatalogWrite        = "catalog:write"
	PermissionSalesRead           = "sales:read"
	PermissionSalesWrite          = "sales:write"
	PermissionSalesApprove        = "sales:approve"
	PermissionChannelsManage      = "channels:manage"
	PermissionChannelsRead        = "channels:read"
	PermissionAutomationRead      = "automation:read"
	PermissionBusinessSettings    = "business:settings"
	PermissionAutomationManage    = "automation:manage"
	PermissionAuditRead           = "audit:read"
	PermissionMerchantAIUse       = "merchant_ai:use"
)

var knownPermissions = map[string]struct{}{
	PermissionBusinessRead: {}, PermissionDashboardRead: {},
	PermissionTeamRead: {}, PermissionTeamManage: {},
	PermissionConversationsRead: {}, PermissionConversationsReply: {}, PermissionConversationsAssign: {},
	PermissionCustomersRead: {}, PermissionCustomersWrite: {},
	PermissionCatalogRead: {}, PermissionCatalogWrite: {},
	PermissionSalesRead: {}, PermissionSalesWrite: {}, PermissionSalesApprove: {},
	PermissionChannelsRead: {}, PermissionChannelsManage: {}, PermissionBusinessSettings: {},
	PermissionAutomationRead: {}, PermissionAutomationManage: {}, PermissionAuditRead: {}, PermissionMerchantAIUse: {},
}

var operationPermissions = buildOperationPermissions()

func buildOperationPermissions() map[string]string {
	permissions := make(map[string]string)
	register := func(permission string, operations ...string) {
		for _, operation := range operations {
			permissions[operation] = permission
		}
	}
	register(PermissionBusinessRead, "getBusiness", "getBusinessPolicy", "getBusinessSubscription")
	register(PermissionBusinessSettings, "createSalesRequirement", "updateBusinessProfile", "updateBusinessPolicy")
	register(PermissionDashboardRead, "getDashboardOverview")
	register(PermissionTeamRead, "listTeamMembers", "listTeamInvitations")
	register(PermissionTeamManage, "createTeamInvitation", "revokeTeamInvitation", "resendTeamInvitation", "updateTeamMemberRole", "updateTeamMemberAccess", "revokeTeamMember")
	register(PermissionConversationsRead, "listConversations", "getConversation", "listConversationMessages", "listCustomerConversations", "markConversationRead")
	register(PermissionConversationsReply, "createOutboundMessage", "addPrivateNote", "updateConversationLabels", "sendCannedReply", "listCannedReplies", "createCannedReply", "updateCannedReply")
	register(PermissionConversationsAssign, "assignConversation", "updateConversation")
	register(PermissionCustomersRead, "listCustomers", "getCustomer")
	register(PermissionCustomersWrite, "createCustomer", "updateCustomer", "mergeCustomer")
	register(PermissionCatalogRead, "listCatalogMedia", "listCatalogs", "getCatalog", "getCatalogEntityContract", "listCatalogItems", "getCatalogItem", "listOffers", "listVariants", "listAttributeSchemas", "getAttributeSchema")
	register(PermissionCatalogWrite, "uploadCatalogImage", "attachCatalogMedia", "setCatalogMediaPrimary", "removeCatalogMedia", "createCatalog", "updateCatalog", "createCatalogItem", "updateCatalogItem", "createOffer", "updateOffer", "createVariant", "updateVariant", "createAttributeSchemaVersion")
	register(PermissionSalesRead, "listSalesWorkflows", "getSalesWorkflow", "listSalesRequirements", "listLeads", "getLead", "listLeadAttributions", "listLeadScores", "listTransactions", "listCustomerTransactions", "getTransaction", "getTransactionReview")
	register(PermissionSalesWrite, "createLead", "updateLead", "qualifyLead", "markLeadLost", "createTransactionDraft", "updateTransactionDraft", "confirmTransaction", "cancelTransaction", "submitTransactionReview")
	register(PermissionSalesApprove, "reviewSalesWorkflow", "approveTransactionReview", "rejectTransactionReview")
	register(PermissionChannelsRead, "listChannelConnections", "getChannelConnection", "getConnectionCapabilities", "previewChannelHistoryPurge")
	register(PermissionChannelsManage, "beginChannelConnection", "reconnectChannel", "disconnectChannel", "purgeChannelHistory", "removeChannel")
	register(PermissionAutomationRead, "listAutomationRules")
	register(PermissionAutomationManage, "createAutomationRule", "updateAutomationRule")
	register(PermissionAuditRead, "listAuditEvents", "getAuditEvent", "listAIDecisions", "getAIDecision")
	register(PermissionMerchantAIUse, "merchantAIChat", "merchantAIExecute", "merchantAIAssist", "requestHumanReview")
	return permissions
}

func RequiredForOperation(operationID string) (string, bool) {
	permission, ok := operationPermissions[operationID]
	return permission, ok
}

func NormalizePermissions(values []string) ([]string, bool) {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		permission := strings.ToLower(strings.TrimSpace(value))
		if permission == PermissionAll {
			return nil, false
		}
		if _, ok := knownPermissions[permission]; !ok {
			return nil, false
		}
		seen[permission] = struct{}{}
	}
	permissions := make([]string, 0, len(seen))
	for permission := range seen {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return permissions, true
}

func Has(actor commands.ActorContext, permission string) bool {
	if actor.Role == "owner" {
		return true
	}
	for _, granted := range actor.Permissions {
		if granted == PermissionAll || granted == permission {
			return true
		}
	}
	return false
}

func CanGrant(actor commands.ActorContext, requested []string) bool {
	if actor.Role == "owner" {
		return true
	}
	for _, permission := range requested {
		if !Has(actor, permission) {
			return false
		}
	}
	return true
}

func KnownPermissions() []string {
	permissions := make([]string, 0, len(knownPermissions))
	for permission := range knownPermissions {
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return permissions
}
