package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type TransactionRepository struct{ adapter *Adapter }

func NewTransactionRepository(adapter *Adapter) *TransactionRepository {
	return &TransactionRepository{adapter: adapter}
}

const transactionSelect = `SELECT id::text, business_id::text, customer_id::text, lead_id::text, transaction_type, state, source_conversation_reference_id::text, currency, total_amount::text, schema_version, requires_human_review, cancellation_reason, resource_version, created_at, updated_at FROM commercial_transactions`

func (r *TransactionRepository) List(ctx context.Context, businessID, state, transactionType, customerID string, limit int, cursor string) (ports.TransactionPage, error) {
	executor, err := r.salesExecutor(ctx, "transaction.list")
	if err != nil {
		return ports.TransactionPage{}, err
	}
	limit, decoded, err := salesPageArgs(limit, cursor, "transaction.list")
	if err != nil {
		return ports.TransactionPage{}, err
	}
	var at any
	var id any
	if decoded != nil {
		at, id = decoded.At, decoded.ID
	}
	rows, err := executor.Query(ctx, transactionSelect+` WHERE business_id = $1::uuid AND ($2 = '' OR state = $2) AND ($3 = '' OR transaction_type = $3) AND ($4 = '' OR customer_id = $4::uuid) AND ($5::timestamptz IS NULL OR (updated_at, id) < ($5::timestamptz, $6::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $7`, businessID, state, transactionType, customerID, at, id, limit+1)
	if err != nil {
		return ports.TransactionPage{}, salesRepositoryError("transaction.list", err)
	}
	defer rows.Close()
	items := make([]ports.TransactionRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanTransaction(rows)
		if scanErr != nil {
			return ports.TransactionPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.TransactionPage{}, salesRepositoryError("transaction.list", err)
	}
	page := ports.TransactionPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeSalesCursor(last.UpdatedAt, last.ID)
	}
	return page, nil
}

