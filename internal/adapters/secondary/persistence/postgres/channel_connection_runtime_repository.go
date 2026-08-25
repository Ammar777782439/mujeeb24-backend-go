package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type channelConnectionCursor struct {
	UpdatedAt time.Time
	ID        string
}

func (r *ChannelConnectionRepository) List(ctx context.Context, businessID, status, channel string, limit int, cursor string) (ports.ChannelConnectionPage, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelConnectionPage{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.ChannelConnectionPage{}, invalidRepositoryInput("channel_connection.list", "business id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return ports.ChannelConnectionPage{}, invalidRepositoryInput("channel_connection.list", "limit must not exceed 100")
	}
	decoded, err := decodeChannelConnectionCursor(cursor)
	if err != nil {
		return ports.ChannelConnectionPage{}, invalidRepositoryInput("channel_connection.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelConnectionPage{}, err
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.UpdatedAt, decoded.ID
	}
	const query = `SELECT id::text,business_id::text,provider_ref,channel,provider_account_ref,provider_connection_ref,status,secret_reference,resource_version,updated_at FROM channel_connections WHERE business_id=$1::uuid AND ($2='' OR status=$2) AND ($3='' OR channel=$3) AND ($4::timestamptz IS NULL OR (updated_at,id)<($4::timestamptz,$5::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $6`
	rows, err := executor.Query(ctx, query, businessID, strings.TrimSpace(status), strings.TrimSpace(channel), cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.ChannelConnectionPage{}, &RepositoryError{Operation: "channel_connection.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.ChannelConnectionRecord, 0, limit)
	for rows.Next() {
		var item ports.ChannelConnectionRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.ProviderReference, &item.Channel, &item.ProviderAccountReference, &item.ProviderConnectionRef, &item.Status, &item.SecretReference, &item.ResourceVersion, &item.UpdatedAt); err != nil {
			return ports.ChannelConnectionPage{}, &RepositoryError{Operation: "channel_connection.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.ChannelConnectionPage{}, &RepositoryError{Operation: "channel_connection.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.ChannelConnectionPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeChannelConnectionCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *ChannelConnectionRepository) Transition(ctx context.Context, transition ports.ChannelConnectionTransition) (ports.ChannelConnectionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ChannelConnectionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(transition.BusinessID) == "" || strings.TrimSpace(transition.ConnectionID) == "" || transition.ExpectedVersion <= 0 || strings.TrimSpace(transition.TargetStatus) == "" || strings.TrimSpace(transition.Action) == "" || strings.TrimSpace(transition.Reason) == "" || strings.TrimSpace(transition.ActorReference) == "" {
		return ports.ChannelConnectionRecord{}, invalidRepositoryInput("channel_connection.transition", "business, connection, expected version, target status, action, reason, and actor are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ChannelConnectionRecord{}, err
	}
	const update = `WITH current AS (SELECT status FROM channel_connections WHERE business_id=$1::uuid AND id=$2::uuid), updated AS (UPDATE channel_connections AS c SET status=$4,resource_version=resource_version+1,updated_at=now() FROM current WHERE c.business_id=$1::uuid AND c.id=$2::uuid AND c.resource_version=$3 AND c.status<>'archived' RETURNING current.status AS from_status,c.id::text,c.business_id::text,c.provider_ref,c.channel,c.provider_account_ref,c.provider_connection_ref,c.status,c.secret_reference,c.resource_version,c.updated_at) INSERT INTO channel_connection_state_events (id,business_id,connection_id,action,from_status,to_status,reason,actor_reference,created_at) SELECT $5::uuid,$1::uuid,$2::uuid,$6,from_status,$4,$7,$8,now() FROM updated RETURNING (SELECT id::text FROM updated),(SELECT business_id::text FROM updated),(SELECT provider_ref FROM updated),(SELECT channel FROM updated),(SELECT provider_account_ref FROM updated),(SELECT provider_connection_ref FROM updated),(SELECT status FROM updated),(SELECT secret_reference FROM updated),(SELECT resource_version FROM updated),(SELECT updated_at FROM updated)`
	var record ports.ChannelConnectionRecord
	err = executor.QueryRow(ctx, update, transition.BusinessID, transition.ConnectionID, transition.ExpectedVersion, transition.TargetStatus, uuid.NewString(), transition.Action, transition.Reason, transition.ActorReference).Scan(&record.ID, &record.BusinessID, &record.ProviderReference, &record.Channel, &record.ProviderAccountReference, &record.ProviderConnectionRef, &record.Status, &record.SecretReference, &record.ResourceVersion, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return ports.ChannelConnectionRecord{}, classifyCoreStaleOrNotFound(ctx, executor, "channel_connection.transition", "channel_connections", transition.BusinessID, transition.ConnectionID, err)
}

func encodeChannelConnectionCursor(record ports.ChannelConnectionRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}
func decodeChannelConnectionCursor(value string) (*channelConnectionCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid channel connection cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid channel connection cursor")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid channel connection cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, errors.New("invalid channel connection cursor")
	}
	return &channelConnectionCursor{UpdatedAt: updatedAt, ID: parts[1]}, nil
}

var _ ports.ChannelConnectionRuntimeRepository = (*ChannelConnectionRepository)(nil)
