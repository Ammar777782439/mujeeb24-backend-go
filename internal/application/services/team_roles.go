package services

import "strings"

const (
	TeamRoleOwner   = "owner"
	TeamRoleAdmin   = "admin"
	TeamRoleManager = "manager"
	TeamRoleAgent   = "agent"
	TeamRoleAnalyst = "analyst"
	TeamRoleViewer  = "viewer"
)

func validTeamRole(role string) bool {
	switch normalizeTeamRole(role) {
	case TeamRoleOwner, TeamRoleAdmin, TeamRoleManager, TeamRoleAgent, TeamRoleAnalyst, TeamRoleViewer:
		return true
	default:
		return false
	}
}

func canManageTeam(role string) bool {
	role = normalizeTeamRole(role)
	return role == TeamRoleOwner || role == TeamRoleAdmin
}

func canReceiveConversationAssignment(role string) bool {
	role = normalizeTeamRole(role)
	return role == TeamRoleOwner || role == TeamRoleAdmin || role == TeamRoleManager || role == TeamRoleAgent
}

func canAssignConversations(role string) bool {
	return canReceiveConversationAssignment(role)
}

func normalizeTeamRole(role string) string { return strings.ToLower(strings.TrimSpace(role)) }
