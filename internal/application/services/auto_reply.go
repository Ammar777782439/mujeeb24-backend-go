package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const (
	AutoReplyModeRestrictedAuto = "restricted_auto"
	AutoReplyActionAnswer       = "answer"
)

type AutoReplyService struct {
	Runtime             ports.AIRuntime
	ContextBuilder      ports.AIContextBuilder
	DecisionRepository  ports.AIDecisionRepository
	ReferenceRepository ports.ConversationReferenceRepository
	OutboundRepository  ports.OutboundMessageRepository
	Outbox              ports.OutboxStore
	Transactions        ports.TransactionManager
	Mode                string
	PolicyVersion       string
	Now                 func() time.Time
	NewID               func() string
}

func NewAutoReplyService(runtime ports.AIRuntime, decisions ports.AIDecisionRepository, references ports.ConversationReferenceRepository, outbound ports.OutboundMessageRepository, outbox ports.OutboxStore, transactions ports.TransactionManager) AutoReplyService {
	return AutoReplyService{
		Runtime:             runtime,
		DecisionRepository:  decisions,
		ReferenceRepository: references,
		OutboundRepository:  outbound,
		Outbox:              outbox,
		Transactions:        transactions,
		Mode:                AutoReplyModeRestrictedAuto,
		PolicyVersion:       "auto-reply-v1",
		Now:                 func() time.Time { return time.Now().UTC() },
		NewID:               uuid.NewString,
	}
}

