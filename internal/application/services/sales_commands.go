package services

import (
	"context"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type SalesCommandServices struct {
	LeadRepository        ports.LeadRepository
	TransactionRepository ports.TransactionRepository
	Transactions          ports.TransactionManager
	Now                   func() time.Time
	NewID                 func() string
}

func NewSalesCommandServices(leadRepository ports.LeadRepository, transactionRepository ports.TransactionRepository, transactions ports.TransactionManager) SalesCommandServices {
	return SalesCommandServices{LeadRepository: leadRepository, TransactionRepository: transactionRepository, Transactions: transactions, Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString}
}

func (s SalesCommandServices) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}
func (s SalesCommandServices) id() string {
	if s.NewID == nil {
		return uuid.NewString()
	}
	return s.NewID()
}
func (s SalesCommandServices) readyLead() error {
	if s.LeadRepository == nil || s.Transactions == nil {
		return appErrors.NotImplemented()
	}
	return nil
}
func (s SalesCommandServices) readyTransaction() error {
	if s.TransactionRepository == nil || s.Transactions == nil {
		return appErrors.NotImplemented()
	}
	return nil
}
func (s SalesCommandServices) within(ctx context.Context, fn func(context.Context) error) error {
	return s.Transactions.Within(ctx, fn)
}

func (s SalesCommandServices) actorOrigin(role string) string {
	switch strings.ToLower(role) {
	case "ai":
		return "ai"
	case "automation":
		return "automation"
	case "system", "system_policy":
		return "system"
	default:
		return "human"
	}
}

type CreateLeadCommandService struct{ SalesCommandServices }

func (s CreateLeadCommandService) Handle(ctx context.Context, command commands.CreateLeadCommand) (commands.LeadResult, error) {
	var result commands.LeadResult
	if err := s.readyLead(); err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.CustomerID == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business and customer are required")
	}
	qualificationContext, err := objectJSON(command.QualificationContext)
	if err != nil {
		return result, err
	}
	now := s.now()
	var record ports.LeadRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.LeadRepository.Create(txCtx, ports.LeadDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), CustomerID: string(command.CustomerID), QualificationContext: qualificationContext, CreatedBy: s.actorOrigin(command.Meta.Actor.Role), CreatedAt: now, UpdatedAt: now})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Lead = leadView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type UpdateLeadCommandService struct{ SalesCommandServices }

