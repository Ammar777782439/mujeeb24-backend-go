package postgres

import (
	"context"
	"fmt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"time"
)

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
	if _, err := e.Exec(ctx, `UPDATE sales_action_jobs SET state='dead_letter',error_code='lease_attempts_exhausted',lease_token=NULL,lease_until=NULL,updated_at=$1 WHERE state='processing' AND lease_until<$1 AND attempts>=5`, now); err != nil {
		return nil, err
	}
	rows, err := e.Query(ctx, `WITH picked AS (SELECT id FROM sales_action_jobs WHERE (state='pending' OR (state='processing' AND lease_until<$1)) AND attempts<5 ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT $2)
 UPDATE sales_action_jobs j SET state='processing',lease_token=$3::uuid,lease_until=$1+interval '2 minutes',attempts=attempts+1,updated_at=$1 FROM picked WHERE j.id=picked.id RETURNING j.id::text,j.lease_token::text,j.business_id::text,j.customer_id::text,j.conversation_id::text,j.message_id::text,j.payload`, now, limit, uuid.NewString())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []ports.SalesActionJob{}
	for rows.Next() {
		var j ports.SalesActionJob
		if err := rows.Scan(&j.ID, &j.LeaseToken, &j.Action.BusinessID, &j.Action.CustomerID, &j.Action.ConversationID, &j.Action.SourceMessageID, &j.Action.Payload); err != nil {
			return nil, err
		}
		j.Action.Now = now
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
func (r *SalesActionJobs) Finish(ctx context.Context, id, token, code string, now time.Time) error {
	state := "completed"
	if code != "" {
		state = "dead_letter"
	}
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	tag, err := e.Exec(ctx, `UPDATE sales_action_jobs SET state=$3,error_code=NULLIF($4,''),lease_token=NULL,lease_until=NULL,updated_at=$5 WHERE id=$1::uuid AND lease_token=$2::uuid AND state='processing'`, id, token, state, code, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("sales action lease was lost")
	}
	return nil
}