func (s AutoReplyService) Handle(ctx context.Context, command commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
	if err := s.validate(command); err != nil {
		return commands.AutoReplyResult{}, err
	}
	if s.Runtime == nil || s.DecisionRepository == nil || s.ReferenceRepository == nil || s.OutboundRepository == nil || s.Outbox == nil || s.Transactions == nil {
		return commands.AutoReplyResult{}, appErrors.NotImplemented()
	}

	policyVersion := s.PolicyVersion
	if strings.TrimSpace(policyVersion) == "" {
		policyVersion = "auto-reply-v1"
	}
	aiInput := ports.AIDecisionInput{
		BusinessID:             string(command.Meta.Actor.BusinessID),
		ConversationID:         string(command.ConversationID),
		SourceMessageReference: command.SourceMessageReference,
		Text:                   command.Text,
		Channel:                command.Channel,
		PolicyVersion:          policyVersion,
	}
	if s.ContextBuilder != nil {
		builtContext, contextErr := s.ContextBuilder.Build(ctx, ports.ContextBuildInput{
			BusinessID:             aiInput.BusinessID,
			ConversationID:         aiInput.ConversationID,
			SourceMessageReference: aiInput.SourceMessageReference,
			Text:                   aiInput.Text,
			Channel:                aiInput.Channel,
			PolicyVersion:          aiInput.PolicyVersion,
		})
		if contextErr != nil {
			return commands.AutoReplyResult{}, contextErr
		}
		aiInput.Context = &builtContext
	}
	proposal, err := s.Runtime.Decide(ctx, aiInput)
	if err != nil {
		return commands.AutoReplyResult{}, err
	}
	if err := validateProposal(proposal); err != nil {
		return commands.AutoReplyResult{}, err
	}

	now := s.now()
	decisionID := s.id()
	conversationID := string(command.ConversationID)
	sourceMessageReference := command.SourceMessageReference
	decisionDraft := ports.AIDecisionDraft{
		ID:                     decisionID,
		BusinessID:             string(command.Meta.Actor.BusinessID),
		ConversationID:         &conversationID,
		SourceMessageReference: &sourceMessageReference,
		IntentBase:             proposal.IntentBase,
		DomainContext:          stringPointer(proposal.DomainContext),
		Entities:               proposal.Entities,
		EvidenceReferences:     proposal.EvidenceReferences,
		RequestedAction:        proposal.RequestedAction,
		ConfidenceValue:        stringPointer(proposal.ConfidenceValue),
		ConfidenceBand:         proposal.ConfidenceBand,
		RequiresHuman:          proposal.RequiresHuman,
		MissingInformation:     proposal.MissingInformation,
		ReasonCodes:            proposal.ReasonCodes,
		PolicyVersion:          nonEmptyOr(proposal.PolicyVersion, policyVersion),
		KnowledgeVersion:       stringPointer(proposal.KnowledgeVersion),
		ModelReference:         stringPointer(proposal.ModelReference),
		SchemaVersion:          proposal.SchemaVersion,
		Lifecycle:              "proposed",
		PolicyDecision:         stringPointer(proposal.PolicyDecision),
		CorrelationID:          uuidStringPointer(command.Meta.CorrelationID),
		CausationID:            uuidStringPointer(command.SourceMessageReference),
		ExpiresAt:              pointerTo(now.Add(5 * time.Minute)),
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	result := commands.AutoReplyResult{Action: proposal.RequestedAction}
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		decision, createErr := s.DecisionRepository.CreateProposed(txCtx, decisionDraft)
		if createErr != nil {
			return mapAIRepositoryError(createErr)
		}
		result.Decision = aiDecisionView(decision)

		if proposal.RequestedAction != AutoReplyActionAnswer || proposal.RequiresHuman || proposal.PolicyDecision != "allowed" {
			return nil
		}
		if strings.TrimSpace(proposal.ResponseText) == "" {
			return fmt.Errorf("%w: answer action requires response text", appErrors.New(appErrors.CodeValidation, "auto reply"))
		}

		reference, referenceErr := s.ReferenceRepository.GetCurrentByConversation(txCtx, string(command.Meta.Actor.BusinessID), conversationID, "provider")
		if referenceErr != nil {
			return mapAIRepositoryError(referenceErr)
		}
		if reference.ProviderRef != command.ProviderRef {
			return appErrors.New(appErrors.CodeInvalidState, "conversation provider reference does not match requested provider")
		}
		if reference.ConnectionID == nil || strings.TrimSpace(*reference.ConnectionID) == "" || strings.TrimSpace(reference.ResourceID) == "" {
			return appErrors.New(appErrors.CodeInvalidState, "conversation provider reference is incomplete")
		}

		outboundID := s.id()
		contentReference := EncodeInlineTextContentReference(proposal.ResponseText)
		outbound, outboundErr := s.OutboundRepository.CreatePending(txCtx, ports.OutboundMessageDraft{
			ID:                      outboundID,
			BusinessID:              string(command.Meta.Actor.BusinessID),
			ConversationID:          conversationID,
			ConversationReferenceID: reference.ID,
			ConnectionID:            *reference.ConnectionID,
			ProviderRef:             command.ProviderRef,
			Channel:                 command.Channel,
			Origin:                  "ai",
			Transport:               "provider",
			ContentReference:        contentReference,
			ProviderIdempotencyKey:  "auto-reply:" + sourceMessageReference,
			CorrelationID:           uuidStringPointer(command.Meta.CorrelationID),
			CausationID:             uuidStringPointer(decision.ID),
		})
		if outboundErr != nil {
			return mapAIRepositoryError(outboundErr)
		}
		outbox, outboxErr := s.Outbox.Enqueue(txCtx, ports.OutboxEntryDraft{
			ID:                s.id(),
			BusinessID:        string(command.Meta.Actor.BusinessID),
			OutboundMessageID: outbound.ID,
			CommandType:       OutboundSendCommandType,
			DedupeKey:         "auto-reply:" + sourceMessageReference,
			AvailableAt:       now,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
		if outboxErr != nil {
			return mapAIRepositoryError(outboxErr)
		}
		result.OutboundMessageID = commands.MessageID(outbound.ID)
		result.OutboxEntryID = commands.ID(outbox.ID)
		result.Enqueued = true
		return nil
	})
	if err != nil {
		return commands.AutoReplyResult{}, err
	}
	return result, nil
}

func (s AutoReplyService) validate(command commands.AutoReplyCommand) error {
	if s.Mode != AutoReplyModeRestrictedAuto {
		return appErrors.New(appErrors.CodeInvalidState, "auto reply is not enabled in restricted_auto mode")
	}
	if command.Meta.Actor.BusinessID == "" || command.ConversationID == "" || strings.TrimSpace(command.SourceMessageReference) == "" || strings.TrimSpace(command.Text) == "" || strings.TrimSpace(command.Channel) == "" || strings.TrimSpace(command.ProviderRef) == "" {
		return appErrors.New(appErrors.CodeValidation, "business, conversation, source message, text, channel, and provider are required")
	}
	if command.Channel != "facebook" && command.Channel != "instagram" && command.Channel != "whatsapp" {
		return appErrors.New(appErrors.CodeValidation, "unsupported auto reply channel")
	}
	return nil
}

func validateProposal(proposal ports.AIDecisionProposal) error {
	if strings.TrimSpace(proposal.IntentBase) == "" || strings.TrimSpace(proposal.RequestedAction) == "" || strings.TrimSpace(proposal.ConfidenceBand) == "" || proposal.SchemaVersion <= 0 || strings.TrimSpace(proposal.PolicyDecision) == "" {
		return appErrors.New(appErrors.CodeValidation, "AI proposal must contain intent, action, confidence band, schema version, and policy decision")
	}
	if proposal.RequestedAction != AutoReplyActionAnswer && proposal.RequestedAction != "ask_clarification" && proposal.RequestedAction != "no_action" {
		return appErrors.New(appErrors.CodeValidation, "AI proposal action is outside the first auto reply slice")
	}
	if proposal.PolicyDecision != "allowed" && proposal.PolicyDecision != "requires_approval" && proposal.PolicyDecision != "denied" {
		return appErrors.New(appErrors.CodeValidation, "AI proposal policy decision is invalid")
	}
	if len(proposal.Entities) > 0 && !jsonObject(proposal.Entities) {
		return appErrors.New(appErrors.CodeValidation, "AI proposal entities must be a JSON object")
	}
	if len(proposal.EvidenceReferences) > 0 && !jsonArray(proposal.EvidenceReferences) {
		return appErrors.New(appErrors.CodeValidation, "AI proposal evidence must be a JSON array")
	}
	return nil
}

func EncodeInlineTextContentReference(text string) string {
	return "content://inline-text/v1/" + base64.RawURLEncoding.EncodeToString([]byte(text))
}

func DecodeInlineTextContentReference(reference string) (string, error) {
	const prefix = "content://inline-text/v1/"
	if !strings.HasPrefix(reference, prefix) {
		return "", errors.New("unsupported content reference")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(reference, prefix))
	if err != nil {
		return "", fmt.Errorf("decode inline text content: %w", err)
	}
	return string(decoded), nil
}

func jsonObject(value []byte) bool {
	var object map[string]json.RawMessage
	return len(value) > 0 && json.Unmarshal(value, &object) == nil && object != nil
}

func jsonArray(value []byte) bool {
	var array []json.RawMessage
	return len(value) > 0 && json.Unmarshal(value, &array) == nil && array != nil
}

func (s AutoReplyService) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s AutoReplyService) id() string {
	if s.NewID == nil {
		return uuid.NewString()
	}
	return s.NewID()
}

func uuidStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil
	}
	return &value
}

func pointerTo(value time.Time) *time.Time { return &value }

var _ commands.AutoReplyHandler = AutoReplyService{}
