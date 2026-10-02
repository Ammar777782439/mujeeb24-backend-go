// Package gemini — Tool Loop implementation for ContractClient.
//
// This file implements the Gemini Function Calling Tool Loop per the
// user's specification. It does NOT create new architecture — it
// reuses:
//   - ports.AICapabilityDispatcher (from gemini.Client.Capabilities())
//   - ports.AIRunRepository (CreateToolCall / UpdateToolCall)
//   - ports.AIRunLifecycle (MarkWaitingTool / MarkRunning)
//   - ports.AIToolCallRecord / AIToolCallPatch
//   - Existing contractGeminiRequest/Response types (extended here)
//
// The loop: Gemini → FunctionCall → Capability Execute → FunctionResponse →
// Gemini → ... → Final Structured Proposal.
//
// No fixed max rounds. The loop terminates when:
//   - Gemini returns a final structured proposal (no functionCall parts)
//   - Tool execution fails with a non-retryable error
//   - Context deadline expires
//   - Provider/resource/economic/security boundary fires

package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// SetRunRepository wires the AIRunRepository for tool call persistence.
// Per the spec: reuse the existing repository instance — do NOT create
// a second one. Bootstrap calls this with the same postgres.NewAIRunTraceRepository
// already wired into AutoReplyService.RunRepository.
func (c *ContractClient) SetRunRepository(repo ports.AIRunRepository) {
	c.runRepo = repo
}

// SetNewID wires a UUID generator for tool call record IDs.
func (c *ContractClient) SetNewID(fn func() string) {
	c.newID = fn
}

// SetLifecycle wires the AIRunLifecyclePort for RUNNING → WAITING_TOOL →
// RUNNING transitions during the Tool Loop. Per fix #2: the caller
// passes the existing services.AIRunLifecycle (which implements
// AIRunLifecyclePort). No cycle — ContractClient uses the ports
// abstraction, not the concrete type.
func (c *ContractClient) SetLifecycle(lc ports.AIRunLifecyclePort) {
	c.lifecycle = lc
}

// buildToolDeclarations extracts tool definitions from the configured
// capability dispatcher and converts them to Gemini Function Declaration
// format.
//
// Per the spec: "لا تكتب Tool definitions يدوياً داخل Gemini client.
// استخرجها من c.base.Capabilities().Definitions()"
func (c *ContractClient) buildToolDeclarations() []contractFunctionDeclaration {
	caps := c.base.Capabilities()
	if caps == nil {
		return nil
	}
	defs := caps.Definitions()
	if len(defs) == 0 {
		return nil
	}
	declarations := make([]contractFunctionDeclaration, 0, len(defs))
	for _, d := range defs {
		declarations = append(declarations, contractFunctionDeclaration{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters,
		})
	}
	return declarations
}

// hasFunctionCall checks if a Gemini response contains any functionCall parts.
func hasFunctionCall(resp contractGeminiResponse) bool {
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.FunctionCall != nil {
				return true
			}
		}
	}
	return false
}

// extractFunctionCalls pulls all functionCall parts from the response.
func extractFunctionCalls(resp contractGeminiResponse) []*contractFunctionCall {
	var calls []*contractFunctionCall
	for _, cand := range resp.Candidates {
		for i := range cand.Content.Parts {
			if cand.Content.Parts[i].FunctionCall != nil {
				calls = append(calls, cand.Content.Parts[i].FunctionCall)
			}
		}
	}
	return calls
}

