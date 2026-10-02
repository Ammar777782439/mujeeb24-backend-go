package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// ===== Test Infrastructure =====

// mockGeminiToolServer is a configurable httptest.Server that emulates
// the Gemini API with function calling support. It returns
// functionCall parts on the first request + a final structured
// proposal on the second request.
type mockGeminiToolServer struct {
	t            *testing.T
	requestCount int64
	// firstResponse: the response with functionCall parts.
	firstResponse string
	// secondResponse: the response with the final structured proposal.
	secondResponse string
	// allResponses: if set, responses are indexed by request count.
	allResponses []string
	// capturedAPIKey: the x-goog-api-key header from the last request.
	capturedAPIKey string
	// capturedBody: the raw body of the last request (for inspection).
	capturedBody []byte
	server       *httptest.Server
}

func newMockGeminiToolServer(t *testing.T) *mockGeminiToolServer {
	m := &mockGeminiToolServer{t: t}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockGeminiToolServer) Close()      { m.server.Close() }
func (m *mockGeminiToolServer) URL() string { return m.server.URL }

func (m *mockGeminiToolServer) handle(w http.ResponseWriter, r *http.Request) {
	m.capturedAPIKey = r.Header.Get("x-goog-api-key")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		m.t.Fatalf("read request body: %v", err)
	}
	m.capturedBody = body

	count := atomic.AddInt64(&m.requestCount, 1)

	var response string
	if len(m.allResponses) > 0 {
		idx := int(count) - 1
		if idx < len(m.allResponses) {
			response = m.allResponses[idx]
		} else {
			response = m.allResponses[len(m.allResponses)-1]
		}
	} else if count == 1 {
		response = m.firstResponse
	} else {
		response = m.secondResponse
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

// stubCapabilityDispatcher is a minimal CustomerSalesToolPort for tests.
type stubCapabilityDispatcher struct {
	definitions []ports.CustomerSalesToolDefinition
	executeFn   func(ctx context.Context, execCtx ports.CustomerSalesToolExecutionContext, name string, rawParams []byte) (ports.CustomerSalesToolResult, error)
	execCount   int64
	lastExecCtx ports.CustomerSalesToolExecutionContext
}

func (s *stubCapabilityDispatcher) Definitions() []ports.CustomerSalesToolDefinition {
	return s.definitions
}

func (s *stubCapabilityDispatcher) Execute(ctx context.Context, execCtx ports.CustomerSalesToolExecutionContext, name string, rawParams []byte) (ports.CustomerSalesToolResult, error) {
	atomic.AddInt64(&s.execCount, 1)
	s.lastExecCtx = execCtx
	if s.executeFn != nil {
		return s.executeFn(ctx, execCtx, name, rawParams)
	}
	return ports.CustomerSalesToolResult{Data: map[string]any{"status": "ok"}}, nil
}

// stubRunRepoForTools is a minimal AIRunRepository that records tool calls.
type stubRunRepoForTools struct {
	createdToolCalls []ports.AIToolCallRecord
	updatedToolCalls map[string]ports.AIToolCallPatch
}

func newStubRunRepoForTools() *stubRunRepoForTools {
	return &stubRunRepoForTools{updatedToolCalls: make(map[string]ports.AIToolCallPatch)}
}

func (s *stubRunRepoForTools) CreateToolCall(_ context.Context, call ports.AIToolCallRecord) (ports.AIToolCallRecord, error) {
	s.createdToolCalls = append(s.createdToolCalls, call)
	return call, nil
}
func (s *stubRunRepoForTools) UpdateToolCall(_ context.Context, callID string, patch ports.AIToolCallPatch) (ports.AIToolCallRecord, error) {
	s.updatedToolCalls[callID] = patch
	return ports.AIToolCallRecord{ID: callID}, nil
}

// Stubs for the rest of AIRunRepository (not used in tool loop tests):
func (s *stubRunRepoForTools) CreateRun(_ context.Context, _ ports.AIRunRecord) (ports.AIRunRecord, error) {
	return ports.AIRunRecord{}, nil
}
func (s *stubRunRepoForTools) GetRun(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
	return ports.AIRunRecord{}, nil
}
func (s *stubRunRepoForTools) GetByIdempotencyKey(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
	return ports.AIRunRecord{}, nil
}
func (s *stubRunRepoForTools) UpdateRunStatus(_ context.Context, _, _ string, _ ports.AIRunStatusPatch) (ports.AIRunRecord, error) {
	return ports.AIRunRecord{}, nil
}
func (s *stubRunRepoForTools) ListRunsByConversation(_ context.Context, _, _ string, _ int) ([]ports.AIRunRecord, error) {
	return nil, nil
}
func (s *stubRunRepoForTools) CreateAttempt(_ context.Context, _ ports.AIRunAttemptRecord) (ports.AIRunAttemptRecord, error) {
	return ports.AIRunAttemptRecord{}, nil
}
func (s *stubRunRepoForTools) UpdateAttempt(_ context.Context, _ string, _ ports.AIRunAttemptPatch) (ports.AIRunAttemptRecord, error) {
	return ports.AIRunAttemptRecord{}, nil
}
func (s *stubRunRepoForTools) ListAttempts(_ context.Context, _ string) ([]ports.AIRunAttemptRecord, error) {
	return nil, nil
}
func (s *stubRunRepoForTools) ListToolCalls(_ context.Context, _ string) ([]ports.AIToolCallRecord, error) {
	return s.createdToolCalls, nil
}
func (s *stubRunRepoForTools) CreateGeminiInteraction(_ context.Context, _ ports.AIGeminiInteractionRecord) (ports.AIGeminiInteractionRecord, error) {
	return ports.AIGeminiInteractionRecord{}, nil
}
func (s *stubRunRepoForTools) ListGeminiInteractions(_ context.Context, _ string) ([]ports.AIGeminiInteractionRecord, error) {
	return nil, nil
}
func (s *stubRunRepoForTools) CreateCatalogBatch(_ context.Context, _ ports.AICatalogBatchRecord) (ports.AICatalogBatchRecord, error) {
	return ports.AICatalogBatchRecord{}, nil
}
func (s *stubRunRepoForTools) UpdateCatalogBatch(_ context.Context, _ string, _ ports.AICatalogBatchPatch) (ports.AICatalogBatchRecord, error) {
	return ports.AICatalogBatchRecord{}, nil
}
func (s *stubRunRepoForTools) ListCatalogBatches(_ context.Context, _ string) ([]ports.AICatalogBatchRecord, error) {
	return nil, nil
}
func (s *stubRunRepoForTools) RecordUsage(_ context.Context, _ ports.AIUsageTelemetryRecord) (ports.AIUsageTelemetryRecord, error) {
	return ports.AIUsageTelemetryRecord{}, nil
}
func (s *stubRunRepoForTools) RecordStageLatency(_ context.Context, _ ports.AIStageLatencyRecord) (ports.AIStageLatencyRecord, error) {
	return ports.AIStageLatencyRecord{}, nil
}

var _ ports.AIRunRepository = (*stubRunRepoForTools)(nil)

// buildGeminiCustomerSalesAdapterWithTools builds a GeminiCustomerSalesAdapter wired with
// a mock Gemini server + stub capability dispatcher + stub run repo.
func buildGeminiCustomerSalesAdapterWithTools(t *testing.T, mock *mockGeminiToolServer, dispatcher ports.CustomerSalesToolPort) (*GeminiCustomerSalesAdapter, *stubRunRepoForTools) {
	client, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{
		BaseURL:        mock.URL(),
		APIKey:         "test-key",
		Model:          "gemini-3.5-flash",
		RequestTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	cc, err := NewGeminiCustomerSalesAdapter(client, dispatcher)
	if err != nil {
		t.Fatalf("build contract client: %v", err)
	}
	runRepo := newStubRunRepoForTools()
	cc.SetRunRepository(runRepo)
	cc.SetNewID(uuid.NewString)
	return cc, runRepo
}

// ===== Tests =====

// Test 1-4: Gemini requests catalog_data → Mujeeb executes → FunctionResponse → final proposal.
// Also proves: multiple tool calls in one decision, and usage accumulation.
func TestToolLoop_GeminiRequestsCapability_MujeebExecutes_FinalProposalReturned(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	// First response: Gemini calls catalog_data.
	mock.firstResponse = `{
                "interactionId": "interaction-A",
                "candidates": [{
                        "content": {
                                "role": "model",
                                "parts": [{
                                        "functionCall": {
                                                "name": "catalog_data",
                                                "args": {"operation": "list_catalogs"},
                                                "id": "call-1"
                                        }
                                }]
                        }
                }],
                "usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 10, "cachedContentTokenCount": 0}
        }`

	// Second response: Gemini returns a final structured proposal.
	mock.secondResponse = `{
                "interactionId": "interaction-B",
                "candidates": [{
                        "content": {
                                "role": "model",
                                "parts": [{"text": "{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"found it\"}"}]
                        }
                }],
                "usageMetadata": {"promptTokenCount": 200, "candidatesTokenCount": 20, "cachedContentTokenCount": 5}
        }`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Retrieve catalog data", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, runRepo := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	out, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{
			BusinessID:     "trusted-business-1",
			ConversationID: "conv-1",
			Text:           "what products do you have?",
		},
		AIRunID: "run-1",
	})

	// Test 4: Final proposal is returned.
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if out.Proposal.Status != "resolved" {
		t.Errorf("expected status=resolved, got %s", out.Proposal.Status)
	}
	if out.Proposal.ResponseText != "found it" {
		t.Errorf("expected response_text='found it', got %s", out.Proposal.ResponseText)
	}

	// Test 1: Gemini requested catalog_data.
	if atomic.LoadInt64(&dispatcher.execCount) != 1 {
		t.Errorf("expected 1 capability execution, got %d", atomic.LoadInt64(&dispatcher.execCount))
	}

	// Test 2: Mujeeb executed the capability — tool call record persisted.
	if len(runRepo.createdToolCalls) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(runRepo.createdToolCalls))
	}
	tc := runRepo.createdToolCalls[0]
	if tc.ToolName != "catalog_data" {
		t.Errorf("expected tool name=catalog_data, got %s", tc.ToolName)
	}

	// Test 3: FunctionResponse returned to Gemini (verified by the mock
	// receiving a second request — if the tool response wasn't sent,
	// Gemini would not have returned a final proposal).
	if atomic.LoadInt64(&mock.requestCount) != 2 {
		t.Errorf("expected 2 Gemini requests (initial + follow-up after tool), got %d", atomic.LoadInt64(&mock.requestCount))
	}

	// Test 9: Usage is accumulated across all Gemini requests.
	// First request: 100 input, 10 output, 0 cached.
	// Second request: 200 input, 20 output, 5 cached.
	// Total: 300 input, 30 output, 5 cached.
	if out.Usage.InputTokens != 300 {
		t.Errorf("expected total InputTokens=300, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 30 {
		t.Errorf("expected total OutputTokens=30, got %d", out.Usage.OutputTokens)
	}
	if out.Usage.CachedTokens != 5 {
		t.Errorf("expected total CachedTokens=5, got %d", out.Usage.CachedTokens)
	}

	// Test 7: Tenant isolation — BusinessID comes from the trusted caller,
	// NOT from Gemini's function call args.
	if dispatcher.lastExecCtx.BusinessID != "trusted-business-1" {
		t.Errorf("expected BusinessID=trusted-business-1 (from caller), got %s", dispatcher.lastExecCtx.BusinessID)
	}
}

