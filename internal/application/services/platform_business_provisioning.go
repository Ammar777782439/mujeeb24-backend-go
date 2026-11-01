package services

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ProvisionBusinessService struct {
	Transactions ports.TransactionManager
	Business     ports.PlatformBusinessLifecyclePort
	Owners       *AssignBusinessOwnerService
	Audit        ports.PlatformAuditRepository
}

type ProvisionBusinessInput struct {
	Actor         commands.ActorContext
	Business      ports.PlatformBusinessCreate
	Owner         AssignBusinessOwnerInput
	CorrelationID string
}

func (s ProvisionBusinessService) Handle(ctx context.Context, in ProvisionBusinessInput) (ports.PlatformBusinessRecord, error) {
	if s.Transactions == nil || s.Business == nil || s.Owners == nil || s.Audit == nil {
		return ports.PlatformBusinessRecord{}, appErrors.NotImplemented()
	}
	in.Owner.Actor, in.Owner.BusinessID, in.Owner.Now, in.Owner.CorrelationID = in.Actor, in.Business.ID, in.Business.Now, in.CorrelationID
	prepared, err := s.Owners.prepare(in.Owner)
	if err != nil {
		return ports.PlatformBusinessRecord{}, err
	}
	var result ports.PlatformBusinessRecord
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		if _, err := s.Business.Create(txCtx, in.Business); err != nil {
			return err
		}
		if _, err := s.Owners.assignInTransaction(txCtx, prepared); err != nil {
			return err
		}
		if err := appendGovernanceAudit(txCtx, s.Audit, in.Actor, "business.created", "business", in.Business.ID, in.Business.ID, in.CorrelationID, prepared.input.Now, map[string]any{"name": in.Business.Name, "slug": in.Business.Slug, "status": "active"}); err != nil {
			return err
		}
		var err error
		result, err = s.Business.GetByID(txCtx, in.Business.ID)
		return err
	})
	if err != nil {
		return ports.PlatformBusinessRecord{}, err
	}
	return result, nil
}
