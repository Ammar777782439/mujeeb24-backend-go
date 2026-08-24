package services

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type ListAIDecisionsQueryService struct{ Repository ports.AIDecisionRepository }

func (s ListAIDecisionsQueryService) Handle(ctx context.Context, query queries.ListAIDecisionsQuery) (commands.ListResult[commands.AIDecisionView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.AIDecisionView]{}, appErrors.NotImplemented()
	}
	page, err := s.Repository.List(ctx, ports.AIDecisionFilter{BusinessID: string(query.Meta.Actor.BusinessID), Lifecycle: query.Lifecycle, ConversationID: optionalConversationString(query.ConversationID), RequiresHuman: query.RequiresHuman, Limit: query.Limit, Cursor: query.Cursor})
	if err != nil {
		return commands.ListResult[commands.AIDecisionView]{}, mapAIRepositoryError(err)
	}
	items := make([]commands.AIDecisionView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, aiDecisionView(record))
	}
	return commands.ListResult[commands.AIDecisionView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type GetAIDecisionQueryService struct{ Repository ports.AIDecisionRepository }

func (s GetAIDecisionQueryService) Handle(ctx context.Context, query queries.GetAIDecisionQuery) (commands.AIDecisionView, error) {
	if s.Repository == nil {
		return commands.AIDecisionView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.Get(ctx, string(query.Meta.Actor.BusinessID), string(query.DecisionID))
	if err != nil {
		return commands.AIDecisionView{}, mapAIRepositoryError(err)
	}
	return aiDecisionView(record), nil
}

type ListAuditEventsQueryService struct{ Repository ports.AuditEventRepository }

func (s ListAuditEventsQueryService) Handle(ctx context.Context, query queries.ListAuditEventsQuery) (commands.ListResult[commands.AuditEventView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.AuditEventView]{}, appErrors.NotImplemented()
	}
	from, until, err := parseAuditRange(query.From, query.Until)
	if err != nil {
		return commands.ListResult[commands.AuditEventView]{}, err
	}
	page, err := s.Repository.List(ctx, ports.AuditEventFilter{BusinessID: string(query.Meta.Actor.BusinessID), ActorType: query.ActorType, Action: query.Action, ResourceType: query.ResourceType, From: from, Until: until, Limit: query.Limit, Cursor: query.Cursor})
	if err != nil {
		return commands.ListResult[commands.AuditEventView]{}, mapAIRepositoryError(err)
	}
	items := make([]commands.AuditEventView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, auditEventView(record))
	}
	return commands.ListResult[commands.AuditEventView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

type GetAuditEventQueryService struct{ Repository ports.AuditEventRepository }

func (s GetAuditEventQueryService) Handle(ctx context.Context, query queries.GetAuditEventQuery) (commands.AuditEventView, error) {
	if s.Repository == nil {
		return commands.AuditEventView{}, appErrors.NotImplemented()
	}
	record, err := s.Repository.Get(ctx, string(query.Meta.Actor.BusinessID), string(query.AuditEventID))
	if err != nil {
		return commands.AuditEventView{}, mapAIRepositoryError(err)
	}
	return auditEventView(record), nil
}

func aiDecisionView(record ports.AIDecisionRecord) commands.AIDecisionView {
	var conversationID *commands.ConversationID
	if record.ConversationID != nil {
		id := commands.ConversationID(*record.ConversationID)
		conversationID = &id
	}
	return commands.AIDecisionView{ID: commands.AIDecisionID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), ConversationID: conversationID, IntentBase: record.IntentBase, DomainContext: valueString(record.DomainContext), Entities: append([]byte(nil), record.Entities...), EvidenceReferences: append([]byte(nil), record.EvidenceReferences...), RequestedAction: record.RequestedAction, RequiresHuman: record.RequiresHuman, MissingInformation: append([]byte(nil), record.MissingInformation...), ReasonCodes: append([]byte(nil), record.ReasonCodes...), PolicyVersion: record.PolicyVersion, Lifecycle: record.Lifecycle, HumanReviewReason: valueString(record.HumanReviewReason), ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10)), CreatedAt: record.CreatedAt}
}

func auditEventView(record ports.AuditEventRecord) commands.AuditEventView {
	return commands.AuditEventView{ID: commands.AuditEventID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), ActorType: record.ActorType, ActorReference: valueString(record.ActorReference), Action: record.Action, ResourceType: record.ResourceType, ResourceID: valueString(record.ResourceID), Metadata: append([]byte(nil), record.Metadata...), DecisionReference: valueString(record.DecisionReference), Result: valueString(record.Result), ReasonCode: valueString(record.ReasonCode), BeforeReference: valueString(record.BeforeReference), AfterReference: valueString(record.AfterReference), OccurredAt: record.OccurredAt}
}

func optionalConversationString(value *commands.ConversationID) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func parseAuditRange(from, until string) (*time.Time, *time.Time, error) {
	var fromTime, untilTime *time.Time
	if from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return nil, nil, appErrors.New(appErrors.CodeValidation, "audit from must be RFC3339")
		}
		fromTime = &parsed
	}
	if until != "" {
		parsed, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return nil, nil, appErrors.New(appErrors.CodeValidation, "audit until must be RFC3339")
		}
		untilTime = &parsed
	}
	if fromTime != nil && untilTime != nil && fromTime.After(*untilTime) {
		return nil, nil, appErrors.New(appErrors.CodeValidation, "audit from must not be after until")
	}
	return fromTime, untilTime, nil
}

func mapAIRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	var kinded interface{ ErrorKind() string }
	if !errors.As(err, &kinded) {
		return err
	}
	switch kinded.ErrorKind() {
	case "not_found":
		return appErrors.New(appErrors.CodeNotFound, "AI or audit resource was not found")
	case "stale":
		return appErrors.New(appErrors.CodeStaleResource, "AI decision version is stale")
	case "conflict":
		return appErrors.New(appErrors.CodeInvalidState, "AI decision state does not accept this request")
	case "invalid":
		return appErrors.New(appErrors.CodeValidation, "AI or audit persistence rejected the request")
	default:
		return err
	}
}

var _ queries.ListAIDecisionsHandler = ListAIDecisionsQueryService{}
var _ queries.GetAIDecisionHandler = GetAIDecisionQueryService{}
var _ queries.ListAuditEventsHandler = ListAuditEventsQueryService{}
var _ queries.GetAuditEventHandler = GetAuditEventQueryService{}
