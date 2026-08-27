package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Quiet Operations Room: this persistence layer records only durable Mujeeb
// obligations; it never calls Chatwoot from a database transaction.
type ChatwootMirrorStore struct{ adapter *Adapter }

func NewChatwootMirrorStore(adapter *Adapter) *ChatwootMirrorStore {
	return &ChatwootMirrorStore{adapter: adapter}
}

func (s *ChatwootMirrorStore) Enqueue(ctx context.Context, draft ports.ChatwootMirrorDraft) (ports.ChatwootMirrorJob, error) {
	if s == nil || s.adapter == nil {
		return ports.ChatwootMirrorJob{}, ErrPoolClosed
	}
	if strings.TrimSpace(draft.BusinessID) == "" || strings.TrimSpace(draft.CommunicationMessageID) == "" {
		return ports.ChatwootMirrorJob{}, invalidRepositoryInput("chatwoot_mirror.enqueue", "business and communication message are required")
	}
	if draft.ID == "" {
		draft.ID = uuid.NewString()
	}
	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = time.Now().UTC()
	}
	if draft.UpdatedAt.IsZero() {
		draft.UpdatedAt = draft.CreatedAt
	}
	executor, err := s.adapter.Executor(ctx)
	if err != nil {
		return ports.ChatwootMirrorJob{}, err
	}
	var job ports.ChatwootMirrorJob
	err = executor.QueryRow(ctx, `
		INSERT INTO chatwoot_mirror_jobs (id, business_id, communication_message_id, status, attempt_count, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'pending', 0, $4, $5)
		ON CONFLICT (business_id, communication_message_id) DO UPDATE
		SET updated_at = chatwoot_mirror_jobs.updated_at
		RETURNING id::text, business_id::text, communication_message_id::text, status, attempt_count,
		          lease_owner, lease_token, lease_expires_at, failure_code, result_code,
		          chatwoot_contact_id, chatwoot_conversation_id, chatwoot_message_id`,
		draft.ID, draft.BusinessID, draft.CommunicationMessageID, draft.CreatedAt, draft.UpdatedAt,
	).Scan(&job.ID, &job.BusinessID, &job.CommunicationMessageID, &job.Status, &job.AttemptCount,
		&job.LeaseOwner, &job.LeaseToken, &job.LeaseExpiresAt, &job.FailureCode, &job.ResultCode,
		&job.ChatwootContactID, &job.ChatwootConversationID, &job.ChatwootMessageID)
	if err != nil {
		return ports.ChatwootMirrorJob{}, classifyRepositoryWriteError("chatwoot_mirror.enqueue", err)
	}
	return job, nil
}

