package services

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ChatwootProviderReferenceInput struct {
	BusinessID             string
	AccountID              string
	InboxID                string
	ChatwootConversationID string
}

type ProviderReferenceBinding struct {
	BusinessID             string
	MujeebConversationID   string
	ProviderRef            string
	Channel                string
	ConnectionID           string
	ProviderAccountID      string
	ProviderConversationID string
}

type ChatwootProviderReferenceResolver struct {
	References  ports.ConversationReferenceRepository
	Connections ports.ChannelConnectionRepository
}

func (r ChatwootProviderReferenceResolver) Resolve(ctx context.Context, input ChatwootProviderReferenceInput) (ProviderReferenceBinding, error) {
	if r.References == nil || r.Connections == nil {
		return ProviderReferenceBinding{}, appErrors.NotImplemented()
	}
	if strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.AccountID) == "" || strings.TrimSpace(input.InboxID) == "" || strings.TrimSpace(input.ChatwootConversationID) == "" {
		return ProviderReferenceBinding{}, appErrors.New(appErrors.CodeValidation, "business, Chatwoot account, inbox, and conversation references are required")
	}
	reference, err := r.References.GetCurrentProviderByChatwoot(ctx, input.BusinessID, input.AccountID, input.InboxID, input.ChatwootConversationID)
	if err != nil {
		if repositoryErrorKind(err) == "not_found" {
			return ProviderReferenceBinding{}, appErrors.New(appErrors.CodeNotFound, "missing_provider_conversation_reference")
		}
		return ProviderReferenceBinding{}, externalDependencyError("provider conversation reference lookup failed", err)
	}
	if reference.BusinessID != input.BusinessID || reference.System != "provider" || !reference.IsCurrent || reference.MappingStatus != "active" || strings.TrimSpace(reference.ProviderRef) == "" || strings.TrimSpace(reference.ResourceID) == "" || reference.ConnectionID == nil || strings.TrimSpace(*reference.ConnectionID) == "" {
		return ProviderReferenceBinding{}, appErrors.New(appErrors.CodeInvalidState, "provider conversation reference is unresolved or stale")
	}
	connection, err := r.Connections.GetByID(ctx, input.BusinessID, *reference.ConnectionID)
	if err != nil {
		return ProviderReferenceBinding{}, mapAIRepositoryError(err)
	}
	if connection.BusinessID != input.BusinessID || connection.Status != "active" || connection.ProviderReference != reference.ProviderRef || strings.TrimSpace(connection.ProviderConnectionRef) == "" || connection.Channel == "" || connection.ProviderAccountReference == nil || strings.TrimSpace(*connection.ProviderAccountReference) == "" {
		return ProviderReferenceBinding{}, appErrors.New(appErrors.CodeInvalidState, "provider conversation reference does not match an active channel connection")
	}
	if !sameExternalRef(reference.ChatwootAccountID, input.AccountID) || !sameExternalRef(reference.ChatwootInboxID, input.InboxID) || !sameExternalRef(reference.ChatwootConversationID, input.ChatwootConversationID) {
		return ProviderReferenceBinding{}, appErrors.New(appErrors.CodeInvalidState, "provider conversation reference has inconsistent Chatwoot binding")
	}
	return ProviderReferenceBinding{BusinessID: input.BusinessID, MujeebConversationID: reference.ConversationID, ProviderRef: reference.ProviderRef, Channel: connection.Channel, ConnectionID: connection.ID, ProviderAccountID: strings.TrimSpace(*connection.ProviderAccountReference), ProviderConversationID: reference.ResourceID}, nil
}

type ChatwootAutoReplyBridge struct {
	Resolver  ChatwootProviderReferenceResolver
	AutoReply AutoReplyService
}

func (b ChatwootAutoReplyBridge) Handle(ctx context.Context, command commands.ChatwootAutoReplyCommand) (commands.ChatwootAutoReplyResult, error) {
	if strings.TrimSpace(string(command.Meta.Actor.BusinessID)) == "" || strings.TrimSpace(string(command.MujeebConversationID)) == "" || strings.TrimSpace(command.SourceMessageReference) == "" || strings.TrimSpace(command.Text) == "" {
		return commands.ChatwootAutoReplyResult{}, appErrors.New(appErrors.CodeValidation, "Chatwoot AutoReply command is incomplete")
	}
	binding, err := b.Resolver.Resolve(ctx, ChatwootProviderReferenceInput{BusinessID: string(command.Meta.Actor.BusinessID), AccountID: command.AccountID, InboxID: command.InboxID, ChatwootConversationID: command.ChatwootConversationID})
	if err != nil {
		if isBindingBlockError(err) {
			return commands.ChatwootAutoReplyResult{Blocked: true, BlockedReason: bindingBlockReason(err)}, nil
		}
		return commands.ChatwootAutoReplyResult{}, err
	}
	result, err := b.AutoReply.Handle(ctx, commands.AutoReplyCommand{Meta: command.Meta, ConversationID: commands.ConversationID(binding.MujeebConversationID), SourceMessageReference: command.SourceMessageReference, Text: command.Text, Channel: binding.Channel, ProviderRef: binding.ProviderRef})
	if err != nil {
		return commands.ChatwootAutoReplyResult{}, err
	}
	return commands.ChatwootAutoReplyResult{Executed: result.Enqueued, AutoReply: result}, nil
}

func sameExternalRef(value *string, expected string) bool {
	return value != nil && strings.TrimSpace(*value) == strings.TrimSpace(expected)
}

func isBindingBlockError(err error) bool {
	var typed *appErrors.Error
	return errors.As(err, &typed) && (typed.Code == appErrors.CodeNotFound || typed.Code == appErrors.CodeInvalidState)
}

func bindingBlockReason(err error) string {
	var typed *appErrors.Error
	if errors.As(err, &typed) {
		if typed.Code == appErrors.CodeNotFound {
			return "missing_provider_conversation_reference"
		}
		if typed.Code == appErrors.CodeInvalidState {
			return "invalid_provider_conversation_reference"
		}
	}
	return "provider_reference_unresolved"
}

var _ = ChatwootAutoReplyBridge{}
