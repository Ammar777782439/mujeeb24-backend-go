package postgres

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ChannelCapabilityRepository struct {
	adapter *Adapter
}

func NewChannelCapabilityRepository(adapter *Adapter) *ChannelCapabilityRepository {
	return &ChannelCapabilityRepository{adapter: adapter}
}

func (r *ChannelCapabilityRepository) ListByConnection(ctx context.Context, businessID, connectionID string) ([]ports.ChannelCapabilityRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if businessID == "" || connectionID == "" {
		return nil, invalidRepositoryInput("channel_capability.list_by_connection", "business and connection ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	var connectionExists bool
	if err := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channel_connections WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, connectionID).Scan(&connectionExists); err != nil {
		return nil, classifyRepositoryGetError("channel_capability.list_by_connection", err)
	}
	if !connectionExists {
		return nil, &RepositoryError{Operation: "channel_capability.list_by_connection", Kind: RepositoryNotFound}
	}
	rows, err := executor.Query(ctx, `SELECT c.connection_id::text, c.capability, c.enabled, c.checked_at, c.evidence_source FROM channel_connection_capabilities AS c WHERE c.connection_id = $1::uuid ORDER BY c.capability ASC`, connectionID)
	if err != nil {
		return nil, &RepositoryError{Operation: "channel_capability.list_by_connection", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.ChannelCapabilityRecord, 0)
	for rows.Next() {
		var item ports.ChannelCapabilityRecord
		if err := rows.Scan(&item.ConnectionID, &item.Name, &item.Enabled, &item.CheckedAt, &item.EvidenceSource); err != nil {
			return nil, &RepositoryError{Operation: "channel_capability.list_by_connection", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "channel_capability.list_by_connection", Kind: RepositoryInvalid, Err: err}
	}
	return items, nil
}

var _ ports.ChannelCapabilityRepository = (*ChannelCapabilityRepository)(nil)
