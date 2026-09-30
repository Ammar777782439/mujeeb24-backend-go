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

// Quiet Operations Room: a delivery callback updates only a known outbound
// message. It is never reinterpreted as a new customer communication.
type DeliveryStatusStore struct{ adapter *Adapter }

func NewDeliveryStatusStore(adapter *Adapter) *DeliveryStatusStore {
	return &DeliveryStatusStore{adapter: adapter}
}

func (s *DeliveryStatusStore) Apply(ctx context.Context, draft ports.DeliveryStatusDraft) (ports.DeliveryStatusResult, error) {
	if s == nil || s.adapter == nil {
		return ports.DeliveryStatusResult{}, ErrPoolClosed
	}
	if strings.TrimSpace(draft.InboundEventID) == "" || strings.TrimSpace(draft.BusinessID) == "" || strings.TrimSpace(draft.ConnectionID) == "" || strings.TrimSpace(draft.ProviderRef) == "" || strings.TrimSpace(draft.ProviderAccountRef) == "" || strings.TrimSpace(draft.ProviderMessageID) == "" || !validDeliveryStatus(draft.Status) {
		return ports.DeliveryStatusResult{}, invalidRepositoryInput("delivery_status.apply", "event, business, connection, provider/account/message, and valid status are required")
	}
	if draft.OccurredAt.IsZero() {
		draft.OccurredAt = time.Now().UTC()
	}
	var result ports.DeliveryStatusResult
	err := s.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := s.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		var eventBusinessID, eventConnectionID, eventAccountRef, eventType string
		if err := executor.QueryRow(txCtx, `SELECT business_id::text, connection_id::text, provider_connection_ref, event_type FROM inbound_event_ledger WHERE id = $1::uuid`, draft.InboundEventID).Scan(&eventBusinessID, &eventConnectionID, &eventAccountRef, &eventType); err != nil {
			return classifyRepositoryGetError("delivery_status.event", err)
		}
		if eventBusinessID != draft.BusinessID || eventConnectionID != draft.ConnectionID || eventAccountRef != draft.ProviderAccountRef || eventType != "delivery_status_changed" {
			return &RepositoryError{Operation: "delivery_status.apply", Kind: RepositoryConflict, Err: errors.New("delivery draft does not match a resolved status event")}
		}
		var existingID string
		err = executor.QueryRow(txCtx, `SELECT id::text FROM outbound_delivery_status_updates WHERE business_id = $1::uuid AND inbound_event_id = $2::uuid`, draft.BusinessID, draft.InboundEventID).Scan(&existingID)
		if err == nil {
			result = ports.DeliveryStatusResult{InboundEventID: draft.InboundEventID, Duplicate: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return classifyRepositoryGetError("delivery_status.dedupe", err)
		}
		var messageID, currentStatus string
		err = executor.QueryRow(txCtx, `SELECT id::text, status FROM outbound_messages WHERE business_id = $1::uuid AND connection_id = $2::uuid AND provider_ref = $3 AND provider_message_id = $4`, draft.BusinessID, draft.ConnectionID, draft.ProviderRef, draft.ProviderMessageID).Scan(&messageID, &currentStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err := executor.Exec(txCtx, `INSERT INTO outbound_delivery_status_updates (id, business_id, inbound_event_id, provider_ref, provider_message_id, delivery_status, occurred_at, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $7)`, uuid.NewString(), draft.BusinessID, draft.InboundEventID, draft.ProviderRef, draft.ProviderMessageID, draft.Status, draft.OccurredAt.UTC()); err != nil {
				return classifyRepositoryWriteError("delivery_status.unmatched", err)
			}
			result = ports.DeliveryStatusResult{InboundEventID: draft.InboundEventID, Ignored: true, Status: draft.Status}
			return markDeliveryEventProcessed(txCtx, executor, draft.InboundEventID, "delivery_status_unmatched")
		}
		if err != nil {
			return classifyRepositoryGetError("delivery_status.outbound", err)
		}
		nextStatus := monotonicDeliveryStatus(currentStatus, draft.Status)
		if _, err := executor.Exec(txCtx, `UPDATE outbound_messages SET status = $3, failure_code = CASE WHEN $3 = 'failed' THEN COALESCE(failure_code, 'provider_delivery_failed') ELSE failure_code END, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid`, draft.BusinessID, messageID, nextStatus, draft.OccurredAt.UTC()); err != nil {
			return classifyRepositoryWriteError("delivery_status.outbound.update", err)
		}
		if _, err := executor.Exec(txCtx, `INSERT INTO outbound_delivery_status_updates (id, business_id, inbound_event_id, outbound_message_id, provider_ref, provider_message_id, delivery_status, occurred_at, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $8)`, uuid.NewString(), draft.BusinessID, draft.InboundEventID, messageID, draft.ProviderRef, draft.ProviderMessageID, draft.Status, draft.OccurredAt.UTC()); err != nil {
			return classifyRepositoryWriteError("delivery_status.record", err)
		}
		result = ports.DeliveryStatusResult{InboundEventID: draft.InboundEventID, OutboundMessageID: messageID, Applied: true, Status: nextStatus}
		return markDeliveryEventProcessed(txCtx, executor, draft.InboundEventID, "delivery_status_applied")
	})
	if err != nil {
		return ports.DeliveryStatusResult{}, err
	}
	return result, nil
}

func validDeliveryStatus(value string) bool {
	switch value {
	case "accepted", "sent", "delivered", "read", "failed":
		return true
	default:
		return false
	}
}
func monotonicDeliveryStatus(current, incoming string) string {
	rank := map[string]int{"pending": 0, "sending": 1, "accepted": 2, "sent": 3, "delivered": 4, "read": 5, "failed": 2, "unknown": 1}
	if current == "read" || current == "delivered" && incoming != "read" {
		return current
	}
	if incoming == "failed" && rank[current] >= rank["sent"] {
		return current
	}
	if rank[incoming] >= rank[current] {
		return incoming
	}
	return current
}

func markDeliveryEventProcessed(ctx context.Context, executor SQLExecutor, eventID, resultCode string) error {
	tag, err := executor.Exec(ctx, `UPDATE inbound_event_ledger SET processing_state = 'processed', processing_result_code = $2, processed_at = now(), updated_at = now() WHERE id = $1::uuid AND processing_state = 'received'`, eventID, resultCode)
	if err != nil {
		return classifyRepositoryWriteError("delivery_status.event.complete", err)
	}
	if tag.RowsAffected() == 0 {
		return &RepositoryError{Operation: "delivery_status.event.complete", Kind: RepositoryConflict, Err: errors.New("delivery status event was not received")}
	}
	return nil
}

var _ ports.DeliveryStatusStore = (*DeliveryStatusStore)(nil)
