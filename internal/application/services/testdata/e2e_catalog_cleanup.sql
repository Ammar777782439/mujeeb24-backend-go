-- E2E Catalog Fixture — cleanup between test runs.
-- Loaded by /internal/application/services/e2e_gemini_catalog_test.go before
-- each subtest to guarantee determinism (the AI Run idempotency key is derived
-- from the inbound message ID, so re-running with the same ID would short-
-- circuit). We delete in FK-safe order: ai runs first, then messages, then
-- outbound/outbox, then conversation_references + conversations + customers +
-- connections + catalog rows.
--
-- Two targeted scopes:
--  * Business A inbound message ID (11111111-...119) — wiped each test
--  * Business B inbound message ID (22222222-...22a) — wiped each test
--
-- Catalog + customer + connection + conversation rows are NOT deleted here;
-- they are upserted by the seed SQL with ON CONFLICT DO UPDATE.

-- AI Run + dependent rows (these are produced by AutoReplyService.Handle,
-- so they may exist from prior runs with the same inbound message ID).
DELETE FROM ai_tool_calls          WHERE ai_run_id IN (SELECT id FROM ai_runs WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222'));
DELETE FROM ai_run_attempts        WHERE ai_run_id IN (SELECT id FROM ai_runs WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222'));
DELETE FROM ai_gemini_interactions WHERE ai_run_id IN (SELECT id FROM ai_runs WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222'));
DELETE FROM ai_catalog_batches     WHERE ai_run_id IN (SELECT id FROM ai_runs WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222'));
DELETE FROM ai_usage_telemetry     WHERE ai_run_id IN (SELECT id FROM ai_runs WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222'));
DELETE FROM ai_stage_latencies     WHERE ai_run_id IN (SELECT id FROM ai_runs WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222'));
DELETE FROM ai_runs                WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');

-- AI decisions (business ai_decisions table) + per-business usage records.
DELETE FROM ai_decisions           WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');
DELETE FROM ai_usage_records       WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');

-- Outbox entries + outbound messages + communication messages.
DELETE FROM outbox_entries        WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');
DELETE FROM outbound_messages     WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');
DELETE FROM communication_messages WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');

-- Conversation state (per contract ③ §1, ADR-039). The summary field lives
-- inside conversation_state.focus JSONB — deleting the row ensures the next
-- Handle() call rebuilds state from scratch and there is no stale summary
-- leaking across CI runs.
DELETE FROM conversation_state     WHERE business_id IN ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222');
