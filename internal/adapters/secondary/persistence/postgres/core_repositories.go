package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
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
	const query = `SELECT id::text, business_id::text, profile, contact_points, locale_preference, status, merged_into_customer_id::text, resource_version, updated_at FROM customers WHERE business_id = $1::uuid AND id = $2::uuid`
	var record ports.CustomerRecord
	if err := executor.QueryRow(ctx, query, businessID, customerID).Scan(&record.ID, &record.BusinessID, &record.Profile, &record.ContactPoints, &record.LocalePreference, &record.Status, &record.MergedIntoCustomer, &record.ResourceVersion, &record.UpdatedAt); err != nil {
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
	// Per migration 000057: include last_gemini_interaction_id per contract ③ §4.
	const query = `SELECT c.id::text, c.business_id::text, c.customer_id::text, cu.profile->>'display_name', c.state, c.ownership, c.ai_mode_override, c.priority, c.assignment_reference, c.resource_version, c.last_activity_at, c.last_gemini_interaction_id FROM conversations c LEFT JOIN customers cu ON cu.business_id = c.business_id AND cu.id = c.customer_id WHERE c.business_id = $1::uuid AND c.id = $2::uuid`
	var record ports.ConversationRecord
	if err := executor.QueryRow(ctx, query, businessID, conversationID).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.CustomerDisplayName, &record.State, &record.Ownership, &record.AIModeOverride, &record.Priority, &record.AssignmentReference, &record.ResourceVersion, &record.LastActivityAt, &record.LastGeminiInteractionID); err != nil {
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
	const query = `SELECT id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, resource_version, updated_at FROM channel_connections WHERE business_id = $1::uuid AND id = $2::uuid`
	var record ports.ChannelConnectionRecord
	if err := executor.QueryRow(ctx, query, businessID, connectionID).Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference, &record.ResourceVersion, &record.UpdatedAt); err != nil {
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
	const query = `SELECT id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, resource_version, updated_at FROM channel_connections WHERE provider_ref = $1 AND (($2 <> '' AND provider_account_ref = $2) OR ($3 <> '' AND provider_connection_ref = $3)) ORDER BY id LIMIT 2`
	rows, err := executor.Query(ctx, query, providerReference, providerAccountReference, providerConnectionReference)
	if err != nil {
		return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.get_by_provider_references", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	var records []ports.ChannelConnectionRecord
	for rows.Next() {
		var record ports.ChannelConnectionRecord
		if err := rows.Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference, &record.ResourceVersion, &record.UpdatedAt); err != nil {
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

func (r *ChannelConnectionRepository) CreatePending(ctx context.Context, businessID, providerRef, channel, providerConnectionRef, secretReference string) (ports.ChannelConnectionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelConnectionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(providerRef) == "" || strings.TrimSpace(channel) == "" || strings.TrimSpace(providerConnectionRef) == "" || strings.TrimSpace(secretReference) == "" {
		return ports.ChannelConnectionRecord{}, invalidRepositoryInput("channel_connection.create_pending", "business, provider, channel, connection reference, and secret reference are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelConnectionRecord{}, err
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	const query = `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'pending', $6, $7, $7) RETURNING id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, resource_version, updated_at`
	var record ports.ChannelConnectionRecord
	if err := executor.QueryRow(ctx, query, id, businessID, providerRef, channel, providerConnectionRef, secretReference, now).Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference, &record.ResourceVersion, &record.UpdatedAt); err != nil {
		return ports.ChannelConnectionRecord{}, &RepositoryError{Operation: "channel_connection.create_pending", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *ChannelConnectionRepository) Activate(ctx context.Context, businessID, id, providerAccountRef, providerConnectionRef string) (ports.ChannelConnectionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelConnectionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(id) == "" || strings.TrimSpace(providerAccountRef) == "" || strings.TrimSpace(providerConnectionRef) == "" {
		return ports.ChannelConnectionRecord{}, invalidRepositoryInput("channel_connection.activate", "business, connection, account, and provider connection references are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelConnectionRecord{}, err
	}
	const query = `UPDATE channel_connections SET provider_account_ref = $3, provider_connection_ref = $4, status = 'active', resource_version = resource_version + 1, updated_at = $5 WHERE business_id = $1::uuid AND id = $2::uuid AND status IN ('pending', 'reconnect_required') RETURNING id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, resource_version, updated_at`
	var record ports.ChannelConnectionRecord
	if err := executor.QueryRow(ctx, query, businessID, id, providerAccountRef, providerConnectionRef, time.Now().UTC()).Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference, &record.ResourceVersion, &record.UpdatedAt); err != nil {
		return ports.ChannelConnectionRecord{}, classifyRepositoryGetError("channel_connection.activate", err)
	}
	return record, nil
}

var _ ports.ChannelConnectionRepository = (*ChannelConnectionRepository)(nil)
var _ ports.ChannelConnectionWriter = (*ChannelConnectionRepository)(nil)

func invalidRepositoryInput(operation, message string) error {
	return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: errors.New(message)}
}
func classifyRepositoryGetError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
}