// executeToolCalls executes all function calls from a Gemini response,
// returning the model's tool-call content + a tool-response content for
// the follow-up request.
//
// Per the spec:
//   - Gemini does NOT set BusinessID. Tenant comes from
//     AICapabilityExecutionContext.BusinessID (from the trusted caller).
//   - Tool call records are persisted via AIRunRepository.CreateToolCall
//   - UpdateToolCall.
//   - The WAITING_TOOL → RUNNING lifecycle transition is handled by
//     the caller (DecideContract).
func (c *ContractClient) executeToolCalls(
	ctx context.Context,
	resp contractGeminiResponse,
	input ports.ContractRuntimeInput,
	businessID, conversationID, runID string,
) ([]contractContent, error) {
	caps := c.base.Capabilities()
	if caps == nil {
		return nil, fmt.Errorf("capability dispatcher not configured but Gemini requested tool calls")
	}

	// The model's response content (role=model with functionCall parts)
	// MUST be appended to contents before the tool response.
	modelContent := contractContent{
		Role:  "model",
		Parts: []contractPart{},
	}
	if len(resp.Candidates) > 0 {
		modelContent.Parts = resp.Candidates[0].Content.Parts
	}

	// The tool response content (role=function with functionResponse parts).
	toolResponseParts := []contractPart{}

	calls := extractFunctionCalls(resp)
	for _, call := range calls {
		// Per the spec: "Gemini لا يحدد BusinessID. يتم تجاهله."
		// The execution context carries the TRUSTED business_id from
		// Mujeeb's caller — NOT from Gemini's function call args.
		execCtx := ports.AICapabilityExecutionContext{
			BusinessID:     businessID,
			ConversationID: conversationID,
			RequestID:      input.DecisionInput.SourceMessageReference,
		}

		// Marshal args for the capability + for the tool call record.
		argsJSON, _ := json.Marshal(call.Args)

		// Persist the tool call record.
		var toolCallID string
		if c.newID != nil {
			toolCallID = c.newID()
		} else {
			toolCallID = uuid.NewString()
		}
		now := time.Now().UTC()
		if c.runRepo != nil && runID != "" {
			tcRec, tcErr := c.runRepo.CreateToolCall(ctx, ports.AIToolCallRecord{
				ID:            toolCallID,
				AIRunID:       runID,
				ToolName:      call.Name,
				ToolCallID:    call.ID,
				Status:        "running",
				RequestParams: argsJSON,
				StartedAt:     now,
				CreatedAt:     now,
			})
			if tcErr != nil {
				log.Printf("[ContractClient] TOOL_CALL_CREATE_FAILED run=%s tool=%s err=%v", runID, call.Name, tcErr)
				// Continue — the tool still executes, just untraced.
			}
			_ = tcRec
		}

		// Execute the capability.
		toolStarted := time.Now()
		result, execErr := caps.Execute(ctx, execCtx, call.Name, argsJSON)

		toolFinished := time.Now()
		toolLatencyMs := int(toolFinished.Sub(toolStarted).Milliseconds())

		// Build the function response part.
		var responseData map[string]any
		var resultPayload []byte
		var tcStatus string
		var tcFailureReason *string

		if execErr != nil {
			// Tool execution failed.
			errMsg := execErr.Error()
			responseData = map[string]any{"error": errMsg}
			resultPayload, _ = json.Marshal(responseData)
			tcStatus = "failed"
			tcFailureReason = &errMsg
			log.Printf("[ContractClient] TOOL_EXECUTION_FAILED run=%s tool=%s err=%v", runID, call.Name, execErr)
		} else {
			// Tool execution succeeded.
			resultPayload, _ = json.Marshal(result.Data)
			if m, ok := result.Data.(map[string]any); ok {
				responseData = m
			} else {
				// Wrap non-map results.
				responseData = map[string]any{"data": result.Data}
			}
			tcStatus = "completed"
			log.Printf("[ContractClient] TOOL_EXECUTED run=%s tool=%s latency_ms=%d", runID, call.Name, toolLatencyMs)
		}

		toolResponseParts = append(toolResponseParts, contractPart{
			FunctionResponse: &contractFunctionResponse{
				Name:     call.Name,
				ID:       call.ID,
				Response: responseData,
			},
		})

		// Update the tool call record.
		if c.runRepo != nil && runID != "" {
			finishedAt := toolFinished
			_, updateErr := c.runRepo.UpdateToolCall(ctx, toolCallID, ports.AIToolCallPatch{
				Status:        tcStatus,
				ResultPayload: resultPayload,
				FailureReason: tcFailureReason,
				FinishedAt:    &finishedAt,
				LatencyMs:     &toolLatencyMs,
			})
			if updateErr != nil {
				log.Printf("[ContractClient] TOOL_CALL_UPDATE_FAILED run=%s tool=%s err=%v", runID, call.Name, updateErr)
			}
		}

		// If tool execution failed with a non-retryable error, stop the loop.
		if execErr != nil {
			return nil, fmt.Errorf("tool %s execution failed (non-retryable): %w", call.Name, execErr)
		}
	}

	// Per fix #2: the FunctionResponse content must use role="user"
	// (not "function") per the Gemini generateContent API. The
	// sequence is: model FunctionCall → user FunctionResponse → Gemini.
	toolResponseContent := contractContent{
		Role:  "user",
		Parts: toolResponseParts,
	}

	return []contractContent{modelContent, toolResponseContent}, nil
}

// accumulateUsage adds usage from one Gemini response into the running total.
func accumulateUsage(total *ports.ContractUsageTelemetry, resp contractGeminiResponse) {
	total.InputTokens += resp.UsageMetadata.PromptTokenCount
	total.CachedTokens += resp.UsageMetadata.CachedContentTokenCount
	total.OutputTokens += resp.UsageMetadata.CandidatesTokenCount
}

