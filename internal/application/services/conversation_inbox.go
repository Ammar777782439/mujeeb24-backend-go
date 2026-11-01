package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

func readInboxConversation(ctx context.Context, repository ports.ConversationInboxRepository, actor commands.ActorContext, id string, lock bool) (ports.ConversationRecord, error) {
	if repository == nil {
		return ports.ConversationRecord{}, appErrors.NotImplemented()
	}
	record, err := repository.GetInboxConversation(ctx, string(actor.BusinessID), id, authorization.ConversationAccess(actor), lock)
	if err != nil {
		return record, mapConversationRepositoryError(err)
	}
	if !authorization.CanReadConversation(actor, record) {
		return ports.ConversationRecord{}, appErrors.New(appErrors.CodeNotFound, "المحادثة غير متاحة لك.")
	}
	return record, nil
}

func conversationViewForActor(record ports.ConversationRecord, actor commands.ActorContext) commands.ConversationView {
	v := conversationView(record)
	v.AssigneeDisplayName = record.AssigneeDisplayName
	v.Channel = record.Channel
	v.LastMessageContentType = record.LastMessageContentType
	v.LastMessageText = record.LastMessageText
	v.LastMessageDirection = record.LastMessageDirection
	v.LastMessageOrigin = record.LastMessageOrigin
	v.LastMessageAuthorName = record.LastMessageAuthorName
	v.UnreadCount = record.UnreadCount
	if !authorization.ConversationAssigned(record) {
		v.AssignmentReference = nil
		v.AssigneeDisplayName = nil
	}
	v.CanReply = authorization.CanReplyToConversation(actor, record)
	v.CanClaim = authorization.CanClaimConversation(actor, record)
	v.CanAssign = canAssignConversations(actor.Role) && authorization.Has(actor, authorization.PermissionConversationsAssign) && authorization.CanReadConversation(actor, record)
	v.CanAddNote = authorization.Has(actor, authorization.PermissionConversationsReply) && authorization.CanReadConversation(actor, record)
	return v
}

type ClaimConversationCommandService struct{ ConversationRuntimeService }

func (s ClaimConversationCommandService) Handle(ctx context.Context, command commands.ClaimConversationCommand) (commands.ConversationResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.ConversationResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.ConversationResult{}, err
	}
	var result commands.ConversationResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		current, err := readInboxConversation(txCtx, s.Inbox, command.Meta.Actor, string(command.ConversationID), true)
		if err != nil {
			return err
		}
		if !authorization.CanClaimConversation(command.Meta.Actor, current) {
			return appErrors.New(appErrors.CodeConflict, "لا يمكن استلام المحادثة بحالتها الحالية. حدّث القائمة.")
		}
		state, ownership, mode, assignee := "human_handling", "human", "disabled", string(command.Meta.Actor.PrincipalID)
		record, err := s.Repository.Update(txCtx, ports.ConversationUpdate{BusinessID: current.BusinessID, ConversationID: current.ID, ExpectedVersion: expected, State: &state, Ownership: &ownership, AIModeOverride: &mode, AssignmentReference: &assignee})
		if err != nil {
			return mapConversationRepositoryError(err)
		}
		view, err := s.viewAfterChange(txCtx, record, command.Meta.Actor)
		if err != nil {
			return err
		}
		result.Conversation = view
		result.ResourceVersion = view.ResourceVersion
		return nil
	})
	if err == nil {
		s.publishConversationChange(ctx, "conversation.assigned", command.Meta.Actor, command.ConversationID)
	}
	return result, err
}

type GetConversationSummaryService struct {
	Inbox  ports.ConversationInboxRepository
	States ports.ConversationStateRepository
}

func (s GetConversationSummaryService) Handle(ctx context.Context, q queries.GetConversationSummaryQuery) (commands.ConversationSummaryView, error) {
	if _, err := readInboxConversation(ctx, s.Inbox, q.Meta.Actor, string(q.ConversationID), false); err != nil {
		return commands.ConversationSummaryView{}, err
	}
	if s.States == nil {
		return commands.ConversationSummaryView{}, appErrors.NotImplemented()
	}
	result := commands.ConversationSummaryView{ConversationID: q.ConversationID, Status: "not_available"}
	state, err := s.States.Get(ctx, string(q.Meta.Actor.BusinessID), string(q.ConversationID))
	if err != nil {
		var kinded interface{ ErrorKind() string }
		if errors.As(err, &kinded) && kinded.ErrorKind() == "not_found" {
			return result, nil
		}
		return result, err
	}
	if strings.TrimSpace(state.Summary) != "" {
		result.Status = "available"
		result.Text = state.Summary
		result.CoveredInboundTurns = state.SummaryTurnCount
	}
	return result, nil
}

type GetConversationInboxOverviewService struct {
	Inbox ports.ConversationInboxRepository
}

func (s GetConversationInboxOverviewService) Handle(ctx context.Context, q queries.GetConversationInboxOverviewQuery) (ports.ConversationInboxOverview, error) {
	if s.Inbox == nil {
		return ports.ConversationInboxOverview{}, appErrors.NotImplemented()
	}
	return s.Inbox.InboxOverview(ctx, ports.ConversationInboxFilter{BusinessID: string(q.Meta.Actor.BusinessID), Access: authorization.ConversationAccess(q.Meta.Actor), Scope: q.Scope, Search: q.Search})
}

func (s ConversationRuntimeService) viewAfterChange(ctx context.Context, record ports.ConversationRecord, actor commands.ActorContext) (commands.ConversationView, error) {
	// A completed transfer may remove the acting agent’s visibility. The row was
	// already authorized and locked before mutation; return its authoritative result.
	access := authorization.ConversationAccess(actor)
	access.Supervisor = true
	current, err := s.Inbox.GetInboxConversation(ctx, record.BusinessID, record.ID, access, false)
	if err != nil {
		return commands.ConversationView{}, err
	}
	return s.viewWithLabels(ctx, current, actor)
}

func (s ConversationRuntimeService) publishConversationChange(ctx context.Context, eventType string, actor commands.ActorContext, id commands.ConversationID) {
	if s.Realtime != nil {
		_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{EventID: uuid.NewString(), EventType: eventType, BusinessID: string(actor.BusinessID), ResourceType: "conversation", ResourceID: string(id), OccurredAt: time.Now().UTC()})
	}
}
