package services

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/google/uuid"
)

type AutomationRuleService struct {
	Repository   ports.AutomationRuleRepository
	Transactions ports.TransactionManager
	Now          func() time.Time
	NewID        func() string
}

type ListAutomationRulesQueryService struct{ AutomationRuleService }
type CreateAutomationRuleCommandService struct{ AutomationRuleService }
type UpdateAutomationRuleCommandService struct{ AutomationRuleService }

func (s ListAutomationRulesQueryService) Handle(ctx context.Context, query queries.ListAutomationRulesQuery) (commands.ListResult[commands.AutomationRuleView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.AutomationRuleView]{}, appErrors.NotImplemented()
	}
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), strings.TrimSpace(query.Status), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.AutomationRuleView]{}, mapAIRepositoryError(err)
	}
	items := make([]commands.AutomationRuleView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, automationRuleView(item))
	}
	return commands.ListResult[commands.AutomationRuleView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s CreateAutomationRuleCommandService) Handle(ctx context.Context, command commands.CreateAutomationRuleCommand) (commands.AutomationRuleResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.AutomationRuleResult{}, appErrors.NotImplemented()
	}
	name, conditions, actionKind, payload, position, err := validateAutomationRuleInput(command.Name, command.Conditions, command.ActionKind, command.ActionPayload, command.Position)
	if err != nil {
		return commands.AutomationRuleResult{}, err
	}
	newID := uuid.NewString
	if s.NewID != nil {
		newID = s.NewID
	}
	now := s.now()
	var result commands.AutomationRuleResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, createErr := s.Repository.Create(txCtx, ports.AutomationRuleCreate{ID: newID(), BusinessID: string(command.Meta.Actor.BusinessID), Name: name, Status: "active", TriggerKind: "inbound_message", Conditions: conditions, ActionKind: actionKind, ActionPayload: payload, Position: position, CreatedAt: now, UpdatedAt: now})
		if createErr != nil {
			return mapAIRepositoryError(createErr)
		}
		result = automationRuleResult(record)
		return nil
	})
	return result, err
}

func (s UpdateAutomationRuleCommandService) Handle(ctx context.Context, command commands.UpdateAutomationRuleCommand) (commands.AutomationRuleResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.AutomationRuleResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.AutomationRuleResult{}, err
	}
	name, err := optionalAutomationName(command.Name)
	if err != nil {
		return commands.AutomationRuleResult{}, err
	}
	status := optionalAutomationStatus(command.Status)
	if command.Status != nil && status == nil {
		return commands.AutomationRuleResult{}, appErrors.New(appErrors.CodeValidation, "automation rule status must be active or disabled")
	}
	conditions, err := optionalAutomationConditions(command.Conditions)
	if err != nil {
		return commands.AutomationRuleResult{}, err
	}
	actionKind, payload, err := optionalAutomationAction(command.ActionKind, command.ActionPayload)
	if err != nil {
		return commands.AutomationRuleResult{}, err
	}
	if command.Position != nil && *command.Position <= 0 {
		return commands.AutomationRuleResult{}, appErrors.New(appErrors.CodeValidation, "automation rule position must be positive")
	}
	if name == nil && status == nil && conditions == nil && actionKind == nil && payload == nil && command.Position == nil {
		return commands.AutomationRuleResult{}, appErrors.New(appErrors.CodeValidation, "at least one automation rule field is required")
	}
	var result commands.AutomationRuleResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, updateErr := s.Repository.Update(txCtx, ports.AutomationRuleUpdate{BusinessID: string(command.Meta.Actor.BusinessID), AutomationRuleID: string(command.AutomationRuleID), ExpectedVersion: expected, Name: name, Status: status, Conditions: conditions, ActionKind: actionKind, ActionPayload: payload, Position: command.Position, UpdatedAt: s.now()})
		if updateErr != nil {
			return mapAIRepositoryError(updateErr)
		}
		result = automationRuleResult(record)
		return nil
	})
	return result, err
}