// Test 5: Multiple tool calls in one decision (two sequential tool calls).
func TestToolLoop_MultipleToolCallsInOneDecision(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	// Three responses: first tool call → second tool call → final proposal.
	mock.allResponses = []string{
		// Response 1: Gemini calls catalog_data (list_catalogs).
		`{"interactionId":"i-1","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalogs"},"id":"c1"}}]}}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":5}}`,
		// Response 2: Gemini calls catalog_data (list_catalog_items).
		`{"interactionId":"i-2","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalog_items"},"id":"c2"}}]}}],"usageMetadata":{"promptTokenCount":60,"candidatesTokenCount":6}}`,
		// Response 3: Final structured proposal.
		`{"interactionId":"i-3","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"done\"}"}]}}],"usageMetadata":{"promptTokenCount":70,"candidatesTokenCount":7}}`,
	}

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog data", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, runRepo := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	out, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{
			BusinessID:     "b-1",
			ConversationID: "c-1",
			Text:           "show me products",
		},
		AIRunID: "run-multi",
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if out.Proposal.ResponseText != "done" {
		t.Errorf("expected response='done', got %s", out.Proposal.ResponseText)
	}

	// Two tool calls executed.
	if atomic.LoadInt64(&dispatcher.execCount) != 2 {
		t.Errorf("expected 2 capability executions, got %d", atomic.LoadInt64(&dispatcher.execCount))
	}
	// Two tool call records persisted.
	if len(runRepo.createdToolCalls) != 2 {
		t.Errorf("expected 2 tool call records, got %d", len(runRepo.createdToolCalls))
	}
	// Three Gemini requests (initial + 2 follow-ups).
	if atomic.LoadInt64(&mock.requestCount) != 3 {
		t.Errorf("expected 3 Gemini requests, got %d", atomic.LoadInt64(&mock.requestCount))
	}
	// Usage accumulated: 50+60+70=180 input, 5+6+7=18 output.
	if out.Usage.InputTokens != 180 {
		t.Errorf("expected total InputTokens=180, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 18 {
		t.Errorf("expected total OutputTokens=18, got %d", out.Usage.OutputTokens)
	}
}

