package services

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/golang.org/x/crypto/bcrypt"
	"github.com/google/uuid"
)

type AssignBusinessOwnerInput struct {
	BusinessID  string
	Email       string
	DisplayName string
	Password    string
	Now         time.Time
}

type AssignBusinessOwnerResult struct {
	BusinessID     string
	PrincipalID    string
	Email          string
	DisplayName    string
	Role           string
	BusinessStatus string
}

// AssignBusinessOwnerService atomically creates/updates the owner principal,
// links the owner membership, and transitions the business from pending_setup
// to active. Plaintext passwords never leave this service and are never stored.
type AssignBusinessOwnerService struct {
	Transactions       ports.TransactionManager
	PrincipalBootstrap ports.PrincipalBootstrapRepository
	Business           ports.PlatformBusinessOwnerPort
}

func (s AssignBusinessOwnerService) Handle(ctx context.Context, in AssignBusinessOwnerInput) (AssignBusinessOwnerResult, error) {
	if s.Transactions == nil || s.PrincipalBootstrap == nil || s.Business == nil {
		return AssignBusinessOwnerResult{}, appErrors.NotImplemented()
	}

	in.BusinessID = strings.TrimSpace(in.BusinessID)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}

	if uuid.Validate(in.BusinessID) != nil {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeValidation, "business_id must be a valid UUID")
	}
	if _, err := mail.ParseAddress(in.Email); err != nil || !strings.Contains(in.Email, "@") {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeValidation, "email must be valid")
	}
	if in.DisplayName == "" {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeValidation, "display_name is required")
	}
	if len(in.Password) < 12 {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeValidation, "password must be at least 12 characters")
	}

	current, err := s.Business.GetByID(ctx, in.BusinessID)
	if err != nil {
		return AssignBusinessOwnerResult{}, err
	}
	if strings.ToLower(strings.TrimSpace(current.PlatformStatus)) != "pending_setup" {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeConflict, "business is not pending_setup; owner assignment is only allowed before activation")
	}
	if current.OwnerIdentitySummary != nil && strings.TrimSpace(*current.OwnerIdentitySummary) != "" {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeConflict, "business already has an active owner")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeExternalDependency, "failed to prepare owner credentials")
	}

	var result AssignBusinessOwnerResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		principal, err := s.PrincipalBootstrap.EnsurePrincipalAndMembership(
			txCtx,
			ports.PrincipalRecord{
				ID:           commands.PrincipalID(uuid.NewString()),
				Email:        in.Email,
				DisplayName:  in.DisplayName,
				PasswordHash: string(passwordHash),
				Status:       "active",
			},
			commands.BusinessID(in.BusinessID),
			"owner",
			[]string{"*"},
			in.Now,
		)
		if err != nil {
			return err
		}

		updated, err := s.Business.ActivateFromPendingSetup(txCtx, in.BusinessID, in.Now)
		if err != nil {
			return err
		}

		result = AssignBusinessOwnerResult{
			BusinessID:     in.BusinessID,
			PrincipalID:    string(principal.ID),
			Email:          principal.Email,
			DisplayName:    principal.DisplayName,
			Role:           "owner",
			BusinessStatus: strings.ToUpper(updated.PlatformStatus),
		}
		return nil
	})
	if err != nil {
		return AssignBusinessOwnerResult{}, err
	}
	return result, nil
}
