package handlers

import (
	"context"
	"encoding/json"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

func (s *Server) dispatchInboxCommand(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "markConversationRead":
		in := input.(*contract.ConversationReadInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.MarkConversationRead == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.MarkConversationRead.Handle(ctx, commands.MarkConversationReadCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), ConversationID: commands.ConversationID(in.ConversationID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.ConversationRead]{}
		out.Body.Data = conversationReadProjection(result)
		return out, true
	case "createCannedReply":
		in := input.(*contract.CannedReplyCreateInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.CreateCannedReply == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.CreateCannedReply.Handle(ctx, commands.CreateCannedReplyCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), Title: in.Body.Title, Shortcut: in.Body.Shortcut, Body: in.Body.Body})
		if err != nil {
			return mapApplicationError(err), true
		}
		return singleCannedReply(result.CannedReply), true
	case "updateCannedReply":
		in := input.(*contract.CannedReplyUpdateInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.UpdateCannedReply == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.UpdateCannedReply.Handle(ctx, commands.UpdateCannedReplyCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), CannedReplyID: commands.CannedReplyID(in.CannedReplyID), Title: in.Body.Title, Shortcut: in.Body.Shortcut, Body: in.Body.Body, Status: in.Body.Status})
		if err != nil {
			return mapApplicationError(err), true
		}
		return singleCannedReply(result.CannedReply), true
	case "sendCannedReply":
		in := input.(*contract.SendCannedReplyInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.SendCannedReply == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		result, err := s.deps.SendCannedReply.Handle(ctx, commands.SendCannedReplyCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), ConversationID: commands.ConversationID(in.ConversationID), CannedReplyID: commands.CannedReplyID(in.CannedReplyID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		return singleMessage(result.Message), true
	case "createAutomationRule":
		in := input.(*contract.AutomationRuleCreateInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.CreateAutomationRule == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		conditions, payload, err := encodeAutomationObjects(in.Body.Conditions, in.Body.ActionPayload)
		if err != nil {
			return mapApplicationError(err), true
		}
		result, err := s.deps.CreateAutomationRule.Handle(ctx, commands.CreateAutomationRuleCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), Name: in.Body.Name, Conditions: conditions, ActionKind: in.Body.ActionKind, ActionPayload: payload, Position: in.Body.Position})
		if err != nil {
			return mapApplicationError(err), true
		}
		return singleAutomationRule(result.AutomationRule), true
	case "updateAutomationRule":
		in := input.(*contract.AutomationRuleUpdateInput)
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		if s.deps.UpdateAutomationRule == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		conditions, payload, err := encodeAutomationObjectsOptional(in.Body.Conditions, in.Body.ActionPayload)
		if err != nil {
			return mapApplicationError(err), true
		}
		result, err := s.deps.UpdateAutomationRule.Handle(ctx, commands.UpdateAutomationRuleCommand{Meta: commandMeta(ctx, actor, in.CommandHeaders), AutomationRuleID: commands.AutomationRuleID(in.AutomationRuleID), Name: in.Body.Name, Status: in.Body.Status, Conditions: conditions, ActionKind: in.Body.ActionKind, ActionPayload: payload, Position: in.Body.Position})
		if err != nil {
			return mapApplicationError(err), true
		}
		return singleAutomationRule(result.AutomationRule), true
	}
	return nil, false
}

func encodeAutomationObjects(conditions, payload map[string]any) ([]byte, []byte, error) {
	if conditions == nil || payload == nil {
		return nil, nil, appErrors.New(appErrors.CodeValidation, "automation conditions and action_payload are required")
	}
	encodedConditions, err := json.Marshal(conditions)
	if err != nil {
		return nil, nil, appErrors.New(appErrors.CodeValidation, "automation conditions must be a JSON object")
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, appErrors.New(appErrors.CodeValidation, "automation action_payload must be a JSON object")
	}
	return encodedConditions, encodedPayload, nil
}
func encodeAutomationObjectsOptional(conditions, payload map[string]any) ([]byte, []byte, error) {
	var encodedConditions, encodedPayload []byte
	var err error
	if conditions != nil {
		encodedConditions, err = json.Marshal(conditions)
		if err != nil {
			return nil, nil, appErrors.New(appErrors.CodeValidation, "automation conditions must be a JSON object")
		}
	}
	if payload != nil {
		encodedPayload, err = json.Marshal(payload)
		if err != nil {
			return nil, nil, appErrors.New(appErrors.CodeValidation, "automation action_payload must be a JSON object")
		}
	}
	return encodedConditions, encodedPayload, nil
}
