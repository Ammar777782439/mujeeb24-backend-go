package services

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
	"github.com/google/uuid"
)

type MarkConversationReadService struct {
	Repository ports.ConversationReadCursorRepository
	Now        func() time.Time
}

func (s MarkConversationReadService) Handle(ctx context.Context, command commands.MarkConversationReadCommand) (commands.ConversationReadResult, error) {
	if s.Repository == nil {
		return commands.ConversationReadResult{}, appErrors.NotImplemented()
	}
	businessID := strings.TrimSpace(string(command.Meta.Actor.BusinessID))
	conversationID := strings.TrimSpace(string(command.ConversationID))
	principalID := strings.TrimSpace(string(command.Meta.Actor.PrincipalID))
	if businessID == "" || conversationID == "" || principalID == "" {
		return commands.ConversationReadResult{}, appErrors.New(appErrors.CodeValidation, "business, conversation, and principal ids are required")
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	cursor, err := s.Repository.MarkRead(ctx, businessID, conversationID, principalID, now)
	if err != nil {
		return commands.ConversationReadResult{}, mapAIRepositoryError(err)
	}
	result := commands.ConversationReadResult{
		MutationResult: commands.MutationResult{ResourceID: commands.ID(conversationID), Status: "read"},
		ConversationID: commands.ConversationID(cursor.ConversationID),
	}
	if cursor.LastReadMessageID != nil {
		messageID := commands.MessageID(*cursor.LastReadMessageID)
		result.LastReadMessageID = &messageID
	}
	return result, nil
}

type CannedReplyService struct {
	Repository   ports.CannedReplyRepository
	Transactions ports.TransactionManager
	Now          func() time.Time
	NewID        func() string
}

type ListCannedRepliesQueryService struct{ CannedReplyService }
type CreateCannedReplyCommandService struct{ CannedReplyService }
type UpdateCannedReplyCommandService struct{ CannedReplyService }

func (s ListCannedRepliesQueryService) Handle(ctx context.Context, query queries.ListCannedRepliesQuery) (commands.ListResult[commands.CannedReplyView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.CannedReplyView]{}, appErrors.NotImplemented()
	}
	page, err := s.Repository.List(ctx, string(query.Meta.Actor.BusinessID), strings.TrimSpace(query.Status), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.CannedReplyView]{}, mapAIRepositoryError(err)
	}
	items := make([]commands.CannedReplyView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, cannedReplyView(item))
	}
	return commands.ListResult[commands.CannedReplyView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s CreateCannedReplyCommandService) Handle(ctx context.Context, command commands.CreateCannedReplyCommand) (commands.CannedReplyResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.CannedReplyResult{}, appErrors.NotImplemented()
	}
	title, shortcut, body, err := validateCannedReplyFields(command.Title, command.Shortcut, command.Body)
	if err != nil {
		return commands.CannedReplyResult{}, err
	}
	now := s.now()
	newID := uuid.NewString
	if s.NewID != nil {
		newID = s.NewID
	}
	var result commands.CannedReplyResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, createErr := s.Repository.Create(txCtx, ports.CannedReplyCreate{ID: newID(), BusinessID: string(command.Meta.Actor.BusinessID), Title: title, Shortcut: shortcut, Body: body, Status: "active", CreatedAt: now, UpdatedAt: now})
		if createErr != nil {
			return mapAIRepositoryError(createErr)
		}
		result = cannedReplyResult(record)
		return nil
	})
	return result, err
}

func (s UpdateCannedReplyCommandService) Handle(ctx context.Context, command commands.UpdateCannedReplyCommand) (commands.CannedReplyResult, error) {
	if s.Repository == nil || s.Transactions == nil {
		return commands.CannedReplyResult{}, appErrors.NotImplemented()
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.CannedReplyResult{}, err
	}
	title, err := optionalCannedReplyText(command.Title, "title", 200, false)
	if err != nil {
		return commands.CannedReplyResult{}, err
	}
	shortcut, err := optionalCannedReplyText(command.Shortcut, "shortcut", 80, true)
	if err != nil {
		return commands.CannedReplyResult{}, err
	}
	body, err := optionalCannedReplyText(command.Body, "body", 10000, false)
	if err != nil {
		return commands.CannedReplyResult{}, err
	}
	status := optionalNormalizedStatus(command.Status)
	if command.Status != nil && status == nil {
		return commands.CannedReplyResult{}, appErrors.New(appErrors.CodeValidation, "canned reply status must be active or archived")
	}
	if title == nil && shortcut == nil && body == nil && status == nil {
		return commands.CannedReplyResult{}, appErrors.New(appErrors.CodeValidation, "at least one canned reply field is required")
	}
	var result commands.CannedReplyResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		record, updateErr := s.Repository.Update(txCtx, ports.CannedReplyUpdate{BusinessID: string(command.Meta.Actor.BusinessID), CannedReplyID: string(command.CannedReplyID), ExpectedVersion: expected, Title: title, Shortcut: shortcut, Body: body, Status: status, UpdatedAt: s.now()})
		if updateErr != nil {
			return mapAIRepositoryError(updateErr)
		}
		result = cannedReplyResult(record)
		return nil
	})
	return result, err
}

