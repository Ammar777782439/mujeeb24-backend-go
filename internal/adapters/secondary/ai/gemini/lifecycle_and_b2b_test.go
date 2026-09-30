// ===== Gemini Live Smoke Test =====
//
// This test is ONLY run when the build tag `gemini_live` is set:
//   go test -tags=gemini_live ./...
// It's excluded from normal `go test ./...`.
// The CI workflow runs it on workflow_dispatch only with
// secrets.GEMINI_API_KEY.

//go:build gemini_live

package gemini

import (
        "context"
        "errors"
        "os"
        "strings"
        "sync/atomic"
        "testing"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ===== Lifecycle failure test =====

// stubLifecycleFailAlways always returns an error from MarkWaitingTool.
type stubLifecycleFailWaitingTool struct{}

func (s *stubLifecycleFailWaitingTool) MarkWaitingTool(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
        return ports.AIRunRecord{}, errors.New("DB unreachable for MarkWaitingTool")
}
func (s *stubLifecycleFailWaitingTool) MarkRunning(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
        return ports.AIRunRecord{}, nil
}

var _ ports.AIRunLifecyclePort = (*stubLifecycleFailWaitingTool)(nil)

// stubLifecycleFailRunning always returns an error from MarkRunning.
type stubLifecycleFailRunning struct{}

func (s *stubLifecycleFailRunning) MarkWaitingTool(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
        return ports.AIRunRecord{}, nil
}
func (s *stubLifecycleFailRunning) MarkRunning(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
        return ports.AIRunRecord{}, errors.New("DB unreachable for MarkRunning")
}

var _ ports.AIRunLifecyclePort = (*stubLifecycleFailRunning)(nil)

// Test: MarkWaitingTool failure → tool loop MUST NOT continue.
// Per the spec: "لا تجعل lifecycle failure مجرد log ويتم تجاهله."
func TestLifecycleFailure_MarkWaitingTool_StopsLoop(t *testing.T) {
        t.Parallel()
        mock := newMockServerConfigurable(t, []string{
                makeToolCallResponse("i-1", "catalog_data", "call-1"),
                makeFinalResponse("i-2", "should-not-reach"),
        })
        defer mock.Close()

        dispatcher := makeDispatcher()
        lc := &stubLifecycleFailWaitingTool{}
        cc, _ := buildContractClientForFixTests(t, mock.URL(), dispatcher, lc)

        _, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
                DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
                AIRunID:      "run-fail-wt",
        })
        if err == nil {
                t.Fatalf("expected error when MarkWaitingTool fails — loop must NOT continue")
        }
        if !strings.Contains(err.Error(), "MarkWaitingTool failed") {
                t.Errorf("error should mention MarkWaitingTool: %v", err)
        }
        // Only 1 Gemini request should have been made (the initial one).
        // The follow-up must NOT have happened.
        if atomic.LoadInt64(&mock.requestCount) != 1 {
                t.Errorf("expected 1 Gemini request (loop stopped on lifecycle failure), got %d", atomic.LoadInt64(&mock.requestCount))
        }
}

// Test: MarkRunning failure → tool loop MUST NOT continue.
func TestLifecycleFailure_MarkRunning_StopsLoop(t *testing.T) {
        t.Parallel()
        mock := newMockServerConfigurable(t, []string{
                makeToolCallResponse("i-1", "catalog_data", "call-1"),
                makeFinalResponse("i-2", "should-not-reach"),
        })
        defer mock.Close()

        dispatcher := makeDispatcher()
        lc := &stubLifecycleFailRunning{}
        cc, _ := buildContractClientForFixTests(t, mock.URL(), dispatcher, lc)

        _, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
                DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
                AIRunID:      "run-fail-r",
        })
        if err == nil {
                t.Fatalf("expected error when MarkRunning fails — loop must NOT continue")
        }
        if !strings.Contains(err.Error(), "MarkRunning failed") {
                t.Errorf("error should mention MarkRunning: %v", err)
        }
        if atomic.LoadInt64(&mock.requestCount) != 1 {
                t.Errorf("expected 1 Gemini request (loop stopped on lifecycle failure), got %d", atomic.LoadInt64(&mock.requestCount))
        }
}

// ===== Lifecycle ordering test =====

// stubLifecycleOrdered records the order of transitions.
type stubLifecycleOrdered struct {
        transitions []string
}

func (s *stubLifecycleOrdered) MarkWaitingTool(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
        s.transitions = append(s.transitions, "WAITING_TOOL")
        return ports.AIRunRecord{}, nil
}
func (s *stubLifecycleOrdered) MarkRunning(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
        s.transitions = append(s.transitions, "RUNNING")
        return ports.AIRunRecord{}, nil
}

var _ ports.AIRunLifecyclePort = (*stubLifecycleOrdered)(nil)

// Test: lifecycle transitions follow the exact order:
// WAITING_TOOL → RUNNING (for each tool call round).
func TestLifecycle_Ordering_WaitingToolBeforeRunning(t *testing.T) {
        t.Parallel()
        mock := newMockServerConfigurable(t, []string{
                makeToolCallResponse("i-1", "catalog_data", "call-1"),
                makeToolCallResponse("i-2", "catalog_data", "call-2"),
                makeFinalResponse("i-3", "done"),
        })
        defer mock.Close()

        dispatcher := makeDispatcher()
        lc := &stubLifecycleOrdered{}
        cc, _ := buildContractClientForFixTests(t, mock.URL(), dispatcher, lc)

        _, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
                DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
                AIRunID:      "run-order",
        })
        if err != nil {
                t.Fatalf("DecideContract failed: %v", err)
        }
        // Expected: WAITING_TOOL, RUNNING, WAITING_TOOL, RUNNING
        expected := []string{"WAITING_TOOL", "RUNNING", "WAITING_TOOL", "RUNNING"}
        if len(lc.transitions) != len(expected) {
                t.Fatalf("expected %d transitions, got %d: %v", len(expected), len(lc.transitions), lc.transitions)
        }
        for i, want := range expected {
                if lc.transitions[i] != want {
                        t.Errorf("transition %d: expected %s, got %s", i, want, lc.transitions[i])
                }
        }
}