// Test 6 + 10: Tool call record saved + updated; tool failure doesn't
// break lifecycle incorrectly.
func TestToolLoop_ToolCallRecordSavedAndUpdated(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	mock.firstResponse = `{
                "interactionId": "i-1",
                "candidates": [{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalogs"},"id":"call-1"}}]}}],
                "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
        }`
	mock.secondResponse = `{
                "interactionId": "i-2",
                "candidates": [{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"ok\"}"}]}}],
                "usageMetadata": {"promptTokenCount": 20, "candidatesTokenCount": 10}
        }`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, runRepo := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-save",
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	// Tool call was created.
	if len(runRepo.createdToolCalls) != 1 {
		t.Fatalf("expected 1 created tool call, got %d", len(runRepo.createdToolCalls))
	}
	tc := runRepo.createdToolCalls[0]
	if tc.AIRunID != "run-save" {
		t.Errorf("expected AIRunID=run-save, got %s", tc.AIRunID)
	}
	if tc.ToolName != "catalog_data" {
		t.Errorf("expected ToolName=catalog_data, got %s", tc.ToolName)
	}
	if tc.Status != "running" {
		t.Errorf("expected initial Status=running, got %s", tc.Status)
	}

	// Tool call was updated (status=completed).
	patch, ok := runRepo.updatedToolCalls[tc.ID]
	if !ok {
		t.Fatalf("expected tool call %s to be updated, but no update found", tc.ID)
	}
	if patch.Status != "completed" {
		t.Errorf("expected updated Status=completed, got %s", patch.Status)
	}
	if patch.FinishedAt == nil {
		t.Errorf("expected FinishedAt to be set")
	}
	if patch.LatencyMs == nil {
		t.Errorf("expected LatencyMs to be set")
	}
}