func (r *TransactionRepository) Get(ctx context.Context, businessID, transactionID string) (ports.TransactionRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.get")
	if err != nil {
		return ports.TransactionRecord{}, err
	}
	if businessID == "" || transactionID == "" {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.get", "business and transaction ids are required")
	}
	return scanTransaction(executor.QueryRow(ctx, transactionSelect+` WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, transactionID))
}

func (r *TransactionRepository) GetReview(ctx context.Context, businessID, transactionID string) (ports.TransactionReviewRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.review.get")
	if err != nil {
		return ports.TransactionReviewRecord{}, err
	}
	if err := ensureSalesParent(ctx, executor, "transaction.review.get", `SELECT EXISTS (SELECT 1 FROM commercial_transactions WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, transactionID); err != nil {
		return ports.TransactionReviewRecord{}, err
	}
	var item ports.TransactionReviewRecord
	err = executor.QueryRow(ctx, `SELECT id::text, business_id::text, transaction_id::text, required, status, reason_codes, reviewer_reference, decision_reason, decided_at, created_at, updated_at FROM transaction_reviews WHERE business_id = $1::uuid AND transaction_id = $2::uuid`, businessID, transactionID).Scan(&item.ID, &item.BusinessID, &item.TransactionID, &item.Required, &item.Status, &item.ReasonCodes, &item.ReviewerReference, &item.DecisionReason, &item.DecidedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, classifyRepositoryGetError("transaction.review.get", err)
	}
	return item, nil
}

func (r *TransactionRepository) CreateDraft(ctx context.Context, draft ports.TransactionDraft) (ports.TransactionRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.create")
	if err != nil {
		return ports.TransactionRecord{}, err
	}
	if draft.ID == "" || draft.BusinessID == "" || draft.CustomerID == "" || draft.TransactionType == "" || draft.SchemaVersion <= 0 || draft.CreatedAt.IsZero() || draft.UpdatedAt.IsZero() {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.create", "required transaction fields are missing")
	}
	if len(draft.Lines) == 0 {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.create", "at least one transaction line is required")
	}
	record, err := scanTransaction(executor.QueryRow(ctx, `INSERT INTO commercial_transactions (id, business_id, customer_id, lead_id, transaction_type, state, currency, schema_version, requires_human_review, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, 'draft', $6, $7, $8, $9, $9) RETURNING id::text, business_id::text, customer_id::text, lead_id::text, transaction_type, state, source_conversation_reference_id::text, currency, total_amount::text, schema_version, requires_human_review, cancellation_reason, resource_version, created_at, updated_at`, draft.ID, draft.BusinessID, draft.CustomerID, draft.LeadID, draft.TransactionType, draft.Currency, draft.SchemaVersion, draft.RequiresHumanReview, draft.CreatedAt))
	if err != nil {
		return ports.TransactionRecord{}, classifyRepositoryWriteError("transaction.create", err)
	}
	if err := r.insertTransactionLines(ctx, executor, draft.BusinessID, draft.ID, draft.Lines, draft.CreatedAt); err != nil {
		return ports.TransactionRecord{}, err
	}
	return record, nil
}

func (r *TransactionRepository) UpdateDraft(ctx context.Context, patch ports.TransactionPatch) (ports.TransactionRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.update")
	if err != nil {
		return ports.TransactionRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || patch.ExpectedVersion <= 0 || patch.UpdatedAt.IsZero() {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.update", "id, business, updated_at, and positive expected version are required")
	}
	if len(patch.Lines) == 0 {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.update", "at least one transaction line is required")
	}
	record, err := scanTransaction(executor.QueryRow(ctx, `UPDATE commercial_transactions SET currency = COALESCE($3, currency), resource_version = resource_version + 1, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid AND state = 'draft' AND resource_version = $5 RETURNING id::text, business_id::text, customer_id::text, lead_id::text, transaction_type, state, source_conversation_reference_id::text, currency, total_amount::text, schema_version, requires_human_review, cancellation_reason, resource_version, created_at, updated_at`, patch.BusinessID, patch.ID, patch.Currency, patch.UpdatedAt, patch.ExpectedVersion))
	if err != nil {
		return ports.TransactionRecord{}, classifyTransactionMutationMiss(ctx, executor, "transaction.update", patch.BusinessID, patch.ID, patch.ExpectedVersion, err, "draft can no longer be edited")
	}
	if _, err := executor.Exec(ctx, `DELETE FROM order_lines WHERE business_id = $1::uuid AND transaction_id = $2::uuid`, patch.BusinessID, patch.ID); err != nil {
		return ports.TransactionRecord{}, salesRepositoryError("transaction.update.lines.delete", err)
	}
	if err := r.insertTransactionLines(ctx, executor, patch.BusinessID, patch.ID, patch.Lines, patch.UpdatedAt); err != nil {
		return ports.TransactionRecord{}, err
	}
	return record, nil
}

func (r *TransactionRepository) Confirm(ctx context.Context, patch ports.TransactionConfirmPatch) (ports.TransactionRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.confirm")
	if err != nil {
		return ports.TransactionRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || strings.TrimSpace(patch.EvidenceReference) == "" || patch.ExpectedVersion <= 0 || patch.UpdatedAt.IsZero() {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.confirm", "id, business, evidence, updated_at, and positive expected version are required")
	}
	record, err := scanTransaction(executor.QueryRow(ctx, `UPDATE commercial_transactions SET state = 'confirmed', resource_version = resource_version + 1, updated_at = $3 WHERE business_id = $1::uuid AND id = $2::uuid AND state IN ('draft', 'awaiting_confirmation') AND resource_version = $4 RETURNING id::text, business_id::text, customer_id::text, lead_id::text, transaction_type, state, source_conversation_reference_id::text, currency, total_amount::text, schema_version, requires_human_review, cancellation_reason, resource_version, created_at, updated_at`, patch.BusinessID, patch.ID, patch.UpdatedAt, patch.ExpectedVersion))
	if err != nil {
		return ports.TransactionRecord{}, classifyTransactionMutationMiss(ctx, executor, "transaction.confirm", patch.BusinessID, patch.ID, patch.ExpectedVersion, err, "transaction cannot be confirmed from its current state")
	}
	if _, err := executor.Exec(ctx, `INSERT INTO transaction_confirmations (id, business_id, transaction_id, status, confirmed_by, confirmed_at, evidence_reference, policy_version, created_at, updated_at) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, 'confirmed', 'human_agent', $3, $4, NULLIF($5, ''), $3, $3) ON CONFLICT (business_id, transaction_id) DO UPDATE SET status = EXCLUDED.status, confirmed_by = EXCLUDED.confirmed_by, confirmed_at = EXCLUDED.confirmed_at, evidence_reference = EXCLUDED.evidence_reference, policy_version = EXCLUDED.policy_version, updated_at = EXCLUDED.updated_at`, patch.BusinessID, patch.ID, patch.UpdatedAt, patch.EvidenceReference, patch.PolicyVersion); err != nil {
		return ports.TransactionRecord{}, salesRepositoryError("transaction.confirm.confirmation", err)
	}
	return record, nil
}

func (r *TransactionRepository) Cancel(ctx context.Context, patch ports.TransactionCancelPatch) (ports.TransactionRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.cancel")
	if err != nil {
		return ports.TransactionRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || strings.TrimSpace(patch.Reason) == "" || patch.ExpectedVersion <= 0 || patch.UpdatedAt.IsZero() {
		return ports.TransactionRecord{}, invalidRepositoryInput("transaction.cancel", "id, business, reason, updated_at, and positive expected version are required")
	}
	record, err := scanTransaction(executor.QueryRow(ctx, `UPDATE commercial_transactions SET state = 'cancelled', cancellation_reason = $3, resource_version = resource_version + 1, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid AND state NOT IN ('completed', 'cancelled', 'expired') AND resource_version = $5 RETURNING id::text, business_id::text, customer_id::text, lead_id::text, transaction_type, state, source_conversation_reference_id::text, currency, total_amount::text, schema_version, requires_human_review, cancellation_reason, resource_version, created_at, updated_at`, patch.BusinessID, patch.ID, patch.Reason, patch.UpdatedAt, patch.ExpectedVersion))
	if err != nil {
		return ports.TransactionRecord{}, classifyTransactionMutationMiss(ctx, executor, "transaction.cancel", patch.BusinessID, patch.ID, patch.ExpectedVersion, err, "transaction cannot be cancelled from its current state")
	}
	return record, nil
}

func (r *TransactionRepository) SubmitReview(ctx context.Context, patch ports.TransactionReviewPatch) (ports.TransactionReviewRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.review.submit")
	if err != nil {
		return ports.TransactionReviewRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || patch.ExpectedVersion <= 0 || patch.UpdatedAt.IsZero() {
		return ports.TransactionReviewRecord{}, invalidRepositoryInput("transaction.review.submit", "id, business, updated_at, and positive expected version are required")
	}
	if err := r.bumpTransactionForReview(ctx, executor, patch.ID, patch.BusinessID, patch.ExpectedVersion, patch.UpdatedAt); err != nil {
		return ports.TransactionReviewRecord{}, err
	}
	var item ports.TransactionReviewRecord
	err = executor.QueryRow(ctx, `INSERT INTO transaction_reviews (id, business_id, transaction_id, required, status, reason_codes, created_at, updated_at) VALUES (gen_random_uuid(), $1::uuid, $2::uuid, TRUE, 'pending', $3::jsonb, $4, $4) ON CONFLICT (business_id, transaction_id) DO UPDATE SET required = TRUE, status = 'pending', reason_codes = EXCLUDED.reason_codes, reviewer_reference = NULL, decision_reason = NULL, decided_at = NULL, updated_at = EXCLUDED.updated_at RETURNING id::text, business_id::text, transaction_id::text, required, status, reason_codes, reviewer_reference, decision_reason, decided_at, created_at, updated_at`, patch.BusinessID, patch.ID, patch.ReasonCodes, patch.UpdatedAt).Scan(&item.ID, &item.BusinessID, &item.TransactionID, &item.Required, &item.Status, &item.ReasonCodes, &item.ReviewerReference, &item.DecisionReason, &item.DecidedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return ports.TransactionReviewRecord{}, salesRepositoryError("transaction.review.submit", err)
	}
	return item, nil
}

func (r *TransactionRepository) DecideReview(ctx context.Context, patch ports.TransactionReviewDecisionPatch) (ports.TransactionReviewRecord, error) {
	executor, err := r.salesExecutor(ctx, "transaction.review.decide")
	if err != nil {
		return ports.TransactionReviewRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || strings.TrimSpace(patch.ReviewerReference) == "" || patch.ExpectedVersion <= 0 || patch.UpdatedAt.IsZero() {
		return ports.TransactionReviewRecord{}, invalidRepositoryInput("transaction.review.decide", "id, business, reviewer, updated_at, and positive expected version are required")
	}
	if err := r.bumpTransactionForReview(ctx, executor, patch.ID, patch.BusinessID, patch.ExpectedVersion, patch.UpdatedAt); err != nil {
		return ports.TransactionReviewRecord{}, err
	}
	status := "rejected"
	if patch.Approved {
		status = "approved"
	}
	var item ports.TransactionReviewRecord
	err = executor.QueryRow(ctx, `UPDATE transaction_reviews SET status = $3, reviewer_reference = $4, decision_reason = NULLIF($6, ''), decided_at = $5, updated_at = $5 WHERE business_id = $1::uuid AND transaction_id = $2::uuid AND required = TRUE AND status = 'pending' RETURNING id::text, business_id::text, transaction_id::text, required, status, reason_codes, reviewer_reference, decision_reason, decided_at, created_at, updated_at`, patch.BusinessID, patch.ID, status, patch.ReviewerReference, patch.UpdatedAt, patch.Reason).Scan(&item.ID, &item.BusinessID, &item.TransactionID, &item.Required, &item.Status, &item.ReasonCodes, &item.ReviewerReference, &item.DecisionReason, &item.DecidedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.TransactionReviewRecord{}, &RepositoryError{Operation: "transaction.review.decide", Kind: RepositoryConflict, Err: errors.New("review is not pending")}
		}
		return ports.TransactionReviewRecord{}, salesRepositoryError("transaction.review.decide", err)
	}
	return item, nil
}

func (r *TransactionRepository) insertTransactionLines(ctx context.Context, executor SQLExecutor, businessID, transactionID string, lines []ports.TransactionLineDraft, at time.Time) error {
	for _, line := range lines {
		if line.ID == "" || line.CatalogItemID == "" || strings.TrimSpace(line.Quantity) == "" {
			return invalidRepositoryInput("transaction.lines.insert", "line id, catalog item, and quantity are required")
		}
		selected := line.SelectedAttributes
		if len(selected) == 0 {
			selected = []byte(`{}`)
		}
		result, err := executor.Exec(ctx, `INSERT INTO order_lines (id, business_id, transaction_id, catalog_item_id, offer_id, variant_id, item_name_snapshot, selected_attributes_snapshot, pricing_snapshot, availability_snapshot, fulfillment_snapshot, quantity, unit_price_snapshot, line_total_snapshot, currency, created_at, updated_at) SELECT $1::uuid, $2::uuid, $3::uuid, ci.id, $5::uuid, $6::uuid, ci.name, $7::jsonb, jsonb_build_object('pricing_mode', o.pricing_mode, 'amount', o.amount::text, 'currency', o.currency), jsonb_build_object('availability_mode', o.availability_mode, 'status', o.availability_status), jsonb_build_object('fulfillment_mode', o.fulfillment_mode), $8::numeric, o.amount, CASE WHEN o.amount IS NULL THEN NULL ELSE o.amount * $8::numeric END, o.currency, $9, $9 FROM catalog_items ci LEFT JOIN offers o ON o.business_id = ci.business_id AND o.catalog_item_id = ci.id AND ($5::uuid IS NULL OR o.id = $5::uuid) WHERE ci.business_id = $2::uuid AND ci.id = $4::uuid AND ($5::uuid IS NULL OR o.id IS NOT NULL)`, line.ID, businessID, transactionID, line.CatalogItemID, line.OfferID, line.VariantID, selected, line.Quantity, at)
		if err != nil {
			return salesRepositoryError("transaction.lines.insert", err)
		}
		if result.RowsAffected() == 0 {
			return &RepositoryError{Operation: "transaction.lines.insert", Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
		}
	}
	return nil
}

func (r *TransactionRepository) bumpTransactionForReview(ctx context.Context, executor SQLExecutor, transactionID, businessID string, expectedVersion int64, updatedAt time.Time) error {
	var ignored string
	err := executor.QueryRow(ctx, `UPDATE commercial_transactions SET requires_human_review = TRUE, resource_version = resource_version + 1, updated_at = $3 WHERE business_id = $1::uuid AND id = $2::uuid AND resource_version = $4 AND state NOT IN ('completed', 'cancelled', 'expired') RETURNING id::text`, businessID, transactionID, updatedAt, expectedVersion).Scan(&ignored)
	if err == nil {
		return nil
	}
	return classifyTransactionMutationMiss(ctx, executor, "transaction.review.submit", businessID, transactionID, expectedVersion, err, "transaction cannot enter review")
}

func scanTransaction(row interface{ Scan(...any) error }) (ports.TransactionRecord, error) {
	var item ports.TransactionRecord
	err := row.Scan(&item.ID, &item.BusinessID, &item.CustomerID, &item.LeadID, &item.TransactionType, &item.State, &item.SourceConversationReferenceID, &item.Currency, &item.TotalAmount, &item.SchemaVersion, &item.RequiresHumanReview, &item.CancellationReason, &item.ResourceVersion, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		return item, nil
	}
	return item, classifyRepositoryGetError("transaction", err)
}

func classifyTransactionMutationMiss(ctx context.Context, executor SQLExecutor, operation, businessID, transactionID string, expected int64, original error, conflictMessage string) error {
	if !errors.Is(original, pgx.ErrNoRows) {
		return classifyRepositoryWriteError(operation, original)
	}
	var currentVersion int64
	var state string
	if err := executor.QueryRow(ctx, `SELECT resource_version, state FROM commercial_transactions WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, transactionID).Scan(&currentVersion, &state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return classifyRepositoryWriteError(operation, err)
	}
	if currentVersion != expected {
		return &RepositoryError{Operation: operation, Kind: RepositoryStale, Err: fmt.Errorf("expected version %d, current version %d", expected, currentVersion)}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: errors.New(conflictMessage + ": " + state)}
}

var _ ports.TransactionRepository = (*TransactionRepository)(nil)

func (r *TransactionRepository) salesExecutor(ctx context.Context, operation string) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}
