package handlers

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

func (s *Server) dispatchConversationCommand(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "assignConversation":
		return s.assignConversation(ctx, input.(*contract.ConversationAssignInput)), true
	default:
		return nil, false
	}
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
