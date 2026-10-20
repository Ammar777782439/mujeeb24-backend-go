package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const salesContinuationLease = 3 * time.Minute
const salesContinuationMaxAttempts = 5

type SalesActionJobs struct{ adapter *Adapter }

func NewSalesActionJobs(a *Adapter) *SalesActionJobs { return &SalesActionJobs{a} }

func (r *SalesActionJobs) Claim(ctx context.Context, limit int, now time.Time) ([]ports.SalesActionJob, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("invalid job batch size")
	}
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = e.Exec(ctx, `UPDATE sales_action_jobs SET state='dead_letter',error_code='lease_attempts_exhausted',
		lease_token=NULL,lease_until=NULL,updated_at=$1
		WHERE state='processing' AND lease_until<$1 AND attempts>=5`, now); err != nil {
		return nil, err
	}
	rows, err := e.Query(ctx, `WITH picked AS (
		SELECT id FROM sales_action_jobs WHERE
		(state='pending' OR (state='processing' AND lease_until<$1))
		AND attempts<$3 AND (next_attempt_at IS NULL OR next_attempt_at<=$1)
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $2)
		UPDATE sales_action_jobs j SET state='processing',lease_token=$4::uuid,
		lease_until=$1+interval '3 minutes',attempts=attempts+1,updated_at=$1
		FROM picked WHERE j.id=picked.id
		RETURNING j.id::text,j.lease_token::text,j.business_id::text,j.customer_id::text,
		j.conversation_id::text,j.message_id::text,j.payload,j.phase`, now, limit, salesContinuationMaxAttempts, uuid.NewString())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []ports.SalesActionJob
	for rows.Next() {
		var j ports.SalesActionJob
		if err := rows.Scan(&j.ID, &j.LeaseToken, &j.Action.BusinessID, &j.Action.CustomerID,
			&j.Action.ConversationID, &j.Action.SourceMessageID, &j.Action.Payload, &j.Phase); err != nil {
			return nil, err
		}
		j.Action.Now = now
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Finish requeues only explicitly classified transient continuation failures.
// All updates check lease ownership and its expiration to fence stale workers.
func (r *SalesActionJobs) Finish(ctx context.Context, id, token, code string, now time.Time) error {
	return r.adapter.Within(ctx, func(tx context.Context) error {
		e, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}
		var phase string
		var attempts int
		err = e.QueryRow(tx, `SELECT phase,attempts FROM sales_action_jobs
			WHERE id=$1::uuid AND lease_token=$2::uuid AND state='processing'
			AND lease_until>$3 FOR UPDATE`, id, token, now).Scan(&phase, &attempts)
		if err != nil {
			return fmt.Errorf("sales action lease was lost: %w", err)
		}
		state := "completed"
		var next *time.Time
		if code == "sales_continuation_queued" && phase == "continue_ai" {
			state = "pending"
			code = ""
		} else if code != "" {
			state = "dead_letter"
			if phase == "continue_ai" && code == "sales_continuation_transient" && attempts < salesContinuationMaxAttempts {
				state = "pending"
				delay := 30 * time.Second * time.Duration(1<<uint(attempts-1))
				when := now.Add(delay)
				next = &when
			}
		} else if phase == "continue_ai" {
			var done bool
			err = e.QueryRow(tx, `SELECT EXISTS(SELECT 1 FROM sales_start_requests
				WHERE business_id=(SELECT business_id FROM sales_action_jobs WHERE id=$1::uuid)
				AND decision_message_id=(SELECT message_id FROM sales_action_jobs WHERE id=$1::uuid)
				AND status='accepted' AND continuation_state='completed')`, id).Scan(&done)
			if err != nil {
				return err
			}
			if !done {
				return errors.New("sales continuation is not durably completed")
			}
		}
		// The initial action advanced the same job to continue_ai atomically.
		// Do not mark it completed while the next phase is still outstanding.
		if phase == "continue_ai" && code == "" && attempts == 1 {
			// Continuation cannot be completed without its committed result
			// (checked above); no special case for the first claim.
		}
		tag, err := e.Exec(tx, `UPDATE sales_action_jobs SET state=$3,error_code=NULLIF($4,''),
			next_attempt_at=$5,lease_token=NULL,lease_until=NULL,updated_at=$6
			WHERE id=$1::uuid AND lease_token=$2::uuid AND state='processing' AND lease_until>$6`,
			id, token, state, code, next, now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("sales action lost lease while finishing")
		}
		return nil
	})
}

