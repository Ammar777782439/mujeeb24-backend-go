package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type InboundAutomationService struct {
	Rules         ports.AutomationRuleRepository
	Executions    ports.AutomationExecutionRepository
	Conversations ports.ConversationRuntimeRepository
	Reader        ports.ConversationRepository
	Labels        ports.ConversationLabelRepository
	Assignees     ports.TeamRepository
	Transactions  ports.TransactionManager
	Now           func() time.Time
	NewID         func() string
}

func (s InboundAutomationService) Handle(ctx context.Context, command commands.ApplyInboundAutomationCommand) (commands.EmptyResult, error) {
	if s.Rules == nil || s.Executions == nil || s.Conversations == nil || s.Reader == nil || s.Labels == nil || s.Assignees == nil || s.Transactions == nil {
		return commands.EmptyResult{}, appErrors.NotImplemented()
	}
	if strings.TrimSpace(string(command.BusinessID)) == "" || strings.TrimSpace(string(command.ConversationID)) == "" || strings.TrimSpace(string(command.InboundEventID)) == "" {
		return commands.EmptyResult{}, appErrors.New(appErrors.CodeValidation, "business, conversation, and inbound event ids are required")
	}
	rules, err := s.Rules.ListActiveInbound(ctx, string(command.BusinessID))
	if err != nil {
		return commands.EmptyResult{}, mapAIRepositoryError(err)
	}
	for _, rule := range rules {
		if ruleMatchesInbound(rule.Conditions, command.Channel, command.Text) {
			if err := s.runRule(ctx, rule, command); err != nil {
				return commands.EmptyResult{}, err
			}
		}
	}
	return commands.EmptyResult{}, nil
}

func (s InboundAutomationService) runRule(ctx context.Context, rule ports.AutomationRuleRecord, command commands.ApplyInboundAutomationCommand) error {
	newID := uuid.NewString
	if s.NewID != nil {
		newID = s.NewID
	}
	return s.Transactions.Within(ctx, func(txCtx context.Context) error {
		current, err := s.Rules.GetByID(txCtx, string(command.BusinessID), rule.ID)
		if err != nil {
			return mapAIRepositoryError(err)
		}
		if current.Status != "active" || current.TriggerKind != "inbound_message" || !ruleMatchesInbound(current.Conditions, command.Channel, command.Text) {
			return nil
		}
		created, execution, err := s.Executions.RecordIfAbsent(txCtx, ports.AutomationExecutionDraft{ID: newID(), BusinessID: string(command.BusinessID), RuleID: current.ID, InboundEventID: string(command.InboundEventID), Result: "processing", ReasonCode: "matched", CreatedAt: s.now(), UpdatedAt: s.now()})
		if err != nil || !created {
			return mapAIRepositoryError(err)
		}
		if actionErr := s.applyAction(txCtx, current, command); actionErr != nil {
			return s.complete(txCtx, execution.ID, "failed", automationReasonCode(actionErr))
		}
		return s.complete(txCtx, execution.ID, "executed", "applied")
	})
}

func (s InboundAutomationService) applyAction(ctx context.Context, rule ports.AutomationRuleRecord, command commands.ApplyInboundAutomationCommand) error {
	conversation, err := s.Reader.GetByID(ctx, string(command.BusinessID), string(command.ConversationID))
	if err != nil {
		return mapAIRepositoryError(err)
	}
	switch rule.ActionKind {
	case "add_label":
		var payload struct {
			Label string `json:"label"`
		}
		if err := json.Unmarshal(rule.ActionPayload, &payload); err != nil || strings.TrimSpace(payload.Label) == "" {
			return appErrors.New(appErrors.CodeValidation, "automation add_label payload requires label")
		}
		if _, err := s.Conversations.AdvanceVersion(ctx, string(command.BusinessID), string(command.ConversationID), conversation.ResourceVersion); err != nil {
			return mapAIRepositoryError(err)
		}
		return mapAIRepositoryError(s.Labels.Apply(ctx, string(command.BusinessID), string(command.ConversationID), []string{payload.Label}, nil))
	case "set_priority":
		var payload struct {
			Priority string `json:"priority"`
		}
		if err := json.Unmarshal(rule.ActionPayload, &payload); err != nil || !validPriority(payload.Priority) {
			return appErrors.New(appErrors.CodeValidation, "automation set_priority payload requires valid priority")
		}
		_, err := s.Conversations.Update(ctx, ports.ConversationUpdate{BusinessID: string(command.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: conversation.ResourceVersion, Priority: automationStringPointer(strings.ToLower(strings.TrimSpace(payload.Priority)))})
		return mapAIRepositoryError(err)
	case "assign_human":
		var payload struct {
			AssigneePrincipalID string `json:"assignee_principal_id"`
		}
		if err := json.Unmarshal(rule.ActionPayload, &payload); err != nil {
			return appErrors.New(appErrors.CodeValidation, "automation assign_human payload requires assignee_principal_id UUID")
		}
		payload.AssigneePrincipalID = strings.TrimSpace(payload.AssigneePrincipalID)
		if uuid.Validate(payload.AssigneePrincipalID) != nil {
			return appErrors.New(appErrors.CodeValidation, "automation assign_human payload requires assignee_principal_id UUID")
		}
		assignee, err := s.Assignees.ResolveActiveMember(ctx, string(command.BusinessID), payload.AssigneePrincipalID)
		if err != nil {
			return mapTeamRepositoryError(err)
		}
		if !canReceiveConversationAssignment(assignee.Role) {
			return appErrors.New(appErrors.CodeForbidden, "team member cannot receive conversation assignments")
		}
		ownership := "human"
		_, err = s.Conversations.Update(ctx, ports.ConversationUpdate{BusinessID: string(command.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: conversation.ResourceVersion, Ownership: &ownership, AssignmentReference: automationStringPointer(payload.AssigneePrincipalID)})
		return mapAIRepositoryError(err)
	default:
		return appErrors.New(appErrors.CodeValidation, "automation action is not supported")
	}
}

func (s InboundAutomationService) complete(ctx context.Context, executionID, result, reason string) error {
	if err := s.Executions.Complete(ctx, ports.AutomationExecutionPatch{ID: executionID, Result: result, ReasonCode: reason, UpdatedAt: s.now()}); err != nil {
		return mapAIRepositoryError(err)
	}
	return nil
}

func (s InboundAutomationService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func automationStringPointer(value string) *string { return &value }

var _ commands.ApplyInboundAutomationHandler = InboundAutomationService{}