func (s *ChatwootMirrorStore) ListClaimable(ctx context.Context, limit int) ([]ports.ChatwootMirrorJob, error) {
	if s == nil || s.adapter == nil {
		return nil, ErrPoolClosed
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.adapter.Pool().Query(ctx, `
		SELECT id::text, business_id::text, communication_message_id::text, status, attempt_count,
		       lease_owner, lease_token, lease_expires_at, failure_code, result_code,
		       chatwoot_contact_id, chatwoot_conversation_id, chatwoot_message_id
		FROM chatwoot_mirror_jobs
		WHERE status = 'pending'
		ORDER BY created_at ASC, id ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, classifyRepositoryGetError("chatwoot_mirror.list_claimable", err)
	}
	defer rows.Close()
	items := make([]ports.ChatwootMirrorJob, 0)
	for rows.Next() {
		var item ports.ChatwootMirrorJob
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CommunicationMessageID, &item.Status, &item.AttemptCount,
			&item.LeaseOwner, &item.LeaseToken, &item.LeaseExpiresAt, &item.FailureCode, &item.ResultCode,
			&item.ChatwootContactID, &item.ChatwootConversationID, &item.ChatwootMessageID); err != nil {
			return nil, classifyRepositoryGetError("chatwoot_mirror.list_claimable", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyRepositoryGetError("chatwoot_mirror.list_claimable", err)
	}
	return items, nil
}

func (s *ChatwootMirrorStore) Claim(ctx context.Context, jobID string, lease ports.MirrorLease) (ports.MirrorClaimResult, error) {
	if s == nil || s.adapter == nil {
		return ports.MirrorClaimResult{}, ErrPoolClosed
	}
	if strings.TrimSpace(jobID) == "" || strings.TrimSpace(lease.Owner) == "" || strings.TrimSpace(lease.Token) == "" || lease.ExpiresAt.IsZero() {
		return ports.MirrorClaimResult{}, invalidRepositoryInput("chatwoot_mirror.claim", "job and lease are required")
	}
	var record ports.ChatwootMirrorJob
	err := s.adapter.Pool().QueryRow(ctx, `
		UPDATE chatwoot_mirror_jobs
		SET status = 'processing', attempt_count = attempt_count + 1, lease_owner = $2, lease_token = $3, lease_expires_at = $4, updated_at = now()
		WHERE id = $1::uuid AND status = 'pending'
		RETURNING id::text, business_id::text, communication_message_id::text, status, attempt_count,
		          lease_owner, lease_token, lease_expires_at, failure_code, result_code,
		          chatwoot_contact_id, chatwoot_conversation_id, chatwoot_message_id`, jobID, lease.Owner, lease.Token, lease.ExpiresAt.UTC()).Scan(
		&record.ID, &record.BusinessID, &record.CommunicationMessageID, &record.Status, &record.AttemptCount,
		&record.LeaseOwner, &record.LeaseToken, &record.LeaseExpiresAt, &record.FailureCode, &record.ResultCode,
		&record.ChatwootContactID, &record.ChatwootConversationID, &record.ChatwootMessageID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MirrorClaimResult{}, nil
	}
	if err != nil {
		return ports.MirrorClaimResult{}, classifyRepositoryWriteError("chatwoot_mirror.claim", err)
	}
	return ports.MirrorClaimResult{Claimed: true, Record: record}, nil
}

func (s *ChatwootMirrorStore) Resolve(ctx context.Context, businessID, jobID string) (ports.ChatwootMirrorDelivery, error) {
	if s == nil || s.adapter == nil {
		return ports.ChatwootMirrorDelivery{}, ErrPoolClosed
	}
	var delivery ports.ChatwootMirrorDelivery
	var accountID, inboxID string
	err := s.adapter.Pool().QueryRow(ctx, `
		SELECT j.business_id::text, j.id::text, m.id::text, r.id::text, c.id::text,
		       COALESCE(NULLIF(c.profile->>'name', ''), 'عميل'), e.external_user_id, r.resource_id,
		       COALESCE(m.text_content, ''), b.account_id, b.inbox_id
		FROM chatwoot_mirror_jobs j
		JOIN communication_messages m ON m.business_id = j.business_id AND m.id = j.communication_message_id
		JOIN conversation_references r ON r.business_id = m.business_id AND r.id = m.conversation_reference_id
		JOIN conversations v ON v.business_id = r.business_id AND v.id = r.conversation_id
		JOIN customers c ON c.business_id = v.business_id AND c.id = v.customer_id
		JOIN external_identities e ON e.business_id = c.business_id AND e.customer_id = c.id AND e.connection_id = r.connection_id AND e.link_status = 'linked'
		JOIN channel_connections cc ON cc.business_id = r.business_id AND cc.id = r.connection_id AND cc.status = 'active'
		JOIN chatwoot_workspace_bindings b ON b.business_id = j.business_id AND b.route_key = ('cw_' || REPLACE(cc.id::text, '-', '')) AND b.active
		WHERE j.business_id = $1::uuid AND j.id = $2::uuid AND j.status = 'processing'
		  AND r.system = 'provider' AND r.mapping_status = 'active' AND r.is_current
		ORDER BY e.updated_at DESC
		LIMIT 1`, businessID, jobID).Scan(
		&delivery.BusinessID, &delivery.JobID, &delivery.CommunicationMessageID, &delivery.ConversationReferenceID,
		&delivery.CustomerID, &delivery.CustomerName, &delivery.CustomerIdentifier, &delivery.ProviderConversationID,
		&delivery.Text, &accountID, &inboxID,
	)
	if err != nil {
		return ports.ChatwootMirrorDelivery{}, classifyRepositoryGetError("chatwoot_mirror.resolve", err)
	}
	parsedAccountID, accountErr := strconv.ParseInt(accountID, 10, 64)
	parsedInboxID, inboxErr := strconv.ParseInt(inboxID, 10, 64)
	if accountErr != nil || inboxErr != nil || parsedAccountID <= 0 || parsedInboxID <= 0 {
		return ports.ChatwootMirrorDelivery{}, &RepositoryError{Operation: "chatwoot_mirror.resolve", Kind: RepositoryConflict, Err: fmt.Errorf("Chatwoot binding ids are invalid")}
	}
	if strings.TrimSpace(delivery.Text) == "" {
		return ports.ChatwootMirrorDelivery{}, &RepositoryError{Operation: "chatwoot_mirror.resolve", Kind: RepositoryConflict, Err: errors.New("non-text communication message cannot be mirrored by current workspace adapter")}
	}
	delivery.AccountID, delivery.InboxID = parsedAccountID, parsedInboxID
	return delivery, nil
}

func (s *ChatwootMirrorStore) MarkCompleted(ctx context.Context, completion ports.ChatwootMirrorCompletion) (ports.ChatwootMirrorJob, error) {
	if s == nil || s.adapter == nil {
		return ports.ChatwootMirrorJob{}, ErrPoolClosed
	}
	if strings.TrimSpace(completion.JobID) == "" || strings.TrimSpace(completion.Owner) == "" || strings.TrimSpace(completion.Token) == "" || completion.AccountID <= 0 || completion.InboxID <= 0 || strings.TrimSpace(completion.ChatwootContactID) == "" || strings.TrimSpace(completion.ChatwootConversationID) == "" || strings.TrimSpace(completion.ChatwootMessageID) == "" {
		return ports.ChatwootMirrorJob{}, invalidRepositoryInput("chatwoot_mirror.complete", "job, lease, and Chatwoot references are required")
	}
	if completion.CompletedAt.IsZero() {
		completion.CompletedAt = time.Now().UTC()
	}
	if completion.UpdatedAt.IsZero() {
		completion.UpdatedAt = completion.CompletedAt
	}
	var result ports.ChatwootMirrorJob
	err := s.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := s.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		var referenceID string
		err = executor.QueryRow(txCtx, `
			UPDATE chatwoot_mirror_jobs
			SET status = 'completed', lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
				result_code = 'chatwoot_mirrored', chatwoot_contact_id = $4, chatwoot_conversation_id = $5, chatwoot_message_id = $6,
				completed_at = $7, updated_at = $8
			WHERE id = $1::uuid AND status = 'processing' AND lease_owner = $2 AND lease_token = $3
			RETURNING id::text, business_id::text, communication_message_id::text`, completion.JobID, completion.Owner, completion.Token,
			completion.ChatwootContactID, completion.ChatwootConversationID, completion.ChatwootMessageID, completion.CompletedAt.UTC(), completion.UpdatedAt.UTC(),
		).Scan(&result.ID, &result.BusinessID, &result.CommunicationMessageID)
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: "chatwoot_mirror.complete", Kind: RepositoryConflict, Err: errors.New("mirror lease was lost")}
		}
		if err != nil {
			return classifyRepositoryWriteError("chatwoot_mirror.complete", err)
		}
		if err := executor.QueryRow(txCtx, `SELECT conversation_reference_id::text FROM communication_messages WHERE business_id = $1::uuid AND id = $2::uuid`, result.BusinessID, result.CommunicationMessageID).Scan(&referenceID); err != nil {
			return classifyRepositoryGetError("chatwoot_mirror.message.get", err)
		}
		if _, err := executor.Exec(txCtx, `UPDATE communication_messages SET chatwoot_message_id = $3 WHERE business_id = $1::uuid AND id = $2::uuid`, result.BusinessID, result.CommunicationMessageID, completion.ChatwootMessageID); err != nil {
			return classifyRepositoryWriteError("chatwoot_mirror.message.update", err)
		}
		if _, err := executor.Exec(txCtx, `UPDATE conversation_references SET chatwoot_account_id = $3, chatwoot_inbox_id = $4, chatwoot_conversation_id = $5, updated_at = $6 WHERE business_id = $1::uuid AND id = $2::uuid AND system = 'provider' AND is_current AND mapping_status = 'active'`, result.BusinessID, referenceID, strconv.FormatInt(completion.AccountID, 10), strconv.FormatInt(completion.InboxID, 10), completion.ChatwootConversationID, completion.UpdatedAt.UTC()); err != nil {
			return classifyRepositoryWriteError("chatwoot_mirror.reference.update", err)
		}
		result.Status = "completed"
		result.ResultCode = stringPtr("chatwoot_mirrored")
		result.ChatwootContactID = stringPtr(completion.ChatwootContactID)
		result.ChatwootConversationID = stringPtr(completion.ChatwootConversationID)
		result.ChatwootMessageID = stringPtr(completion.ChatwootMessageID)
		return nil
	})
	if err != nil {
		return ports.ChatwootMirrorJob{}, err
	}
	return result, nil
}

func (s *ChatwootMirrorStore) MoveToDeadLetter(ctx context.Context, failure ports.ChatwootMirrorFailure) (ports.ChatwootMirrorJob, error) {
	if s == nil || s.adapter == nil {
		return ports.ChatwootMirrorJob{}, ErrPoolClosed
	}
	if strings.TrimSpace(failure.JobID) == "" || strings.TrimSpace(failure.Owner) == "" || strings.TrimSpace(failure.Token) == "" || strings.TrimSpace(failure.FailureCode) == "" {
		return ports.ChatwootMirrorJob{}, invalidRepositoryInput("chatwoot_mirror.dead_letter", "job, lease, and failure code are required")
	}
	if failure.UpdatedAt.IsZero() {
		failure.UpdatedAt = time.Now().UTC()
	}
	var job ports.ChatwootMirrorJob
	err := s.adapter.Pool().QueryRow(ctx, `
		UPDATE chatwoot_mirror_jobs SET status = 'dead_letter', lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL, failure_code = $4, updated_at = $5
		WHERE id = $1::uuid AND status = 'processing' AND lease_owner = $2 AND lease_token = $3
		RETURNING id::text, business_id::text, communication_message_id::text, status, attempt_count, failure_code`, failure.JobID, failure.Owner, failure.Token, failure.FailureCode, failure.UpdatedAt.UTC()).Scan(&job.ID, &job.BusinessID, &job.CommunicationMessageID, &job.Status, &job.AttemptCount, &job.FailureCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChatwootMirrorJob{}, &RepositoryError{Operation: "chatwoot_mirror.dead_letter", Kind: RepositoryConflict, Err: errors.New("mirror lease was lost")}
	}
	if err != nil {
		return ports.ChatwootMirrorJob{}, classifyRepositoryWriteError("chatwoot_mirror.dead_letter", err)
	}
	return job, nil
}

func stringPtr(value string) *string { return &value }

var _ ports.ChatwootMirrorStore = (*ChatwootMirrorStore)(nil)
