package services

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/google/uuid"
)

type ConversationRuntimeService struct {
	Repository   ports.ConversationRuntimeRepository
	Reader       ports.ConversationRepository
	Inbox        ports.ConversationInboxRepository
	Assignees    ports.TeamMemberResolver
	Labels       ports.ConversationLabelRepository
	References   ports.ConversationReferenceRepository
	Messages     ports.MessageRepository
	NoteTargets  ports.PrivateNoteTargetReader
	PrivateNotes ports.PrivateNoteRepository
	Transactions ports.TransactionManager
	Realtime     ports.RealtimePublisher
}

type ListConversationsQueryService struct{ ConversationRuntimeService }
type ListCustomerConversationsQueryService struct{ ConversationRuntimeService }
type UpdateConversationCommandService struct{ ConversationRuntimeService }
type AssignConversationCommandService struct{ ConversationRuntimeService }
type UpdateConversationLabelsCommandService struct{ ConversationRuntimeService }
type AddPrivateNoteCommandService struct{ ConversationRuntimeService }

func (s ListConversationsQueryService) Handle(ctx context.Context, query queries.ListConversationsQuery) (commands.ListResult[commands.ConversationView], error) {
	if s.Inbox == nil {
		return commands.ListResult[commands.ConversationView]{}, appErrors.NotImplemented()
	}
	var customerID *string
	if query.CustomerID != nil {
		value := string(*query.CustomerID)
		customerID = &value
	}
	page, err := s.Inbox.ListInbox(ctx, ports.ConversationInboxFilter{BusinessID: string(query.Meta.Actor.BusinessID), Access: authorization.ConversationAccess(query.Meta.Actor), Scope: query.Scope, Search: query.Search, State: query.State, Ownership: query.Ownership, Channel: query.Channel, CustomerID: customerID, Limit: query.Limit, Cursor: query.Cursor})
	if err != nil {
		return commands.ListResult[commands.ConversationView]{}, err
	}
	items := make([]commands.ConversationView, 0, len(page.Items))
	for _, record := range page.Items {
		view, viewErr := s.viewWithLabels(ctx, record, query.Meta.Actor)
		if viewErr != nil {
			return commands.ListResult[commands.ConversationView]{}, viewErr
		}
		items = append(items, view)
	}
	return commands.ListResult[commands.ConversationView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s ListCustomerConversationsQueryService) Handle(ctx context.Context, query queries.ListCustomerConversationsQuery) (commands.ListResult[commands.ConversationView], error) {
	if s.Inbox == nil {
		return commands.ListResult[commands.ConversationView]{}, appErrors.NotImplemented()
	}
	customerID := string(query.CustomerID)
	page, err := s.Inbox.ListInbox(ctx, ports.ConversationInboxFilter{BusinessID: string(query.Meta.Actor.BusinessID), Access: authorization.ConversationAccess(query.Meta.Actor), CustomerID: &customerID, Limit: query.Limit, Cursor: query.Cursor})
	if err != nil {
		return commands.ListResult[commands.ConversationView]{}, err
	}
	items := make([]commands.ConversationView, 0, len(page.Items))
	for _, record := range page.Items {
		view, viewErr := s.viewWithLabels(ctx, record, query.Meta.Actor)
		if viewErr != nil {
			return commands.ListResult[commands.ConversationView]{}, viewErr
		}
		items = append(items, view)
	}
	return commands.ListResult[commands.ConversationView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s UpdateConversationCommandService) Handle(ctx context.Context, command commands.UpdateConversationCommand) (commands.ConversationResult, error) {
	if !canAssignConversations(command.Meta.Actor.Role) || !authorization.Has(command.Meta.Actor, authorization.PermissionConversationsAssign) {
		return commands.ConversationResult{}, appErrors.New(appErrors.CodeForbidden, "ليس لديك صلاحية تعديل المحادثة.")
	}
	if s.Repository == nil || s.Transactions == nil {
		return commands.ConversationResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.ConversationResult{}, err
	}
	var result commands.ConversationResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if _, accessErr := readInboxConversation(txCtx, s.Inbox, command.Meta.Actor, string(command.ConversationID), true); accessErr != nil {
			return accessErr
		}
		record, updateErr := s.Repository.Update(txCtx, ports.ConversationUpdate{BusinessID: string(command.Meta.Actor.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: expected, State: command.State, Ownership: command.Ownership, AIModeOverride: command.AIModeOverride, Priority: command.Priority})
		if updateErr != nil {
			return mapConversationRepositoryError(updateErr)
		}
		view, viewErr := s.viewAfterChange(txCtx, record, command.Meta.Actor)
		if viewErr != nil {
			return viewErr
		}
		result.Conversation = view
		result.ResourceVersion = view.ResourceVersion
		return nil
	})
	if err == nil && s.Realtime != nil {
		data, _ := json.Marshal(result.Conversation)
		_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
			EventID:      uuid.NewString(),
			EventType:    "conversation.state_changed",
			BusinessID:   string(command.Meta.Actor.BusinessID),
			ResourceType: "conversation",
			ResourceID:   string(command.ConversationID),
			OccurredAt:   time.Now().UTC(),
			Data:         data,
		})
	}
	return result, err
}

