package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type RepositoryErrorKind string

const (
	RepositoryNotFound RepositoryErrorKind = "not_found"
	RepositoryConflict RepositoryErrorKind = "conflict"
	RepositoryStale    RepositoryErrorKind = "stale"
	RepositoryInvalid  RepositoryErrorKind = "invalid"
)

type RepositoryError struct {
	Operation string
	Kind      RepositoryErrorKind
	Err       error
}

func (e *RepositoryError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("repository %s: %s", e.Operation, e.Kind)
	}
	return fmt.Sprintf("repository %s: %s: %v", e.Operation, e.Kind, e.Err)
}
func (e *RepositoryError) Unwrap() error { return e.Err }
func (e *RepositoryError) RepositoryKind() RepositoryErrorKind {
	if e == nil {
		return ""
	}
	return e.Kind
}
func (e *RepositoryError) ErrorKind() string {
	if e == nil {
		return ""
	}
	return string(e.Kind)
}
func IsRepositoryKind(err error, kind RepositoryErrorKind) bool {
	var target *RepositoryError
	return errors.As(err, &target) && target.Kind == kind
}

type BusinessRepository struct{ adapter *Adapter }

func NewBusinessRepository(adapter *Adapter) *BusinessRepository {
	return &BusinessRepository{adapter: adapter}
}

