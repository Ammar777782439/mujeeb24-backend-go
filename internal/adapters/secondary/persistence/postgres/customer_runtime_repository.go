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

type customerCursor struct {
	UpdatedAt time.Time
	ID        string
}

func (r *CustomerRepository) List(ctx context.Context, businessID, search, status string, limit int, cursor string) (ports.CustomerPage, error) {
	if r == nil || r.adapter == nil {
		return ports.CustomerPage{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.CustomerPage{}, invalidRepositoryInput("customer.list", "business id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return ports.CustomerPage{}, invalidRepositoryInput("customer.list", "limit must not exceed 100")
	}
	decoded, err := decodeCustomerCursor(cursor)
	if err != nil {
		return ports.CustomerPage{}, invalidRepositoryInput("customer.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CustomerPage{}, err
	}
	var cursorUpdatedAt any
	var cursorID any
	if decoded != nil {
		cursorUpdatedAt, cursorID = decoded.UpdatedAt, decoded.ID
	}
	const query = `SELECT id::text, business_id::text, profile, contact_points, locale_preference, status, merged_into_customer_id::text, resource_version, updated_at FROM customers WHERE business_id = $1::uuid AND ($2 = '' OR status = $2) AND ($3 = '' OR profile::text ILIKE '%' || $3 || '%' OR contact_points::text ILIKE '%' || $3 || '%') AND ($4::timestamptz IS NULL OR (updated_at, id) < ($4::timestamptz, $5::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $6`
	rows, err := executor.Query(ctx, query, businessID, strings.TrimSpace(status), strings.TrimSpace(search), cursorUpdatedAt, cursorID, limit+1)
	if err != nil {
		return ports.CustomerPage{}, &RepositoryError{Operation: "customer.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.CustomerRecord, 0, limit)
	for rows.Next() {
		var record ports.CustomerRecord
		if err := rows.Scan(&record.ID, &record.BusinessID, &record.Profile, &record.ContactPoints, &record.LocalePreference, &record.Status, &record.MergedIntoCustomer, &record.ResourceVersion, &record.UpdatedAt); err != nil {
			return ports.CustomerPage{}, &RepositoryError{Operation: "customer.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.CustomerPage{}, &RepositoryError{Operation: "customer.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.CustomerPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeCustomerCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *CustomerRepository) Create(ctx context.Context, create ports.CustomerCreate) (ports.CustomerRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CustomerRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.BusinessID) == "" {
		return ports.CustomerRecord{}, invalidRepositoryInput("customer.create", "id and business are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CustomerRecord{}, err
	}
	const query = `INSERT INTO customers (id,business_id,profile,contact_points,locale_preference,status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::jsonb,$4::jsonb,$5,'active',now(),now()) RETURNING id::text,business_id::text,profile,contact_points,locale_preference,status,merged_into_customer_id::text,resource_version,updated_at`
	var record ports.CustomerRecord
	if err := executor.QueryRow(ctx, query, create.ID, create.BusinessID, jsonObjectOrDefault(create.Profile), jsonArrayOrDefault(create.ContactPoints), create.LocalePreference).Scan(&record.ID, &record.BusinessID, &record.Profile, &record.ContactPoints, &record.LocalePreference, &record.Status, &record.MergedIntoCustomer, &record.ResourceVersion, &record.UpdatedAt); err != nil {
		return ports.CustomerRecord{}, classifyRepositoryWriteError("customer.create", err)
	}
	return record, nil
}

func (r *CustomerRepository) Update(ctx context.Context, update ports.CustomerUpdate) (ports.CustomerRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CustomerRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(update.BusinessID) == "" || strings.TrimSpace(update.CustomerID) == "" || update.ExpectedVersion <= 0 {
		return ports.CustomerRecord{}, invalidRepositoryInput("customer.update", "business, customer, and expected resource version are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CustomerRecord{}, err
	}
	const query = `UPDATE customers SET profile=COALESCE($4::jsonb,profile),contact_points=COALESCE($5::jsonb,contact_points),locale_preference=COALESCE($6,locale_preference),resource_version=resource_version+1,updated_at=now() WHERE business_id=$1::uuid AND id=$2::uuid AND resource_version=$3 AND status='active' RETURNING id::text,business_id::text,profile,contact_points,locale_preference,status,merged_into_customer_id::text,resource_version,updated_at`
	var record ports.CustomerRecord
	err = executor.QueryRow(ctx, query, update.BusinessID, update.CustomerID, update.ExpectedVersion, nilIfEmptyJSON(update.Profile), nilIfEmptyJSON(update.ContactPoints), update.LocalePreference).Scan(&record.ID, &record.BusinessID, &record.Profile, &record.ContactPoints, &record.LocalePreference, &record.Status, &record.MergedIntoCustomer, &record.ResourceVersion, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return ports.CustomerRecord{}, classifyCoreStaleOrNotFound(ctx, executor, "customer.update", "customers", update.BusinessID, update.CustomerID, err)
}

func (r *CustomerRepository) Merge(ctx context.Context, merge ports.CustomerMerge) (ports.CustomerRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CustomerRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(merge.BusinessID) == "" || strings.TrimSpace(merge.CustomerID) == "" || strings.TrimSpace(merge.TargetCustomerID) == "" || merge.CustomerID == merge.TargetCustomerID || merge.ExpectedVersion <= 0 || strings.TrimSpace(merge.Reason) == "" {
		return ports.CustomerRecord{}, invalidRepositoryInput("customer.merge", "business, distinct source/target, reason, and expected resource version are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CustomerRecord{}, err
	}
	const query = `UPDATE customers AS source SET status='merged',merged_into_customer_id=$4::uuid,resource_version=resource_version+1,updated_at=now() WHERE source.business_id=$1::uuid AND source.id=$2::uuid AND source.resource_version=$3 AND source.status='active' AND EXISTS (SELECT 1 FROM customers AS target WHERE target.business_id=source.business_id AND target.id=$4::uuid AND target.status='active') RETURNING source.id::text,source.business_id::text,source.profile,source.contact_points,source.locale_preference,source.status,source.merged_into_customer_id::text,source.resource_version,source.updated_at`
	var record ports.CustomerRecord
	err = executor.QueryRow(ctx, query, merge.BusinessID, merge.CustomerID, merge.ExpectedVersion, merge.TargetCustomerID).Scan(&record.ID, &record.BusinessID, &record.Profile, &record.ContactPoints, &record.LocalePreference, &record.Status, &record.MergedIntoCustomer, &record.ResourceVersion, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return ports.CustomerRecord{}, classifyCoreStaleOrNotFound(ctx, executor, "customer.merge", "customers", merge.BusinessID, merge.CustomerID, err)
}

func nilIfEmptyJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func jsonObjectOrDefault(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`{}`)
	}
	return value
}

func jsonArrayOrDefault(value []byte) []byte {
	if len(value) == 0 {
		return []byte(`[]`)
	}
	return value
}

func encodeCustomerCursor(record ports.CustomerRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}
func decodeCustomerCursor(value string) (*customerCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid customer cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid customer cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid customer cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, errors.New("invalid customer cursor")
	}
	return &customerCursor{UpdatedAt: updatedAt, ID: parts[1]}, nil
}
func classifyCoreStaleOrNotFound(ctx context.Context, executor SQLExecutor, operation, table, businessID, id string, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
	}
	var exists bool
	if checkErr := executor.QueryRow(ctx, fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM %s WHERE business_id=$1::uuid AND id=$2::uuid)", table), businessID, id).Scan(&exists); checkErr != nil {
		return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: checkErr}
	}
	if !exists {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryStale, Err: err}
}

var _ ports.CustomerRuntimeRepository = (*CustomerRepository)(nil)
