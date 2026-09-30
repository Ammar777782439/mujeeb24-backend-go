package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PaymentRepository implements ports.PaymentRepository against Postgres.
//
// Per Platform Administration Contract V1 §27: Payment Record is APPEND ONLY.
// There is intentionally NO Update / Delete method on this port — corrections
// go through a new documented adjustment PaymentRecord, never silent edits.
type PaymentRepository struct {
	adapter *Adapter
}

func NewPaymentRepository(adapter *Adapter) *PaymentRepository {
	return &PaymentRepository{adapter: adapter}
}

const paymentSelectColumns = `id::text, subscription_id::text, business_id::text, amount_yer, method, reference, paid_at, recorded_by::text, created_at`

func scanPayment(scanner interface {
	Scan(dest ...any) error
}, record *ports.PaymentRecord) error {
	return scanner.Scan(
		&record.ID, &record.SubscriptionID, &record.BusinessID,
		&record.AmountYER, &record.Method, &record.Reference,
		&record.PaidAt, &record.RecordedBy, &record.CreatedAt,
	)
}

func (r *PaymentRepository) Append(ctx context.Context, create ports.PaymentCreate) (ports.PaymentRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PaymentRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.SubscriptionID) == "" || strings.TrimSpace(create.BusinessID) == "" {
		return ports.PaymentRecord{}, invalidRepositoryInput("payment.append", "subscription_id and business_id are required")
	}
	if create.AmountYER <= 0 {
		return ports.PaymentRecord{}, invalidRepositoryInput("payment.append", "amount_yer must be positive")
	}
	method := strings.TrimSpace(strings.ToUpper(create.Method))
	switch method {
	case "CASH", "BANK_TRANSFER", "MOBILE_MONEY", "OTHER":
		// ok
	default:
		return ports.PaymentRecord{}, invalidRepositoryInput("payment.append", "method must be one of CASH / BANK_TRANSFER / MOBILE_MONEY / OTHER")
	}
	if strings.TrimSpace(create.Reference) == "" {
		return ports.PaymentRecord{}, invalidRepositoryInput("payment.append", "reference is required (per Contract §27)")
	}
	if strings.TrimSpace(create.RecordedBy) == "" {
		return ports.PaymentRecord{}, invalidRepositoryInput("payment.append", "recorded_by is required")
	}
	if create.PaidAt.IsZero() {
		create.PaidAt = time.Now().UTC()
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PaymentRecord{}, err
	}
	paymentID := uuid.NewString()
	var record ports.PaymentRecord
	err = executor.QueryRow(ctx,
		`INSERT INTO subscription_payments (id, subscription_id, business_id, amount_yer, method, reference, paid_at, recorded_by, created_at)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8::uuid, $9)
		 RETURNING `+paymentSelectColumns,
		paymentID, create.SubscriptionID, create.BusinessID,
		create.AmountYER, method, create.Reference,
		create.PaidAt, create.RecordedBy, create.Now,
	).Scan(
		&record.ID, &record.SubscriptionID, &record.BusinessID,
		&record.AmountYER, &record.Method, &record.Reference,
		&record.PaidAt, &record.RecordedBy, &record.CreatedAt,
	)
	if err != nil {
		return ports.PaymentRecord{}, classifyRepositoryWriteError("payment.append", err)
	}
	return record, nil
}

func (r *PaymentRepository) List(ctx context.Context, filter ports.PaymentListFilter) (ports.PaymentPage, error) {
	if r == nil || r.adapter == nil {
		return ports.PaymentPage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	decoded, err := decodePaymentCursor(filter.Cursor)
	if err != nil {
		return ports.PaymentPage{}, invalidRepositoryInput("payment.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PaymentPage{}, err
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.CreatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx,
		`SELECT `+paymentSelectColumns+`
		 FROM subscription_payments
		 WHERE ($1 = '' OR subscription_id::text = $1)
		   AND ($2 = '' OR business_id::text = $2)
		   AND ($3 = '' OR method = $3)
		   AND ($4::timestamptz IS NULL OR (created_at, id) < ($4, $5::uuid))
		 ORDER BY created_at DESC, id DESC
		 LIMIT $6`,
		strings.TrimSpace(filter.SubscriptionID), strings.TrimSpace(filter.BusinessID), strings.TrimSpace(filter.Method),
		cursorAt, cursorID, filter.Limit+1,
	)
	if err != nil {
		return ports.PaymentPage{}, &RepositoryError{Operation: "payment.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.PaymentRecord, 0, filter.Limit)
	for rows.Next() {
		var record ports.PaymentRecord
		if err := scanPayment(rows, &record); err != nil {
			return ports.PaymentPage{}, &RepositoryError{Operation: "payment.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.PaymentPage{}, &RepositoryError{Operation: "payment.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.PaymentPage{Items: items}
	if len(items) > filter.Limit {
		page.HasMore = true
		page.Items = items[:filter.Limit]
		page.NextCursor = encodePaymentCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *PaymentRepository) GetByID(ctx context.Context, paymentID string) (ports.PaymentRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PaymentRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(paymentID) == "" {
		return ports.PaymentRecord{}, invalidRepositoryInput("payment.get", "payment id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PaymentRecord{}, err
	}
	var record ports.PaymentRecord
	err = executor.QueryRow(ctx,
		`SELECT `+paymentSelectColumns+` FROM subscription_payments WHERE id = $1::uuid`,
		paymentID,
	).Scan(
		&record.ID, &record.SubscriptionID, &record.BusinessID,
		&record.AmountYER, &record.Method, &record.Reference,
		&record.PaidAt, &record.RecordedBy, &record.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PaymentRecord{}, &RepositoryError{Operation: "payment.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.PaymentRecord{}, &RepositoryError{Operation: "payment.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

type paymentCursor struct {
	CreatedAt time.Time
	ID        string
}

func encodePaymentCursor(record ports.PaymentRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodePaymentCursor(value string) (*paymentCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid payment cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid payment cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid payment cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, fmt.Errorf("invalid payment cursor")
	}
	return &paymentCursor{CreatedAt: createdAt, ID: parts[1]}, nil
}

var _ ports.PaymentRepository = (*PaymentRepository)(nil)