// Test 10: Tool failure returns error (doesn't silently continue).
func TestToolLoop_ToolFailureReturnsError(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	mock.firstResponse = `{
                "interactionId": "i-1",
                "candidates": [{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{},"id":"c1"}}]}}],
                "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
        }`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
		executeFn: func(_ context.Context, _ ports.CustomerSalesToolExecutionContext, _ string, _ []byte) (ports.CustomerSalesToolResult, error) {
			return ports.CustomerSalesToolResult{}, errors.New("capability DB unreachable")
		},
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-fail",
	})
	if err == nil {
		t.Fatalf("expected error when tool execution fails")
	}
	if !strings.Contains(err.Error(), "catalog_data") {
		t.Errorf("error should mention the tool name: %v", err)
	}
	if !strings.Contains(err.Error(), "non-retryable") {
		t.Errorf("error should mention non-retryable: %v", err)
	}
}

// Test 7 (explicit): Tenant isolation — Gemini sends business_id in args,
// but it's ignored. The trusted BusinessID from the caller is used.
func TestToolLoop_TenantIsolation_GeminiBusinessIDIgnored(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	// Gemini sends business_id="another-business" in the function call args.
	mock.firstResponse = `{
                "interactionId": "i-1",
                "candidates": [{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{"business_id":"another-business","operation":"list_catalogs"},"id":"c1"}}]}}],
                "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
        }`
	mock.secondResponse = `{
                "interactionId": "i-2",
                "candidates": [{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"ok\"}"}]}}],
                "usageMetadata": {"promptTokenCount": 20, "candidatesTokenCount": 10}
        }`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "trusted-biz", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-tenant",
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	// The capability's execution context should have BusinessID from the
	// CALLER (trusted-biz), NOT from Gemini's args (another-business).
	if dispatcher.lastExecCtx.BusinessID != "trusted-biz" {
		t.Errorf("tenant isolation violated: expected BusinessID=trusted-biz, got %s", dispatcher.lastExecCtx.BusinessID)
	}
}

