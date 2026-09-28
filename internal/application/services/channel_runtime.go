package services

import (
	"context"
	"strconv"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type ChannelRuntimeService struct {
	Reader       ports.ChannelConnectionRepository
	Runtime      ports.ChannelConnectionRuntimeRepository
	Transactions ports.TransactionManager
}
type ListChannelConnectionsQueryService struct{ ChannelRuntimeService }
type GetChannelConnectionQueryService struct{ ChannelRuntimeService }
type ReconnectChannelCommandService struct{ ChannelRuntimeService }
type DisconnectChannelCommandService struct{ ChannelRuntimeService }

func (s ListChannelConnectionsQueryService) Handle(ctx context.Context, query queries.ListChannelConnectionsQuery) (commands.ListResult[commands.ChannelConnectionView], error) {
	if s.Runtime == nil {
		return commands.ListResult[commands.ChannelConnectionView]{}, appErrors.NotImplemented()
	}
	page, err := s.Runtime.List(ctx, string(query.Meta.Actor.BusinessID), query.Status, query.Channel, query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.ChannelConnectionView]{}, err
	}
	items := make([]commands.ChannelConnectionView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, channelConnectionView(item))
	}
	return commands.ListResult[commands.ChannelConnectionView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s GetChannelConnectionQueryService) Handle(ctx context.Context, query queries.GetChannelConnectionQuery) (commands.ChannelConnectionView, error) {
	if s.Reader == nil {
		return commands.ChannelConnectionView{}, appErrors.NotImplemented()
	}
	record, err := s.Reader.GetByID(ctx, string(query.Meta.Actor.BusinessID), string(query.ConnectionID))
	if err != nil {
		return commands.ChannelConnectionView{}, err
	}
	return channelConnectionView(record), nil
}

func (s ReconnectChannelCommandService) Handle(ctx context.Context, command commands.ReconnectChannelCommand) (commands.ChannelConnectionResult, error) {
	return s.transition(ctx, command.Meta, command.ConnectionID, command.Reason, "reconnect_required", "reconnect_requested")
}
func (s DisconnectChannelCommandService) Handle(ctx context.Context, command commands.DisconnectChannelCommand) (commands.ChannelConnectionResult, error) {
	return s.transition(ctx, command.Meta, command.ConnectionID, command.Reason, "disconnected", "disconnect_requested")
}

func (s ChannelRuntimeService) transition(ctx context.Context, meta commands.CommandMeta, connectionID commands.ConnectionID, reason, targetStatus, action string) (commands.ChannelConnectionResult, error) {
	if s.Runtime == nil || s.Transactions == nil {
		return commands.ChannelConnectionResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(meta.ExpectedVersion)
	if err != nil {
		return commands.ChannelConnectionResult{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeValidation, "connection action reason is required")
	}
	var result commands.ChannelConnectionResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, transitionErr := s.Runtime.Transition(txCtx, ports.ChannelConnectionTransition{BusinessID: string(meta.Actor.BusinessID), ConnectionID: string(connectionID), ExpectedVersion: expected, TargetStatus: targetStatus, Action: action, Reason: reason, ActorReference: string(meta.Actor.PrincipalID)})
		if transitionErr != nil {
			return transitionErr
		}
		result.Connection = channelConnectionView(record)
		result.ResourceVersion = result.Connection.ResourceVersion
		return nil
	})
	return result, err
}

func channelConnectionView(record ports.ChannelConnectionRecord) commands.ChannelConnectionView {
	account := ""
	if record.ProviderAccountReference != nil {
		account = *record.ProviderAccountReference
	}
	return commands.ChannelConnectionView{ID: commands.ConnectionID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), Provider: record.ProviderReference, Channel: record.Channel, Status: record.Status, ExternalAccountReference: account, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}

var _ queries.ListChannelConnectionsHandler = ListChannelConnectionsQueryService{}
var _ queries.GetChannelConnectionHandler = GetChannelConnectionQueryService{}
var _ commands.ReconnectChannelHandler = ReconnectChannelCommandService{}
var _ commands.DisconnectChannelHandler = DisconnectChannelCommandService{}