func (s UpdateLeadCommandService) Handle(ctx context.Context, command commands.UpdateLeadCommand) (commands.LeadResult, error) {
	var result commands.LeadResult
	if err := s.readyLead(); err != nil {
		return result, err
	}
	version, err := expectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.LeadID == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business and lead are required")
	}
	contextJSON, err := objectJSON(command.QualificationContext)
	if err != nil {
		return result, err
	}
	var record ports.LeadRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.LeadRepository.Update(txCtx, ports.LeadPatch{ID: string(command.LeadID), BusinessID: string(command.Meta.Actor.BusinessID), QualificationContext: contextJSON, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Lead = leadView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type QualifyLeadCommandService struct{ SalesCommandServices }

func (s QualifyLeadCommandService) Handle(ctx context.Context, command commands.QualifyLeadCommand) (commands.LeadResult, error) {
	var result commands.LeadResult
	if err := s.readyLead(); err != nil {
		return result, err
	}
	version, err := expectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.LeadID == "" || strings.TrimSpace(command.Reason) == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business, lead, and qualification reason are required")
	}
	evidence, err := stringArrayJSON(command.EvidenceReferences)
	if err != nil {
		return result, err
	}
	var record ports.LeadRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.LeadRepository.Qualify(txCtx, ports.LeadQualificationPatch{ID: string(command.LeadID), BusinessID: string(command.Meta.Actor.BusinessID), QualifiedBy: s.actorOrigin(command.Meta.Actor.Role), QualificationReason: command.Reason, QualificationEvidence: evidence, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Lead = leadView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type MarkLeadLostCommandService struct{ SalesCommandServices }

func (s MarkLeadLostCommandService) Handle(ctx context.Context, command commands.MarkLeadLostCommand) (commands.LeadResult, error) {
	var result commands.LeadResult
	if err := s.readyLead(); err != nil {
		return result, err
	}
	version, err := expectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.LeadID == "" || strings.TrimSpace(command.LostReason) == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business, lead, and lost reason are required")
	}
	var record ports.LeadRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.LeadRepository.MarkLost(txCtx, ports.LeadLostPatch{ID: string(command.LeadID), BusinessID: string(command.Meta.Actor.BusinessID), LostReason: command.LostReason, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Lead = leadView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.Status
	return result, nil
}

type CreateTransactionDraftCommandService struct{ SalesCommandServices }

func (s CreateTransactionDraftCommandService) Handle(ctx context.Context, command commands.CreateTransactionDraftCommand) (commands.TransactionResult, error) {
	var result commands.TransactionResult
	if err := s.readyTransaction(); err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.CustomerID == "" || !validTransactionType(command.TransactionType) || len(command.Lines) == 0 {
		return result, appErrors.New(appErrors.CodeValidation, "business, customer, transaction type, and lines are required")
	}
	lines, err := transactionLineDrafts(command.Lines, s.id)
	if err != nil {
		return result, err
	}
	now := s.now()
	var record ports.TransactionRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.TransactionRepository.CreateDraft(txCtx, ports.TransactionDraft{ID: s.id(), BusinessID: string(command.Meta.Actor.BusinessID), CustomerID: string(command.CustomerID), LeadID: optionalStringID(command.LeadID), TransactionType: command.TransactionType, Currency: command.Currency, SchemaVersion: 1, RequiresHumanReview: false, CreatedAt: now, UpdatedAt: now, Lines: lines})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Transaction = transactionView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.State
	return result, nil
}

type UpdateTransactionDraftCommandService struct{ SalesCommandServices }

func (s UpdateTransactionDraftCommandService) Handle(ctx context.Context, command commands.UpdateTransactionDraftCommand) (commands.TransactionResult, error) {
	var result commands.TransactionResult
	if err := s.readyTransaction(); err != nil {
		return result, err
	}
	version, err := expectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.TransactionID == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business and transaction are required")
	}
	lines, err := transactionLineDrafts(command.Lines, s.id)
	if err != nil {
		return result, err
	}
	var record ports.TransactionRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.TransactionRepository.UpdateDraft(txCtx, ports.TransactionPatch{ID: string(command.TransactionID), BusinessID: string(command.Meta.Actor.BusinessID), Currency: command.Currency, ExpectedVersion: version, UpdatedAt: s.now(), Lines: lines})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Transaction = transactionView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.State
	return result, nil
}

type ConfirmTransactionCommandService struct{ SalesCommandServices }

func (s ConfirmTransactionCommandService) Handle(ctx context.Context, command commands.ConfirmTransactionCommand) (commands.TransactionResult, error) {
	var result commands.TransactionResult
	if err := s.readyTransaction(); err != nil {
		return result, err
	}
	version, err := expectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.TransactionID == "" || strings.TrimSpace(command.EvidenceReference) == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business, transaction, and evidence are required")
	}
	var record ports.TransactionRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.TransactionRepository.Confirm(txCtx, ports.TransactionConfirmPatch{ID: string(command.TransactionID), BusinessID: string(command.Meta.Actor.BusinessID), EvidenceReference: command.EvidenceReference, PolicyVersion: command.PolicyVersion, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Transaction = transactionView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.State
	return result, nil
}

type CancelTransactionCommandService struct{ SalesCommandServices }

func (s CancelTransactionCommandService) Handle(ctx context.Context, command commands.CancelTransactionCommand) (commands.TransactionResult, error) {
	var result commands.TransactionResult
	if err := s.readyTransaction(); err != nil {
		return result, err
	}
	version, err := expectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return result, err
	}
	if command.Meta.Actor.BusinessID == "" || command.TransactionID == "" || strings.TrimSpace(command.Reason) == "" {
		return result, appErrors.New(appErrors.CodeValidation, "business, transaction, and cancellation reason are required")
	}
	var record ports.TransactionRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		record, err = s.TransactionRepository.Cancel(txCtx, ports.TransactionCancelPatch{ID: string(command.TransactionID), BusinessID: string(command.Meta.Actor.BusinessID), Reason: command.Reason, ExpectedVersion: version, UpdatedAt: s.now()})
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return result, err
	}
	result.Transaction = transactionView(record)
	result.ResourceID = commands.ID(record.ID)
	result.ResourceVersion = commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))
	result.Status = record.State
	return result, nil
}

type SubmitTransactionReviewCommandService struct{ SalesCommandServices }

func (s SubmitTransactionReviewCommandService) Handle(ctx context.Context, command commands.SubmitTransactionReviewCommand) (commands.TransactionReviewResult, error) {
	var result commands.TransactionReviewResult
	return s.submitOrDecide(ctx, command.Meta, string(command.TransactionID), command.ReasonCodes, "", false, &result)
}
func (s SalesCommandServices) submitOrDecide(ctx context.Context, meta commands.CommandMeta, transactionID string, reasonCodes []string, reviewer string, approved bool, result *commands.TransactionReviewResult) (commands.TransactionReviewResult, error) {
	if err := s.readyTransaction(); err != nil {
		return *result, err
	}
	version, err := expectedVersion(meta.ExpectedVersion)
	if err != nil {
		return *result, err
	}
	if meta.Actor.BusinessID == "" || transactionID == "" {
		return *result, appErrors.New(appErrors.CodeValidation, "business and transaction are required")
	}
	reasons, err := stringArrayJSON(reasonCodes)
	if err != nil {
		return *result, err
	}
	var record ports.TransactionReviewRecord
	err = s.within(ctx, func(txCtx context.Context) error {
		var err error
		if reviewer == "" {
			record, err = s.TransactionRepository.SubmitReview(txCtx, ports.TransactionReviewPatch{ID: transactionID, BusinessID: string(meta.Actor.BusinessID), ReasonCodes: reasons, ExpectedVersion: version, UpdatedAt: s.now()})
		} else {
			record, err = s.TransactionRepository.DecideReview(txCtx, ports.TransactionReviewDecisionPatch{ID: transactionID, BusinessID: string(meta.Actor.BusinessID), ReviewerReference: reviewer, Approved: approved, ExpectedVersion: version, UpdatedAt: s.now()})
		}
		return mapSalesRepositoryError(err)
	})
	if err != nil {
		return *result, err
	}
	result.Review = transactionReviewView(record)
	result.ResourceID = commands.ID(record.ID)
	result.Status = record.Status
	return *result, nil
}

type ApproveTransactionReviewCommandService struct{ SalesCommandServices }

func (s ApproveTransactionReviewCommandService) Handle(ctx context.Context, command commands.ApproveTransactionReviewCommand) (commands.TransactionReviewResult, error) {
	var result commands.TransactionReviewResult
	return s.submitOrDecide(ctx, command.Meta, string(command.TransactionID), nil, command.ReviewerReference, true, &result)
}

type RejectTransactionReviewCommandService struct{ SalesCommandServices }

func (s RejectTransactionReviewCommandService) Handle(ctx context.Context, command commands.RejectTransactionReviewCommand) (commands.TransactionReviewResult, error) {
	var result commands.TransactionReviewResult
	return s.submitOrDecide(ctx, command.Meta, string(command.TransactionID), nil, command.ReviewerReference, false, &result)
}

func expectedVersion(value *commands.ResourceVersion) (int64, error) {
	if value == nil || *value == "" {
		return 0, appErrors.New(appErrors.CodeValidation, "If-Match resource version is required")
	}
	version, err := strconv.ParseInt(string(*value), 10, 64)
	if err != nil || version <= 0 {
		return 0, appErrors.New(appErrors.CodeValidation, "If-Match resource version must be a positive integer")
	}
	return version, nil
}
func objectJSON(value map[string]any) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, appErrors.New(appErrors.CodeValidation, "object must be valid JSON")
	}
	return encoded, nil
}
func stringArrayJSON(values []string) ([]byte, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, appErrors.New(appErrors.CodeValidation, "array must be valid JSON")
	}
	return encoded, nil
}
func optionalStringID(value *commands.LeadID) *string {
	if value == nil || *value == "" {
		return nil
	}
	id := string(*value)
	return &id
}
func validTransactionType(value string) bool {
	switch value {
	case "order", "booking", "appointment", "service_request", "reservation", "quote", "subscription":
		return true
	default:
		return false
	}
}
func transactionLineDrafts(values []commands.TransactionLine, newID func() string) ([]ports.TransactionLineDraft, error) {
	if values == nil {
		return nil, nil
	}
	lines := make([]ports.TransactionLineDraft, 0, len(values))
	for _, value := range values {
		if value.CatalogItemID == "" {
			return nil, appErrors.New(appErrors.CodeValidation, "catalog item is required")
		}
		quantity := strings.TrimSpace(string(value.Quantity))
		rational, ok := new(big.Rat).SetString(quantity)
		if !ok || rational.Sign() <= 0 {
			return nil, appErrors.New(appErrors.CodeValidation, "quantity must be a positive decimal")
		}
		selected, err := objectJSON(value.SelectedAttributes)
		if err != nil {
			return nil, err
		}
		var offerID, variantID *string
		if value.OfferID != nil && *value.OfferID != "" {
			id := string(*value.OfferID)
			offerID = &id
		}
		if value.VariantID != nil && *value.VariantID != "" {
			id := string(*value.VariantID)
			variantID = &id
		}
		lines = append(lines, ports.TransactionLineDraft{ID: newID(), CatalogItemID: string(value.CatalogItemID), OfferID: offerID, VariantID: variantID, Quantity: quantity, SelectedAttributes: selected})
	}
	return lines, nil
}
func transactionReviewView(record ports.TransactionReviewRecord) commands.TransactionReviewView {
	return commands.TransactionReviewView{Required: record.Required, Status: record.Status, ReasonCodes: jsonStrings(record.ReasonCodes), ReviewerReference: valueString(record.ReviewerReference)}
}

var _ commands.CreateLeadHandler = CreateLeadCommandService{}
var _ commands.UpdateLeadHandler = UpdateLeadCommandService{}
var _ commands.QualifyLeadHandler = QualifyLeadCommandService{}
var _ commands.MarkLeadLostHandler = MarkLeadLostCommandService{}
var _ commands.CreateTransactionDraftHandler = CreateTransactionDraftCommandService{}
var _ commands.UpdateTransactionDraftHandler = UpdateTransactionDraftCommandService{}
var _ commands.ConfirmTransactionHandler = ConfirmTransactionCommandService{}
var _ commands.CancelTransactionHandler = CancelTransactionCommandService{}
var _ commands.SubmitTransactionReviewHandler = SubmitTransactionReviewCommandService{}
var _ commands.ApproveTransactionReviewHandler = ApproveTransactionReviewCommandService{}
var _ commands.RejectTransactionReviewHandler = RejectTransactionReviewCommandService{}