// runToolLoop implements the core Gemini Function Calling loop.
//
// Flow:
//  1. Send initial request with tools attached.
//  2. Inspect response.
//  3. If functionCall parts exist → execute capabilities → append
//     functionResponse → send next request.
//  4. If no functionCall → parse final structured proposal → return.
//
// No fixed max rounds. The loop terminates when:
//   - Gemini returns a final structured proposal
//   - Tool execution fails non-retryably
//   - Context deadline expires
//   - Provider/resource/economic/security boundary fires
func (c *ContractClient) runToolLoop(
	ctx context.Context,
	reqBody contractGeminiRequest,
	rc *resolvedAIConfig,
	input ports.ContractRuntimeInput,
	businessID, conversationID, runID string,
	startedAt time.Time,
) (ports.ContractRuntimeOutput, error) {
	// Per the spec: "Usage يتم تجميعه عبر جميع Gemini requests."
	// Don't return only the last request's usage — accumulate.
	var totalUsage ports.ContractUsageTelemetry
	totalUsage.Model = rc.model

	var lastInteractionID string
	// Per fix #1: count actual Gemini requests.
	modelRequestCount := 0

	// The loop — no fixed max rounds.
	for {
		// Check context deadline before each iteration.
		if err := ctx.Err(); err != nil {
			return ports.ContractRuntimeOutput{}, fmt.Errorf("tool loop context cancelled: %w", err)
		}

		// Send the request.
		resp, err := c.sendContractRequest(ctx, reqBody, rc)
		if err != nil {
			return ports.ContractRuntimeOutput{}, fmt.Errorf("gemini request in tool loop: %w", err)
		}
		// Per fix #1: each sendContractRequest = 1 model request.
		modelRequestCount++

		// Accumulate usage from this request.
		accumulateUsage(&totalUsage, resp)
		if resp.InteractionID != "" {
			lastInteractionID = resp.InteractionID
		}

		// Check if Gemini returned function calls.
		if !hasFunctionCall(resp) {
			// No function calls — parse the final structured proposal.
			proposal, parseErr := parseContractProposal(resp)
			if parseErr != nil {
				return ports.ContractRuntimeOutput{}, parseErr
			}

			totalUsage.LatencyMs = time.Since(startedAt).Milliseconds()
			totalUsage.ModelRequests = modelRequestCount

			return ports.ContractRuntimeOutput{
				Proposal: proposal,
				GeminiInteraction: ports.GeminiInteractionContext{
					PreviousInteractionID:  input.GeminiInteraction.PreviousInteractionID,
					ResultingInteractionID: lastInteractionID,
					Store:                  input.GeminiInteraction.Store,
				},
				Usage:     totalUsage,
				LatencyMs: totalUsage.LatencyMs,
			}, nil
		}

		// Per fix #2: mark WAITING_TOOL before executing tools.
		// Per the spec: "لا تجعل lifecycle failure مجرد log ويتم تجاهله."
		// If the lifecycle transition fails, the loop MUST stop —
		// continuing would mean the run is in an inconsistent state
		// (RUNNING on the server but tools are executing without the
		// WAITING_TOOL marker).
		if c.lifecycle != nil && runID != "" && businessID != "" {
			if _, err := c.lifecycle.MarkWaitingTool(ctx, businessID, runID); err != nil {
				return ports.ContractRuntimeOutput{}, fmt.Errorf("lifecycle MarkWaitingTool failed (cannot continue tool loop in inconsistent state): %w", err)
			}
		}

		// Execute all function calls from the response.
		toolContents, execErr := c.executeToolCalls(ctx, resp, input, businessID, conversationID, runID)
		if execErr != nil {
			return ports.ContractRuntimeOutput{}, execErr
		}

		// Per fix #2: mark RUNNING before sending the next Gemini request.
		// Same rule: failure here stops the loop — the run can't stay
		// in WAITING_TOOL when tools are already executed.
		if c.lifecycle != nil && runID != "" && businessID != "" {
			if _, err := c.lifecycle.MarkRunning(ctx, businessID, runID); err != nil {
				return ports.ContractRuntimeOutput{}, fmt.Errorf("lifecycle MarkRunning failed (cannot continue tool loop in inconsistent state): %w", err)
			}
		}

		// Append the tool call response to the contents for the next request.
		reqBody.Contents = append(reqBody.Contents, toolContents...)

		// Clear previous_interaction_id for follow-up requests.
		reqBody.PreviousInteractionID = ""
		reqBody.Store = false
	}
}

// contractFunctionDeclaration is the Gemini Function Declaration format.
type contractFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// contractFunctionCall is Gemini's functionCall part in a response.
type contractFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
	ID   string         `json:"id,omitempty"`
}

// contractFunctionResponse is the functionResponse part sent back to Gemini.
type contractFunctionResponse struct {
	Name     string         `json:"name"`
	ID       string         `json:"id,omitempty"`
	Response map[string]any `json:"response"`
}