// Test: No tools configured → backward-compatible single-request path.
func TestToolLoop_NoTools_BackwardCompatible(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	// No function call — just a direct structured proposal.
	mock.firstResponse = `{
                "interactionId": "i-1",
                "candidates": [{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"hello\"}"}]}}],
                "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5}
        }`
	mock.secondResponse = "{}" // should never be reached

	// No capabilities configured.
	dispatcher := &stubCapabilityDispatcher{
		definitions: nil,
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	out, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if out.Proposal.ResponseText != "hello" {
		t.Errorf("expected response='hello', got %s", out.Proposal.ResponseText)
	}
	// Only 1 Gemini request (no tool loop).
	if atomic.LoadInt64(&mock.requestCount) != 1 {
		t.Errorf("expected 1 Gemini request (no tools), got %d", atomic.LoadInt64(&mock.requestCount))
	}
}

// Test: Context deadline stops the loop.
func TestToolLoop_ContextDeadlineStopsLoop(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	// Gemini always returns a function call (infinite loop without deadline).
	mock.allResponses = nil
	mock.firstResponse = `{"interactionId":"i-1","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{},"id":"c1"}}]}}],"usageMetadata":{"promptTokenCount":10}}`
	mock.secondResponse = mock.firstResponse // keep looping

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	// Short deadline — should stop the loop.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := cc.Decide(ctx, ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-timeout",
	})
	if err == nil {
		t.Fatalf("expected error on context deadline")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("error should mention context: %v", err)
	}
}

// Test: Tools are extracted from Capabilities().Definitions().
func TestToolLoop_ToolsExtractedFromCapabilitiesDefinitions(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	mock.firstResponse = `{"interactionId":"i-1","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"ok\"}"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Retrieve catalog data", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
			{Name: "catalog_authoring", Description: "Author catalog items", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	// Per fix #1: "tools" is an ARRAY, each element uses
	// "functionDeclarations" (camelCase).
	var reqBody map[string]any
	if err := json.Unmarshal(mock.capturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	toolsRaw, ok := reqBody["tools"]
	if !ok {
		t.Fatalf("expected tools field in request body")
	}
	toolsArr, ok := toolsRaw.([]any)
	if !ok {
		t.Fatalf("expected tools to be an array (fix #1), got %T", toolsRaw)
	}
	if len(toolsArr) != 1 {
		t.Fatalf("expected 1 tools entry, got %d", len(toolsArr))
	}
	firstEntry, ok := toolsArr[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tools[0] to be an object")
	}
	funcDeclsRaw, ok := firstEntry["functionDeclarations"]
	if !ok {
		// Check if old snake_case is present (would mean fix is broken).
		if _, snakeCasePresent := firstEntry["function_declarations"]; snakeCasePresent {
			t.Fatalf("expected camelCase 'functionDeclarations' (fix #1), got snake_case 'function_declarations'")
		}
		t.Fatalf("expected functionDeclarations key in tools[0]")
	}
	funcDecls, ok := funcDeclsRaw.([]any)
	if !ok {
		t.Fatalf("expected functionDeclarations to be an array")
	}
	if len(funcDecls) != 2 {
		t.Errorf("expected 2 function declarations, got %d", len(funcDecls))
	}
	first := funcDecls[0].(map[string]any)
	if first["name"] != "catalog_data" {
		t.Errorf("expected first tool name=catalog_data, got %v", first["name"])
	}
	if first["description"] != "Retrieve catalog data" {
		t.Errorf("expected first tool description, got %v", first["description"])
	}
}

// Test fix #2: FunctionResponse is inside role="user" (not "function").
func TestToolLoop_FunctionResponseRoleIsUser(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	mock.firstResponse = `{"interactionId":"i-1","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{},"id":"c1"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
	mock.secondResponse = `{"interactionId":"i-2","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"ok\"}"}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10}}`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-role",
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	// Inspect the SECOND request body (the follow-up after tool execution).
	// The mock captures only the last body — we need the second one.
	// Since the mock captures capturedBody on each call, the second
	// request's body is the one we inspect.
	var secondReqBody map[string]any
	if err := json.Unmarshal(mock.capturedBody, &secondReqBody); err != nil {
		t.Fatalf("unmarshal second request body: %v", err)
	}
	contents, ok := secondReqBody["contents"].([]any)
	if !ok {
		t.Fatalf("expected contents array")
	}
	if len(contents) < 3 {
		t.Fatalf("expected at least 3 content entries (user + model + user), got %d", len(contents))
	}
	// The third entry (index 2) should be the tool response with role="user".
	thirdEntry, ok := contents[2].(map[string]any)
	if !ok {
		t.Fatalf("expected contents[2] to be an object")
	}
	role, ok := thirdEntry["role"].(string)
	if !ok {
		t.Fatalf("expected role field in contents[2]")
	}
	if role != "user" {
		t.Errorf("expected role=user for FunctionResponse (fix #2), got role=%s", role)
	}
	// Verify it contains a functionResponse part.
	parts, ok := thirdEntry["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("expected parts array in tool response")
	}
	firstPart, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected parts[0] to be an object")
	}
	if _, ok := firstPart["functionResponse"]; !ok {
		t.Errorf("expected functionResponse in the user-role part (fix #2)")
	}
}

