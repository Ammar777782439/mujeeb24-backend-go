package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type MerchantAIChatService struct {
	Sessions           ports.MerchantAISessionRepository
	AIRuntime          ports.AIRuntime
	DecisionRepository ports.AIDecisionRepository
	Transactions       ports.TransactionManager
	Now                func() time.Time
	NewID              func() string
	MaxContextMessages int
}

func NewMerchantAIChatService(
	sessions ports.MerchantAISessionRepository,
	aiRuntime ports.AIRuntime,
	decisionRepository ports.AIDecisionRepository,
	transactions ports.TransactionManager,
) MerchantAIChatService {
	return MerchantAIChatService{
		Sessions:           sessions,
		AIRuntime:          aiRuntime,
		DecisionRepository: decisionRepository,
		Transactions:       transactions,
		Now:                func() time.Time { return time.Now().UTC() },
		NewID:              uuid.NewString,
		MaxContextMessages: 8,
	}
}

func (s MerchantAIChatService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s MerchantAIChatService) id() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return uuid.NewString()
}

func (s MerchantAIChatService) maxMessages() int {
	if s.MaxContextMessages > 0 {
		return s.MaxContextMessages
	}
	return 8
}

func (s MerchantAIChatService) Handle(ctx context.Context, command commands.MerchantAIChatCommand) (commands.MerchantAIChatResult, error) {
	businessID := strings.TrimSpace(string(command.Meta.Actor.BusinessID))
	if businessID == "" {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeValidation, "business_id is required")
	}
	principalID := strings.TrimSpace(string(command.Meta.Actor.PrincipalID))
	if principalID == "" {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeUnauthenticated, "authenticated principal is required")
	}
	text := strings.TrimSpace(command.Message)
	if text == "" {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeValidation, "message cannot be empty")
	}
	if len([]rune(text)) > 10000 {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeValidation, "message exceeds 10000 characters")
	}
	if s.Sessions == nil || s.Transactions == nil {
		return commands.MerchantAIChatResult{}, appErrors.NotImplemented()
	}

	now := s.now()
	var (
		sessionID     string
		merchantMsgID string
		recentMsgs    []ports.MerchantAIMessageRecord
	)

	// Phase 1: Short local DB transaction to create or verify session and persist merchant message
	err := s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if command.SessionID != nil && strings.TrimSpace(*command.SessionID) != "" {
			reqSessionID := strings.TrimSpace(*command.SessionID)
			session, err := s.Sessions.GetSession(txCtx, businessID, principalID, reqSessionID)
			if err != nil {
				var kinded interface{ ErrorKind() string }
				if errors.As(err, &kinded) && kinded.ErrorKind() == "not_found" {
					return appErrors.New(appErrors.CodeNotFound, "merchant AI session not found")
				}
				return err
			}
			sessionID = session.ID
		} else {
			sessionID = s.id()
			_, err := s.Sessions.CreateSession(txCtx, ports.MerchantAISessionRecord{
				ID:          sessionID,
				BusinessID:  businessID,
				PrincipalID: principalID,
				CreatedAt:   now,
				UpdatedAt:   now,
			})
			if err != nil {
				return err
			}
		}

		merchantMsgID = s.id()
		_, err := s.Sessions.AppendMessage(txCtx, ports.MerchantAIMessageDraft{
			ID:         merchantMsgID,
			BusinessID: businessID,
			SessionID:  sessionID,
			SenderType: "merchant",
			Text:       text,
			CreatedAt:  now,
		})
		if err != nil {
			return err
		}

		recent, err := s.Sessions.ListRecentMessages(txCtx, businessID, sessionID, s.maxMessages())
		if err != nil {
			return err
		}
		recentMsgs = recent
		return nil
	})
	if err != nil {
		return commands.MerchantAIChatResult{}, err
	}

	if s.AIRuntime == nil {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeExternalDependency, "AI runtime is not configured")
	}

	// Phase 2: External AI Runtime call (strictly outside DB transactions)
	recentEvidence := make([]ports.AIRecentMessageEvidence, 0, len(recentMsgs))
	for _, rm := range recentMsgs {
		direction := "inbound"
		if rm.SenderType == "assistant" {
			direction = "outbound"
		}
		recentEvidence = append(recentEvidence, ports.AIRecentMessageEvidence{
			Reference:     rm.ID,
			Direction:     direction,
			Origin:        rm.SenderType,
			Text:          rm.Text,
			OccurredAt:    rm.CreatedAt,
			SchemaVersion: 1,
		})
	}

	aiContext := &ports.AIContext{
		SchemaVersion: 1,
		Freshness:     AIContextFresh,
		Business: ports.AIContextBusiness{
			Reference: businessID,
		},
		RecentMessages: recentEvidence,
		GeneratedAt:    now,
		ExpiresAt:      now.Add(5 * time.Minute),
	}

	aiInput := ports.AIDecisionInput{
		BusinessID:             businessID,
		ConversationID:         sessionID,
		SourceMessageReference: merchantMsgID,
		Text:                   text,
		Channel:                "dashboard",
		PolicyVersion:          "merchant-ai-v1",
		Context:                aiContext,
	}

	proposal, err := s.AIRuntime.Decide(ctx, aiInput)
	if err != nil {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeExternalDependency, "AI runtime invocation failed: "+err.Error())
	}

	action := strings.TrimSpace(proposal.RequestedAction)
	if action == "" {
		action = "answer"
	}
	assistantText := strings.TrimSpace(proposal.ResponseText)
	if assistantText == "" {
		if action == "ask_clarification" {
			assistantText = "يرجى توضيح البيانات المطلوبة لمتابعة طلبك."
		} else {
			assistantText = "تمت معالجة طلبك بنجاح."
		}
	}

	// Phase 3: Short local DB transaction to persist assistant response, decision audit, and touch session
	postNow := s.now()
	assistantMsgID := s.id()
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		_, appendErr := s.Sessions.AppendMessage(txCtx, ports.MerchantAIMessageDraft{
			ID:         assistantMsgID,
			BusinessID: businessID,
			SessionID:  sessionID,
			SenderType: "assistant",
			Text:       assistantText,
			CreatedAt:  postNow,
		})
		if appendErr != nil {
			return appendErr
		}

		if s.DecisionRepository != nil {
			decisionID := s.id()
			sourceRef := merchantMsgID
			schemaVersion := proposal.SchemaVersion
			if schemaVersion <= 0 {
				schemaVersion = 1
			}
			confidenceBand := strings.TrimSpace(proposal.ConfidenceBand)
			if confidenceBand == "" {
				confidenceBand = "high"
			}
			intentBase := strings.TrimSpace(proposal.IntentBase)
			if intentBase == "" {
				intentBase = "merchant_ai_chat"
			}
			decisionDraft := ports.AIDecisionDraft{
				ID:                     decisionID,
				BusinessID:             businessID,
				ConversationID:         nil,
				SourceMessageReference: &sourceRef,
				IntentBase:             intentBase,
				DomainContext:          stringPointer(proposal.DomainContext),
				Entities:               proposal.Entities,
				EvidenceReferences:     proposal.EvidenceReferences,
				RequestedAction:        action,
				ConfidenceValue:        stringPointer(proposal.ConfidenceValue),
				ConfidenceBand:         confidenceBand,
				RequiresHuman:          proposal.RequiresHuman,
				MissingInformation:     proposal.MissingInformation,
				ReasonCodes:            proposal.ReasonCodes,
				PolicyVersion:          nonEmptyOr(proposal.PolicyVersion, "merchant-ai-v1"),
				ModelReference:         stringPointer(proposal.ModelReference),
				SchemaVersion:          schemaVersion,
				Lifecycle:              "proposed",
				PolicyDecision:         stringPointer(proposal.PolicyDecision),
				ExecutionReference:     &sessionID,
				CorrelationID:          uuidStringPointer(command.Meta.CorrelationID),
				CausationID:            uuidStringPointer(merchantMsgID),
				CreatedAt:              postNow,
				UpdatedAt:              postNow,
			}
			if _, createErr := s.DecisionRepository.CreateProposed(txCtx, decisionDraft); createErr != nil {
				return createErr
			}
		}

		if touchErr := s.Sessions.TouchSession(txCtx, businessID, sessionID, postNow); touchErr != nil {
			return touchErr
		}
		return nil
	})
	if err != nil {
		return commands.MerchantAIChatResult{}, err
	}

	return commands.MerchantAIChatResult{
		Message:   assistantText,
		Action:    action,
		SessionID: sessionID,
	}, nil
}

var _ commands.MerchantAIChatHandler = MerchantAIChatService{}
