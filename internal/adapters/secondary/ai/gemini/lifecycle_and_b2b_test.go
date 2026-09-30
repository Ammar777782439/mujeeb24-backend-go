package gemini

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ===== Lifecycle failure test =====
//
// These tests verify that lifecycle transition failures STOP the
// tool loop — not just log + continue. Per the spec:
// "لا تجعل lifecycle failure مجرد log ويتم تجاهله."

// stubLifecycleFailWaitingTool always returns an error from MarkWaitingTool.
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
// Verifies: error returned + only 1 Gemini request made (no follow-up).
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
	if atomic.LoadInt64(&mock.requestCount) != 1 {
		t.Errorf("expected 1 Gemini request (loop stopped on lifecycle failure), got %d", atomic.LoadInt64(&mock.requestCount))
	}
}

// Test: MarkRunning failure → tool loop MUST NOT continue.
// Verifies: error returned + only 1 Gemini request made (no follow-up).
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
// With 2 tool rounds: WAITING_TOOL, RUNNING, WAITING_TOOL, RUNNING.
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
//
// Tests the B2B path as closely as possible without the full
// MerchantCatalogAIAgent (which needs SessionWriter, ContextBuilder,
// ValidationPipeline, Repository — too many stubs).
//
// Verifies the critical link:
//   ContractRuntimeInput.AIRunID → ContractClient → AIToolCallRecord.AIRunID

func TestB2B_AIRunID_Path_ContractRuntimeInputToToolCallRecord(t *testing.T) {
	t.Parallel()
	mock := newMockServerConfigurable(t, []string{
		makeToolCallResponse("i-1", "catalog_data", "call-1"),
		makeFinalResponse("i-2", "ok"),
	})
	defer mock.Close()

	dispatcher := makeDispatcher()
	cc, runRepo := buildContractClientForFixTests(t, mock.URL(), dispatcher, nil)

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
