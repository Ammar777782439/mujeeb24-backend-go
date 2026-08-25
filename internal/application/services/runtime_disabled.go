package services

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

type ChannelProvisioningDisabledService struct{}

func (ChannelProvisioningDisabledService) Handle(context.Context, commands.BeginChannelConnectionCommand) (commands.BeginChannelConnectionResult, error) {
	return commands.BeginChannelConnectionResult{}, appErrors.New(appErrors.CodeInvalidState, "channel provisioning is disabled; enable configured SocialAPI and Chatwoot provisioning to begin a connection")
}

type WebhookReceiverDisabledService struct{ Receiver string }

func (s WebhookReceiverDisabledService) Handle(context.Context, commands.IngestWebhookCommand) (commands.WebhookAcceptedResult, error) {
	return commands.WebhookAcceptedResult{}, appErrors.New(appErrors.CodeExternalDependency, s.Receiver+" webhook receiver is not configured")
}

var _ commands.BeginChannelConnectionHandler = ChannelProvisioningDisabledService{}
var _ commands.IngestSocialAPIWebhookHandler = WebhookReceiverDisabledService{}
var _ commands.IngestChatwootWebhookHandler = WebhookReceiverDisabledService{}