// Test fix #3: thoughtSignature is preserved in the model response part
// when it's appended to the follow-up request.
func TestToolLoop_ThoughtSignaturePreservedInFollowUp(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	// First response includes a thoughtSignature on the model's functionCall part.
	mock.firstResponse = `{"interactionId":"i-1","candidates":[{"content":{"role":"model","parts":[{"thoughtSignature":"sig-abc123","functionCall":{"name":"catalog_data","args":{},"id":"c1"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
	mock.secondResponse = `{"interactionId":"i-2","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"ok\"}"}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10}}`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, _ := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-sig",
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	// Inspect the second request body — the model content (index 1)
	// should carry the thoughtSignature from the first response.
	var secondReqBody map[string]any
	if err := json.Unmarshal(mock.capturedBody, &secondReqBody); err != nil {
		t.Fatalf("unmarshal second request body: %v", err)
	}
	contents, ok := secondReqBody["contents"].([]any)
	if !ok {
		t.Fatalf("expected contents array")
	}
	if len(contents) < 2 {
		t.Fatalf("expected at least 2 content entries")
	}
	// The second entry (index 1) should be the model's original response
	// (with functionCall + thoughtSignature preserved).
	modelEntry, ok := contents[1].(map[string]any)
	if !ok {
		t.Fatalf("expected contents[1] to be an object")
	}
	parts, ok := modelEntry["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("expected parts array in model entry")
	}
	firstPart, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected parts[0] to be an object")
	}
	sig, ok := firstPart["thoughtSignature"].(string)
	if !ok || sig != "sig-abc123" {
		t.Errorf("expected thoughtSignature=sig-abc123 to be preserved in follow-up request (fix #3), got %v", firstPart["thoughtSignature"])
	}
}

// Test fix #4 + #6: AIRunID is saved in AIToolCallRecord.
func TestToolLoop_AIRunIDSavedInToolCallRecord(t *testing.T) {
	t.Parallel()
	mock := newMockGeminiToolServer(t)
	defer mock.Close()

	mock.firstResponse = `{"interactionId":"i-1","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"catalog_data","args":{},"id":"c1"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
	mock.secondResponse = `{"interactionId":"i-2","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"ok\"}"}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10}}`

	dispatcher := &stubCapabilityDispatcher{
		definitions: []ports.CustomerSalesToolDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}

	cc, runRepo := buildGeminiCustomerSalesAdapterWithTools(t, mock, dispatcher)

	_, err := cc.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:       "run-airunid-test",
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}

	if len(runRepo.createdToolCalls) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(runRepo.createdToolCalls))
	}
	tc := runRepo.createdToolCalls[0]
	if tc.AIRunID != "run-airunid-test" {
		t.Errorf("expected AIRunID=run-airunid-test in tool call record (fix #4), got %s", tc.AIRunID)
	}
}
