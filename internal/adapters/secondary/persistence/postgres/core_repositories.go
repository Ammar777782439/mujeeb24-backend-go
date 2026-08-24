package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type CustomerRepository struct{ adapter *Adapter }

func NewCustomerRepository(adapter *Adapter) *CustomerRepository {
	return &CustomerRepository{adapter: adapter}
}
func (r *CustomerRepository) GetByID(ctx context.Context, businessID, customerID string) (ports.CustomerRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CustomerRecord{}, ErrPoolClosed
	}
	if businessID == "" || customerID == "" {
		return ports.CustomerRecord{}, invalidRepositoryInput("customer.get_by_id", "business and customer ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CustomerRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, profile, contact_points, locale_preference, status, merged_into_customer_id::text FROM customers WHERE business_id = $1::uuid AND id = $2::uuid`
	var record ports.CustomerRecord
	if err := executor.QueryRow(ctx, query, businessID, customerID).Scan(&record.ID, &record.BusinessID, &record.Profile, &record.ContactPoints, &record.LocalePreference, &record.Status, &record.MergedIntoCustomer); err != nil {
		return record, classifyRepositoryGetError("customer.get_by_id", err)
	}
	return record, nil
}

var _ ports.CustomerRepository = (*CustomerRepository)(nil)

type ConversationRepository struct{ adapter *Adapter }

func NewConversationRepository(adapter *Adapter) *ConversationRepository {
	return &ConversationRepository{adapter: adapter}
}
func (r *ConversationRepository) GetByID(ctx context.Context, businessID, conversationID string) (ports.ConversationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationRecord{}, ErrPoolClosed
	}
	if businessID == "" || conversationID == "" {
		return ports.ConversationRecord{}, invalidRepositoryInput("conversation.get_by_id", "business and conversation ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, customer_id::text, state, ownership, ai_mode_override, priority, assignment_reference FROM conversations WHERE business_id = $1::uuid AND id = $2::uuid`
	var record ports.ConversationRecord
	if err := executor.QueryRow(ctx, query, businessID, conversationID).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.State, &record.Ownership, &record.AIModeOverride, &record.Priority, &record.AssignmentReference); err != nil {
		return record, classifyRepositoryGetError("conversation.get_by_id", err)
	}
	return record, nil
}

var _ ports.ConversationRepository = (*ConversationRepository)(nil)

type ChannelConnectionRepository struct{ adapter *Adapter }

func NewChannelConnectionRepository(adapter *Adapter) *ChannelConnectionRepository {
	return &ChannelConnectionRepository{adapter: adapter}
}
func (r *ChannelConnectionRepository) GetByID(ctx context.Context, businessID, connectionID string) (ports.ChannelConnectionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelConnectionRecord{}, ErrPoolClosed
	}
	if businessID == "" || connectionID == "" {
		return ports.ChannelConnectionRecord{}, invalidRepositoryInput("channel_connection.get_by_id", "business and connection ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelConnectionRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference FROM channel_connections WHERE business_id = $1::uuid AND id = $2::uuid`
	var record ports.ChannelConnectionRecord
	if err := executor.QueryRow(ctx, query, businessID, connectionID).Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference); err != nil {
		return record, classifyRepositoryGetError("channel_connection.get_by_id", err)
	}
	return record, nil
}

func (r *ChannelConnectionRepository) GetByProviderReferences(ctx context.Context, providerReference, providerAccountReference, providerConnectionReference string) (ports.ChannelConnectionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelConnectionRecord{}, ErrPoolClosed
	}
	providerReference = strings.TrimSpace(providerReference)
	providerAccountReference = strings.TrimSpace(providerAccountReference)
	providerConnectionReference = strings.TrimSpace(providerConnectionReference)
	if providerReference == "" || (providerAccountReference == "" && providerConnectionReference == "") {
		return ports.ChannelConnectionRecord{}, invalidRepositoryInput("channel_connection.get_by_provider_references", "provider and account or connection reference are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelConnectionRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference FROM channel_connections WHERE provider_ref = $1 AND (($2 <> '' AND provider_account_ref = $2) OR ($3 <> '' AND provider_connection_ref = $3)) ORDER BY id LIMIT 2`
	rows, err := executor.Query(ctx, query, providerReference, providerAccountReference, providerConnectionReference)
	if err != nil {
		return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.get_by_provider_references", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	var records []ports.ChannelConnectionRecord
	for rows.Next() {
		var record ports.ChannelConnectionRecord
		if err := rows.Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference); err != nil {
			return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.get_by_provider_references", Kind: RepositoryInvalid, Err: err}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.get_by_provider_references", Kind: RepositoryInvalid, Err: err}
	}
	if len(records) == 0 {
		return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.get_by_provider_references", Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
	}
	if len(records) > 1 {
		return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.get_by_provider_references", Kind: RepositoryConflict, Err: errors.New("provider references match multiple channel connections")}
	}
	return records[0], nil
}

var _ ports.ChannelConnectionRepository = (*ChannelConnectionRepository)(nil)

func invalidRepositoryInput(operation, message string) error {
	return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: errors.New(message)}
}
func classifyRepositoryGetError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
}
