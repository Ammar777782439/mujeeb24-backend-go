package services

import (
	"context"
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

type BeginChannelConnectionHandler struct {
	Provisioning ChannelProvisioningService
}

func (h BeginChannelConnectionHandler) Handle(ctx context.Context, command commands.BeginChannelConnectionCommand) (commands.BeginChannelConnectionResult, error) {
	if h.Provisioning.Sessions == nil {
		return commands.BeginChannelConnectionResult{}, errors.New("channel provisioning handler is not configured")
	}
	session, err := h.Provisioning.Start(ctx, string(command.Meta.Actor.BusinessID), command.Provider, command.Channel, command.DisplayName, command.Meta.IdempotencyKey)
	if err != nil {
		return commands.BeginChannelConnectionResult{}, err
	}
	return commands.BeginChannelConnectionResult{
		MutationResult: commands.MutationResult{ResourceID: commands.ID(session.ID), Status: string(session.Status), Accepted: true},
		Provisioning:   commands.ChannelProvisioningView{ID: commands.ID(session.ID), BusinessID: commands.BusinessID(session.BusinessID), Provider: session.ProviderRef, Channel: session.Channel, Status: string(session.Status), AuthorizationURL: session.AuthorizationURL},
	}, nil
}

var _ commands.BeginChannelConnectionHandler = (*BeginChannelConnectionHandler)(nil)
