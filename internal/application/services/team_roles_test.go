package services

import "testing"

func TestTeamRoleCapabilities(t *testing.T) {
	if !canManageTeam(" OWNER ") || canManageTeam("manager") {
		t.Fatal("team management capability is not restricted to owner/admin")
	}
	if !canReceiveConversationAssignment("agent") || canReceiveConversationAssignment("analyst") || canReceiveConversationAssignment("viewer") {
		t.Fatal("assignment capability does not match supported roles")
	}
	if !canAssignConversations("manager") || canAssignConversations("analyst") || canAssignConversations("viewer") {
		t.Fatal("conversation assignment authority does not match supported roles")
	}
	if !validTeamRole("admin") || validTeamRole("platform_super_admin") {
		t.Fatal("business role validation must not accept a platform role")
	}
}
