package handlers

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

func (s *Server) ChatWithMerchantAI(ctx context.Context, in *contract.MerchantAIChatInput) (*contract.Single[contract.MerchantAIChatResponse], error) {
	if s.deps.ChatWithMerchantAI == nil {
		return nil, mapApplicationError(appErrors.NotImplemented())
	}
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	var sessionID *string
	if in.Body.SessionID != nil && *in.Body.SessionID != "" {
		sid := string(*in.Body.SessionID)
		sessionID = &sid
	}
	cmd := commands.MerchantAIChatCommand{
		Meta:      commandMeta(ctx, actor, in.CommandHeaders),
		SessionID: sessionID,
		Message:   in.Body.Message,
	}
	result, err := s.deps.ChatWithMerchantAI.Handle(ctx, cmd)
	if err != nil {
		return nil, mapApplicationError(err)
	}
	out := &contract.Single[contract.MerchantAIChatResponse]{}
	out.Body.RequestID = in.XRequestID
	out.Body.Data = contract.MerchantAIChatResponse{
		Message: result.Message,
		Action:  result.Action,
	}
	if result.SessionID != "" {
		sid := contract.UUID(result.SessionID)
		out.Body.Data.SessionID = &sid
	}
	return out, nil
}
