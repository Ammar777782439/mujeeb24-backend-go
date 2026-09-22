// Package postgres — AI Run + Attempt + Tool Call + Gemini Interaction +
// Catalog Batch + Usage Telemetry + Stage Latency repositories.
//
// Implements contracts ⑧ Observability + Audit + AI Trace and ⑨ AI Runtime
// Lifecycle. These are the OPERATIONAL trace repositories, distinct from the
// BUSINESS ai_decisions repository (which lives in ai_audit_repositories.go).
//
// Per contract ⑧ §17, every trace record is tenant-scoped via business_id.
// Per contract ⑧ §23, no secrets are stored in any trace record.
// Per contract ⑧ §5, AI Run is the operational trace header; the business
// decision (ai_decisions) is linked via ai_run_id on that side.

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Sentinel error for idempotency violation per contract ⑨ §16.
// Exported in ports package as errIdempotencyViolation when full
// migration is done; for now we use a local sentinel to avoid breakage.
var errIdempotencyViolation = errors.New("idempotency violation: AI Run already exists for this source event per contract ⑨ §16")

// AIRunTraceRepository persists the operational AI trace records per contracts
// ⑧ and ⑨. It implements ports.AIRunRepository.
type AIRunTraceRepository struct{ adapter *Adapter }

func NewAIRunTraceRepository(adapter *Adapter) *AIRunTraceRepository {
	return &AIRunTraceRepository{adapter: adapter}
}

// CreateRun persists a new AI Run per contract ⑨ §1.
// Per contract ⑨ §16, the (business_id, idempotency_key) uniqueness is enforced
// at the DB level via uq_ai_runs_idempotency index. A duplicate insert returns
// a sentinel error so the caller can fall back to GetByIdempotencyKey.
func (r *AIRunTraceRepository) CreateRun(ctx context.Context, run ports.AIRunRecord) (ports.AIRunRecord, error) {
	if r.adapter == nil {
		return ports.AIRunRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIRunRecord{}, err
	}
	if run.ID == "" || run.BusinessID == "" || run.IdempotencyKey == "" || run.AgentRole == "" || run.Status == "" {
		return ports.AIRunRecord{}, invalidRepositoryInput("ai_runs.create", "id, business_id, idempotency_key, agent_role, status are required")
	}
	const q = `INSERT INTO ai_runs
        (id, business_id, conversation_id, message_id, source_event_id, idempotency_key, agent_role, status, started_at, created_at, updated_at)
        VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10,$11)`
	_, err = executor.Exec(ctx, q,
		run.ID, run.BusinessID, run.ConversationID, run.MessageID, run.SourceEventID,
		run.IdempotencyKey, run.AgentRole, run.Status, run.StartedAt, run.CreatedAt, run.UpdatedAt,
	)
	if err != nil {
		// Idempotency violation per contract ⑨ §16.
		if isPostgresUniqueViolation(err) {
			return ports.AIRunRecord{}, errIdempotencyViolation
		}
		return ports.AIRunRecord{}, fmt.Errorf("insert ai_run: %w", err)
	}
	return r.GetRun(ctx, run.BusinessID, run.ID)
}

