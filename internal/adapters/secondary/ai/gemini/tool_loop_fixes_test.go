package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// ===== Test infrastructure for fix #1 + #2 =====

// stubLifecyclePort records MarkWaitingTool / MarkRunning calls.
type stubLifecyclePort struct {
	waitingToolCalls int64
	runningCalls     int64
}

func (s *stubLifecyclePort) MarkWaitingTool(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
	atomic.AddInt64(&s.waitingToolCalls, 1)
	return ports.AIRunRecord{}, nil
}
func (s *stubLifecyclePort) MarkRunning(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
	atomic.AddInt64(&s.runningCalls, 1)
	return ports.AIRunRecord{}, nil
}

var _ ports.AIRunLifecyclePort = (*stubLifecyclePort)(nil)

// mockServerConfigurable is a mock that returns configurable responses
// per request count. Used by tests #1-3.
type mockServerConfigurable struct {
	t           *testing.T
	server      *httptest.Server
	responses   []string
	requestCount int64
}

func newMockServerConfigurable(t *testing.T, responses []string) *mockServerConfigurable {
	m := &mockServerConfigurable{t: t, responses: responses}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	return m
}

func (m *mockServerConfigurable) Close() { m.server.Close() }
func (m *mockServerConfigurable) URL() string { return m.server.URL }

func (m *mockServerConfigurable) handle(w http.ResponseWriter, r *http.Request) {
	count := atomic.AddInt64(&m.requestCount, 1)
	var response string
	if int(count) <= len(m.responses) {
		response = m.responses[count-1]
	} else {
		response = m.responses[len(m.responses)-1]
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

// buildContractClientForFixTests builds a ContractClient with tools
// + lifecycle stub wired.
func buildContractClientForFixTests(t *testing.T, mockURL string, dispatcher ports.AICapabilityDispatcher, lc ports.AIRunLifecyclePort) (*ContractClient, *stubRunRepoForTools) {
	client, err := NewClient(Config{
		BaseURL:        mockURL,
		APIKey:         "test-key",
		Model:          "gemini-3.5-flash",
		SystemPrompt:   "test",
		RequestTimeout: 5 * time.Second,
		Capabilities:   dispatcher,
	})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	cc, err := NewContractClient(client)
	if err != nil {
		t.Fatalf("build contract client: %v", err)
	}
	runRepo := newStubRunRepoForTools()
	cc.SetRunRepository(runRepo)
	cc.SetNewID(uuid.NewString)
	if lc != nil {
		cc.SetLifecycle(lc)
	}
	return cc, runRepo
}

func makeDispatcher() *stubCapabilityDispatcher {
	return &stubCapabilityDispatcher{
		definitions: []ports.AICapabilityDefinition{
			{Name: "catalog_data", Description: "Catalog", Parameters: map[string]any{"type": "object"}},
		},
	}
}

func makeToolCallResponse(interactionID, toolName, callID string) string {
	return `{"interactionId":"` + interactionID + `","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"` + toolName + `","args":{},"id":"` + callID + `"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
}

func makeFinalResponse(interactionID, responseText string) string {
	return `{"interactionId":"` + interactionID + `","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"` + responseText + `\"}"}]}}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10}}`
}

func makeNoToolResponse(interactionID, responseText string) string {
	return `{"interactionId":"` + interactionID + `","candidates":[{"content":{"role":"model","parts":[{"text":"{\"status\":\"resolved\",\"action\":\"answer\",\"response_text\":\"` + responseText + `\"}"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
}

// ===== Tests =====

// Test 1: Non-tool path → ModelRequests == 1.
func TestFix1_NonToolPath_ModelRequestsIs1(t *testing.T) {
	t.Parallel()
	mock := newMockServerConfigurable(t, []string{
		makeNoToolResponse("i-1", "hello"),
	})
	defer mock.Close()

	dispatcher := makeDispatcher()
	cc, _ := buildContractClientForFixTests(t, mock.URL(), dispatcher, nil)

	out, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
	})
	if err != nil {
		t.Fatalf("DecideContract failed: %v", err)
	}
	if out.Usage.ModelRequests != 1 {
		t.Errorf("expected ModelRequests=1 (non-tool path), got %d", out.Usage.ModelRequests)
	}
}

// Test 2: Tool loop (1 tool) → ModelRequests == 2, ToolCalls == 1.
func TestFix1_ToolLoop_ModelRequestsIs2_ToolCallsIs1(t *testing.T) {
	t.Parallel()
	mock := newMockServerConfigurable(t, []string{
		makeToolCallResponse("i-1", "catalog_data", "call-1"),
		makeFinalResponse("i-2", "done"),
	})
	defer mock.Close()

	dispatcher := makeDispatcher()
	cc, runRepo := buildContractClientForFixTests(t, mock.URL(), dispatcher, nil)

	out, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "what products?"},
		AIRunID:      "run-1",
	})
	if err != nil {
		t.Fatalf("DecideContract failed: %v", err)
	}
	// ModelRequests should be 2 (initial + follow-up after tool).
	if out.Usage.ModelRequests != 2 {
		t.Errorf("expected ModelRequests=2 (tool loop), got %d", out.Usage.ModelRequests)
	}
	// ToolCalls should be 1.
	if len(runRepo.createdToolCalls) != 1 {
		t.Errorf("expected 1 tool call record, got %d", len(runRepo.createdToolCalls))
	}
}