func (s AutomationRuleService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type InboundAutomationService struct {
	Rules         ports.AutomationRuleRepository
	Executions    ports.AutomationExecutionRepository
	Conversations ports.ConversationRuntimeRepository
	Reader        ports.ConversationRepository
	Labels        ports.ConversationLabelRepository
	Transactions  ports.TransactionManager
	Now           func() time.Time
	NewID         func() string
}

func (s InboundAutomationService) Handle(ctx context.Context, command commands.ApplyInboundAutomationCommand) (commands.EmptyResult, error) {
	if s.Rules == nil || s.Executions == nil || s.Conversations == nil || s.Reader == nil || s.Labels == nil || s.Transactions == nil {
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
		if !ruleMatchesInbound(rule.Conditions, command.Channel, command.Text) {
			continue
		}
		if err := s.runRule(ctx, rule, command); err != nil {
			return commands.EmptyResult{}, err
		}
	}
	return commands.EmptyResult{}, nil
}

func (s InboundAutomationService) runRule(ctx context.Context, rule ports.AutomationRuleRecord, command commands.ApplyInboundAutomationCommand) error {
	newID := uuid.NewString
	if s.NewID != nil {
		newID = s.NewID
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	return s.Transactions.Within(ctx, func(txCtx context.Context) error {
		current, err := s.Rules.GetByID(txCtx, string(command.BusinessID), rule.ID)
		if err != nil {
			return mapAIRepositoryError(err)
		}
		if current.Status != "active" || current.TriggerKind != "inbound_message" || !ruleMatchesInbound(current.Conditions, command.Channel, command.Text) {
			return nil
		}
		created, execution, err := s.Executions.RecordIfAbsent(txCtx, ports.AutomationExecutionDraft{ID: newID(), BusinessID: string(command.BusinessID), RuleID: current.ID, InboundEventID: string(command.InboundEventID), Result: "processing", ReasonCode: "matched", CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return mapAIRepositoryError(err)
		}
		if !created {
			return nil
		}
		if actionErr := s.applyAction(txCtx, current, command); actionErr != nil {
			if completeErr := s.Executions.Complete(txCtx, ports.AutomationExecutionPatch{ID: execution.ID, Result: "failed", ReasonCode: automationReasonCode(actionErr), UpdatedAt: now}); completeErr != nil {
				return mapAIRepositoryError(completeErr)
			}
			return nil
		}
		if err := s.Executions.Complete(txCtx, ports.AutomationExecutionPatch{ID: execution.ID, Result: "executed", ReasonCode: "applied", UpdatedAt: now}); err != nil {
			return mapAIRepositoryError(err)
		}
		return nil
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
		_, err := s.Conversations.Update(ctx, ports.ConversationUpdate{BusinessID: string(command.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: conversation.ResourceVersion, Priority: stringPointer(strings.ToLower(strings.TrimSpace(payload.Priority)))})
		return mapAIRepositoryError(err)
	case "assign_human":
		var payload struct {
			AssigneeReference string `json:"assignee_reference"`
		}
		if err := json.Unmarshal(rule.ActionPayload, &payload); err != nil || strings.TrimSpace(payload.AssigneeReference) == "" {
			return appErrors.New(appErrors.CodeValidation, "automation assign_human payload requires assignee_reference")
		}
		ownership := "human"
		_, err := s.Conversations.Update(ctx, ports.ConversationUpdate{BusinessID: string(command.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: conversation.ResourceVersion, Ownership: &ownership, AssignmentReference: stringPointer(strings.TrimSpace(payload.AssigneeReference))})
		return mapAIRepositoryError(err)
	default:
		return appErrors.New(appErrors.CodeValidation, "automation action is not supported")
	}
}

func automationRuleView(record ports.AutomationRuleRecord) commands.AutomationRuleView {
	return commands.AutomationRuleView{ID: commands.AutomationRuleID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), Name: record.Name, Status: record.Status, TriggerKind: record.TriggerKind, Conditions: record.Conditions, ActionKind: record.ActionKind, ActionPayload: record.ActionPayload, Position: record.Position, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10)), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func automationRuleResult(record ports.AutomationRuleRecord) commands.AutomationRuleResult {
	view := automationRuleView(record)
	return commands.AutomationRuleResult{MutationResult: commands.MutationResult{ResourceID: commands.ID(record.ID), ResourceVersion: view.ResourceVersion, Status: record.Status}, AutomationRule: view}
}

func validateAutomationRuleInput(name string, conditions []byte, actionKind string, payload []byte, position int) (string, []byte, string, []byte, int, error) {
	normalizedName, err := requiredCannedReplyText(name, "automation rule name", 200, false)
	if err != nil {
		return "", nil, "", nil, 0, err
	}
	conditions, err = normalizeAutomationConditions(conditions)
	if err != nil {
		return "", nil, "", nil, 0, err
	}
	actionKind, payload, err = normalizeAutomationAction(actionKind, payload)
	if err != nil {
		return "", nil, "", nil, 0, err
	}
	if position <= 0 {
		position = 100
	}
	return normalizedName, conditions, actionKind, payload, position, nil
}

func optionalAutomationName(value *string) (*string, error) {
	return optionalCannedReplyText(value, "automation rule name", 200, false)
}

func optionalAutomationStatus(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized != "active" && normalized != "disabled" {
		return nil
	}
	return &normalized
}

func optionalAutomationConditions(value []byte) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return normalizeAutomationConditions(value)
}

func optionalAutomationAction(kind *string, payload []byte) (*string, []byte, error) {
	if kind == nil && payload == nil {
		return nil, nil, nil
	}
	if kind == nil || payload == nil {
		return nil, nil, appErrors.New(appErrors.CodeValidation, "automation action kind and payload must change together")
	}
	normalizedKind, normalizedPayload, err := normalizeAutomationAction(*kind, payload)
	if err != nil {
		return nil, nil, err
	}
	return &normalizedKind, normalizedPayload, nil
}

func normalizeAutomationConditions(raw []byte) ([]byte, error) {
	var conditions struct {
		Channel      string `json:"channel"`
		TextContains string `json:"text_contains"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &conditions) != nil {
		return nil, appErrors.New(appErrors.CodeValidation, "automation conditions must be a valid JSON object")
	}
	conditions.Channel = strings.ToLower(strings.TrimSpace(conditions.Channel))
	conditions.TextContains = strings.ToLower(strings.TrimSpace(conditions.TextContains))
	if conditions.Channel == "" && conditions.TextContains == "" {
		return nil, appErrors.New(appErrors.CodeValidation, "automation conditions require channel or text_contains")
	}
	result, _ := json.Marshal(conditions)
	return result, nil
}

func normalizeAutomationAction(kind string, raw []byte) (string, []byte, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "add_label" && kind != "set_priority" && kind != "assign_human" {
		return "", nil, appErrors.New(appErrors.CodeValidation, "automation action must be add_label, set_priority, or assign_human")
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return "", nil, appErrors.New(appErrors.CodeValidation, "automation action payload must be a valid JSON object")
	}
	var normalized []byte
	switch kind {
	case "add_label":
		var payload struct {
			Label string `json:"label"`
		}
		if json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.Label) == "" {
			return "", nil, appErrors.New(appErrors.CodeValidation, "automation add_label payload requires label")
		}
		payload.Label = strings.ToLower(strings.TrimSpace(payload.Label))
		normalized, _ = json.Marshal(payload)
	case "set_priority":
		var payload struct {
			Priority string `json:"priority"`
		}
		if json.Unmarshal(raw, &payload) != nil || !validPriority(payload.Priority) {
			return "", nil, appErrors.New(appErrors.CodeValidation, "automation set_priority payload requires valid priority")
		}
		payload.Priority = strings.ToLower(strings.TrimSpace(payload.Priority))
		normalized, _ = json.Marshal(payload)
	case "assign_human":
		var payload struct {
			AssigneeReference string `json:"assignee_reference"`
		}
		if json.Unmarshal(raw, &payload) != nil || strings.TrimSpace(payload.AssigneeReference) == "" {
			return "", nil, appErrors.New(appErrors.CodeValidation, "automation assign_human payload requires assignee_reference")
		}
		payload.AssigneeReference = strings.TrimSpace(payload.AssigneeReference)
		normalized, _ = json.Marshal(payload)
	}
	return kind, normalized, nil
}

func ruleMatchesInbound(raw []byte, channel, text string) bool {
	var conditions struct {
		Channel      string `json:"channel"`
		TextContains string `json:"text_contains"`
	}
	if json.Unmarshal(raw, &conditions) != nil {
		return false
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	text = strings.ToLower(strings.TrimSpace(text))
	return (conditions.Channel == "" || conditions.Channel == channel) && (conditions.TextContains == "" || strings.Contains(text, conditions.TextContains))
}

func validPriority(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "normal", "high", "urgent":
		return true
	default:
		return false
	}
}

func automationReasonCode(err error) string {
	var typed *appErrors.Error
	if errors.As(err, &typed) && typed.Code == appErrors.CodeNotFound {
		return "target_not_found"
	}
	return "action_rejected"
}

var _ queries.ListAutomationRulesHandler = ListAutomationRulesQueryService{}
var _ commands.CreateAutomationRuleHandler = CreateAutomationRuleCommandService{}
var _ commands.UpdateAutomationRuleHandler = UpdateAutomationRuleCommandService{}
var _ commands.ApplyInboundAutomationHandler = InboundAutomationService{}