func (s AssignConversationCommandService) Handle(ctx context.Context, command commands.AssignConversationCommand) (commands.ConversationResult, error) {
	if s.Repository == nil || s.Assignees == nil || s.Transactions == nil {
		return commands.ConversationResult{}, appErrors.NotImplemented()
	}
	if !canAssignConversations(command.Meta.Actor.Role) || !authorization.Has(command.Meta.Actor, authorization.PermissionConversationsAssign) {
		return commands.ConversationResult{}, appErrors.New(appErrors.CodeForbidden, "current team role cannot assign conversations")
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.ConversationResult{}, err
	}
	assignment := strings.TrimSpace(string(command.AssigneePrincipalID))
	if uuid.Validate(assignment) != nil {
		return commands.ConversationResult{}, appErrors.New(appErrors.CodeValidation, "assignee_principal_id must be a UUID")
	}
	ownership := "human"
	state, aiMode := "human_handling", "disabled"
	var result commands.ConversationResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if _, accessErr := readInboxConversation(txCtx, s.Inbox, command.Meta.Actor, string(command.ConversationID), true); accessErr != nil {
			return accessErr
		}
		assignee, assigneeErr := s.Assignees.ResolveActiveMember(txCtx, string(command.Meta.Actor.BusinessID), assignment)
		if assigneeErr != nil {
			return mapTeamRepositoryError(assigneeErr)
		}
		if !eligibleConversationAssignee(assignee) {
			return appErrors.New(appErrors.CodeForbidden, "team member cannot receive conversation assignments")
		}
		record, updateErr := s.Repository.Update(txCtx, ports.ConversationUpdate{BusinessID: string(command.Meta.Actor.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: expected, State: &state, AIModeOverride: &aiMode, Ownership: &ownership, AssignmentReference: &assignment})
		if updateErr != nil {
			return mapConversationRepositoryError(updateErr)
		}
		view, viewErr := s.viewAfterChange(txCtx, record, command.Meta.Actor)
		if viewErr != nil {
			return viewErr
		}
		result.Conversation = view
		result.ResourceVersion = view.ResourceVersion
		return nil
	})
	if err == nil && s.Realtime != nil {
		data, _ := json.Marshal(result.Conversation)
		_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
			EventID:      uuid.NewString(),
			EventType:    "conversation.assigned",
			BusinessID:   string(command.Meta.Actor.BusinessID),
			ResourceType: "conversation",
			ResourceID:   string(command.ConversationID),
			OccurredAt:   time.Now().UTC(),
			Data:         data,
		})
	}
	return result, err
}

func (s UpdateConversationLabelsCommandService) Handle(ctx context.Context, command commands.UpdateConversationLabelsCommand) (commands.ConversationResult, error) {
	if s.Repository == nil || s.Labels == nil || s.Transactions == nil {
		return commands.ConversationResult{}, appErrors.NotImplemented()
	}
	if len(command.Add) == 0 && len(command.Remove) == 0 {
		return commands.ConversationResult{}, appErrors.New(appErrors.CodeValidation, "at least one label add or remove is required")
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.ConversationResult{}, err
	}
	var result commands.ConversationResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if _, accessErr := readInboxConversation(txCtx, s.Inbox, command.Meta.Actor, string(command.ConversationID), true); accessErr != nil {
			return accessErr
		}
		record, versionErr := s.Repository.AdvanceVersion(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID), expected)
		if versionErr != nil {
			return versionErr
		}
		if labelErr := s.Labels.Apply(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID), command.Add, command.Remove); labelErr != nil {
			return labelErr
		}
		view, viewErr := s.viewAfterChange(txCtx, record, command.Meta.Actor)
		if viewErr != nil {
			return viewErr
		}
		result.Conversation = view
		result.ResourceVersion = view.ResourceVersion
		return nil
	})
	return result, err
}