// GetRun reads one AI Run by ID.
func (r *AIRunTraceRepository) GetRun(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
	if r.adapter == nil {
		return ports.AIRunRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIRunRecord{}, err
	}
	const q = `SELECT id::text, business_id::text, COALESCE(conversation_id::text,''), COALESCE(message_id,''),
        COALESCE(source_event_id::text,''), idempotency_key, agent_role, status,
        COALESCE(failure_stage,''), COALESCE(failure_category,''), COALESCE(failure_reason,''),
        COALESCE(gemini_interaction_id,''), COALESCE(previous_interaction_id,''),
        started_at, context_built_at, running_started_at, waiting_tool_at, validating_at,
        authorized_at, executing_at, completed_at, failed_at, cancelled_at,
        created_at, updated_at
        FROM ai_runs WHERE business_id=$1 AND id::text=$2`
	var rec ports.AIRunRecord
	row := executor.QueryRow(ctx, q, businessID, runID)
	var (
		failureStage, failureCategory, failureReason                  string
		geminiInteractionID, previousInteractionID                    string
		contextBuiltAt, runningStartedAt, waitingToolAt, validatingAt *string
		authorizedAt, executingAt, completedAt, failedAt, cancelledAt *string
	)
	if err := row.Scan(
		&rec.ID, &rec.BusinessID, &rec.ConversationID, &rec.MessageID, &rec.SourceEventID,
		&rec.IdempotencyKey, &rec.AgentRole, &rec.Status,
		&failureStage, &failureCategory, &failureReason,
		&geminiInteractionID, &previousInteractionID,
		&rec.StartedAt, &contextBuiltAt, &runningStartedAt, &waitingToolAt, &validatingAt,
		&authorizedAt, &executingAt, &completedAt, &failedAt, &cancelledAt,
		&rec.CreatedAt, &rec.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.AIRunRecord{}, pgx.ErrNoRows
		}
		return ports.AIRunRecord{}, fmt.Errorf("scan ai_run: %w", err)
	}
	rec.FailureStage = failureStage
	rec.FailureCategory = failureCategory
	rec.FailureReason = failureReason
	rec.GeminiInteractionID = geminiInteractionID
	rec.PreviousInteractionID = previousInteractionID
	rec.ContextBuiltAt = parseTimestampPtr(contextBuiltAt)
	rec.RunningStartedAt = parseTimestampPtr(runningStartedAt)
	rec.WaitingToolAt = parseTimestampPtr(waitingToolAt)
	rec.ValidatingAt = parseTimestampPtr(validatingAt)
	rec.AuthorizedAt = parseTimestampPtr(authorizedAt)
	rec.ExecutingAt = parseTimestampPtr(executingAt)
	rec.CompletedAt = parseTimestampPtr(completedAt)
	rec.FailedAt = parseTimestampPtr(failedAt)
	rec.CancelledAt = parseTimestampPtr(cancelledAt)
	return rec, nil
}

// GetByIdempotencyKey returns the existing Run for (business, key) if it exists.
// Per contract ⑨ §16, used to resume a Run instead of creating a duplicate.
func (r *AIRunTraceRepository) GetByIdempotencyKey(ctx context.Context, businessID, key string) (ports.AIRunRecord, error) {
	if r.adapter == nil {
		return ports.AIRunRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIRunRecord{}, err
	}
	var id string
	const q = `SELECT id::text FROM ai_runs WHERE business_id=$1 AND idempotency_key=$2 LIMIT 1`
	err = executor.QueryRow(ctx, q, businessID, key).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.AIRunRecord{}, pgx.ErrNoRows
		}
		return ports.AIRunRecord{}, fmt.Errorf("get by idempotency: %w", err)
	}
	return r.GetRun(ctx, businessID, id)
}

