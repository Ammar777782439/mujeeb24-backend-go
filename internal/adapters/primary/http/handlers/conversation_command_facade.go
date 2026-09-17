package handlers

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

func (s *Server) dispatchConversationCommand(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "updateConversation":
		return s.updateConversation(ctx, input.(*contract.ConversationUpdateInput)), true
	case "assignConversation":
		return s.assignConversation(ctx, input.(*contract.ConversationAssignInput)), true
	case "updateConversationLabels":
		return s.updateConversationLabels(ctx, input.(*contract.ConversationLabelsInput)), true
	default:
		return nil, false
	}
}

func (s *Server) updateConversation(ctx context.Context, input *contract.ConversationUpdateInput) any {
	if s.deps.UpdateConversation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}

	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}

	result, err := s.deps.UpdateConversation.Handle(ctx, commands.UpdateConversationCommand{
		Meta:           commandMeta(ctx, actor, input.CommandHeaders),
		ConversationID: commands.ConversationID(input.ConversationID),
		State:          optionalStringPtr(input.Body.State),
		Ownership:      optionalStringPtr(input.Body.Ownership),
		AIModeOverride: optionalStringPtr(input.Body.AIModeOverride),
		Priority:       optionalStringPtr(input.Body.Priority),
	})
	if err != nil {
		return mapApplicationError(err)
	}

	return singleConversation(result.Conversation)
}

func (s *Server) updateConversationLabels(ctx context.Context, input *contract.ConversationLabelsInput) any {
	if s.deps.UpdateConversationLabels == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}

	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}

	result, err := s.deps.UpdateConversationLabels.Handle(ctx, commands.UpdateConversationLabelsCommand{
		Meta:           commandMeta(ctx, actor, input.CommandHeaders),
		ConversationID: commands.ConversationID(input.ConversationID),
		Add:            input.Body.Add,
		Remove:         input.Body.Remove,
	})
	if err != nil {
		return mapApplicationError(err)
	}

	return singleConversation(result.Conversation)
}

func (s *Server) assignConversation(ctx context.Context, input *contract.ConversationAssignInput) any {
	if s.deps.AssignConversation == nil {
		return mapApplicationError(appErrors.NotImplemented())
	}

	actor, err := s.requireScope(ctx, input.BusinessID)
	if err != nil {
		return mapApplicationError(err)
	}

	result, err := s.deps.AssignConversation.Handle(ctx, commands.AssignConversationCommand{
		Meta:                commandMeta(ctx, actor, input.CommandHeaders),
		ConversationID:      commands.ConversationID(input.ConversationID),
		AssigneePrincipalID: commands.PrincipalID(input.Body.AssigneePrincipalID),
	})
	if err != nil {
		return mapApplicationError(err)
	}

	return singleConversation(result.Conversation)
}

