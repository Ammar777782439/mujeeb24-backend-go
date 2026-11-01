package authorization

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"testing"
)

func TestConversationOwnershipAndPermissions(t *testing.T) {
	assignee := "employee-a"
	record := ports.ConversationRecord{BusinessID: "business", Ownership: "human", State: "human_handling", AssignmentReference: &assignee}
	for _, tc := range []struct {
		name, role, id     string
		permissions        []string
		read, reply, claim bool
	}{
		{"assigned agent", "agent", "employee-a", []string{"conversations:read", "conversations:reply"}, true, true, false},
		{"other agent with wildcard", "agent", "employee-b", []string{"*"}, false, false, false},
		{"owner with no grants", "owner", "owner", nil, false, false, false},
		{"read-only owner", "owner", "owner", []string{"conversations:read"}, true, false, false},
		{"supervisor reply", "manager", "manager", []string{"conversations:read", "conversations:reply"}, true, true, false},
		{"supervisor reassign", "admin", "admin", []string{"*"}, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := commands.ActorContext{BusinessID: "business", PrincipalID: commands.PrincipalID(tc.id), Role: tc.role, Permissions: tc.permissions}
			if CanReadConversation(actor, record) != tc.read || CanReplyToConversation(actor, record) != tc.reply || CanClaimConversation(actor, record) != tc.claim {
				t.Fatal("ownership or explicit permission boundary failed")
			}
		})
	}
	actor := commands.ActorContext{BusinessID: "business", PrincipalID: "employee-a", Role: "agent", Permissions: []string{"conversations:read", "conversations:reply"}}
	record.AssignmentReference = nil
	if !CanReadConversation(actor, record) || CanReplyToConversation(actor, record) || !CanClaimConversation(actor, record) {
		t.Fatal("unassigned conversation must be claimed before public reply")
	}
	record.BusinessID = "another"
	if CanReadConversation(actor, record) || CanClaimConversation(actor, record) {
		t.Fatal("conversation crossed business boundary")
	}
}
