package services

import (
	"context"
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
	var result commands.AutomationRuleResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, createErr := s.Repository.Create(txCtx, ports.AutomationRuleCreate{ID: newID(), BusinessID: string(command.Meta.Actor.BusinessID), Name: name, Status: "active", TriggerKind: "inbound_message", Conditions: conditions, ActionKind: actionKind, ActionPayload: payload, Position: position, CreatedAt: s.now(), UpdatedAt: s.now()})
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

func automationRuleView(record ports.AutomationRuleRecord) commands.AutomationRuleView {
	return commands.AutomationRuleView{ID: commands.AutomationRuleID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), Name: record.Name, Status: record.Status, TriggerKind: record.TriggerKind, Conditions: record.Conditions, ActionKind: record.ActionKind, ActionPayload: record.ActionPayload, Position: record.Position, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10)), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func automationRuleResult(record ports.AutomationRuleRecord) commands.AutomationRuleResult {
	view := automationRuleView(record)
	return commands.AutomationRuleResult{MutationResult: commands.MutationResult{ResourceID: commands.ID(record.ID), ResourceVersion: view.ResourceVersion, Status: record.Status}, AutomationRule: view}
}

var _ queries.ListAutomationRulesHandler = ListAutomationRulesQueryService{}
var _ commands.CreateAutomationRuleHandler = CreateAutomationRuleCommandService{}
var _ commands.UpdateAutomationRuleHandler = UpdateAutomationRuleCommandService{}
