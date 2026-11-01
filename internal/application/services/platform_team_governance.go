package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// PlatformTeamGovernanceService is deliberately separate from merchant scope.
// It grants no access to conversations, catalog, or other merchant content.
type PlatformTeamGovernanceService struct {
	Repository   ports.PlatformTeamRepository
	Business     ports.PlatformBusinessLifecyclePort
	Transactions ports.TransactionManager
	Audit        ports.PlatformAuditRepository
}

func requirePlatformGovernanceActor(actor commands.ActorContext) error {
	if actor.Role != "platform_super_admin" || uuid.Validate(string(actor.PrincipalID)) != nil {
		return appErrors.New(appErrors.CodeForbidden, "platform super admin scope required")
	}
	return nil
}

func (s PlatformTeamGovernanceService) validate(ctx context.Context, actor commands.ActorContext, businessID string) error {
	if err := requirePlatformGovernanceActor(actor); err != nil {
		return err
	}
	if uuid.Validate(businessID) != nil {
		return appErrors.New(appErrors.CodeValidation, "valid business_id is required")
	}
	if s.Repository == nil || s.Business == nil {
		return appErrors.NotImplemented()
	}
	_, err := s.Business.GetByID(ctx, businessID)
	return mapGovernanceRepositoryError(err, "تعذّر إتمام العملية بحالة المنشأة الحالية.")
}

func (s PlatformTeamGovernanceService) ListMembers(ctx context.Context, actor commands.ActorContext, businessID string, limit int, cursor string) (commands.ListResult[commands.TeamMemberView], error) {
	if err := s.validate(ctx, actor, businessID); err != nil {
		return commands.ListResult[commands.TeamMemberView]{}, err
	}
	page, err := s.Repository.ListMembers(ctx, businessID, limit, cursor)
	if err != nil {
		return commands.ListResult[commands.TeamMemberView]{}, mapTeamRepositoryError(err)
	}
	if err := appendGovernanceAudit(ctx, s.Audit, actor, "team.members.viewed", "business", businessID, businessID, "", time.Now().UTC(), map[string]any{"count": len(page.Items)}); err != nil {
		return commands.ListResult[commands.TeamMemberView]{}, err
	}
	return teamMemberList(page), nil
}

func (s PlatformTeamGovernanceService) ListInvitations(ctx context.Context, actor commands.ActorContext, businessID string, limit int, cursor string) (commands.ListResult[commands.TeamInvitationView], error) {
	if err := s.validate(ctx, actor, businessID); err != nil {
		return commands.ListResult[commands.TeamInvitationView]{}, err
	}
	page, err := s.Repository.ListInvitations(ctx, businessID, limit, cursor, time.Now().UTC())
	if err != nil {
		return commands.ListResult[commands.TeamInvitationView]{}, mapTeamRepositoryError(err)
	}
	if err := appendGovernanceAudit(ctx, s.Audit, actor, "team.invitations.viewed", "business", businessID, businessID, "", time.Now().UTC(), map[string]any{"count": len(page.Items)}); err != nil {
		return commands.ListResult[commands.TeamInvitationView]{}, err
	}
	items := make([]commands.TeamInvitationView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, teamInvitationView(item))
	}
	return commands.ListResult[commands.TeamInvitationView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s PlatformTeamGovernanceService) UpdatePermissions(ctx context.Context, actor commands.ActorContext, change ports.TeamMemberPermissionChange, reason, correlationID string) (commands.TeamMemberView, error) {
	if err := s.validate(ctx, actor, change.BusinessID); err != nil {
		return commands.TeamMemberView{}, err
	}
	permissions, ok := authorization.NormalizePermissions(change.Permissions)
	reason = strings.TrimSpace(reason)
	if uuid.Validate(change.PrincipalID) != nil || !ok || len(permissions) == 0 || len(reason) < 3 || len(reason) > 500 {
		return commands.TeamMemberView{}, appErrors.New(appErrors.CodeValidation, "valid member, at least one known permission, and an audit reason are required")
	}
	if s.Transactions == nil || s.Audit == nil {
		return commands.TeamMemberView{}, appErrors.NotImplemented()
	}
	change.Permissions = permissions
	change.UpdatedAt = time.Now().UTC()
	var result ports.TeamMemberRecord
	err := s.Transactions.Within(ctx, func(txCtx context.Context) error {
		before, after, err := s.Repository.UpdateMemberPermissions(txCtx, change)
		if err != nil {
			return mapTeamRepositoryError(err)
		}
		if err := appendGovernanceAudit(txCtx, s.Audit, actor, "team.member.permissions_updated", "business_membership", change.BusinessID, change.PrincipalID, correlationID, change.UpdatedAt, map[string]any{"reason": reason, "role": after.Role, "before_permissions": before.Permissions, "permissions": after.Permissions}); err != nil {
			return err
		}
		result = after
		return nil
	})
	if err != nil {
		return commands.TeamMemberView{}, err
	}
	return teamMemberView(result), nil
}

func appendGovernanceAudit(ctx context.Context, audit ports.PlatformAuditRepository, actor commands.ActorContext, action, targetType, businessID, targetID, correlationID string, now time.Time, metadata map[string]any) error {
	if audit == nil {
		return appErrors.NotImplemented()
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	actorID := string(actor.PrincipalID)
	_, err = audit.Append(ctx, ports.PlatformAuditDraft{ActorPlatformAdminID: &actorID, Action: action, TargetType: targetType, TargetID: &targetID, BusinessID: &businessID, Result: "SUCCESS", CorrelationID: &correlationID, OccurredAt: now, Metadata: encoded})
	return err
}