// Test 3: Two tools then final → ModelRequests == 3, ToolCalls == 2.
func TestFix1_ToolLoop_TwoTools_ModelRequestsIs3_ToolCallsIs2(t *testing.T) {
	t.Parallel()
	mock := newMockServerConfigurable(t, []string{
		makeToolCallResponse("i-1", "catalog_data", "call-1"),
		makeToolCallResponse("i-2", "catalog_data", "call-2"),
		makeFinalResponse("i-3", "done"),
	})
	defer mock.Close()

	dispatcher := makeDispatcher()
	cc, runRepo := buildContractClientForFixTests(t, mock.URL(), dispatcher, nil)

	out, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "show me"},
		AIRunID:      "run-2",
	})
	if err != nil {
		t.Fatalf("DecideContract failed: %v", err)
	}
	if out.Usage.ModelRequests != 3 {
		t.Errorf("expected ModelRequests=3 (two tools), got %d", out.Usage.ModelRequests)
	}
	if len(runRepo.createdToolCalls) != 2 {
		t.Errorf("expected 2 tool call records, got %d", len(runRepo.createdToolCalls))
	}
}

// Test 4: Lifecycle — RUNNING → WAITING_TOOL → RUNNING during the loop.
func TestFix2_Lifecycle_RunningToWaitingToolToRunning(t *testing.T) {
	t.Parallel()
	mock := newMockServerConfigurable(t, []string{
		makeToolCallResponse("i-1", "catalog_data", "call-1"),
		makeFinalResponse("i-2", "ok"),
	})
	defer mock.Close()

	dispatcher := makeDispatcher()
	lc := &stubLifecyclePort{}
	cc, _ := buildContractClientForFixTests(t, mock.URL(), dispatcher, lc)

	_, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
		AIRunID:      "run-lc",
	})
	if err != nil {
		t.Fatalf("DecideContract failed: %v", err)
	}
	// Should have 1 WAITING_TOOL call + 1 RUNNING call.
	if atomic.LoadInt64(&lc.waitingToolCalls) != 1 {
		t.Errorf("expected 1 MarkWaitingTool call, got %d", atomic.LoadInt64(&lc.waitingToolCalls))
	}
	if atomic.LoadInt64(&lc.runningCalls) != 1 {
		t.Errorf("expected 1 MarkRunning call, got %d", atomic.LoadInt64(&lc.runningCalls))
	}
}

// Test 5: B2B — AIRunID is saved in AIToolCallRecord.
// This test uses the same ContractClient + tool loop path as B2B
// (the B2B agent calls the same ContractClient.DecideContract).
// The key assertion: when AIRunID is passed in ContractRuntimeInput,
// it appears in the AIToolCallRecord.
func TestFix3_B2B_AIRunIDSavedInToolCallRecord(t *testing.T) {
	t.Parallel()
	mock := newMockServerConfigurable(t, []string{
		makeToolCallResponse("i-1", "catalog_data", "call-1"),
		makeFinalResponse("i-2", "ok"),
	})
	defer mock.Close()

	dispatcher := makeDispatcher()
	cc, runRepo := buildContractClientForFixTests(t, mock.URL(), dispatcher, nil)

	// Simulate what the MerchantCatalogAIAgent does: it creates a run
	// (with ID "run-b2b-1") and passes it through BuildForTurn →
	// ContractRuntimeInput.AIRunID.
	_, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{BusinessID: "b2b-biz", ConversationID: "session-1", Text: "add product"},
		AIRunID:      "run-b2b-1",
	})
	if err != nil {
		t.Fatalf("DecideContract failed: %v", err)
	}
	if len(runRepo.createdToolCalls) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(runRepo.createdToolCalls))
	}
	tc := runRepo.createdToolCalls[0]
	if tc.AIRunID != "run-b2b-1" {
		t.Errorf("expected AIRunID=run-b2b-1 (B2B run), got %s", tc.AIRunID)
	}
}

// Test 6: Verify tools request shape is array + functionDeclarations.
func TestFix6_ToolsShapeArrayAndCamelCase(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(makeNoToolResponse("i-1", "ok")))
	}))
	defer server.Close()

	dispatcher := makeDispatcher()
	cc, _ := buildContractClientForFixTests(t, server.URL, dispatcher, nil)

	_, err := cc.DecideContract(context.Background(), ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{BusinessID: "b-1", ConversationID: "c-1", Text: "hello"},
	})
	if err != nil {
		t.Fatalf("DecideContract failed: %v", err)
	}

	var reqBody map[string]any
	if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	toolsRaw, ok := reqBody["tools"]
	if !ok {
		t.Fatalf("expected tools field")
	}
	toolsArr, ok := toolsRaw.([]any)
	if !ok {
		t.Fatalf("expected tools to be array, got %T", toolsRaw)
	}
	if len(toolsArr) != 1 {
		t.Fatalf("expected 1 tools entry, got %d", len(toolsArr))
	}
	entry := toolsArr[0].(map[string]any)
	if _, ok := entry["functionDeclarations"]; !ok {
		if _, snake := entry["function_declarations"]; snake {
			t.Fatalf("expected camelCase functionDeclarations, got snake_case")
		}
		t.Fatalf("expected functionDeclarations key")
	}
}
