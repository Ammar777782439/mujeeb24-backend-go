package authorization

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

func TestEveryMerchantBusinessOperationHasPermission(t *testing.T) {
	operations := []string{
		"addPrivateNote", "approveTransactionReview", "assignConversation", "beginChannelConnection",
		"cancelTransaction", "confirmTransaction", "createAttributeSchemaVersion", "createAutomationRule",
		"createCannedReply", "createCatalog", "createCatalogItem", "createCustomer", "createLead", "createOffer",
		"createOutboundMessage", "createTeamInvitation", "createTransactionDraft", "createVariant", "disconnectChannel",
		"getAIDecision", "getAttributeSchema", "getAuditEvent", "getBusiness", "getBusinessPolicy", "getCatalog",
		"getCatalogEntityContract", "getCatalogItem", "getChannelConnection", "getConnectionCapabilities",
		"getConversation", "getCustomer", "getDashboardOverview", "getLead", "getTransaction", "getTransactionReview",
		"listAIDecisions", "listAttributeSchemas", "listAuditEvents", "listAutomationRules", "listCannedReplies",
		"listCatalogItems", "listCatalogs", "listChannelConnections", "listConversationMessages", "listConversations",
		"listCustomerConversations", "listCustomerTransactions", "listCustomers", "listLeadAttributions", "listLeadScores",
		"listLeads", "listOffers", "listTeamInvitations", "listTeamMembers", "listTransactions", "listVariants",
		"markConversationRead", "markLeadLost", "merchantAIChat", "merchantAIExecute", "mergeCustomer",
		"previewChannelHistoryPurge", "purgeChannelHistory", "qualifyLead", "reconnectChannel", "rejectTransactionReview",
		"requestHumanReview", "resendTeamInvitation", "revokeTeamInvitation", "revokeTeamMember", "sendCannedReply",
		"submitTransactionReview", "updateAutomationRule", "updateBusinessPolicy", "updateBusinessProfile",
		"updateCannedReply", "updateCatalog", "updateCatalogItem", "updateConversation", "updateConversationLabels",
		"updateCustomer", "updateLead", "updateOffer", "updateTeamMemberAccess", "updateTeamMemberRole",
		"updateTransactionDraft", "updateVariant",
	}
	for _, operation := range operations {
		if permission, ok := RequiredForOperation(operation); !ok || permission == "" {
			t.Errorf("merchant operation %q is not classified", operation)
		}
	}
}

func TestNormalizePermissionsAndGrantBoundary(t *testing.T) {
	permissions, ok := NormalizePermissions([]string{" conversations:read ", "CONVERSATIONS:READ", "team:read"})
	if !ok || len(permissions) != 2 || permissions[0] != "conversations:read" || permissions[1] != "team:read" {
		t.Fatalf("unexpected normalization result: %#v ok=%t", permissions, ok)
	}
	if _, ok := NormalizePermissions([]string{"*"}); ok {
		t.Fatal("wildcard must never be assignable through team APIs")
	}
	actor := commands.ActorContext{Role: "admin", Permissions: []string{"team:read"}}
	if !CanGrant(actor, []string{"team:read"}) || CanGrant(actor, []string{"team:manage"}) {
		t.Fatal("admin grant boundary did not enforce subset rule")
	}
	if !CanGrant(commands.ActorContext{Role: "owner"}, KnownPermissions()) {
		t.Fatal("owner must be able to grant every registered permission")
	}
}
