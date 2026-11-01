package authorization

import (
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func ConversationAccess(actor commands.ActorContext) ports.ConversationAccess {
	role := strings.ToLower(strings.TrimSpace(actor.Role))
	return ports.ConversationAccess{PrincipalID: string(actor.PrincipalID), Supervisor: role == "owner" || role == "admin" || role == "manager"}
}

func ConversationAssigned(record ports.ConversationRecord) bool {
	return record.Ownership == "human" && record.AssignmentReference != nil && strings.TrimSpace(*record.AssignmentReference) != ""
}

func CanReadConversation(actor commands.ActorContext, record ports.ConversationRecord) bool {
	return string(actor.BusinessID) == record.BusinessID && string(actor.PrincipalID) != "" && Has(actor, PermissionConversationsRead) &&
		(ConversationAccess(actor).Supervisor || !ConversationAssigned(record) || *record.AssignmentReference == string(actor.PrincipalID))
}

func CanReplyToConversation(actor commands.ActorContext, record ports.ConversationRecord) bool {
	return CanReadConversation(actor, record) && Has(actor, PermissionConversationsReply) && record.State != "closed" && ConversationAssigned(record) &&
		(ConversationAccess(actor).Supervisor || *record.AssignmentReference == string(actor.PrincipalID))
}

func CanClaimConversation(actor commands.ActorContext, record ports.ConversationRecord) bool {
	role := strings.ToLower(strings.TrimSpace(actor.Role))
	eligible := role == "owner" || role == "admin" || role == "manager" || role == "agent"
	return eligible && CanReadConversation(actor, record) && Has(actor, PermissionConversationsReply) && record.State != "closed" &&
		(!ConversationAssigned(record) || (ConversationAccess(actor).Supervisor && Has(actor, PermissionConversationsAssign) && *record.AssignmentReference != string(actor.PrincipalID)))
}