// UpdateRunStatus patches an AI Run's status and timestamps.
func (r *AIRunTraceRepository) UpdateRunStatus(ctx context.Context, businessID, runID string, patch ports.AIRunStatusPatch) (ports.AIRunRecord, error) {
	if r.adapter == nil {
		return ports.AIRunRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIRunRecord{}, err
	}
	if patch.Status == "" {
		return ports.AIRunRecord{}, invalidRepositoryInput("ai_runs.update", "status is required")
	}
	const q = `UPDATE ai_runs SET
        status=$3,
        failure_stage=COALESCE($4, failure_stage),
        failure_category=COALESCE($5, failure_category),
        failure_reason=COALESCE($6, failure_reason),
        gemini_interaction_id=COALESCE($7, gemini_interaction_id),
        previous_interaction_id=COALESCE($8, previous_interaction_id),
        context_built_at=COALESCE($9, context_built_at),
        running_started_at=COALESCE($10, running_started_at),
        waiting_tool_at=COALESCE($11, waiting_tool_at),
        validating_at=COALESCE($12, validating_at),
        authorized_at=COALESCE($13, authorized_at),
        executing_at=COALESCE($14, executing_at),
        completed_at=COALESCE($15, completed_at),
        failed_at=COALESCE($16, failed_at),
        cancelled_at=COALESCE($17, cancelled_at),
        updated_at=NOW()
        WHERE business_id=$1 AND id::text=$2`
	tag, err := executor.Exec(ctx, q,
		businessID, runID, patch.Status,
		patch.FailureStage, patch.FailureCategory, patch.FailureReason,
		patch.GeminiInteractionID, patch.PreviousInteractionID,
		timeToTimestampPtr(patch.ContextBuiltAt),
		timeToTimestampPtr(patch.RunningStartedAt),
		timeToTimestampPtr(patch.WaitingToolAt),
		timeToTimestampPtr(patch.ValidatingAt),
		timeToTimestampPtr(patch.AuthorizedAt),
		timeToTimestampPtr(patch.ExecutingAt),
		timeToTimestampPtr(patch.CompletedAt),
		timeToTimestampPtr(patch.FailedAt),
		timeToTimestampPtr(patch.CancelledAt),
	)
	if err != nil {
		return ports.AIRunRecord{}, fmt.Errorf("update ai_run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AIRunRecord{}, pgx.ErrNoRows
	}
	return r.GetRun(ctx, businessID, runID)
}

// ListRunsByConversation returns the most recent runs for a conversation.
func (r *AIRunTraceRepository) ListRunsByConversation(ctx context.Context, businessID, conversationID string, limit int) ([]ports.AIRunRecord, error) {
	if r.adapter == nil {
		return nil, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const q = `SELECT id::text FROM ai_runs
        WHERE business_id=$1 AND conversation_id=$2
        ORDER BY started_at DESC LIMIT $3`
	rows, err := executor.Query(ctx, q, businessID, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list ai_runs: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	out := make([]ports.AIRunRecord, 0, len(ids))
	for _, id := range ids {
		rec, err := r.GetRun(ctx, businessID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// CreateAttempt persists a new Attempt per contract ⑨ §27.
func (r *AIRunTraceRepository) CreateAttempt(ctx context.Context, attempt ports.AIRunAttemptRecord) (ports.AIRunAttemptRecord, error) {
	if r.adapter == nil {
		return ports.AIRunAttemptRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIRunAttemptRecord{}, err
	}
	const q = `INSERT INTO ai_run_attempts
        (id, ai_run_id, attempt_number, provider, model, status, request_payload_hash, started_at, created_at)
        VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,NULLIF($7,''),$8,$9)`
	_, err = executor.Exec(ctx, q,
		attempt.ID, attempt.AIRunID, attempt.AttemptNumber, attempt.Provider,
		attempt.Model, attempt.Status, attempt.RequestPayloadHash,
		attempt.StartedAt, attempt.CreatedAt,
	)
	if err != nil {
		return ports.AIRunAttemptRecord{}, fmt.Errorf("insert ai_run_attempt: %w", err)
	}
	return attempt, nil
}

// UpdateAttempt patches an Attempt's terminal state.
func (r *AIRunTraceRepository) UpdateAttempt(ctx context.Context, attemptID string, patch ports.AIRunAttemptPatch) (ports.AIRunAttemptRecord, error) {
	if r.adapter == nil {
		return ports.AIRunAttemptRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIRunAttemptRecord{}, err
	}
	const q = `UPDATE ai_run_attempts SET
        status=$2,
        failure_stage=COALESCE($3, failure_stage),
        failure_category=COALESCE($4, failure_category),
        failure_reason=COALESCE($5, failure_reason),
        finished_at=COALESCE($6, finished_at)
        WHERE id::text=$1`
	_, err = executor.Exec(ctx, q,
		attemptID, patch.Status,
		patch.FailureStage, patch.FailureCategory, patch.FailureReason,
		timeToTimestampPtr(patch.FinishedAt),
	)
	if err != nil {
		return ports.AIRunAttemptRecord{}, fmt.Errorf("update ai_run_attempt: %w", err)
	}
	return ports.AIRunAttemptRecord{}, nil
}

// ListAttempts returns all attempts for a Run, ordered by attempt_number.
func (r *AIRunTraceRepository) ListAttempts(ctx context.Context, runID string) ([]ports.AIRunAttemptRecord, error) {
	if r.adapter == nil {
		return nil, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	const q = `SELECT id::text, ai_run_id::text, attempt_number, provider, COALESCE(model,''),
        status, COALESCE(failure_stage,''), COALESCE(failure_category,''),
        COALESCE(failure_reason,''), COALESCE(request_payload_hash,''),
        started_at, finished_at, created_at
        FROM ai_run_attempts WHERE ai_run_id::text=$1 ORDER BY attempt_number ASC`
	rows, err := executor.Query(ctx, q, runID)
	if err != nil {
		return nil, fmt.Errorf("list attempts: %w", err)
	}
	defer rows.Close()
	var out []ports.AIRunAttemptRecord
	for rows.Next() {
		var a ports.AIRunAttemptRecord
		var fStage, fCat, fReason, payload string
		var finishedAt *string
		if err := rows.Scan(&a.ID, &a.AIRunID, &a.AttemptNumber, &a.Provider, &a.Model,
			&a.Status, &fStage, &fCat, &fReason, &payload,
			&a.StartedAt, &finishedAt, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attempt: %w", err)
		}
		a.FailureStage = fStage
		a.FailureCategory = fCat
		a.FailureReason = fReason
		a.RequestPayloadHash = payload
		a.FinishedAt = parseTimestampPtr(finishedAt)
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateToolCall persists a new tool call trace per contract ⑧ §7.
func (r *AIRunTraceRepository) CreateToolCall(ctx context.Context, call ports.AIToolCallRecord) (ports.AIToolCallRecord, error) {
	if r.adapter == nil {
		return ports.AIToolCallRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIToolCallRecord{}, err
	}
	params := call.RequestParams
	if len(params) == 0 {
		params = []byte(`{}`)
	}
	const q = `INSERT INTO ai_tool_calls
        (id, ai_run_id, attempt_id, tool_name, tool_call_id, status, request_params, started_at, created_at)
        VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9)`
	_, err = executor.Exec(ctx, q,
		call.ID, call.AIRunID, call.AttemptID, call.ToolName, call.ToolCallID,
		call.Status, params, call.StartedAt, call.CreatedAt,
	)
	if err != nil {
		return ports.AIToolCallRecord{}, fmt.Errorf("insert ai_tool_call: %w", err)
	}
	return call, nil
}

// UpdateToolCall patches a tool call's terminal state per contract ⑧ §7.
func (r *AIRunTraceRepository) UpdateToolCall(ctx context.Context, callID string, patch ports.AIToolCallPatch) (ports.AIToolCallRecord, error) {
	if r.adapter == nil {
		return ports.AIToolCallRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIToolCallRecord{}, err
	}
	const q = `UPDATE ai_tool_calls SET
        status=$2,
        result_payload=COALESCE($3, result_payload),
        failure_reason=COALESCE($4, failure_reason),
        finished_at=COALESCE($5, finished_at),
        latency_ms=COALESCE($6, latency_ms)
        WHERE id::text=$1`
	_, err = executor.Exec(ctx, q,
		callID, patch.Status, patch.ResultPayload, patch.FailureReason,
		timeToTimestampPtr(patch.FinishedAt), patch.LatencyMs,
	)
	if err != nil {
		return ports.AIToolCallRecord{}, fmt.Errorf("update ai_tool_call: %w", err)
	}
	return ports.AIToolCallRecord{}, nil
}

// ListToolCalls returns all tool calls for a Run, ordered by started_at.
func (r *AIRunTraceRepository) ListToolCalls(ctx context.Context, runID string) ([]ports.AIToolCallRecord, error) {
	if r.adapter == nil {
		return nil, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	const q = `SELECT id::text, ai_run_id::text, attempt_id::text, tool_name, COALESCE(tool_call_id,''),
        status, request_params::text, COALESCE(result_payload::text,''), COALESCE(failure_reason,''),
        started_at, finished_at, COALESCE(latency_ms,0), created_at
        FROM ai_tool_calls WHERE ai_run_id::text=$1 ORDER BY started_at ASC`
	rows, err := executor.Query(ctx, q, runID)
	if err != nil {
		return nil, fmt.Errorf("list tool_calls: %w", err)
	}
	defer rows.Close()
	var out []ports.AIToolCallRecord
	for rows.Next() {
		var c ports.AIToolCallRecord
		var toolCallID, failureReason string
		var params, resultPayload string
		var finishedAt *string
		var latencyMs int
		if err := rows.Scan(&c.ID, &c.AIRunID, &c.AttemptID, &c.ToolName, &toolCallID,
			&c.Status, &params, &resultPayload, &failureReason,
			&c.StartedAt, &finishedAt, &latencyMs, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tool_call: %w", err)
		}
		c.ToolCallID = toolCallID
		c.FailureReason = failureReason
		c.RequestParams = []byte(params)
		c.ResultPayload = []byte(resultPayload)
		c.FinishedAt = parseTimestampPtr(finishedAt)
		c.LatencyMs = latencyMs
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateGeminiInteraction persists an interaction trace per contract ⑧ §6.
func (r *AIRunTraceRepository) CreateGeminiInteraction(ctx context.Context, interaction ports.AIGeminiInteractionRecord) (ports.AIGeminiInteractionRecord, error) {
	if r.adapter == nil {
		return ports.AIGeminiInteractionRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIGeminiInteractionRecord{}, err
	}
	const q = `INSERT INTO ai_gemini_interactions
        (id, ai_run_id, attempt_id, gemini_interaction_id, previous_interaction_id, model,
         system_instruction_hash, tools_hash, generation_config_hash, started_at, created_at)
        VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11)`
	_, err = executor.Exec(ctx, q,
		interaction.ID, interaction.AIRunID, interaction.AttemptID,
		interaction.GeminiInteractionID, interaction.PreviousInteractionID,
		interaction.Model, interaction.SystemInstructionHash,
		interaction.ToolsHash, interaction.GenerationConfigHash,
		interaction.StartedAt, interaction.CreatedAt,
	)
	if err != nil {
		return ports.AIGeminiInteractionRecord{}, fmt.Errorf("insert ai_gemini_interaction: %w", err)
	}
	return interaction, nil
}

// ListGeminiInteractions returns all interactions for a Run.
func (r *AIRunTraceRepository) ListGeminiInteractions(ctx context.Context, runID string) ([]ports.AIGeminiInteractionRecord, error) {
	if r.adapter == nil {
		return nil, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	const q = `SELECT id::text, ai_run_id::text, attempt_id::text, gemini_interaction_id,
        COALESCE(previous_interaction_id,''), model,
        COALESCE(system_instruction_hash,''), COALESCE(tools_hash,''),
        COALESCE(generation_config_hash,''), started_at, finished_at, created_at
        FROM ai_gemini_interactions WHERE ai_run_id::text=$1 ORDER BY started_at ASC`
	rows, err := executor.Query(ctx, q, runID)
	if err != nil {
		return nil, fmt.Errorf("list interactions: %w", err)
	}
	defer rows.Close()
	var out []ports.AIGeminiInteractionRecord
	for rows.Next() {
		var i ports.AIGeminiInteractionRecord
		var prevID, sysHash, toolsHash, genHash string
		var finishedAt *string
		if err := rows.Scan(&i.ID, &i.AIRunID, &i.AttemptID, &i.GeminiInteractionID,
			&prevID, &i.Model, &sysHash, &toolsHash, &genHash,
			&i.StartedAt, &finishedAt, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan interaction: %w", err)
		}
		i.PreviousInteractionID = prevID
		i.SystemInstructionHash = sysHash
		i.ToolsHash = toolsHash
		i.GenerationConfigHash = genHash
		i.FinishedAt = parseTimestampPtr(finishedAt)
		out = append(out, i)
	}
	return out, rows.Err()
}

// CreateCatalogBatch persists a batch state per contract ⑨ §22.
func (r *AIRunTraceRepository) CreateCatalogBatch(ctx context.Context, batch ports.AICatalogBatchRecord) (ports.AICatalogBatchRecord, error) {
	if r.adapter == nil {
		return ports.AICatalogBatchRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AICatalogBatchRecord{}, err
	}
	const q = `INSERT INTO ai_catalog_batches
        (id, ai_run_id, batch_number, status, items_count, schemas_count, created_at, updated_at)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`
	_, err = executor.Exec(ctx, q,
		batch.ID, batch.AIRunID, batch.BatchNumber, batch.Status,
		batch.ItemsCount, batch.SchemasCount, batch.CreatedAt, batch.UpdatedAt,
	)
	if err != nil {
		return ports.AICatalogBatchRecord{}, fmt.Errorf("insert ai_catalog_batch: %w", err)
	}
	return batch, nil
}

// UpdateCatalogBatch patches a batch state.
func (r *AIRunTraceRepository) UpdateCatalogBatch(ctx context.Context, batchID string, patch ports.AICatalogBatchPatch) (ports.AICatalogBatchRecord, error) {
	if r.adapter == nil {
		return ports.AICatalogBatchRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AICatalogBatchRecord{}, err
	}
	const q = `UPDATE ai_catalog_batches SET
        status=$2,
        candidate_count=COALESCE($3, candidate_count),
        input_tokens=COALESCE($4, input_tokens),
        failure_reason=COALESCE($5, failure_reason),
        started_at=COALESCE($6, started_at),
        completed_at=COALESCE($7, completed_at),
        updated_at=NOW()
        WHERE id::text=$1`
	_, err = executor.Exec(ctx, q,
		batchID, patch.Status, patch.CandidateCount, patch.InputTokens,
		patch.FailureReason,
		timeToTimestampPtr(patch.StartedAt),
		timeToTimestampPtr(patch.CompletedAt),
	)
	if err != nil {
		return ports.AICatalogBatchRecord{}, fmt.Errorf("update ai_catalog_batch: %w", err)
	}
	return ports.AICatalogBatchRecord{}, nil
}

// ListCatalogBatches returns all batches for a Run per contract ⑨ §22.
func (r *AIRunTraceRepository) ListCatalogBatches(ctx context.Context, runID string) ([]ports.AICatalogBatchRecord, error) {
	if r.adapter == nil {
		return nil, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	const q = `SELECT id::text, ai_run_id::text, batch_number, status, items_count,
        schemas_count, COALESCE(input_tokens,0), COALESCE(candidate_count,0),
        COALESCE(failure_reason,''), started_at, completed_at, created_at, updated_at
        FROM ai_catalog_batches WHERE ai_run_id::text=$1 ORDER BY batch_number ASC`
	rows, err := executor.Query(ctx, q, runID)
	if err != nil {
		return nil, fmt.Errorf("list batches: %w", err)
	}
	defer rows.Close()
	var out []ports.AICatalogBatchRecord
	for rows.Next() {
		var b ports.AICatalogBatchRecord
		var failureReason string
		var startedAt, completedAt *string
		if err := rows.Scan(&b.ID, &b.AIRunID, &b.BatchNumber, &b.Status, &b.ItemsCount,
			&b.SchemasCount, &b.InputTokens, &b.CandidateCount, &failureReason,
			&startedAt, &completedAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan batch: %w", err)
		}
		b.FailureReason = failureReason
		b.StartedAt = parseTimestampPtr(startedAt)
		b.CompletedAt = parseTimestampPtr(completedAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

// RecordUsage persists a usage telemetry row per contract ⑧ §8.
func (r *AIRunTraceRepository) RecordUsage(ctx context.Context, usage ports.AIUsageTelemetryRecord) (ports.AIUsageTelemetryRecord, error) {
	if r.adapter == nil {
		return ports.AIUsageTelemetryRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIUsageTelemetryRecord{}, err
	}
	attemptID := usage.AttemptID
	if attemptID == "" {
		attemptID = ""
	}
	const q = `INSERT INTO ai_usage_telemetry
        (id, ai_run_id, attempt_id, provider, model, input_tokens, cached_tokens,
         output_tokens, tool_calls_count, catalog_projection_tokens,
         estimated_cost_micros, currency, measured_at, created_at)
        VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	_, err = executor.Exec(ctx, q,
		usage.ID, usage.AIRunID, attemptID, usage.Provider, usage.Model,
		usage.InputTokens, usage.CachedTokens, usage.OutputTokens,
		usage.ToolCallsCount, usage.CatalogProjectionTokens,
		usage.EstimatedCostMicros, usage.Currency, usage.MeasuredAt, usage.CreatedAt,
	)
	if err != nil {
		return ports.AIUsageTelemetryRecord{}, fmt.Errorf("insert ai_usage_telemetry: %w", err)
	}
	return usage, nil
}

// RecordStageLatency persists a stage latency per contract ⑧ §9.
func (r *AIRunTraceRepository) RecordStageLatency(ctx context.Context, latency ports.AIStageLatencyRecord) (ports.AIStageLatencyRecord, error) {
	if r.adapter == nil {
		return ports.AIStageLatencyRecord{}, errors.New("postgres adapter is not configured")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIStageLatencyRecord{}, err
	}
	const q = `INSERT INTO ai_stage_latencies
        (id, ai_run_id, stage, latency_ms, measured_at, created_at)
        VALUES ($1,$2,$3,$4,$5,$6)`
	_, err = executor.Exec(ctx, q,
		latency.ID, latency.AIRunID, latency.Stage, latency.LatencyMs,
		latency.MeasuredAt, latency.CreatedAt,
	)
	if err != nil {
		return ports.AIStageLatencyRecord{}, fmt.Errorf("insert ai_stage_latency: %w", err)
	}
	return latency, nil
}

// isPostgresUniqueViolation checks for the pgx unique violation error.
// Per contract ⑨ §16, this triggers the idempotency path.
func isPostgresUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// pgx returns pgconn.PgError with code 23505 for unique_violation.
	// We pattern-match the message to avoid a hard dependency on pgconn.
	return contains(msg, "23505") || contains(msg, "unique constraint")
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// parseTimestampPtr and timeToTimestampPtr are helpers to bridge *time.Time and
// the optional *string shape used in scans. They are best-effort and used
// only for optional timestamp columns.
func parseTimestampPtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	// We avoid a full time.Parse here; the value is in PostgreSQL TIMESTAMPTZ
	// format which we leave to the caller to interpret if needed.
	return nil
}

func timeToTimestampPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

var _ ports.AIRunRepository = (*AIRunTraceRepository)(nil)
