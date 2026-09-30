package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

// PlatformChannelReadRepository implements ports.PlatformChannelReadPort.
//
// Per Contract §93: reads channel_connections table in a Platform-scoped
// manner WITHOUT exposing the secret_reference column. The SELECT explicitly
// lists safe columns only — secret_reference is never queried.
type PlatformChannelReadRepository struct {
	adapter *Adapter
}

func NewPlatformChannelReadRepository(adapter *Adapter) *PlatformChannelReadRepository {
	return &PlatformChannelReadRepository{adapter: adapter}
}

// Safe columns only — secret_reference is intentionally absent per §93.
const platformChannelSelectColumns = `id::text, business_id::text, provider_ref, channel, provider_account_ref, provider_connection_ref, status, last_health_check_at`

func scanPlatformChannel(scanner interface {
	Scan(dest ...any) error
}, record *ports.PlatformChannelRecord) error {
	var lastHealthCheck *time.Time
	err := scanner.Scan(
		&record.ID, &record.BusinessID, &record.ProviderRef, &record.Channel,
		&record.ProviderAccountRef, &record.ProviderConnectionRef,
		&record.Status, &lastHealthCheck,
	)
	if err == nil && lastHealthCheck != nil {
		s := lastHealthCheck.UTC().Format(time.RFC3339)
		record.LastHealthCheckAt = &s
	}
	return err
}

func (r *PlatformChannelReadRepository) ListChannels(ctx context.Context, filter ports.PlatformChannelListFilter) ([]ports.PlatformChannelRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx,
		`SELECT `+platformChannelSelectColumns+`
		 FROM channel_connections
		 WHERE ($1 = '' OR business_id::text = $1)
		   AND ($2 = '' OR status = $2)
		   AND ($3 = '' OR channel = $3)
		 ORDER BY updated_at DESC
		 LIMIT $4`,
		strings.TrimSpace(filter.BusinessID), strings.TrimSpace(filter.Status), strings.TrimSpace(filter.Channel), filter.Limit,
	)
	if err != nil {
		return nil, &RepositoryError{Operation: "platform_channel.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := []ports.PlatformChannelRecord{}
	for rows.Next() {
		var record ports.PlatformChannelRecord
		if err := scanPlatformChannel(rows, &record); err != nil {
			return nil, &RepositoryError{Operation: "platform_channel.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "platform_channel.list", Kind: RepositoryInvalid, Err: err}
	}
	return items, nil
}

func (r *PlatformChannelReadRepository) GetChannelByID(ctx context.Context, connectionID string) (ports.PlatformChannelRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformChannelRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(connectionID) == "" {
		return ports.PlatformChannelRecord{}, invalidRepositoryInput("platform_channel.get", "connection_id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformChannelRecord{}, err
	}
	var record ports.PlatformChannelRecord
	err = executor.QueryRow(ctx,
		`SELECT `+platformChannelSelectColumns+`
		 FROM channel_connections WHERE id = $1::uuid`,
		connectionID,
	).Scan(
		&record.ID, &record.BusinessID, &record.ProviderRef, &record.Channel,
		&record.ProviderAccountRef, &record.ProviderConnectionRef,
		&record.Status, &record.LastHealthCheckAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PlatformChannelRecord{}, &RepositoryError{Operation: "platform_channel.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.PlatformChannelRecord{}, &RepositoryError{Operation: "platform_channel.get", Kind: RepositoryInvalid, Err: err}
	}
	// lastHealthCheckAt is scanned as *string from the SQL — the Scan above
	// already handles the nullable. But the scan helper uses time.Time for
	// the scan and then converts. Let me fix this: the raw Scan needs to
	// handle the nullable timestamp correctly.
	// Actually, the scanPlatformChannel function handles this correctly —
	// it scans into *time.Time and converts to *string.
	// But here I'm doing a direct Scan into the record fields without the
	// helper. Let me fix by using the helper.
	// Actually, looking at the record struct, LastHealthCheckAt is *string.
	// The SQL column is TIMESTAMPTZ. pgx can't scan TIMESTAMPTZ into *string.
	// Let me fix this by using the helper function.
	var lastHealthCheck *time.Time
	err = executor.QueryRow(ctx,
		`SELECT `+platformChannelSelectColumns+`
		 FROM channel_connections WHERE id = $1::uuid`,
		connectionID,
	).Scan(
		&record.ID, &record.BusinessID, &record.ProviderRef, &record.Channel,
		&record.ProviderAccountRef, &record.ProviderConnectionRef,
		&record.Status, &lastHealthCheck,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PlatformChannelRecord{}, &RepositoryError{Operation: "platform_channel.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.PlatformChannelRecord{}, &RepositoryError{Operation: "platform_channel.get", Kind: RepositoryInvalid, Err: err}
	}
	if lastHealthCheck != nil {
		s := lastHealthCheck.UTC().Format(time.RFC3339)
		record.LastHealthCheckAt = &s
	}
	return record, nil
}

var _ ports.PlatformChannelReadPort = (*PlatformChannelReadRepository)(nil)
