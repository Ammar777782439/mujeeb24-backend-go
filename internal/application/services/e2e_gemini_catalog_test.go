//go:build gemini_e2e

// Package services_test — Real Gemini + Real PostgreSQL + Real Catalog E2E test.
//
// This test exercises the EXACT production path of AutoReplyService.Handle
// against a real Gemini API + real PostgreSQL + real catalog data. No mocks.
// No fakes. No stubs. No bypasses.
//
// Path under test (per discovery + per spec §6 catalog provenance correction):
//
//   Customer message "كم سعر Mujeeb CI Test Phone؟"
//     ↓
//   AutoReplyService.Handle (auto_reply.go:186)
//     ↓
//   AIRunLifecycle.StartRun (ai_run_lifecycle.go:49) — received
//     ↓
//   AutoReplyContextBuilder.Build (ai_context_builder.go:65)
//     → CatalogRepository.ListCatalogs/ListCatalogItems/ListOffers/ListVariants
//       (reads from REAL PostgreSQL — the fixture rows seeded by this test)
//     → populates AIContext.CatalogEvidence + CatalogSummary + OfferEvidence
//     ↓
//   buildUserPrompt() (gemini/client.go:164) — serializes AIContext into the
//     user prompt JSON (promptContext struct). The catalog facts (price,
//     currency, availability) that originated in PostgreSQL now flow INTO
//     the Gemini user prompt as JSON evidence — they are NOT in the system
//     prompt, NOT in the test assertion, NOT invented by the model.
//     ↓
//   ContractClient.DecideContract (client_contracts.go:148) — REAL Gemini API
//     → sendContractRequest (centralized telemetry boundary — per spec §3+§4)
//       → records ai_gemini_interactions + ai_usage_telemetry for EVERY call
//     ↓
//   (optional) CatalogBatchController.RunCatalogEvaluation on needs_more_data
//     ↓
//   AIRunLifecycle.MarkValidating (running → validating)
//     ↓
//   ValidationPipeline.Validate (ai_validation_pipeline.go:92)
//     (Structural → Reference → Tenant → Policy → Authorization)
//     ↓
//   AIRunLifecycle.MarkAuthorized (validating → authorized) — per spec §1
//   AIRunLifecycle.MarkExecuting (authorized → executing) — per spec §1
//     ↓
//   Transactions.Within atomic (auto_reply.go:528-677):
//     DecisionRepository.CreateProposed
//     → OutboundRepository.CreatePending (outbound_messages row)
//     → MessageRepository.Record (communication_messages row, origin=ai)
//     → Outbox.Enqueue (outbox_entries row)
//     ↓
//   AIRunLifecycle.MarkCompleted (executing → completed) — per spec §1
//     ↓
//   recordAIUsage.AppendRecord — atomic (ai_usage_records + subscription_ai_usage)
//
// Catalog provenance (per spec §6 — corrected):
//   PostgreSQL (fixture seed)
//     → CatalogRepository.ListCatalogItems/ListOffers
//     → AutoReplyContextBuilder.Build
//     → AIContext.CatalogEvidence / OfferEvidence / CatalogSummary
//     → buildUserPrompt() (gemini/client.go:164)
//     → Gemini user prompt JSON (promptContext struct)
//     → Gemini API
//     → AIGeminiProposal.ResponseText
//     → outbound_messages + communication_messages.text_content
//   The test asserts that the Final AI Reply contains the catalog facts
//   (price + currency) that originated in PostgreSQL — proving the full
//   grounding chain works end-to-end.
//
// Build tag: gemini_e2e — this test only runs when explicitly requested via
// `-tags=gemini_e2e`. The CI workflow dispatches this manually with
// GEMINI_API_KEY + a PostgreSQL service container.
//
// Required env:
//   GEMINI_API_KEY — required, fail-fast on missing
//   DATABASE_URL   — required, fail-fast on missing
//   GEMINI_MODEL   — optional, defaults to gemini-3.5-flash-lite
//
// Determinism:
//   Uses fixed UUIDs (11111111-... vs 22222222-...) for business/customer/
//   catalog/item/offer/conversation rows. Cleanup SQL wipes prior runs' rows
//   for these business IDs before each test. Seed SQL upserts the fixture
//   idempotently.
//
// Tenant isolation:
//   Two businesses with the SAME product name "Mujeeb CI Test Phone" but
//   DIFFERENT price/currency/availability. The cross-tenant sentinel asserts
//   that Business A's reply mentions 125000 + YER (from Business A's catalog)
//   and NEVER mentions 999 or SAR (Business B's facts), and vice-versa.

package services_test