// LoadContinuation reads the original human-authored message; the postback is only
// evidence of acceptance, never the message passed to the model.
func (r *SalesActionJobs) LoadContinuation(ctx context.Context, j ports.SalesActionJob) (ports.SalesStartContinuation, error) {
	var out ports.SalesStartContinuation
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return out, err
	}
	var continuationState string
	err = e.QueryRow(ctx, `SELECT s.id::text,s.workflow_id::text,s.business_id::text,
		s.customer_id::text,s.conversation_id::text,s.source_message_id::text,
		COALESCE(m.text_content,''),cc.channel,cr.provider_ref,s.continuation_state,
		(w.state='awaiting_review' OR c.state='waiting_human' OR c.ownership='human')
		FROM sales_action_jobs j JOIN sales_start_requests s
		ON s.business_id=j.business_id AND s.decision_message_id=j.message_id
		JOIN sales_workflows w ON w.business_id=s.business_id AND w.id=s.workflow_id
		JOIN conversations c ON c.business_id=s.business_id AND c.id=s.conversation_id
		JOIN communication_messages m ON m.business_id=s.business_id AND m.id=s.source_message_id
		JOIN conversation_references cr ON cr.business_id=s.business_id AND cr.conversation_id=s.conversation_id
			AND cr.is_current AND cr.system='provider'
		JOIN channel_connections cc ON cc.business_id=cr.business_id AND cc.id=cr.connection_id
		WHERE j.id=$1::uuid AND j.lease_token=$2::uuid AND j.state='processing'
			AND j.phase='continue_ai' AND j.lease_until>now()
			AND s.status='accepted' AND s.business_id=$3::uuid AND s.customer_id=$4::uuid
			AND s.conversation_id=$5::uuid`, j.ID, j.LeaseToken, j.Action.BusinessID,
		j.Action.CustomerID, j.Action.ConversationID).Scan(
		&out.RequestID, &out.WorkflowID, &out.BusinessID, &out.CustomerID, &out.ConversationID,
		&out.SourceMessageID, &out.Text, &out.Channel, &out.ProviderRef, &continuationState, &out.AwaitingHumanReview)
	if err != nil {
		return out, err
	}
	out.Completed = continuationState == "completed"
	return out, nil
}

// CompleteContinuation must be called in the same short DB transaction as
// the outbound + outbox write. The lease check fences an expired AI worker.
func (r *SalesActionJobs) CompleteContinuation(ctx context.Context, j ports.SalesActionJob, startID string) error {
	if _, ok := transactionFromContext(ctx); !ok {
		return errors.New("completion requires a transaction")
	}
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	tag, err := e.Exec(ctx, `UPDATE sales_start_requests s SET continuation_state='completed',updated_at=now()
		FROM sales_action_jobs j
		WHERE s.business_id=j.business_id AND s.id=$1::uuid
			AND s.decision_message_id=j.message_id AND s.status='accepted'
			AND s.continuation_state='pending'
			AND j.id=$2::uuid AND j.lease_token=$3::uuid
			AND j.phase='continue_ai' AND j.state='processing' AND j.lease_until>now()
			AND (
				EXISTS(SELECT 1 FROM outbox_entries o
					WHERE o.business_id=s.business_id
					AND o.dedupe_key='sales-start-cont:'||s.id::text)
				OR EXISTS(SELECT 1 FROM sales_workflows w
					WHERE w.business_id=s.business_id AND w.id=s.workflow_id
					AND w.state='awaiting_review')
				OR EXISTS(SELECT 1 FROM conversations c
					WHERE c.business_id=s.business_id AND c.id=s.conversation_id
					AND c.state='waiting_human')
				OR EXISTS(SELECT 1 FROM conversations c
					WHERE c.business_id=s.business_id AND c.id=s.conversation_id
					AND c.ownership='human')
			)`,
		startID, j.ID, j.LeaseToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("sales continuation lease lost or already completed")
	}
	return nil
}

var _ ports.SalesActionJobRepository = (*SalesActionJobs)(nil)
