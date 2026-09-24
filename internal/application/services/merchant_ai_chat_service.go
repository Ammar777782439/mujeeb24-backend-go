package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type MerchantAIChatService struct {
	Sessions           ports.MerchantAISessionRepository
	MerchantAIRuntime  ports.MerchantAIRuntime
	Transactions       ports.TransactionManager
	Now                func() time.Time
	NewID              func() string
	MaxContextMessages int
}

func NewMerchantAIChatService(
	sessions ports.MerchantAISessionRepository,
	merchantAIRuntime ports.MerchantAIRuntime,
	transactions ports.TransactionManager,
) MerchantAIChatService {
	return MerchantAIChatService{
		Sessions:           sessions,
		MerchantAIRuntime:  merchantAIRuntime,
		Transactions:       transactions,
		Now:                func() time.Time { return time.Now().UTC() },
		NewID:              uuid.NewString,
		MaxContextMessages: 12,
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
	return 12
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

	// Phase 1: Short local DB transaction to verify or create session and persist merchant message
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

	if s.MerchantAIRuntime == nil {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeExternalDependency, "Merchant AI runtime is not configured")
	}

	// Phase 2: External Merchant AI Runtime call (strictly outside DB transactions)
	chatInput := ports.MerchantAIChatInput{
		BusinessID:     businessID,
		PrincipalID:    principalID,
		SessionID:      sessionID,
		CurrentMessage: text,
		History:        recentMsgs,
	}

	chatOutput, err := s.MerchantAIRuntime.Chat(ctx, chatInput)
	if err != nil {
		return commands.MerchantAIChatResult{}, appErrors.New(appErrors.CodeExternalDependency, "Merchant AI runtime invocation failed: "+err.Error())
	}

	action := strings.TrimSpace(chatOutput.Action)
	if action == "" {
		action = "answer"
	}
	assistantText := strings.TrimSpace(chatOutput.ResponseText)
	if assistantText == "" {
		if action == "ask_clarification" {
			assistantText = "يرجى توضيح البيانات أو تفاصيل الصنف لمتابعة إضافته بالكتالوج."
		} else {
			assistantText = "تم تنفيذ طلبك في الكتالوج بنجاح."
		}
	}

	// Phase 3: Short local DB transaction to persist assistant response and touch session (Zero ai_decisions)
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
