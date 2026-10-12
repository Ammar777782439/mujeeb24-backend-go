package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/catalogvalidation"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"strings"
	"time"
)

type SalesWorkflowAdminService struct {
	Repository ports.SalesWorkflowAdminRepository
	Workflow   CustomerSalesWorkflowService
}

func salesAdminAccess(actor commands.ActorContext, permission string) error {
	if actor.BusinessID == "" || actor.PrincipalID == "" || !authorization.Has(actor, permission) {
		return appErrors.New(appErrors.CodeForbidden, "sales permission required")
	}
	return nil
}
func (s SalesWorkflowAdminService) Get(ctx context.Context, a commands.ActorContext, id string) (ports.SalesWorkflowRecord, error) {
	if err := salesAdminAccess(a, authorization.PermissionSalesRead); err != nil {
		return ports.SalesWorkflowRecord{}, err
	}
	w, err := s.Repository.GetWorkflow(ctx, string(a.BusinessID), id)
	return w, mapSalesRepositoryError(err)
}
func (s SalesWorkflowAdminService) List(ctx context.Context, a commands.ActorContext, customer, state string, limit int, cursor string) (ports.SalesWorkflowPage, error) {
	if err := salesAdminAccess(a, authorization.PermissionSalesRead); err != nil {
		return ports.SalesWorkflowPage{}, err
	}
	page, err := s.Repository.ListWorkflows(ctx, string(a.BusinessID), customer, state, limit, cursor)
	return page, mapSalesRepositoryError(err)
}
func (s SalesWorkflowAdminService) Review(ctx context.Context, a commands.ActorContext, id string, version int64, approved bool) (ports.SalesWorkflowResult, error) {
	var result ports.SalesWorkflowResult
	if err := salesAdminAccess(a, authorization.PermissionSalesApprove); err != nil {
		return result, err
	}
	if version <= 0 {
		return result, appErrors.New(appErrors.CodeValidation, "If-Match is required")
	}
	now := time.Now().UTC()
	err := s.Workflow.Transactions.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = s.Repository.ReviewWorkflow(tx, ports.SalesWorkflowReview{BusinessID: string(a.BusinessID), WorkflowID: id, ActorReference: string(a.PrincipalID), ExpectedVersion: version, Approved: approved, Now: now})
		if err != nil {
			return err
		}
		if result.Duplicate {
			return nil
		}
		if err := s.Workflow.record(tx, string(a.BusinessID), "", "", result, now); err != nil {
			return err
		}
		return s.Workflow.notify(tx, string(a.BusinessID), result, fmt.Sprintf("sales-review:%s:%d", id, version), now)
	})
	return result, mapSalesRepositoryError(err)
}
func (s SalesWorkflowAdminService) PutRequirement(ctx context.Context, a commands.ActorContext, req ports.SalesRequirement) error {
	if err := salesAdminAccess(a, authorization.PermissionBusinessSettings); err != nil {
		return err
	}
	if !validTransactionType(req.TransactionType) || req.SchemaVersion < 1 {
		return appErrors.New(appErrors.CodeValidation, "valid transaction type and schema version required")
	}
	if req.Definitions == nil {
		req.Definitions = []ports.SalesRequirementDefinition{}
	}
	seen := map[string]bool{}
	for _, d := range req.Definitions {
		if strings.TrimSpace(d.Key) == "" || strings.TrimSpace(d.Label) == "" || seen[d.Key] {
			return appErrors.New(appErrors.CodeValidation, "unique requirement keys and labels required")
		}
		seen[d.Key] = true
		if err := catalogvalidation.ValidateDefinition(d.DataType, d.Rules); err != nil {
			return appErrors.New(appErrors.CodeValidation, err.Error())
		}
		if _, err := json.Marshal(d.Rules); err != nil {
			return appErrors.New(appErrors.CodeValidation, "invalid validation rules")
		}
	}
	return mapSalesRepositoryError(s.Workflow.Transactions.Within(ctx, func(tx context.Context) error { return s.Repository.PutRequirement(tx, string(a.BusinessID), req) }))
}
func (s SalesWorkflowAdminService) Requirements(ctx context.Context, a commands.ActorContext) ([]ports.SalesRequirement, error) {
	if err := salesAdminAccess(a, authorization.PermissionSalesRead); err != nil {
		return nil, err
	}
	req, err := s.Repository.ListRequirements(ctx, string(a.BusinessID))
	return req, mapSalesRepositoryError(err)
}

var _ ports.SalesAdminPort = SalesWorkflowAdminService{}