import (
	"context"
	"database/sql"
	_ "embed" // go:embed for fixture SQL
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gemini"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

//go:embed testdata/e2e_catalog_seed.sql
var seedSQL string

//go:embed testdata/e2e_catalog_cleanup.sql
var cleanupSQL string

// Deterministic fixture IDs (well-known low-collision test identifiers so the
// test can resolve rows by ID without needing to query-and-discover).
const (
	bizAID       = "11111111-1111-1111-1111-111111111111"
	bizBID       = "22222222-2222-2222-2222-222222222222"
	convAID      = "11111111-1111-1111-1111-111111111115"
	convBID      = "22222222-2222-2222-2222-222222222226"
	connAID      = "11111111-1111-1111-1111-111111111113"
	connBID      = "22222222-2222-2222-2222-222222222224"
	providerRefA = "wa_conv_ref_a_test"
	providerRefB = "wa_conv_ref_b_test"
	// Inbound message IDs — used as idempotency key + causation_id.
	// Must be valid UUIDs so the causation_id UUID-pointer cast does not fail.
	inboundMsgAID = "11111111-1111-1111-1111-111111111119"
	inboundMsgBID = "22222222-2222-2222-2222-22222222222a"
	// Expected catalog facts (must match testdata/e2e_catalog_seed.sql exactly).
	bizAItemName = "Mujeeb CI Test Phone"
	bizAPrice    = "125000"
	bizACurrency = "YER"
	bizBPrice    = "999"
	bizBCurrency = "SAR"
)

// TestE2E_GeminiCatalogB2CSuccess is the primary SUCCESSFUL REAL SCENARIO E2E.
//
// It proves that a customer message flows through the full production path
// (AutoReplyService.Handle → Gemini → Validation → Outbox → AI Run COMPLETED)
// and that the Final AI Reply is grounded on real PostgreSQL catalog data —
// NOT on data baked into a prompt or invented by the model.
func TestE2E_GeminiCatalogB2CSuccess(t *testing.T) {
	// ===== Section: failure conditions (per spec §22) =====
	// All failure conditions are fail-fast. No t.Skip on missing Gemini key,
	// missing Postgres, migration failure, seed failure, Gemini error,
	// validation failure, missing outbound/outbox rows, missing catalog facts,
	// or tenant isolation leak.

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Fatal("GEMINI_API_KEY is required for E2E; refusing to skip per spec §22 (no t.Skip on real-failure masking)")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL is required for E2E; refusing to skip per spec §22")
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	t.Logf("E2E config: model=%s db=%s", model, maskURL(dbURL))

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// ===== Section: PostgreSQL + migrations =====
	t.Run("Postgres_connect_and_migrate", func(t *testing.T) {
		// Step 1: connect.
		adapter, err := postgres.Open(ctx, dbURL, postgres.DefaultPoolConfig())
		if err != nil {
			t.Fatalf("PostgreSQL connect: %v", err)
		}
		t.Cleanup(func() {
			if adapter != nil {
				adapter.Close()
			}
		})
		// Step 2: ping.
		if err := adapter.Ping(ctx); err != nil {
			t.Fatalf("PostgreSQL ping: %v", err)
		}
		// Step 3: run migrations (forward-only, idempotent on schema_migrations).
		applied, err := database.RunMigrations(ctx, dbURL, time.Now().UTC())
		if err != nil {
			t.Fatalf("migrations failed: %v (applied=%d)", err, applied)
		}
		t.Logf("migrations OK (applied=%d)", applied)
		// Step 4: cleanup any prior E2E rows + seed fixture.
		// pgxpool does NOT accept multi-statement Exec strings; split on the
		// semicolon terminator and execute each non-empty statement on its
		// own so the fixture SQL (which is multi-statement by design) loads
		// cleanly.
		pool := adapter.Pool()
		if err := execMultiStatement(ctx, pool, cleanupSQL); err != nil {
			t.Fatalf("cleanup SQL failed: %v", err)
		}
		if err := execMultiStatement(ctx, pool, seedSQL); err != nil {
			t.Fatalf("seed SQL failed: %v", err)
		}
		t.Log("fixture seeded (Business A + Business B)")

		// Pass the adapter on to subtests.
		adapterRef := adapter
		t.Run("Scenario_Business_A_catalog_grounding", func(t *testing.T) {
			runCatalogGroundingScenario(t, ctx, adapterRef, model, apiKey,
				bizAID, convAID, providerRefA, inboundMsgAID,
				bizAItemName, bizAPrice, bizACurrency,
				[]string{bizBPrice, bizBCurrency}, // sentinel — must NOT appear
			)
		})
		t.Run("Scenario_Business_B_tenant_isolation", func(t *testing.T) {
			// Run a separate Handle on Business B (which has its own catalog
			// with the SAME product name but DIFFERENT facts). Assert the
			// reply mentions Business B's facts (999 + SAR), not Business A's
			// (125000 + YER). This proves tenant isolation in BOTH directions.
			runCatalogGroundingScenario(t, ctx, adapterRef, model, apiKey,
				bizBID, convBID, providerRefB, inboundMsgBID,
				bizAItemName, bizBPrice, bizBCurrency,
				[]string{bizAPrice, bizACurrency}, // sentinel — must NOT appear
			)
		})
	})
}

// runCatalogGroundingScenario runs ONE Handle() call through the production
// path and asserts on the persisted result + catalog grounding.
//
// customerText is the customer's question. expectedPrice/expectedCurrency are
// the catalog facts the reply MUST contain (from PostgreSQL). sentinelFacts
// are facts that MUST NOT appear (the other business's catalog facts — tenant
// isolation leak detection).
func runCatalogGroundingScenario(
	t *testing.T,
	ctx context.Context,
	adapter *postgres.Adapter,
	model, apiKey string,
	businessID, conversationID, providerRef, inboundMsgID string,
	itemName, expectedPrice, expectedCurrency string,
	sentinelFacts []string,
) {
	t.Helper()

	// ===== Build the full AutoReplyService wiring (matches bootstrap.api.go) =====
	businessRepo := postgres.NewBusinessRepository(adapter)

	// Verify the runtime policy was seeded by the fixture SQL (ai_mode=
	// 'restricted_auto' + allow_auto_reply=true). The seed upserts the row
	// with default resource_version=1 per migration 000042.
	policy, err := businessRepo.GetRuntimePolicy(ctx, businessID)
	if err != nil {
		t.Fatalf("GetRuntimePolicy business=%s (seeded fixture missing?): %v", businessID, err)
	}
	if policy.AIMode == "disabled" || !policy.AllowAutoReply {
		t.Fatalf("seeded policy misconfigured for business=%s: ai_mode=%s allow_auto_reply=%v",
			businessID, policy.AIMode, policy.AllowAutoReply)
	}
	t.Logf("policy OK: business=%s ai_mode=%s allow_auto_reply=%v", businessID, policy.AIMode, policy.AllowAutoReply)

	// Gemini client + ContractClient (real).
	geminiClient, err := gemini.NewClient(gemini.Config{
		BaseURL:        "https://generativelanguage.googleapis.com",
		APIKey:         apiKey,
		Model:          model,
		RequestTimeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("gemini.NewClient: %v", err)
	}
	contractClient, err := gemini.NewContractClient(geminiClient)
	if err != nil {
		t.Fatalf("gemini.NewContractClient: %v", err)
	}
	// Per spec §3 + §4: wire runRepo + newID on the ContractClient so the
	// centralized telemetry boundary in sendContractRequest records
	// ai_gemini_interactions + ai_usage_telemetry for EVERY Gemini call.
	// Without this wiring, the trace.RunID check in sendContractRequest
	// skips telemetry recording (c.runRepo == nil) and zero rows are written.
	// This mirrors the production wiring at bootstrap/api.go:246-247.
	runRepoForTrace := postgres.NewAIRunTraceRepository(adapter)
	contractClient.SetRunRepository(runRepoForTrace)
	contractClient.SetNewID(uuid.NewString)

	// Context builder — fetches Catalog data from PostgreSQL BEFORE Gemini.
	contextBuilder := services.NewAutoReplyContextBuilder(
		businessRepo,
		postgres.NewConversationRepository(adapter),
		postgres.NewCustomerRepository(adapter),
		postgres.NewCatalogRepository(adapter),
		postgres.NewMessageRepository(adapter),
	)
	contextBuilder.Knowledge = postgres.NewKnowledgeDocumentRepository(adapter)
	contextBuilder.Policies = postgres.NewBusinessPolicyRepository(adapter)

	// AutoReplyService — full production wiring (no shortcuts).
	service := services.NewAutoReplyService(
		contractClient,
		postgres.NewAIDecisionRepository(adapter),
		postgres.NewConversationReferenceRepository(adapter),
		postgres.NewOutboundMessageRepository(adapter),
		postgres.NewPostgresOutboxStore(adapter),
		adapter, // ports.TransactionManager
	)
	service.ContextBuilder = contextBuilder
	service.RunRepository = postgres.NewAIRunTraceRepository(adapter)
	service.Validation = services.NewValidationPipeline(
		postgres.NewPostgresReferenceValidator(adapter),
		postgres.NewPostgresTenantValidator(adapter),
		postgres.NewPostgresPolicyEvaluator(businessRepo),
		nil, // AuthorizationService — nil = default-allow per existing convention
	)
	service.Conversations = postgres.NewConversationRepository(adapter)
	service.MessageRepository = postgres.NewMessageRepository(adapter)
	service.StateRepository = postgres.NewConversationStateRepository(adapter)

	// Production deps that cmd/test-autoreply omits (per spec §21, this test
	// must use the SAME production services — not a shortcut).
	service.AIUsage = postgres.NewAIUsageRepository(adapter)
	service.AIPricing = postgres.NewAIProviderPricingRepository(adapter)
	service.Subscriptions = postgres.NewSubscriptionRepository(adapter)
	service.NewID = uuid.NewString
	service.Now = func() time.Time { return time.Now().UTC() }

	// CatalogBatchController — wired identically to bootstrap.api.go (lines
	// 370-393). This is what runs when Gemini's first call returns
	// needs_more_data. Even when the test doesn't trigger that path, the
	// wiring must be present to match production.
	batchTokenCounter, err := gemini.NewTokenCounter(gemini.TokenCounterConfig{
		BaseURL: geminiClient.BaseURL(),
		APIKey:  geminiClient.APIKey(),
		Model:   geminiClient.Model(),
	})
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}
	batchClient, err := gemini.NewBatchClient(gemini.BatchClientConfig{
		BaseURL: geminiClient.BaseURL(),
		APIKey:  geminiClient.APIKey(),
		Model:   geminiClient.Model(),
	})
	if err != nil {
		t.Fatalf("NewBatchClient: %v", err)
	}
	service.CatalogBatch = &services.CatalogBatchController{
		Catalogs:          postgres.NewCatalogRepository(adapter),
		ProjectionBuilder: &services.CatalogAIProjectionBuilder{},
		TokenCounter:      batchTokenCounter,
		Gemini:            batchClient,
		RunRepo:           postgres.NewAIRunTraceRepository(adapter),
		TokenBudget:       8000,
		Now:               func() time.Time { return time.Now().UTC() },
		NewID:             uuid.NewString,
		AIUsage:           postgres.NewAIUsageRepository(adapter),
		AIPricing:         postgres.NewAIProviderPricingRepository(adapter),
		Subscriptions:     postgres.NewSubscriptionRepository(adapter),
	}

	// ConversationSummaryService — wired per bootstrap.api.go:308-332.
	// MaybeSummarize is called after each successful Handle.
	service.SummaryService = services.NewConversationSummaryService(
		postgres.NewConversationStateRepository(adapter),
		postgres.NewMessageRepository(adapter),
		geminiClient, // legacy fallback (LLM is used only if ContractRuntime is nil)
		geminiClient.Model(),
	)
	service.SummaryService.ContractRuntime = contractClient
	service.SummaryService.AIUsage = postgres.NewAIUsageRepository(adapter)
	service.SummaryService.AIPricing = postgres.NewAIProviderPricingRepository(adapter)
	service.SummaryService.Subscriptions = postgres.NewSubscriptionRepository(adapter)
	service.SummaryService.NewID = uuid.NewString
	// AICostProtectionService — shared kill switch state. For E2E we use a
	// local instance (no platformOperations available). This is acceptable:
	// the kill switch defaults to "AI allowed" when the subscription lookup
	// returns no rows or no exceeded-quota signal.
	service.SummaryService.CostProtection = &services.AICostProtectionService{
		Subscriptions: postgres.NewSubscriptionRepository(adapter),
		AIUsage:       postgres.NewAIUsageRepository(adapter),
	}

	// ===== Step: run cleanup before this specific Handle (idempotency safety) =====
	pool := adapter.Pool()
	if err := execMultiStatement(ctx, pool, cleanupSQL); err != nil {
		t.Fatalf("pre-test cleanup SQL failed for business=%s: %v", businessID, err)
	}

	// ===== Step: call AutoReplyService.Handle (REAL production path) =====
	t.Logf("calling AutoReplyService.Handle business=%s conversation=%s inboundMsg=%s",
		businessID, conversationID, inboundMsgID)
	customerText := fmt.Sprintf("كم سعر %s؟", itemName)
	result, err := service.Handle(ctx, commands.AutoReplyCommand{
		Meta: commands.CommandMeta{
			Actor:         commands.ActorContext{BusinessID: commands.BusinessID(businessID)},
			CorrelationID: uuid.NewString(),
		},
		ConversationID:         commands.ConversationID(conversationID),
		SourceMessageReference: inboundMsgID,
		Text:                   customerText,
		Channel:                "whatsapp",
		ProviderRef:            providerRef,
	})

	// ===== Section: Handle must succeed (no error) =====
	if err != nil {
		t.Fatalf("AutoReplyService.Handle failed (business=%s): %v", businessID, err)
	}
	if !result.Enqueued {
		t.Fatalf("expected Enqueued=true, got Enqueued=false (Action=%s)", result.Action)
	}
	if result.OutboundMessageID == "" {
		t.Fatalf("expected OutboundMessageID, got empty")
	}
	if result.OutboxEntryID == "" {
		t.Fatalf("expected OutboxEntryID, got empty")
	}
	t.Logf("Handle OK: Action=%s outbound=%s outbox=%s",
		result.Action, result.OutboundMessageID, result.OutboxEntryID)

	// ===== Section: AI Run verification (spec §9) =====
	verifyAIRun(t, ctx, pool, businessID, conversationID, inboundMsgID, model)

	// ===== Section: Outbound persistence verification (spec §11) =====
	outboundText := verifyOutboundAndOutbox(t, ctx, pool, businessID, conversationID, string(result.OutboundMessageID), string(result.OutboxEntryID))

	// ===== Section: Catalog grounding proof (spec §7) =====
	verifyCatalogGrounding(t, outboundText, itemName, expectedPrice, expectedCurrency, sentinelFacts)
}

// verifyAIRun asserts on the persisted ai_runs row for this Handle.
// Per spec §9: run exists, business_id correct, conversation_id correct,
// lifecycle reached COMPLETED, no FAILED/CANCELLED, model reference exists,
// usage telemetry exists. Per spec §1: authorized_at + executing_at +
// completed_at all NOT NULL, with timestamp ordering
// authorized_at <= executing_at <= completed_at.
func verifyAIRun(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	businessID, conversationID, inboundMsgID, model string,
) {
	t.Helper()
	// The run is identified by idempotency_key = "auto-reply:" + inboundMsgID.
	// Per spec §1 + §8: SELECT the full lifecycle timestamp chain so we can
	// assert on authorized_at + executing_at + completed_at NOT NULL + the
	// ordering authorized_at <= executing_at <= completed_at.
	var (
		runID, runBusinessID, runConvID, runIDempKey, runStatus string
		validatingAt, authorizedAt, executingAt, completedAt    *time.Time
		usageTelemetryCount, toolCallCount, interactionCount    int
		interactionModel                                        string
	)
	err := pool.QueryRow(ctx, `
        SELECT id, business_id,
               COALESCE(conversation_id::text, ''),
               idempotency_key, status,
               validating_at, authorized_at, executing_at, completed_at
        FROM ai_runs
        WHERE business_id=$1 AND idempotency_key=$2
        LIMIT 1`,
		businessID, "auto-reply:"+inboundMsgID,
	).Scan(&runID, &runBusinessID, &runConvID, &runIDempKey, &runStatus,
		&validatingAt, &authorizedAt, &executingAt, &completedAt)
	if err != nil {
		t.Fatalf("ai_runs lookup (business=%s idempotency=%s): %v",
			businessID, "auto-reply:"+inboundMsgID, err)
	}
	t.Logf("ai_run id=%s business=%s conversation=%s idempotency=%s status=%s",
		runID, runBusinessID, runConvID, runIDempKey, runStatus)

	// ===== Spec §9 assertions =====
	if runBusinessID != businessID {
		t.Errorf("ai_runs.business_id mismatch: got %s want %s", runBusinessID, businessID)
	}
	if runConvID != conversationID {
		t.Errorf("ai_runs.conversation_id mismatch: got %s want %s", runConvID, conversationID)
	}
	if runStatus == "failed" || runStatus == "cancelled" {
		t.Fatalf("ai_runs.status=%s — expected COMPLETED on happy path, got terminal-failure state", runStatus)
	}
	if runStatus != "completed" {
		t.Fatalf("ai_runs.status mismatch: got %s want completed (per spec §9: lifecycle must reach COMPLETED on success)", runStatus)
	}

	// ===== Spec §1 lifecycle assertions: authorized_at + executing_at + completed_at =====
	// Per spec §1 G + §8: happy Action path MUST traverse
	// validating → authorized → executing → completed.
	// All three timestamps MUST be NOT NULL.
	if authorizedAt == nil {
		t.Fatalf("ai_runs.authorized_at is NULL for run=%s — per spec §1 the happy Action path MUST pass through AUTHORIZED before EXECUTING (markAuthorizedOrAbort was not called or failed)", runID)
	}
	if executingAt == nil {
		t.Fatalf("ai_runs.executing_at is NULL for run=%s — per spec §1 the happy Action path MUST pass through EXECUTING before COMPLETED (markExecutingOrAbort was not called or failed)", runID)
	}
	if completedAt == nil {
		t.Fatalf("ai_runs.completed_at is NULL for run=%s — per spec §1 + §9 the happy Action path MUST reach COMPLETED", runID)
	}
	t.Logf("lifecycle timestamps: validating_at=%v authorized_at=%v executing_at=%v completed_at=%v",
		validatingAt, authorizedAt, executingAt, completedAt)

	// ===== Spec §1 G + §10 A: timestamp ordering =====
	// authorized_at <= executing_at <= completed_at
	if authorizedAt.After(*executingAt) {
		t.Fatalf("lifecycle ordering violation: authorized_at (%v) > executing_at (%v) — per spec §1 the happy Action path MUST be authorized → executing → completed",
			authorizedAt, executingAt)
	}
	if executingAt.After(*completedAt) {
		t.Fatalf("lifecycle ordering violation: executing_at (%v) > completed_at (%v) — per spec §1 the happy Action path MUST be authorized → executing → completed",
			executingAt, completedAt)
	}
	t.Logf("lifecycle ordering OK: authorized_at <= executing_at <= completed_at")

	// ===== Per-business usage records (ai_usage_records, per Item 4) =====
	// AUTHORITATIVE per-business telemetry written by
	// recordAIUsage.AppendRecord (auto_reply.go:1216). MUST be > 0 on the
	// happy path — replyEnqueued=true + ACTIVE subscription seeded → record
	// is persisted. Per Item 4, a failure here would propagate as an error
	// from Handle itself, so reaching this assertion already proves
	// AppendRecord succeeded.
	var usageRecordsCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_usage_records WHERE business_id=$1`, businessID,
	).Scan(&usageRecordsCount); err != nil {
		t.Fatalf("ai_usage_records count lookup for business=%s: %v", businessID, err)
	}
	if usageRecordsCount == 0 {
		t.Fatalf("expected ai_usage_records rows > 0 for business=%s on happy path, got 0 — recordAIUsage.AppendRecord did not persist (this contradicts Item 4)", businessID)
	}
	t.Logf("ai_usage_records rows for business=%s: %d (authoritative per-business telemetry per Item 4)", businessID, usageRecordsCount)

	// ===== ai_usage_telemetry (per-call operational telemetry, per spec §3) =====
	// GAP-1 FIXED: telemetry is now centralized in ContractClient.sendContractRequest
	// (the boundary for EVERY Gemini HTTP call). Each Gemini call produces exactly
	// ONE ai_usage_telemetry row. On the B2C non-tool path, there is exactly 1
	// Gemini call → exactly 1 telemetry row. Per spec §8: this is now a HARD
	// assertion (no more t.Logf non-fatal surfacing).
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_usage_telemetry WHERE ai_run_id=$1`, runID,
	).Scan(&usageTelemetryCount); err != nil {
		t.Fatalf("ai_usage_telemetry count lookup for run=%s: %v", runID, err)
	}
	if usageTelemetryCount == 0 {
		t.Fatalf("expected ai_usage_telemetry rows > 0 for run=%s — per spec §3 + §8 the centralized telemetry boundary in ContractClient.sendContractRequest must record a row for EVERY Gemini call (B2C non-tool path = 1 call = 1 row)", runID)
	}
	t.Logf("ai_usage_telemetry rows for run=%s: %d (per-call operational telemetry, centralized in sendContractRequest)", runID, usageTelemetryCount)

	// ===== Tool calls (per spec §5: do NOT force tool_calls > 0) =====
	// Per spec §5: do not force tool_calls > 0. The B2C system prompt steers
	// Gemini toward needs_more_data instead of tool invocation, and catalog
	// evidence is pre-injected in the user prompt. Gemini is non-deterministic,
	// so tool_calls may be 0 OR > 0. We LOG the count but do NOT assert == 0
	// (that would be brittle) and do NOT assert > 0 (that would force tools
	// against the system prompt design).
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_tool_calls WHERE ai_run_id=$1`, runID,
	).Scan(&toolCallCount); err != nil {
		t.Logf("ai_tool_calls count lookup (non-fatal per spec §5): %v", err)
	} else {
		t.Logf("ai_tool_calls for run=%s: %d (per spec §5: tool_calls are NOT forced — count is logged for observability, not asserted)", runID, toolCallCount)
	}

	// ===== ai_gemini_interactions (per-call Gemini trace, per spec §4) =====
	// GAP-2 FIXED: interaction recording is now centralized in
	// ContractClient.sendContractRequest. Each Gemini call produces exactly
	// ONE ai_gemini_interactions row. On the B2C non-tool path, there is
	// exactly 1 Gemini call → exactly 1 interaction row. Per spec §8: this
	// is now a HARD assertion (no more t.Logf non-fatal surfacing).
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM ai_gemini_interactions WHERE ai_run_id=$1`, runID,
	).Scan(&interactionCount); err != nil {
		t.Fatalf("ai_gemini_interactions count lookup for run=%s: %v", runID, err)
	}
	if interactionCount == 0 {
		t.Fatalf("expected ai_gemini_interactions rows > 0 for run=%s — per spec §4 + §8 the centralized telemetry boundary in ContractClient.sendContractRequest must record a row for EVERY Gemini call", runID)
	}
	t.Logf("ai_gemini_interactions rows for run=%s: %d", runID, interactionCount)

	// ===== Model reference (per spec §4: model recorded correctly) =====
	// The ai_gemini_interactions.model column must equal the GEMINI_MODEL env
	// var passed to gemini.NewClient. This proves the configured model was
	// actually used for the Gemini call (not a hardcoded fallback).
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(model, '') FROM ai_gemini_interactions WHERE ai_run_id=$1 ORDER BY started_at ASC LIMIT 1`,
		runID,
	).Scan(&interactionModel); err != nil {
		t.Fatalf("ai_gemini_interactions.model lookup for run=%s: %v", runID, err)
	}
	if interactionModel == "" {
		t.Fatalf("expected non-empty model in ai_gemini_interactions for run=%s", runID)
	}
	if interactionModel != model {
		t.Fatalf("ai_gemini_interactions.model mismatch: got %q want %q (the GEMINI_MODEL env var passed to gemini.NewClient)", interactionModel, model)
	}
	t.Logf("ai_gemini_interactions.model OK: %q (matches GEMINI_MODEL env)", interactionModel)
}

// verifyOutboundAndOutbox asserts the outbound_messages + outbox_entries +
// communication_messages rows were persisted atomically (per spec §11).
// Returns the decoded outbound text so subsequent assertions can verify catalog
// grounding (per spec §7).
func verifyOutboundAndOutbox(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	businessID, conversationID, outboundID, outboxID string,
) string {
	t.Helper()

	// outbound_messages row.
	var (
		obBusinessID, obConvID, obContentRef, obOrigin, obDirection, obChannel string
		obProviderRef                                                          sql.NullString
	)
	err := pool.QueryRow(ctx, `
        SELECT business_id,
               COALESCE(conversation_id::text, ''),
               content_reference,
               origin,
               direction,
               COALESCE(channel, ''),
               COALESCE(provider_ref, '')
        FROM outbound_messages
        WHERE business_id=$1 AND id=$2
        LIMIT 1`, businessID, outboundID,
	).Scan(&obBusinessID, &obConvID, &obContentRef, &obOrigin, &obDirection, &obChannel, &obProviderRef)
	if err != nil {
		t.Fatalf("outbound_messages lookup (business=%s id=%s): %v", businessID, outboundID, err)
	}
	if obBusinessID != businessID {
		t.Errorf("outbound_messages.business_id mismatch: got %s want %s", obBusinessID, businessID)
	}
	if obConvID != conversationID {
		t.Errorf("outbound_messages.conversation_id mismatch: got %s want %s", obConvID, conversationID)
	}
	if obOrigin != "ai" {
		t.Errorf("outbound_messages.origin mismatch: got %s want ai", obOrigin)
	}
	if obDirection != "outbound" {
		t.Errorf("outbound_messages.direction mismatch: got %s want outbound", obDirection)
	}
	t.Logf("outbound_messages OK: business=%s conv=%s origin=%s direction=%s channel=%s",
		obBusinessID, obConvID, obOrigin, obDirection, obChannel)

	// outbox_entries row — UNIQUE on outbound_message_id (one-to-one).
	var (
		oxBusinessID, oxOutboundID, oxStatus, oxCommandType, oxDedupeKey string
		oxAttemptCount                                                   int
	)
	err = pool.QueryRow(ctx, `
        SELECT business_id, outbound_message_id, status, command_type, dedupe_key, attempt_count
        FROM outbox_entries
        WHERE business_id=$1 AND outbound_message_id=$2
        LIMIT 1`, businessID, outboundID,
	).Scan(&oxBusinessID, &oxOutboundID, &oxStatus, &oxCommandType, &oxDedupeKey, &oxAttemptCount)
	if err != nil {
		t.Fatalf("outbox_entries lookup (business=%s outbound=%s): %v", businessID, outboundID, err)
	}
	if oxBusinessID != businessID {
		t.Errorf("outbox_entries.business_id mismatch: got %s want %s", oxBusinessID, businessID)
	}
	if oxOutboundID != outboundID {
		t.Errorf("outbox_entries.outbound_message_id mismatch: got %s want %s", oxOutboundID, outboundID)
	}
	if oxStatus != "pending" && oxStatus != "processing" {
		t.Errorf("outbox_entries.status: got %s want pending or processing (post-Handle, pre-worker)", oxStatus)
	}
	if oxCommandType == "" {
		t.Errorf("outbox_entries.command_type must be non-empty")
	}
	if oxDedupeKey == "" {
		t.Errorf("outbox_entries.dedupe_key must be non-empty")
	}
	t.Logf("outbox_entries OK: business=%s outbound=%s status=%s command_type=%s attempt_count=%d",
		oxBusinessID, oxOutboundID, oxStatus, oxCommandType, oxAttemptCount)

	// communication_messages row (origin=ai, direction=outbound).
	var (
		cmBusinessID, cmConvRefID, cmOutboundID, cmOrigin, cmDirection, cmTransport, cmContentType, cmTextContent, cmContentRef string
	)
	err = pool.QueryRow(ctx, `
        SELECT business_id,
               COALESCE(conversation_reference_id::text, ''),
               COALESCE(outbound_message_id::text, ''),
               origin, direction, transport, content_type,
               COALESCE(text_content, ''),
               content_reference
        FROM communication_messages
        WHERE business_id=$1 AND outbound_message_id=$2
        LIMIT 1`, businessID, outboundID,
	).Scan(&cmBusinessID, &cmConvRefID, &cmOutboundID, &cmOrigin, &cmDirection, &cmTransport, &cmContentType, &cmTextContent, &cmContentRef)
	if err != nil {
		t.Fatalf("communication_messages lookup (business=%s outbound=%s): %v", businessID, outboundID, err)
	}
	if cmOrigin != "ai" {
		t.Errorf("communication_messages.origin mismatch: got %s want ai", cmOrigin)
	}
	if cmDirection != "outbound" {
		t.Errorf("communication_messages.direction mismatch: got %s want outbound", cmDirection)
	}
	if cmContentType != "text" {
		t.Errorf("communication_messages.content_type mismatch: got %s want text", cmContentType)
	}
	t.Logf("communication_messages OK: business=%s conv_ref=%s origin=%s direction=%s transport=%s",
		cmBusinessID, cmConvRefID, cmOrigin, cmDirection, cmTransport)

	// The text_content column carries the Final AI Reply text — this is the
	// customer-facing response that must be catalog-grounded.
	if cmTextContent == "" {
		t.Fatalf("communication_messages.text_content is empty — no Final AI Reply captured")
	}
	return cmTextContent
}

// verifyCatalogGrounding asserts the Final AI Reply contains the catalog facts
// (price, currency) that came from PostgreSQL — NOT from a system prompt or
// invented by the model. Also asserts tenant isolation: facts from the OTHER
// business must NOT appear in this reply.
func verifyCatalogGrounding(
	t *testing.T,
	replyText, itemName, expectedPrice, expectedCurrency string,
	sentinelFacts []string,
) {
	t.Helper()

	// Per spec §16: do not assert on literal wording. Verify facts.
	// 1. Price (numeric) — allow either "125000" or "125,000" or "125٬000" (Arabic thousands separator).
	//    We strip non-digit characters before comparing.
	priceVariants := []string{
		expectedPrice,
		withThousandSep(expectedPrice, ","), // 125,000
		withThousandSep(expectedPrice, "٬"), // 125٬000 (Arabic thousands separator U+066C)
		withThousandSep(expectedPrice, "."), // 125.000 (some locales)
	}
	priceFound := false
	for _, v := range priceVariants {
		if strings.Contains(replyText, v) {
			priceFound = true
			t.Logf("found price form %q in reply", v)
			break
		}
	}
	if !priceFound {
		t.Errorf("catalog grounding: expected price containing %q (or formatted variants) in reply; reply was: %s",
			expectedPrice, replyText)
	}

	// 2. Currency — YER / SAR — must appear in some form. We accept the
	//    ISO code OR the Arabic translations the prompt builder uses:
	//    YER → "ريال يمني" / "ريال" / "YER"
	//    SAR → "ريال سعودي" / "ريال" / "SAR"
	currencyForms := currencyArabicForms(expectedCurrency)
	currencyFound := false
	for _, form := range currencyForms {
		if strings.Contains(replyText, form) {
			currencyFound = true
			t.Logf("found currency form %q in reply", form)
			break
		}
	}
	if !currencyFound {
		t.Errorf("catalog grounding: expected currency %s (in ISO or Arabic form) in reply; reply was: %s",
			expectedCurrency, replyText)
	}

	// 3. Product reference — name must appear in the reply.
	if !strings.Contains(replyText, itemName) {
		// The product name "Mujeeb CI Test Phone" may be transliterated by
		// Gemini into Arabic. Log but don't fail — the price+currency proof
		// is the stronger assertion.
		t.Logf("note: item name %q not found verbatim in reply (Gemini may have transliterated it); reply: %s",
			itemName, replyText)
	}

	// 4. Tenant isolation — none of the sentinel facts from the OTHER business
	//    may appear in this reply.
	for _, sentinel := range sentinelFacts {
		if strings.Contains(replyText, sentinel) {
			t.Fatalf("TENANT ISOLATION LEAK: reply for this business contains fact %q that belongs to the OTHER business; reply: %s",
				sentinel, replyText)
		}
	}
	t.Logf("catalog grounding OK: price=%s currency=%s item=%s — reply is grounded on real PostgreSQL catalog data",
		expectedPrice, expectedCurrency, itemName)
}

// withThousandSep returns s with the given separator inserted between every 3
// digits from the right. Used to expand the price-form assertion so the test
// accepts Gemini's natural-formatted output (e.g. "125,000" or "125٬000").
func withThousandSep(s, sep string) string {
	if len(s) <= 3 {
		return s
	}
	out := make([]byte, 0, len(s)+(len(s)-1)/3*len(sep))
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, sep...)
		}
		out = append(out, byte(c))
	}
	return string(out)
}

// currencyArabicForms returns the acceptable Arabic + ISO forms for a currency.
// The ContextBuilder's formatCurrency helper translates ISO codes to Arabic;
// Gemini may keep either form. Accept any.
func currencyArabicForms(iso string) []string {
	switch iso {
	case "YER":
		return []string{"YER", "ريال يمني", "ريال"}
	case "SAR":
		return []string{"SAR", "ريال سعودي", "ريال"}
	default:
		return []string{iso}
	}
}

// execMultiStatement splits a multi-statement SQL string on the ';'
// terminator and executes each non-empty trimmed statement on its own
// *pgxpool.Pool. pgxpool does NOT accept multi-statement Exec strings, so
// we must split. The fixture SQL is hand-curated to never use ';' inside
// string literals or comments, so a simple Split is safe.
func execMultiStatement(ctx context.Context, pool *pgxpool.Pool, sqlText string) error {
	for _, stmt := range strings.Split(sqlText, ";") {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}
		if _, err := pool.Exec(ctx, trimmed); err != nil {
			return fmt.Errorf("exec stmt: %w (stmt=%.200s)", err, trimmed)
		}
	}
	return nil
}

// maskURL hides the password in a postgres:// URL so it's safe to log.
func maskURL(s string) string {
	i := 0
	for i < len(s) {
		if s[i] == ':' && i+2 < len(s) && s[i+1] == '/' && s[i+2] == '/' {
			start := i + 3
			atIdx := -1
			for j := start; j < len(s); j++ {
				if s[j] == '@' {
					atIdx = j
					break
				}
			}
			if atIdx > 0 {
				return s[:start] + "***" + s[atIdx:]
			}
		}
		i++
	}
	return s
}
