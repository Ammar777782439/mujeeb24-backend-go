package services

import (
	"context"
	"encoding/json"
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
	Assignees    ports.TeamRepository
	Labels       ports.ConversationLabelRepository
	References   ports.ConversationReferenceRepository
	Messages     ports.MessageRepository
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
	if s.Repository == nil {
		return commands.ListResult[commands.ConversationView]{}, appErrors.NotImplemented()
	}
	var customerID *string
	if query.CustomerID != nil {
		value := string(*query.CustomerID)
		customerID = &value
	}
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), query.State, query.Ownership, query.Channel, customerID, query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.ConversationView]{}, err
	}
	items := make([]commands.ConversationView, 0, len(page.Items))
	for _, record := range page.Items {
		view, viewErr := s.viewWithLabels(ctx, record)
		if viewErr != nil {
			return commands.ListResult[commands.ConversationView]{}, viewErr
		}
		items = append(items, view)
	}
	return commands.ListResult[commands.ConversationView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s ListCustomerConversationsQueryService) Handle(ctx context.Context, query queries.ListCustomerConversationsQuery) (commands.ListResult[commands.ConversationView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.ConversationView]{}, appErrors.NotImplemented()
	}
	customerID := string(query.CustomerID)
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), "", "", "", &customerID, query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.ConversationView]{}, err
	}
	items := make([]commands.ConversationView, 0, len(page.Items))
	for _, record := range page.Items {
		view, viewErr := s.viewWithLabels(ctx, record)
		if viewErr != nil {
			return commands.ListResult[commands.ConversationView]{}, viewErr
		}
		items = append(items, view)
	}
	return commands.ListResult[commands.ConversationView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s UpdateConversationCommandService) Handle(ctx context.Context, command commands.UpdateConversationCommand) (commands.ConversationResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.ConversationResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.ConversationResult{}, err
	}
	var result commands.ConversationResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, updateErr := s.Repository.Update(txCtx, ports.ConversationUpdate{BusinessID: string(command.Meta.Actor.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: expected, State: command.State, Ownership: command.Ownership, AIModeOverride: command.AIModeOverride, Priority: command.Priority})
		if updateErr != nil {
			return updateErr
		}
		view, viewErr := s.viewWithLabels(txCtx, record)
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
	if !canAssignConversations(command.Meta.Actor.Role) {
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
	var result commands.ConversationResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		assignee, assigneeErr := s.Assignees.ResolveActiveMember(txCtx, string(command.Meta.Actor.BusinessID), assignment)
		if assigneeErr != nil {
			return mapTeamRepositoryError(assigneeErr)
		}
		if !canReceiveConversationAssignment(assignee.Role) {
			return appErrors.New(appErrors.CodeForbidden, "team member cannot receive conversation assignments")
		}
		record, updateErr := s.Repository.Update(txCtx, ports.ConversationUpdate{BusinessID: string(command.Meta.Actor.BusinessID), ConversationID: string(command.ConversationID), ExpectedVersion: expected, Ownership: &ownership, AssignmentReference: &assignment})
		if updateErr != nil {
			return updateErr
		}
		view, viewErr := s.viewWithLabels(txCtx, record)
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
		record, versionErr := s.Repository.AdvanceVersion(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID), expected)
		if versionErr != nil {
			return versionErr
		}
		if labelErr := s.Labels.Apply(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID), command.Add, command.Remove); labelErr != nil {
			return labelErr
		}
		view, viewErr := s.viewWithLabels(txCtx, record)
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
	var result commands.MessageResult
	err := s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if _, conversationErr := s.Reader.GetByID(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConversationID)); conversationErr != nil {
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
		id := uuid.NewString()
		message, messageErr := s.Messages.Record(txCtx, ports.CommunicationMessageDraft{ID: id, BusinessID: string(command.Meta.Actor.BusinessID), ConversationReferenceID: reference.ID, Direction: "outbound", Origin: "human", Transport: "mujeeb", ContentType: "text", TextContent: &text, ContentReference: "private-note:" + id, Visibility: "private", OccurredAt: now, CreatedAt: now})
		if messageErr != nil {
			return messageErr
		}
		result.Message = messageView(message)
		return nil
	})
	if err == nil && s.Realtime != nil {
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
	return result, err
}

func (s ConversationRuntimeService) viewWithLabels(ctx context.Context, record ports.ConversationRecord) (commands.ConversationView, error) {
	view := conversationView(record)
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