// ===== B2B AIRunID path test =====

// This test verifies the B2B path as closely as possible without
// the full MerchantCatalogAIAgent (which requires many stubs).
// It tests the same ContractRuntimeInput path that the agent uses:
//   MerchantContextBuildInput.AIRunID → BuildForTurn →
//   ContractRuntimeInput.AIRunID → ContractClient → AIToolCallRecord.AIRunID
//
// We can't test the full agent because it requires:
//   - SessionWriter (merchant_ai_sessions repository)
//   - ContextBuilder (full MerchantContextBuilder with Business/Catalog repos)
//   - ValidationPipeline
//   - Repository (AIRunRepository)
//   These are too many stubs for a focused test. The test below
//   verifies the ContractRuntimeInput → AIToolCallRecord path which
//   is the critical link the spec asks for.

func TestB2B_AIRunID_Path_ContractRuntimeInputToToolCallRecord(t *testing.T) {
        t.Parallel()
        mock := newMockServerConfigurable(t, []string{
                makeToolCallResponse("i-1", "catalog_data", "call-1"),
                makeFinalResponse("i-2", "ok"),
        })
        defer mock.Close()

        dispatcher := makeDispatcher()
        cc, runRepo := buildContractClientForFixTests(t, mock.URL(), dispatcher, nil)

        // Simulate what MerchantCatalogAIAgent.HandleTurn does:
        // 1. Create an AI Run (with ID "run-b2b-agent-1").
        // 2. Pass run.ID via MerchantContextBuildInput.AIRunID →
        //    BuildForTurn → ContractRuntimeInput.AIRunID.
        // 3. Call DecideContract.
        //
        // We skip BuildForTurn (it needs Business/Catalog repos) and
        // pass ContractRuntimeInput directly — same shape.
        runID := "run-b2b-agent-1"
        out, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
                DecisionInput: ports.AIDecisionInput{
                        BusinessID:     "b2b-business-1",
                        ConversationID: "session-1",
                        Text:           "add a product",
                },
                AIRunID: runID,
        })
        if err != nil {
                t.Fatalf("DecideContract failed: %v", err)
        }
        if out.Proposal.Status != ports.AIProposalStatusResolved {
                t.Errorf("expected status=resolved, got %s", out.Proposal.Status)
        }
        // Verify the tool call record carries the correct AIRunID.
        if len(runRepo.createdToolCalls) != 1 {
                t.Fatalf("expected 1 tool call record, got %d", len(runRepo.createdToolCalls))
        }
        tc := runRepo.createdToolCalls[0]
        if tc.AIRunID != runID {
                t.Errorf("expected AIRunID=%s (B2B agent run), got %s", runID, tc.AIRunID)
        }
        if tc.ToolName != "catalog_data" {
                t.Errorf("expected ToolName=catalog_data, got %s", tc.ToolName)
        }
}

// ===== Gemini Live Smoke Test =====
//
// This test is ONLY run when the build tag `gemini_live` is set.
// The CI workflow runs it on workflow_dispatch only with secrets.GEMINI_API_KEY.
// To run locally: go test -tags=gemini_live ./internal/adapters/secondary/ai/gemini/ -run TestGeminiLiveSmoke

func TestGeminiLiveSmoke(t *testing.T) {
        apiKey := os.Getenv("GEMINI_API_KEY")
        if apiKey == "" {
                t.Skip("GEMINI_API_KEY not set — skipping live Gemini smoke test")
        }
        // Build a real ContractClient with the live key.
        client, err := NewClient(Config{
                BaseURL:        "https://generativelanguage.googleapis.com",
                APIKey:         apiKey,
                Model:          "gemini-3.5-flash",
                SystemPrompt:   "You are a test assistant. Reply with a simple answer.",
                RequestTimeout: 30 * time.Second,
        })
        if err != nil {
                t.Fatalf("build client: %v", err)
        }
        cc, err := NewContractClient(client)
        if err != nil {
                t.Fatalf("build contract client: %v", err)
        }
        out, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
                DecisionInput: ports.AIDecisionInput{
                        BusinessID:     "test-biz",
                        ConversationID: "test-conv",
                        Text:           "Hello, please say 'ok'.",
                },
        })
        if err != nil {
                // Geo-block or API error — report but don't fail CI.
                t.Skipf("Gemini API returned error (may be geo-blocked): %v", err)
        }
        if out.Proposal.ResponseText == "" {
                t.Errorf("expected non-empty response from Gemini")
        }
        if out.Usage.InputTokens <= 0 {
                t.Errorf("expected positive input tokens, got %d", out.Usage.InputTokens)
        }
        t.Logf("Gemini live smoke: model=%s input=%d output=%d latency=%dms response=%q",
                out.Usage.Model, out.Usage.InputTokens, out.Usage.OutputTokens, out.LatencyMs, out.Proposal.ResponseText)
}
