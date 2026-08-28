package services

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

type ChannelProvisioningDisabledService struct{}

func (ChannelProvisioningDisabledService) Handle(context.Context, commands.BeginChannelConnectionCommand) (commands.BeginChannelConnectionResult, error) {
	return commands.BeginChannelConnectionResult{}, appErrors.New(appErrors.CodeInvalidState, "channel provisioning is disabled; enable configured SocialAPI provisioning to begin a connection")
}

// ChannelProvisioningUnavailableService keeps existing channel runtimes alive
// while exposing incomplete optional provisioning configuration at the endpoint.
type ChannelProvisioningUnavailableService struct{ Cause error }

func (s ChannelProvisioningUnavailableService) Handle(ctx context.Context, command commands.BeginChannelConnectionCommand) (commands.BeginChannelConnectionResult, error) {
	if s.Cause == nil {
		return ChannelProvisioningDisabledService{}.Handle(ctx, command)
	}
	return commands.BeginChannelConnectionResult{}, appErrors.New(appErrors.CodeInvalidState, "channel provisioning is not configured: "+s.Cause.Error())
}

type WebhookReceiverDisabledService struct{ Receiver string }

func (s WebhookReceiverDisabledService) Handle(context.Context, commands.IngestWebhookCommand) (commands.WebhookAcceptedResult, error) {
	return commands.WebhookAcceptedResult{}, appErrors.New(appErrors.CodeExternalDependency, s.Receiver+" webhook receiver is not configured")
}

var _ commands.BeginChannelConnectionHandler = ChannelProvisioningDisabledService{}
var _ commands.BeginChannelConnectionHandler = ChannelProvisioningUnavailableService{}
var _ commands.IngestSocialAPIWebhookHandler = WebhookReceiverDisabledService{}