type SendCannedReplyCommandService struct {
	Repository ports.CannedReplyRepository
	Outbound   commands.CreateOutboundMessageHandler
}

func (s SendCannedReplyCommandService) Handle(ctx context.Context, command commands.SendCannedReplyCommand) (commands.MessageResult, error) {
	if s.Repository == nil || s.Outbound == nil {
		return commands.MessageResult{}, appErrors.NotImplemented()
	}
	if strings.TrimSpace(command.Meta.IdempotencyKey) == "" {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "Idempotency-Key is required")
	}
	reply, err := s.Repository.GetByID(ctx, string(command.Meta.Actor.BusinessID), string(command.CannedReplyID))
	if err != nil {
		return commands.MessageResult{}, mapAIRepositoryError(err)
	}
	if reply.Status != "active" {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeConflict, "canned reply is not active")
	}
	delegated := command.Meta
	delegated.IdempotencyKey = "canned-reply:" + reply.ID + ":" + strings.TrimSpace(command.Meta.IdempotencyKey)
	return s.Outbound.Handle(ctx, commands.CreateOutboundMessageCommand{Meta: delegated, ConversationID: command.ConversationID, Text: reply.Body})
}

func (s CannedReplyService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func cannedReplyResult(record ports.CannedReplyRecord) commands.CannedReplyResult {
	view := cannedReplyView(record)
	return commands.CannedReplyResult{MutationResult: commands.MutationResult{ResourceID: commands.ID(record.ID), ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10)), Status: record.Status}, CannedReply: view}
}

func cannedReplyView(record ports.CannedReplyRecord) commands.CannedReplyView {
	return commands.CannedReplyView{ID: commands.CannedReplyID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), Title: record.Title, Shortcut: record.Shortcut, Body: record.Body, Status: record.Status, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10)), CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func validateCannedReplyFields(title, shortcut, body string) (string, string, string, error) {
	normalizedTitle, err := requiredCannedReplyText(title, "title", 200, false)
	if err != nil {
		return "", "", "", err
	}
	normalizedShortcut, err := requiredCannedReplyText(shortcut, "shortcut", 80, true)
	if err != nil {
		return "", "", "", err
	}
	normalizedBody, err := requiredCannedReplyText(body, "body", 10000, false)
	if err != nil {
		return "", "", "", err
	}
	return normalizedTitle, normalizedShortcut, normalizedBody, nil
}

func optionalCannedReplyText(value *string, field string, limit int, normalizeShortcut bool) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized, err := requiredCannedReplyText(*value, field, limit, normalizeShortcut)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func requiredCannedReplyText(value, field string, limit int, normalizeShortcut bool) (string, error) {
	value = strings.TrimSpace(value)
	if normalizeShortcut {
		value = strings.TrimPrefix(strings.ToLower(value), "/")
	}
	if value == "" || len([]rune(value)) > limit {
		return "", appErrors.New(appErrors.CodeValidation, "canned reply "+field+" is required and exceeds its limit")
	}
	return value, nil
}

func optionalNormalizedStatus(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized != "active" && normalized != "archived" {
		return nil
	}
	return &normalized
}

var _ commands.MarkConversationReadHandler = MarkConversationReadService{}
var _ queries.ListCannedRepliesHandler = ListCannedRepliesQueryService{}
var _ commands.CreateCannedReplyHandler = CreateCannedReplyCommandService{}
var _ commands.UpdateCannedReplyHandler = UpdateCannedReplyCommandService{}
var _ commands.SendCannedReplyHandler = SendCannedReplyCommandService{}
