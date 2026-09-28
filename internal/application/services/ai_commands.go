package services

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type RequestHumanReviewCommandService struct {
	DecisionRepository ports.AIDecisionRepository
	AuditRepository    ports.AuditEventRepository
	Transactions       ports.TransactionManager
	Now                func() time.Time
	NewID              func() string
}

func NewRequestHumanReviewCommandService(decisionRepository ports.AIDecisionRepository, auditRepository ports.AuditEventRepository, transactions ports.TransactionManager) RequestHumanReviewCommandService {
	return RequestHumanReviewCommandService{DecisionRepository: decisionRepository, AuditRepository: auditRepository, Transactions: transactions, Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}
}

func (s RequestHumanReviewCommandService) Handle(ctx context.Context, command commands.RequestHumanReviewCommand) (commands.AIDecisionResult, error) {
	if s.DecisionRepository == nil || s.AuditRepository == nil || s.Transactions == nil {
		return commands.AIDecisionResult{}, appErrors.NotImplemented()
	}
	if command.Meta.Actor.BusinessID == "" || command.Meta.Actor.PrincipalID == "" || command.DecisionID == "" || command.Reason == "" || command.Meta.ExpectedVersion == nil {
		return commands.AIDecisionResult{}, appErrors.New(appErrors.CodeValidation, "business, decision, reason, actor, and If-Match are required")
	}
	expectedVersion, err := strconv.ParseInt(string(*command.Meta.ExpectedVersion), 10, 64)
	if err != nil || expectedVersion <= 0 {
		return commands.AIDecisionResult{}, appErrors.New(appErrors.CodeValidation, "If-Match must contain a positive resource version")
	}
	now := s.now()
	var decision ports.AIDecisionRecord
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		updated, updateErr := s.DecisionRepository.RequestHumanReview(txCtx, ports.HumanReviewPatch{BusinessID: string(command.Meta.Actor.BusinessID), DecisionID: string(command.DecisionID), Reason: command.Reason, RequestedBy: string(command.Meta.Actor.PrincipalID), RequestedAt: now, ExpectedVersion: expectedVersion})
		if updateErr != nil {
			return mapAIRepositoryError(updateErr)
		}
		decision = updated
		decisionID := string(command.DecisionID)
		actorReference := string(command.Meta.Actor.PrincipalID)
		resourceID := decisionID
		var correlationID *string
		if command.Meta.CorrelationID != "" {
			value := command.Meta.CorrelationID
			correlationID = &value
		}
		_, appendErr := s.AuditRepository.Append(txCtx, ports.AuditEventDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), ActorType: "human_agent", ActorReference: &actorReference, Action: "human.handoff_requested", ResourceType: "ai_decision", ResourceID: &resourceID, Metadata: []byte(`{"source":"dashboard"}`), DecisionReference: &decisionID, Result: stringPtr("accepted"), ReasonCode: stringPtr("human_review_required"), CorrelationID: correlationID, OccurredAt: now, CreatedAt: now, SchemaVersion: 1, RedactionVersion: 1})
		if appendErr != nil {
			return mapAIRepositoryError(appendErr)
		}
		return nil
	})
	if err != nil {
		return commands.AIDecisionResult{}, err
	}
	return commands.AIDecisionResult{MutationResult: commands.MutationResult{ResourceID: commands.ID(decision.ID), ResourceVersion: commands.ResourceVersion(strconv.FormatInt(decision.ResourceVersion, 10)), Status: "human_review_requested", Accepted: true}, Decision: aiDecisionView(decision)}, nil
}

func (s RequestHumanReviewCommandService) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s RequestHumanReviewCommandService) id() string {
	if s.NewID == nil {
		return uuid.NewString()
	}
	return s.NewID()
}

func stringPtr(value string) *string { return &value }

var _ commands.RequestHumanReviewHandler = RequestHumanReviewCommandService{}
