package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestInboundAutomationAssignHumanRequiresAssignableActiveMember(t *testing.T) {
	assigneeID := "00000000-0000-0000-0000-000000000112"
	for _, testCase := range []struct {
		name           string
		role           string
		wantCompletion string
		wantUpdate     bool
	}{
		{name: "active agent", role: TeamRoleAgent, wantCompletion: "executed", wantUpdate: true},
		{name: "viewer cannot receive assignment", role: TeamRoleViewer, wantCompletion: "failed", wantUpdate: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{"assignee_principal_id": assigneeID})
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			rule := ports.AutomationRuleRecord{
				ID:            "rule-1",
				BusinessID:    "business-1",
				Status:        "active",
				TriggerKind:   "inbound_message",
				Conditions:    []byte(`{"text_contains":"مساعدة"}`),
				ActionKind:    "assign_human",
				ActionPayload: payload,
			}
			rules := &automationRulesFixture{rule: rule}
			executions := &automationExecutionsFixture{}
			conversations := &automationConversationFixture{record: ports.ConversationRecord{ResourceVersion: 1}}
			assignees := &automationAssigneeFixture{record: ports.TeamMemberRecord{PrincipalID: assigneeID, Role: testCase.role, Status: "active"}}
			service := InboundAutomationService{
				Rules:         rules,
				Executions:    executions,
				Conversations: conversations,
				Reader:        conversations,
				Labels:        automationLabelsFixture{},
				Assignees:     assignees,
				Transactions:  passthroughTransactionManager{},
				Now:           func() time.Time { return time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC) },
				NewID:         func() string { return "execution-1" },
			}

			_, err = service.Handle(context.Background(), commands.ApplyInboundAutomationCommand{
				BusinessID:     "business-1",
				ConversationID: "conversation-1",
				InboundEventID: "event-1",
				Channel:        "instagram",
				Text:           "أحتاج مساعدة",
			})
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if executions.completion.Result != testCase.wantCompletion {
				t.Fatalf("expected completion %q, got %#v", testCase.wantCompletion, executions.completion)
			}
			if conversations.updateCalled != testCase.wantUpdate {
				t.Fatalf("unexpected update called=%t", conversations.updateCalled)
			}
			if testCase.wantUpdate && (conversations.update.AssignmentReference == nil || *conversations.update.AssignmentReference != assigneeID) {
				t.Fatalf("expected principal assignment, got %#v", conversations.update)
			}
			if assignees.businessID != "business-1" || assignees.principalID != assigneeID {
				t.Fatalf("unexpected assignee lookup business=%q principal=%q", assignees.businessID, assignees.principalID)
			}
		})
	}
}

type automationRulesFixture struct {
	rule ports.AutomationRuleRecord
}

func (r *automationRulesFixture) List(context.Context, string, string, int, string) (ports.AutomationRulePage, error) {
	return ports.AutomationRulePage{}, nil
}

func (r *automationRulesFixture) ListActiveInbound(context.Context, string) ([]ports.AutomationRuleRecord, error) {
	return []ports.AutomationRuleRecord{r.rule}, nil
}

func (r *automationRulesFixture) GetByID(context.Context, string, string) (ports.AutomationRuleRecord, error) {
	return r.rule, nil
}

func (r *automationRulesFixture) Create(context.Context, ports.AutomationRuleCreate) (ports.AutomationRuleRecord, error) {
	return r.rule, nil
}

func (r *automationRulesFixture) Update(context.Context, ports.AutomationRuleUpdate) (ports.AutomationRuleRecord, error) {
	return r.rule, nil
}

type automationExecutionsFixture struct {
	completion ports.AutomationExecutionPatch
}

func (r *automationExecutionsFixture) RecordIfAbsent(_ context.Context, draft ports.AutomationExecutionDraft) (bool, ports.AutomationExecutionDraft, error) {
	return true, draft, nil
}

func (r *automationExecutionsFixture) Complete(_ context.Context, patch ports.AutomationExecutionPatch) error {
	r.completion = patch
	return nil
}

type automationConversationFixture struct {
	record       ports.ConversationRecord
	update       ports.ConversationUpdate
	updateCalled bool
}

func (r *automationConversationFixture) GetByID(context.Context, string, string) (ports.ConversationRecord, error) {
	return r.record, nil
}

func (r *automationConversationFixture) List(context.Context, string, string, string, string, *string, int, string) (ports.ConversationPage, error) {
	return ports.ConversationPage{}, nil
}

func (r *automationConversationFixture) Update(_ context.Context, update ports.ConversationUpdate) (ports.ConversationRecord, error) {
	r.update = update
	r.updateCalled = true
	return r.record, nil
}

func (r *automationConversationFixture) AdvanceVersion(context.Context, string, string, int64) (ports.ConversationRecord, error) {
	return r.record, nil
}

func (r *automationConversationFixture) TransitionLifecycle(context.Context, ports.ConversationLifecycleTransition) (ports.ConversationRecord, error) {
	return r.record, nil
}

// UpdateLastGeminiInteractionID satisfies the contract ③ §4 method added to
// ports.ConversationRuntimeRepository. Tests don't exercise Gemini chaining
// end-to-end; this no-op allows the fake to satisfy the interface.
func (r *automationConversationFixture) UpdateLastGeminiInteractionID(_ context.Context, _, _, _ string) error {
	return nil
}

type automationLabelsFixture struct{}

func (automationLabelsFixture) List(context.Context, string, string) ([]string, error) {
	return nil, nil
}

func (automationLabelsFixture) Apply(context.Context, string, string, []string, []string) error {
	return nil
}

type automationAssigneeFixture struct {
	record      ports.TeamMemberRecord
	businessID  string
	principalID string
}

func (r *automationAssigneeFixture) ListMembers(context.Context, string, int, string) (ports.TeamMemberPage, error) {
	return ports.TeamMemberPage{}, nil
}

func (r *automationAssigneeFixture) ResolveActiveMember(_ context.Context, businessID, principalID string) (ports.TeamMemberRecord, error) {
	r.businessID = businessID
	r.principalID = principalID
	return r.record, nil
}

func (r *automationAssigneeFixture) CreateInvitation(context.Context, ports.TeamInvitationCreate) (ports.TeamInvitationRecord, error) {
	return ports.TeamInvitationRecord{}, nil
}

func (r *automationAssigneeFixture) AcceptInvitation(context.Context, ports.TeamInvitationAcceptance) (ports.TeamInvitationRecord, error) {
	return ports.TeamInvitationRecord{}, nil
}

func (r *automationAssigneeFixture) UpdateMemberRole(context.Context, ports.TeamMemberRoleChange) (ports.TeamMemberRecord, error) {
	return ports.TeamMemberRecord{}, nil
}

func (r *automationAssigneeFixture) RevokeMember(context.Context, ports.TeamMemberRevocation) (ports.TeamMemberRecord, error) {
	return ports.TeamMemberRecord{}, nil
}