func (s AddPrivateNoteCommandService) Handle(ctx context.Context, command commands.AddPrivateNoteCommand) (commands.MessageResult, error) {
	if s.Repository == nil || s.Reader == nil || s.References == nil || s.Messages == nil || s.Transactions == nil {
		return commands.MessageResult{}, appErrors.NotImplemented()
	}
	text := strings.TrimSpace(command.Text)
	if text == "" {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "private note text is required")
	}
	created := true
	var result commands.MessageResult
	err := s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if _, conversationErr := readInboxConversation(txCtx, s.Inbox, command.Meta.Actor, string(command.ConversationID), true); conversationErr != nil {
			return conversationErr
		}
		reference, referenceErr := s.References.GetCurrentByConversation(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID), "provider")
		if referenceErr != nil {
			return referenceErr
		}
		if reference.MappingStatus != "active" {
			return appErrors.New(appErrors.CodeConflict, "current provider reference is not active")
		}
		now := time.Now().UTC()
		id := privateNoteID(string(command.Meta.Actor.BusinessID), string(command.ConversationID), string(command.Meta.Actor.PrincipalID), command.Meta.IdempotencyKey)
		referenceText := "private-note:" + id
		if command.RelatedMessageID != nil {
			if uuid.Validate(*command.RelatedMessageID) != nil {
				return appErrors.New(appErrors.CodeValidation, "related_message_id must be a UUID")
			}
			if s.NoteTargets == nil {
				return appErrors.NotImplemented()
			}
			target, targetErr := s.NoteTargets.GetPrivateNoteTarget(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID), *command.RelatedMessageID)
			if targetErr != nil {
				return mapConversationRepositoryError(targetErr)
			}
			quote := ""
			if target.TextContent != nil {
				quote = *target.TextContent
			}
			referenceText = encodePrivateNoteReference(id, target.ID, quote)
		}
		author := string(command.Meta.Actor.PrincipalID)
		draft := ports.CommunicationMessageDraft{AuthorPrincipalID: &author, ID: id, BusinessID: string(command.Meta.Actor.BusinessID), ConversationReferenceID: reference.ID, Direction: "outbound", Origin: "human", Transport: "mujeeb", ContentType: "text", TextContent: &text, ContentReference: referenceText, Visibility: "private", OccurredAt: now, CreatedAt: now}
		var message ports.CommunicationMessageRecord
		var messageErr error
		if command.Meta.IdempotencyKey != "" {
			if s.PrivateNotes == nil {
				return appErrors.NotImplemented()
			}
			message, created, messageErr = s.PrivateNotes.RecordPrivateNoteOnce(txCtx, string(command.ConversationID), draft)
		} else {
			message, messageErr = s.Messages.Record(txCtx, draft)
		}
		if messageErr != nil {
			return messageErr
		}
		result.Message = messageView(message)
		return nil
	})
	if err == nil && created && s.Realtime != nil {
		data, _ := json.Marshal(result.Message)
		_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
			EventID:      uuid.NewString(),
			EventType:    "conversation.note_added",
			BusinessID:   string(command.Meta.Actor.BusinessID),
			ResourceType: "conversation",
			ResourceID:   string(command.ConversationID),
			OccurredAt:   time.Now().UTC(),
			Data:         data,
		})
	}
	return result, mapConversationRepositoryError(err)
}

func (s ConversationRuntimeService) viewWithLabels(ctx context.Context, record ports.ConversationRecord, actor commands.ActorContext) (commands.ConversationView, error) {
	view := conversationViewForActor(record, actor)
	if s.Labels == nil {
		return view, nil
	}
	labels, err := s.Labels.List(ctx, record.BusinessID, record.ID)
	if err != nil {
		return commands.ConversationView{}, err
	}
	view.Labels = labels
	return view, nil
}

var _ queries.ListConversationsHandler = ListConversationsQueryService{}
var _ queries.ListCustomerConversationsHandler = ListCustomerConversationsQueryService{}
var _ commands.UpdateConversationHandler = UpdateConversationCommandService{}
var _ commands.AssignConversationHandler = AssignConversationCommandService{}
var _ commands.UpdateConversationLabelsHandler = UpdateConversationLabelsCommandService{}
var _ commands.AddPrivateNoteHandler = AddPrivateNoteCommandService{}

func eligibleConversationAssignee(member ports.TeamMemberRecord) bool {
	actor := commands.ActorContext{Permissions: member.Permissions}
	return member.Status == "active" && canReceiveConversationAssignment(member.Role) && authorization.Has(actor, authorization.PermissionConversationsRead) && authorization.Has(actor, authorization.PermissionConversationsReply)
}

func mapConversationRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "المحادثة أو الرسالة غير موجودة في هذه المنشأة.")
	case "stale":
		return appErrors.New(appErrors.CodeStaleResource, "تغيرت المحادثة. حدّثها ثم أعد المحاولة.")
	case "conflict":
		return appErrors.New(appErrors.CodeConflict, "تعذّر تعديل المحادثة بحالتها الحالية.")
	default:
		return err
	}
}
