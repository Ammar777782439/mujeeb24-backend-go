package services

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AssignBusinessOwnerInput struct {
	Actor         commands.ActorContext
	BusinessID    string
	Email         string
	DisplayName   string
	Password      string
	Permissions   []string
	CorrelationID string
	Now           time.Time
}

type AssignBusinessOwnerResult struct {
	BusinessID     string
	PrincipalID    string
	Email          string
	DisplayName    string
	Role           string
	Permissions    []string
	BusinessStatus string
}

// AssignBusinessOwnerService provisions a new owner and activates a pending
// business atomically. It never replaces credentials of an existing principal.
type AssignBusinessOwnerService struct {
	Transactions  ports.TransactionManager
	OwnerAccounts ports.OwnerPrincipalRepository
	Business      ports.PlatformBusinessOwnerPort
	Audit         ports.PlatformAuditRepository
}

type preparedBusinessOwner struct {
	input     AssignBusinessOwnerInput
	principal ports.PrincipalRecord
}

func (s AssignBusinessOwnerService) prepare(in AssignBusinessOwnerInput) (preparedBusinessOwner, error) {
	if s.Transactions == nil || s.OwnerAccounts == nil || s.Business == nil || s.Audit == nil {
		return preparedBusinessOwner{}, appErrors.NotImplemented()
	}
	if err := requirePlatformGovernanceActor(in.Actor); err != nil {
		return preparedBusinessOwner{}, err
	}
	in.BusinessID = strings.TrimSpace(in.BusinessID)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	address, err := mail.ParseAddress(in.Email)
	if uuid.Validate(in.BusinessID) != nil || err != nil || address.Address != in.Email || !strings.Contains(in.Email, "@") || len(in.Email) > 320 || in.DisplayName == "" || len(in.DisplayName) > 200 {
		return preparedBusinessOwner{}, appErrors.New(appErrors.CodeValidation, "valid business_id, email and display_name are required")
	}
	if len(in.Password) < minimumEmployeePasswordLength || len(in.Password) > maximumBcryptPasswordBytes {
		return preparedBusinessOwner{}, appErrors.New(appErrors.CodeValidation, "password must be between 12 and 72 bytes")
	}
	// Omission retains compatibility with existing platform clients. New clients
	// send an explicit set; neither custom sets nor owner role imply a wildcard.
	if in.Permissions == nil {
		in.Permissions = authorization.KnownPermissions()
	}
	permissions, ok := authorization.NormalizePermissions(in.Permissions)
	if !ok || len(permissions) == 0 {
		return preparedBusinessOwner{}, appErrors.New(appErrors.CodeValidation, "at least one known owner permission is required")
	}
	in.Permissions = permissions
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return preparedBusinessOwner{}, appErrors.New(appErrors.CodeExternalDependency, "failed to prepare owner credentials")
	}
	in.Password = ""
	return preparedBusinessOwner{input: in, principal: ports.PrincipalRecord{ID: commands.PrincipalID(uuid.NewString()), Email: in.Email, DisplayName: in.DisplayName, PasswordHash: string(passwordHash), Status: "active"}}, nil
}

func (s AssignBusinessOwnerService) Handle(ctx context.Context, in AssignBusinessOwnerInput) (AssignBusinessOwnerResult, error) {
	prepared, err := s.prepare(in)
	if err != nil {
		return AssignBusinessOwnerResult{}, err
	}
	var result AssignBusinessOwnerResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		var assignErr error
		result, assignErr = s.assignInTransaction(txCtx, prepared)
		return assignErr
	})
	if err != nil {
		return AssignBusinessOwnerResult{}, err
	}
	return result, nil
}

// assignInTransaction is composed by both standalone assignment and atomic
// business creation. It does not start a nested transaction.
func (s AssignBusinessOwnerService) assignInTransaction(ctx context.Context, prepared preparedBusinessOwner) (AssignBusinessOwnerResult, error) {
	in := prepared.input
	current, err := s.Business.GetByID(ctx, in.BusinessID)
	if err != nil {
		return AssignBusinessOwnerResult{}, mapGovernanceRepositoryError(err, "المنشأة لديها مالك أو مفعلة بالفعل.")
	}
	if current.PlatformStatus != "pending_setup" || (current.OwnerIdentitySummary != nil && strings.TrimSpace(*current.OwnerIdentitySummary) != "") {
		return AssignBusinessOwnerResult{}, appErrors.New(appErrors.CodeConflict, "business already has an owner or is not pending_setup")
	}
	// Claim the pending business before creating the principal. Concurrent
	// assignments cannot commit two owners; every later failure rolls this back.
	if _, err = s.Business.ActivateFromPendingSetup(ctx, in.BusinessID, in.Now); err != nil {
		return AssignBusinessOwnerResult{}, mapGovernanceRepositoryError(err, "المنشأة لديها مالك أو مفعلة بالفعل.")
	}
	principal, err := s.OwnerAccounts.CreateOwnerPrincipalAndMembership(ctx, prepared.principal, commands.BusinessID(in.BusinessID), in.Permissions, in.Now)
	if err != nil {
		return AssignBusinessOwnerResult{}, mapGovernanceRepositoryError(err, "البريد الإلكتروني للمالك مستخدم مسبقًا.")
	}
	result := AssignBusinessOwnerResult{BusinessID: in.BusinessID, PrincipalID: string(principal.ID), Email: principal.Email, DisplayName: principal.DisplayName, Role: TeamRoleOwner, Permissions: in.Permissions, BusinessStatus: "ACTIVE"}
	err = appendGovernanceAudit(ctx, s.Audit, in.Actor, "business.owner_assigned", "business", in.BusinessID, in.BusinessID, in.CorrelationID, in.Now, map[string]any{"principal_id": result.PrincipalID, "role": result.Role, "permissions": result.Permissions})
	if err != nil {
		return AssignBusinessOwnerResult{}, err
	}
	return result, nil
}