func (r *BusinessRepository) GetByID(ctx context.Context, businessID string) (ports.BusinessRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.BusinessRecord{}, ErrPoolClosed
	}
	if businessID == "" {
		return ports.BusinessRecord{}, &RepositoryError{Operation: "business.get_by_id", Kind: RepositoryInvalid, Err: errors.New("business id is required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.BusinessRecord{}, err
	}
	const query = `SELECT id::text, name, slug, status, vertical_type, timezone, default_currency, locale, resource_version, created_at, updated_at FROM businesses WHERE id = $1::uuid`
	var record ports.BusinessRecord
	if err := executor.QueryRow(ctx, query, businessID).Scan(&record.ID, &record.Name, &record.Slug, &record.Status, &record.VerticalType, &record.Timezone, &record.DefaultCurrency, &record.Locale, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.BusinessRecord{}, &RepositoryError{Operation: "business.get_by_id", Kind: RepositoryNotFound, Err: err}
		}
		return ports.BusinessRecord{}, &RepositoryError{Operation: "business.get_by_id", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *BusinessRepository) UpdateProfile(ctx context.Context, update ports.BusinessProfileUpdate) (ports.BusinessRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.BusinessRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(update.BusinessID) == "" || update.ExpectedVersion <= 0 {
		return ports.BusinessRecord{}, invalidRepositoryInput("business.update_profile", "business id and expected resource version are required")
	}
	if update.Name == nil && update.VerticalType == nil && update.Timezone == nil && update.DefaultCurrency == nil && update.Locale == nil {
		return ports.BusinessRecord{}, invalidRepositoryInput("business.update_profile", "at least one profile field is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.BusinessRecord{}, err
	}
	const query = `UPDATE businesses SET name = COALESCE($3, name), vertical_type = COALESCE($4, vertical_type), timezone = COALESCE($5, timezone), default_currency = COALESCE($6, default_currency), locale = COALESCE($7, locale), resource_version = resource_version + 1, updated_at = $8 WHERE id = $1::uuid AND resource_version = $2 RETURNING id::text, name, slug, status, vertical_type, timezone, default_currency, locale, resource_version, created_at, updated_at`
	var record ports.BusinessRecord
	err = executor.QueryRow(ctx, query, update.BusinessID, update.ExpectedVersion, nilIfBlank(update.Name), nilIfBlank(update.VerticalType), nilIfBlank(update.Timezone), nilIfBlank(update.DefaultCurrency), nilIfBlank(update.Locale), time.Now().UTC()).Scan(&record.ID, &record.Name, &record.Slug, &record.Status, &record.VerticalType, &record.Timezone, &record.DefaultCurrency, &record.Locale, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if checkErr := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM businesses WHERE id = $1::uuid)`, update.BusinessID).Scan(&exists); checkErr != nil {
			return ports.BusinessRecord{}, &RepositoryError{Operation: "business.update_profile", Kind: RepositoryInvalid, Err: checkErr}
		}
		if !exists {
			return ports.BusinessRecord{}, &RepositoryError{Operation: "business.update_profile", Kind: RepositoryNotFound, Err: err}
		}
		return ports.BusinessRecord{}, &RepositoryError{Operation: "business.update_profile", Kind: RepositoryStale, Err: err}
	}
	return ports.BusinessRecord{}, &RepositoryError{Operation: "business.update_profile", Kind: RepositoryInvalid, Err: err}
}

func (r *BusinessRepository) GetRuntimePolicy(ctx context.Context, businessID string) (ports.BusinessRuntimePolicyRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.BusinessRuntimePolicyRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.BusinessRuntimePolicyRecord{}, invalidRepositoryInput("business_policy.get", "business id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.BusinessRuntimePolicyRecord{}, err
	}
	const query = `SELECT business_id::text, ai_mode, default_human_review, allow_auto_reply, allow_auto_lead_creation, allow_auto_transaction_draft, allow_auto_confirmation, resource_version FROM business_policies WHERE business_id = $1::uuid`
	var record ports.BusinessRuntimePolicyRecord
	if err := executor.QueryRow(ctx, query, businessID).Scan(&record.BusinessID, &record.AIMode, &record.DefaultHumanReview, &record.AllowAutoReply, &record.AllowAutoLeadCreation, &record.AllowAutoTransactionDraft, &record.AllowAutoConfirmation, &record.ResourceVersion); err != nil {
		return ports.BusinessRuntimePolicyRecord{}, classifyRepositoryGetError("business_policy.get", err)
	}
	return record, nil
}

func (r *BusinessRepository) UpdateRuntimePolicy(ctx context.Context, update ports.BusinessRuntimePolicyUpdate) (ports.BusinessRuntimePolicyRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.BusinessRuntimePolicyRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(update.BusinessID) == "" || update.ExpectedVersion <= 0 {
		return ports.BusinessRuntimePolicyRecord{}, invalidRepositoryInput("business_policy.update", "business id and expected resource version are required")
	}
	if update.AIMode == nil && update.DefaultHumanReview == nil && update.AllowAutoReply == nil && update.AllowAutoLeadCreation == nil && update.AllowAutoTransactionDraft == nil && update.AllowAutoConfirmation == nil {
		return ports.BusinessRuntimePolicyRecord{}, invalidRepositoryInput("business_policy.update", "at least one policy field is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.BusinessRuntimePolicyRecord{}, err
	}
	const query = `UPDATE business_policies SET ai_mode = COALESCE($3, ai_mode), default_human_review = COALESCE($4, default_human_review), allow_auto_reply = COALESCE($5, allow_auto_reply), allow_auto_lead_creation = COALESCE($6, allow_auto_lead_creation), allow_auto_transaction_draft = COALESCE($7, allow_auto_transaction_draft), allow_auto_confirmation = COALESCE($8, allow_auto_confirmation), resource_version = resource_version + 1, updated_at = $9 WHERE business_id = $1::uuid AND resource_version = $2 RETURNING business_id::text, ai_mode, default_human_review, allow_auto_reply, allow_auto_lead_creation, allow_auto_transaction_draft, allow_auto_confirmation, resource_version`
	var record ports.BusinessRuntimePolicyRecord
	err = executor.QueryRow(ctx, query, update.BusinessID, update.ExpectedVersion, nilIfBlank(update.AIMode), update.DefaultHumanReview, update.AllowAutoReply, update.AllowAutoLeadCreation, update.AllowAutoTransactionDraft, update.AllowAutoConfirmation, time.Now().UTC()).Scan(&record.BusinessID, &record.AIMode, &record.DefaultHumanReview, &record.AllowAutoReply, &record.AllowAutoLeadCreation, &record.AllowAutoTransactionDraft, &record.AllowAutoConfirmation, &record.ResourceVersion)
	if err == nil {
		return record, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if checkErr := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM business_policies WHERE business_id = $1::uuid)`, update.BusinessID).Scan(&exists); checkErr != nil {
			return ports.BusinessRuntimePolicyRecord{}, &RepositoryError{Operation: "business_policy.update", Kind: RepositoryInvalid, Err: checkErr}
		}
		if !exists {
			return ports.BusinessRuntimePolicyRecord{}, &RepositoryError{Operation: "business_policy.update", Kind: RepositoryNotFound, Err: err}
		}
		return ports.BusinessRuntimePolicyRecord{}, &RepositoryError{Operation: "business_policy.update", Kind: RepositoryStale, Err: err}
	}
	return ports.BusinessRuntimePolicyRecord{}, &RepositoryError{Operation: "business_policy.update", Kind: RepositoryInvalid, Err: err}
}

func nilIfBlank(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

var _ ports.BusinessRepository = (*BusinessRepository)(nil)
var _ ports.BusinessManagementRepository = (*BusinessRepository)(nil)
