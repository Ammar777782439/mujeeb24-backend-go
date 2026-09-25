// Package services — AutoReplyService refactored to contract-aligned flow.
//
// Implements contracts ③ §1 (Mujeeb = canonical conversation state),
// ④ §4 (Gemini output is AIGeminiProposal only),
// ⑥ §2 (Structural→Reference→Tenant→Ownership→Policy→Authorization→EffectiveDecision),
// ⑨ §2 (AI Run lifecycle),
// ⑧ §5 (operational trace via ai_runs + ai_run_attempts + ai_tool_calls).
//
// Per contract ④ §5, Gemini does NOT decide requires_approval; that's
// PolicyEvaluator's job (now part of ValidationPipeline).
//
// Per contract ⑥ §11, Mujeeb does NOT re-interpret customer intent during
// validation; that is Gemini's role.
//
// Per contract ⑥ §19, Execution only happens after Authorization succeeds;
// the AI never has a direct execution channel.

package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const (
	AutoReplyModeRestrictedAuto = "restricted_auto"
	// Per contract ④ §4, the legacy "ask_clarification" value has been replaced
	// by "clarification" to align with the closed action enum. Migration 000056
	// re-maps existing rows.
	AutoReplyActionAnswer        = "answer"
	AutoReplyActionClarification = "clarification"
)

// HandoffFarewellMessage is fixed Mujeeb-owned farewell content for
// subscription/activation requests. It is never model output, so sending it
// cannot hallucinate prices or terms. The persisted decision keeps
// RequiresHuman=true so the dashboard hands the conversation to staff.
const HandoffFarewellMessage = "يسعدنا اختيارك! تم استلام طلبك، وسيقوم أحد ممثلي المبيعات بالتواصل معك فوراً لإتمام خطوات التفعيل والربط."

// isSubscriptionHandoffIntent reports whether the model's intent asks to
// subscribe, activate, or purchase. This is product routing (which farewell
// flow applies), not reference resolution: it never selects catalog entities.
func isSubscriptionHandoffIntent(intent string) bool {
	value := strings.ToLower(strings.TrimSpace(intent))
	if value == "" {
		return false
	}
	for _, keyword := range []string{"subscri", "activat", "purchase", "اشتراك", "تفعيل", "فعّل", "شراء"} {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}

// AutoReplyService drives the contract ⑥ post-Gemini flow for one customer turn.
//
// Per contract ⑨ §1, each Handle() call is one AI Run.
// Per contract ⑨ §2, the Run progresses: RECEIVED → CONTEXT_BUILT → RUNNING
// → VALIDATING → AUTHORIZED → EXECUTING → COMPLETED (or FAILED/CANCELLED).
// Per contract ⑥ §2, after Gemini produces AIGeminiProposal, the ValidationPipeline
// runs Structural→Reference→Tenant→Ownership→Policy→Authorization.
// Per contract ⑥ §19, Execution happens only after Authorization succeeds.
//
// Per contract ④ §5, requires_approval is decided by Mujeeb only; the AI
// proposal's status/action are validated but never overridden by Mujeeb
// (per contract ⑥ §11 we do NOT re-interpret intent).
type AutoReplyService struct {
	// Runtime is the contract ④ §8 ContractRuntime (Gemini ContractClient).
	// Per contract ④ §8, this is the only way to call Gemini.
	Runtime ports.ContractRuntime

	// ContextBuilder per contract ③ §2 builds the AIContext.
	ContextBuilder ports.AIContextBuilder

	// Validation is the contract ⑥ §2 pipeline. If nil, validation is
	// skipped (defaulting to "allowed"); production deployments MUST wire it.
	Validation *ValidationPipeline

	// RunRepository persists AI Run trace per contract ⑧ §5. If nil, the
	// lifecycle is run in-memory only (useful for tests).
	RunRepository ports.AIRunRepository

	// CatalogBatchController drives the contract ② token-aware batching
	// when Gemini's first response indicates catalog data is needed.
	// Per contract ② §9, the flow is: Gemini → needs_catalog → build
	// projection → token-count → batch → evaluate → aggregate candidates
	// → final evaluate → AIGeminiProposal.
	// If nil, catalog evaluation is skipped (the AI replies with whatever
	// it can infer from the context alone).
	CatalogBatch *CatalogBatchController

	// EntityContractPayload is the JSON-encoded Catalog Entity Contract per
	// contract ⑤ §7. Built once at bootstrap and reused for every call.
	EntityContractPayload []byte

	// DecisionRepository persists the business ai_decisions row.
	DecisionRepository ports.AIDecisionRepository
	// ReferenceRepository resolves conversation provider references.
	ReferenceRepository ports.ConversationReferenceRepository
	// OutboundRepository creates outbound messages.
	OutboundRepository ports.OutboundMessageRepository
	// Outbox enqueues the actual send.
	Outbox ports.OutboxStore
	// MessageRepository records the communication message row.
	MessageRepository ports.MessageRepository
	// Transactions wraps multi-step DB writes.
	Transactions ports.TransactionManager
	// StateRepository persists ConversationState per contract ③ §1.
	StateRepository ports.ConversationStateRepository
	// Conversations for state machine transitions (waiting_human, etc.).
	Conversations ports.ConversationRuntimeRepository
	// Realtime publishes dashboard events.
	Realtime ports.RealtimePublisher

	Mode          string
	PolicyVersion string
	Now           func() time.Time
	NewID         func() string
}

// NewAutoReplyService wires the required dependencies for the contract-aligned
// AutoReply flow. Optional dependencies (ContextBuilder, Validation,
// RunRepository, etc.) are set on the returned struct by the caller.
func NewAutoReplyService(runtime ports.ContractRuntime, decisions ports.AIDecisionRepository, references ports.ConversationReferenceRepository, outbound ports.OutboundMessageRepository, outbox ports.OutboxStore, transactions ports.TransactionManager) AutoReplyService {
	return AutoReplyService{
		Runtime:             runtime,
		DecisionRepository:  decisions,
		ReferenceRepository: references,
		OutboundRepository:  outbound,
		Outbox:              outbox,
		Transactions:        transactions,
		Mode:                AutoReplyModeRestrictedAuto,
		PolicyVersion:       "auto-reply-v1",
		Now:                 func() time.Time { return time.Now().UTC() },
		NewID:               uuid.NewString,
	}
}

// Handle processes one customer turn end-to-end per contracts ③④⑥⑧⑨.
//
// Flow:
//  1. Validate input command.
//  2. Start AI Run (RECEIVED) per contract ⑨ §1. Idempotent on
//     (business_id, source_message_reference).
//  3. Build context (CONTEXT_BUILT) per contract ③ §2.
//  4. Call Gemini via Runtime.DecideContract (RUNNING) per contract ④ §8.
//  5. Run ValidationPipeline (VALIDATING) per contract ⑥ §2.
//  6. If validation fails → Mark FAILED per contract ⑨ §18; no execution.
//  7. If policy denied → Mark FAILED; no execution.
//  8. If policy requires_approval → Mark AUTHORIZED, persist decision, return
//     (no execution; human approval needed first).
//  9. If policy allowed → Mark AUTHORIZED → EXECUTING.
//
// 10. Execute (outbound message + outbox + message row) per contract ⑥ §19.
// 11. Mark COMPLETED per contract ⑨ §31.
func (s AutoReplyService) Handle(ctx context.Context, command commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
	businessID := string(command.Meta.Actor.BusinessID)
	conversationID := string(command.ConversationID)
	log.Printf("[AutoReply] START business=%s conversation=%s text=%q", businessID, conversationID, truncate(command.Text, 80))

	if err := s.validate(command); err != nil {
		log.Printf("[AutoReply] VALIDATE_FAILED business=%s err=%v", businessID, err)
		return commands.AutoReplyResult{}, err
	}
	if s.Runtime == nil || s.DecisionRepository == nil || s.ReferenceRepository == nil || s.OutboundRepository == nil || s.Outbox == nil || s.Transactions == nil {
		log.Printf("[AutoReply] NOT_WIRED business=%s runtime=%v decisions=%v", businessID, s.Runtime != nil, s.DecisionRepository != nil)
		return commands.AutoReplyResult{}, appErrors.NotImplemented()
	}

	sourceMessageRef := command.SourceMessageReference
	policyVersion := s.policyVersionOr()

	// Per contract ⑨ §1, start an AI Run. Per ⑨ §16, the (business_id, key)
	// uniqueness prevents duplicate Runs for the same source event.
	var run ports.AIRunRecord
	if s.RunRepository != nil {
		lc := NewAIRunLifecycle(s.RunRepository)
		started, err := lc.StartRun(ctx, StartRunInput{
			BusinessID:     businessID,
			ConversationID: conversationID,
			MessageID:      sourceMessageRef,
			IdempotencyKey: "auto-reply:" + sourceMessageRef,
			AgentRole:      ports.AIRunAgentRoleCustomerSales,
			NewRunID:       s.NewID,
		})
		if err != nil {
			log.Printf("[AutoReply] START_RUN_FAILED business=%s err=%v", businessID, err)
			return commands.AutoReplyResult{}, err
		}
		run = started
	}

	// Per contract ③ §2, build the AIContext.
	var loadedState *ports.ConversationStateRecord
	if s.StateRepository != nil {
		if st, err := s.StateRepository.Get(ctx, businessID, conversationID); err == nil {
			loadedState = &st
		}
	}
	var builtContext *ports.AIContext
	if s.ContextBuilder != nil {
		bc, contextErr := s.ContextBuilder.Build(ctx, ports.ContextBuildInput{
			BusinessID:             businessID,
			ConversationID:         conversationID,
			SourceMessageReference: sourceMessageRef,
			Text:                   command.Text,
			Channel:                command.Channel,
			PolicyVersion:          policyVersion,
			ConversationState:      loadedState,
		})
		if contextErr != nil {
			log.Printf("[AutoReply] CONTEXT_BUILD_FAILED business=%s err=%v", businessID, contextErr)
			s.markFailedSafe(ctx, run, ports.AIRunFailureStageContextBuild, string(ports.AIRunFailureCategoryInfrastructure), contextErr.Error())
			return commands.AutoReplyResult{}, contextErr
		}
		log.Printf("[AutoReply] CONTEXT_BUILT business=%s ownership=%s state=%s", businessID, bc.Conversation.Ownership, bc.Conversation.State)
		// Per contract ③ §1, if conversation is owned by human or waiting for human,
		// AI does not respond.
		if strings.EqualFold(bc.Conversation.Ownership, "human") || strings.EqualFold(bc.Conversation.State, "waiting_human") {
			log.Printf("[AutoReply] SKIPPED business=%s reason=human_owned_or_waiting_human ownership=%s state=%s", businessID, bc.Conversation.Ownership, bc.Conversation.State)
			s.markCompletedSafe(ctx, run)
			return commands.AutoReplyResult{Action: "no_action", Enqueued: false}, nil
		}
		builtContext = &bc
	}

	// Per contract ⑨ §3, mark CONTEXT_BUILT → RUNNING.
	s.markContextBuiltSafe(ctx, run)
	s.markRunningSafe(ctx, run)

	// Per contract ④ §8, call Gemini via the ContractRuntime.
	log.Printf("[AutoReply] GEMINI_CALL business=%s conversation=%s run=%s", businessID, conversationID, run.ID)
	out, err := s.Runtime.DecideContract(ctx, ports.ContractRuntimeInput{
		DecisionInput: ports.AIDecisionInput{
			BusinessID:             businessID,
			ConversationID:         conversationID,
			SourceMessageReference: sourceMessageRef,
			Text:                   command.Text,
			Channel:                command.Channel,
			PolicyVersion:          policyVersion,
			Context:                builtContext,
		},
		// Per contract ③ §4, carry previous_interaction_id (empty for first turn
		// in tests; in production this comes from the conversation row).
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID: "",
			Store:                 true,
		},
		// Per contract ⑤ §7, pass the Catalog Entity Contract payload (may be nil).
		EntityContractPayload: s.EntityContractPayload,
	})
	if err != nil {
		log.Printf("[AutoReply] GEMINI_FAILED business=%s err=%v", businessID, err)
		s.markFailedSafe(ctx, run, ports.AIRunFailureStageGeminiRequest, string(ports.AIRunFailureCategoryProviderPermanent), err.Error())
		return commands.AutoReplyResult{}, err
	}
	proposal := out.Proposal
	log.Printf("[AutoReply] GEMINI_OK business=%s status=%s action=%s tokens_in=%d tokens_out=%d latency=%dms",
		businessID, proposal.Status, proposal.Action, out.Usage.InputTokens, out.Usage.OutputTokens, out.LatencyMs)

	// Per contract ② §9 — Catalog Evaluation flow.
	//
	// When Gemini's first response indicates it needs catalog data
	// (status=needs_more_data AND the context lacks catalog evidence),
	// invoke the CatalogBatchController to:
	//   1. Build the Catalog AI Projection from PostgreSQL (contract ① §6)
	//   2. Token-count and split into batches (contract ② §2)
	//   3. Evaluate each batch independently (contract ② §5-8)
	//   4. Aggregate candidates and run Final Evaluation (contract ② §6)
	//
	// The Final Evaluation produces a new AIGeminiProposal that replaces
	// the initial one. This is the contract ② §9 flow:
	//   Customer Message → Gemini → needs_catalog? → Catalog Evaluation
	//   → Final Gemini → AI Proposal → Validation → Execution
	//
	// Per contract ② "ما أغلقناه": no semantic search, no product matching
	// inside Mujeeb. Mujeeb only builds the projection and counts tokens.
	if s.CatalogBatch != nil && proposal.Status == ports.AIProposalStatusNeedsMoreData {
		// Per contract ② §9, only invoke catalog evaluation when the
		// context lacks catalog evidence (otherwise Gemini already has
		// what it needs).
		if builtContext == nil || len(builtContext.CatalogEvidence) == 0 {
			// Per contract ⑨ §3, mark RUNNING again (back from VALIDATING
			// to RUNNING for the batch evaluation loop).
			s.markRunningSafe(ctx, run)

			// Per contract ② §9, run the full catalog evaluation pipeline.
			entityContract := CatalogEntityContractPayload{}
			if len(s.EntityContractPayload) > 0 {
				_ = json.Unmarshal(s.EntityContractPayload, &entityContract)
			}
			finalProposal, err := s.CatalogBatch.RunCatalogEvaluation(ctx, CatalogEvaluationInput{
				AIRunID:             run.ID,
				AttemptID:           "", // no separate attempt tracking in this path
				BusinessID:          businessID,
				ConversationID:      conversationID,
				CatalogScope:        "", // evaluate all active catalogs for the business
				CustomerMessage:     command.Text,
				ConversationContext: derefAIContext(builtContext),
				EntityContract:      entityContract,
			})
			if err != nil {
				// Per contract ⑨ §18, mark FAILED. The initial proposal
				// (needs_more_data) is still persisted for audit.
				s.markFailedSafe(ctx, run, ports.AIRunFailureStageGeminiRequest, string(ports.AIRunFailureCategoryProviderPermanent), "catalog evaluation: "+err.Error())
				// Fall back to the initial proposal — the customer gets
				// a needs_more_data response instead of silence.
			} else {
				// Replace the initial proposal with the final one from
				// the contract ② §6 Final Evaluation.
				proposal = finalProposal
			}
		}
	}

	// Per contract ⑨ §3, mark VALIDATING.
	s.markValidatingSafe(ctx, run)

	// Per contract ⑥ §2, run the validation pipeline.
	var effective ports.EffectiveDecision
	if s.Validation != nil {
		ed, failure := s.Validation.Validate(ctx, ValidationInput{
			DecisionID:         "", // linked later when ai_decisions is created
			BusinessID:         businessID,
			ConversationID:     conversationID,
			Proposal:           proposal,
			Context:            builtContext,
			EvidenceItemIDs:    extractItemIDs(builtContext),
			EvidenceVariantIDs: extractVariantIDs(builtContext),
			EvidenceOfferIDs:   extractOfferIDs(builtContext),
		})
		if failure != nil {
			s.markFailedSafe(ctx, run, failure.Stage, string(failure.Category), failure.Reason)
			// Per contract ⑥ §21, no Execution when validation fails.
			// Persist a "blocked" decision for audit trail.
			now := s.now()
			blockedDraft := ports.AIDecisionDraft{
				ID:                     s.id(),
				BusinessID:             businessID,
				ConversationID:         &conversationID,
				SourceMessageReference: &sourceMessageRef,
				IntentBase:             string(proposal.Status),
				Entities:               []byte(`{}`),
				EvidenceReferences:     []byte(`[]`),
				RequestedAction:        string(proposal.Action),
				ConfidenceBand:         "unknown",
				RequiresHuman:          true,
				MissingInformation:     []byte(`["` + failure.Reason + `"]`),
				ReasonCodes:            []byte(`["validation_failed"]`),
				PolicyVersion:          policyVersion,
				SchemaVersion:          1,
				Lifecycle:              "expired",
				PolicyDecision:         stringPtr("denied"),
				ModelReference:         stringPtr(out.Usage.Model),
				CreatedAt:              now,
				UpdatedAt:              now,
			}
			if run.ID != "" {
				blockedDraft.AIRunID = &run.ID
			}
			_ = s.persistDecision(ctx, blockedDraft)
			return commands.AutoReplyResult{Action: "no_action", Enqueued: false}, nil
		}
		effective = ed
	} else {
		// No validation configured — default to allowed (test convenience).
		effective = ports.EffectiveDecision{
			EffectiveAction: string(proposal.Action),
			PolicyDecision:  "allowed",
		}
	}

	// Per contract ⑥ §14, handoff for subscription/activation requests uses
	// fixed Mujeeb-owned farewell (never model text). This is product routing,
	// not reference resolution.
	//
	// Per contract ④ §4, the model returns one of the closed status values
	// (resolved/ambiguous/not_found/needs_more_data). None of these directly
	// convey "subscription" intent, so we infer it from ResponseText (which
	// the model produces per its system prompt). This is product routing,
	// not reference resolution.
	farewellHandoff := false
	var farewellReasonCodes []string
	if proposal.Action == ports.AIProposalActionHumanRequest &&
		effective.PolicyDecision == "allowed" &&
		isSubscriptionHandoffIntent(proposal.ResponseText) {
		farewellHandoff = true
		// Override the proposal's response text with the fixed farewell and
		// the action to "answer" so the sendable check enqueues the message.
		// Per contract ⑥ §14, handoff is governed by Mujeeb policy, not by
		// Gemini prompt text — Mujeeb decides what to send.
		proposal.ResponseText = HandoffFarewellMessage
		proposal.Action = ports.AIProposalActionAnswer
		farewellReasonCodes = []string{"handoff_farewell_sent"}
	}

	now := s.now()
	decisionID := s.id()

	// Per contract ⑥ §17, persist the Effective Decision (or proposal if no
	// validation ran).
	decisionDraft := ports.AIDecisionDraft{
		ID:                     decisionID,
		BusinessID:             businessID,
		ConversationID:         &conversationID,
		SourceMessageReference: &sourceMessageRef,
		IntentBase:             string(proposal.Status),
		Entities:               []byte(`{}`),
		EvidenceReferences:     encodeProposalSelectedAsJSON(proposal.Selected),
		RequestedAction:        string(proposal.Action),
		ConfidenceBand:         "medium",
		RequiresHuman:          proposal.Action == ports.AIProposalActionHumanRequest || farewellHandoff,
		MissingInformation:     []byte(`[]`),
		ReasonCodes:            encodeReasonCodes(farewellReasonCodes),
		PolicyVersion:          policyVersion,
		ModelReference:         stringPtr(out.Usage.Model),
		SchemaVersion:          1,
		Lifecycle:              "proposed",
		PolicyDecision:         stringPtr(effective.PolicyDecision),
		CorrelationID:          uuidStringPointer(command.Meta.CorrelationID),
		CausationID:            uuidStringPointer(sourceMessageRef),
		ExpiresAt:              pointerTo(now.Add(5 * time.Minute)),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if run.ID != "" {
		decisionDraft.AIRunID = &run.ID
	}

	result := commands.AutoReplyResult{Action: string(proposal.Action)}
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		decision, createErr := s.DecisionRepository.CreateProposed(txCtx, decisionDraft)
		if createErr != nil {
			return mapAIRepositoryError(createErr)
		}
		result.Decision = aiDecisionView(decision)

		// Per contract ⑥ §17, if Effective Decision is "denied", no execution.
		if effective.PolicyDecision == "denied" {
			return nil
		}
		// Per contract ⑥ §17, if Effective Decision is "requires_approval",
		// persist and wait for human approval — no execution.
		if effective.PolicyDecision == "requires_approval" && !farewellHandoff {
			// Transition conversation to waiting_human.
			if s.Conversations != nil {
				st := "waiting_human"
				if _, convErr := s.Conversations.TransitionLifecycle(txCtx, ports.ConversationLifecycleTransition{
					BusinessID:     businessID,
					ConversationID: conversationID,
					State:          &st,
					LastActivityAt: &now,
				}); convErr != nil {
					return mapAIRepositoryError(convErr)
				}
			}
			return nil
		}

		// Per contract ⑥ §19, Execution only after Authorization.
		// Mark EXECUTING per contract ⑨ §3.
		s.markExecutingSafe(ctx, run)

		// Per contract ③ §1, Mujeeb owns the conversation state. After a
		// successful AI reply (answer or clarification), transition the
		// conversation to "waiting_customer" with ownership "ai" so the
		// dashboard reflects the current state.
		if s.Conversations != nil {
			var targetState *string
			var targetOwnership *string
			switch proposal.Action {
			case ports.AIProposalActionAnswer, ports.AIProposalActionClarification:
				st := "waiting_customer"
				targetState = &st
				own := "ai"
				targetOwnership = &own
			case ports.AIProposalActionHumanRequest:
				st := "waiting_human"
				targetState = &st
			case ports.AIProposalActionLeadDraft, ports.AIProposalActionOrderDraft:
				st := "waiting_human"
				targetState = &st
			}
			if targetState != nil {
				if _, convErr := s.Conversations.TransitionLifecycle(txCtx, ports.ConversationLifecycleTransition{
					BusinessID:     businessID,
					ConversationID: conversationID,
					State:          targetState,
					Ownership:      targetOwnership,
					LastActivityAt: &now,
				}); convErr != nil {
					return mapAIRepositoryError(convErr)
				}
			}
		}

		// Per contract ⑥ §21, only answer/clarification are customer-facing
		// messaging actions. Lead/Order drafts follow their own flow.
		sendable := proposal.Action == ports.AIProposalActionAnswer ||
			proposal.Action == ports.AIProposalActionClarification
		if !sendable {
			return nil
		}

		if strings.TrimSpace(proposal.ResponseText) == "" {
			return fmt.Errorf("%w: reply action requires response text", appErrors.New(appErrors.CodeValidation, "auto reply"))
		}

		reference, referenceErr := s.ReferenceRepository.GetCurrentByConversation(txCtx, businessID, conversationID, "provider")
		if referenceErr != nil {
			return mapAIRepositoryError(referenceErr)
		}
		if reference.ProviderRef != command.ProviderRef {
			return appErrors.New(appErrors.CodeInvalidState, "conversation provider reference does not match requested provider")
		}
		if reference.ConnectionID == nil || strings.TrimSpace(*reference.ConnectionID) == "" || strings.TrimSpace(reference.ResourceID) == "" {
			return appErrors.New(appErrors.CodeInvalidState, "conversation provider reference is incomplete")
		}

		outboundID := s.id()
		contentReference := EncodeInlineTextContentReference(proposal.ResponseText)
		outbound, outboundErr := s.OutboundRepository.CreatePending(txCtx, ports.OutboundMessageDraft{
			ID:                      outboundID,
			BusinessID:              businessID,
			ConversationID:          conversationID,
			ConversationReferenceID: reference.ID,
			ConnectionID:            *reference.ConnectionID,
			ProviderRef:             command.ProviderRef,
			Channel:                 command.Channel,
			Origin:                  "ai",
			Transport:               "provider",
			ContentReference:        contentReference,
			ProviderIdempotencyKey:  "auto-reply:" + sourceMessageRef,
			CorrelationID:           uuidStringPointer(command.Meta.CorrelationID),
			CausationID:             uuidStringPointer(decision.ID),
		})
		if outboundErr != nil {
			return mapAIRepositoryError(outboundErr)
		}
		if s.MessageRepository != nil {
			msgDraft := ports.CommunicationMessageDraft{
				ID:                      s.id(),
				BusinessID:              businessID,
				ConversationReferenceID: reference.ID,
				OutboundMessageID:       &outbound.ID,
				Direction:               "outbound",
				Origin:                  "ai",
				Transport:               "provider",
				ContentType:             "text",
				TextContent:             &proposal.ResponseText,
				ContentReference:        contentReference,
				Visibility:              "public",
				OccurredAt:              now,
				CreatedAt:               now,
			}
			if _, msgErr := s.MessageRepository.Record(txCtx, msgDraft); msgErr != nil {
				return mapAIRepositoryError(msgErr)
			}
		}
		outboxEntry, outboxErr := s.Outbox.Enqueue(txCtx, ports.OutboxEntryDraft{
			ID:                s.id(),
			BusinessID:        businessID,
			OutboundMessageID: outbound.ID,
			CommandType:       OutboundSendCommandType,
			DedupeKey:         "auto-reply:" + sourceMessageRef,
			AvailableAt:       now,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
		if outboxErr != nil {
			return mapAIRepositoryError(outboxErr)
		}
		result.OutboundMessageID = commands.MessageID(outbound.ID)
		result.OutboxEntryID = commands.ID(outboxEntry.ID)
		result.Enqueued = true
		return nil
	})
	if err != nil {
		log.Printf("[AutoReply] EXECUTE_FAILED business=%s err=%v", businessID, err)
		s.markFailedSafe(ctx, run, ports.AIRunFailureStageExecution, string(ports.AIRunFailureCategoryExecutionFailure), err.Error())
		return commands.AutoReplyResult{}, err
	}

	// Per contract ⑨ §31, Mark COMPLETED.
	s.markCompletedSafe(ctx, run)
	log.Printf("[AutoReply] COMPLETED business=%s action=%s enqueued=%v outbound=%s outbox=%s",
		businessID, result.Action, result.Enqueued, result.OutboundMessageID, result.OutboxEntryID)

	// Per contract ⑧ §5, publish realtime events for the dashboard.
	if s.Realtime != nil && result.Enqueued {
		data, _ := json.Marshal(map[string]any{
			"decision_id":           result.Decision.ID,
			"outbound_message_id":   result.OutboundMessageID,
			"conversation_id":       conversationID,
			"text":                  proposal.ResponseText,
			"action":                result.Action,
			"ai_run_id":             run.ID,
			"gemini_interaction_id": out.GeminiInteraction.ResultingInteractionID,
		})
		var correlationID *string
		if command.Meta.CorrelationID != "" {
			correlationID = &command.Meta.CorrelationID
		}
		_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
			EventID:       uuid.NewString(),
			EventType:     "conversation.ai_replied",
			BusinessID:    businessID,
			ResourceType:  "conversation",
			ResourceID:    conversationID,
			OccurredAt:    now,
			CorrelationID: correlationID,
			Data:          data,
		})
	}

	return result, nil
}

func (s AutoReplyService) validate(command commands.AutoReplyCommand) error {
	if s.Mode != AutoReplyModeRestrictedAuto {
		return appErrors.New(appErrors.CodeInvalidState, "auto reply is not enabled in restricted_auto mode")
	}
	if command.Meta.Actor.BusinessID == "" || command.ConversationID == "" || strings.TrimSpace(command.SourceMessageReference) == "" || strings.TrimSpace(command.Text) == "" || strings.TrimSpace(command.Channel) == "" || strings.TrimSpace(command.ProviderRef) == "" {
		return appErrors.New(appErrors.CodeValidation, "business, conversation, source message, text, channel, and provider are required")
	}
	if command.Channel != "facebook" && command.Channel != "instagram" && command.Channel != "whatsapp" {
		return appErrors.New(appErrors.CodeValidation, "unsupported auto reply channel")
	}
	return nil
}

// EncodeInlineTextContentReference encodes text as an inline content reference.
func EncodeInlineTextContentReference(text string) string {
	return "content://inline-text/v1/" + base64.RawURLEncoding.EncodeToString([]byte(text))
}

// DecodeInlineTextContentReference decodes an inline content reference.
func DecodeInlineTextContentReference(reference string) (string, error) {
	const prefix = "content://inline-text/v1/"
	if !strings.HasPrefix(reference, prefix) {
		return "", errors.New("unsupported content reference")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(reference, prefix))
	if err != nil {
		return "", fmt.Errorf("decode inline text content: %w", err)
	}
	return string(decoded), nil
}

func (s AutoReplyService) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s AutoReplyService) id() string {
	if s.NewID == nil {
		return uuid.NewString()
	}
	return s.NewID()
}

func (s AutoReplyService) policyVersionOr() string {
	if strings.TrimSpace(s.PolicyVersion) == "" {
		return "auto-reply-v1"
	}
	return s.PolicyVersion
}

// markContextBuiltSafe, markRunningSafe, etc. are no-ops when RunRepository
// is nil (e.g., unit tests that don't need the trace). They're safe to call
// on a zero-value AIRunRecord.
func (s AutoReplyService) markContextBuiltSafe(ctx context.Context, run ports.AIRunRecord) {
	if s.RunRepository == nil || run.ID == "" {
		return
	}
	lc := NewAIRunLifecycle(s.RunRepository)
	_, _ = lc.MarkContextBuilt(ctx, run.BusinessID, run.ID)
}

func (s AutoReplyService) markRunningSafe(ctx context.Context, run ports.AIRunRecord) {
	if s.RunRepository == nil || run.ID == "" {
		return
	}
	lc := NewAIRunLifecycle(s.RunRepository)
	_, _ = lc.MarkRunning(ctx, run.BusinessID, run.ID)
}

func (s AutoReplyService) markValidatingSafe(ctx context.Context, run ports.AIRunRecord) {
	if s.RunRepository == nil || run.ID == "" {
		return
	}
	lc := NewAIRunLifecycle(s.RunRepository)
	_, _ = lc.MarkValidating(ctx, run.BusinessID, run.ID)
}

func (s AutoReplyService) markExecutingSafe(ctx context.Context, run ports.AIRunRecord) {
	if s.RunRepository == nil || run.ID == "" {
		return
	}
	lc := NewAIRunLifecycle(s.RunRepository)
	_, _ = lc.MarkExecuting(ctx, run.BusinessID, run.ID)
}

func (s AutoReplyService) markCompletedSafe(ctx context.Context, run ports.AIRunRecord) {
	if s.RunRepository == nil || run.ID == "" {
		return
	}
	lc := NewAIRunLifecycle(s.RunRepository)
	_, _ = lc.MarkCompleted(ctx, run.BusinessID, run.ID)
}

func (s AutoReplyService) markFailedSafe(ctx context.Context, run ports.AIRunRecord, stage, category, reason string) {
	if s.RunRepository == nil || run.ID == "" {
		return
	}
	lc := NewAIRunLifecycle(s.RunRepository)
	_, _ = lc.MarkFailed(ctx, run.BusinessID, run.ID, stage, category, reason)
}

// persistDecision is a non-transactional best-effort persist for the blocked
// decision path. Used when validation fails and we still want an audit row.
func (s AutoReplyService) persistDecision(ctx context.Context, draft ports.AIDecisionDraft) error {
	if s.DecisionRepository == nil {
		return nil
	}
	_, err := s.DecisionRepository.CreateProposed(ctx, draft)
	return err
}

// encodeReasonCodes serializes a list of reason codes as JSON for persistence.
func encodeReasonCodes(codes []string) []byte {
	if len(codes) == 0 {
		return []byte(`[]`)
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, c := range codes {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('"')
		sb.WriteString(c)
		sb.WriteByte('"')
	}
	sb.WriteByte(']')
	return []byte(sb.String())
}

// encodeProposalSelectedAsJSON serializes the contract ④ §4 selected[] as
// the legacy ai_decisions.evidence_references JSON shape for persistence.
func encodeProposalSelectedAsJSON(selected []ports.SelectedReference) []byte {
	if len(selected) == 0 {
		return []byte(`[]`)
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, ref := range selected {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"item_id":"`)
		sb.WriteString(ref.ItemID)
		sb.WriteByte('"')
		if ref.VariantID != nil && *ref.VariantID != "" {
			sb.WriteString(`,"variant_id":"`)
			sb.WriteString(*ref.VariantID)
			sb.WriteByte('"')
		}
		if ref.OfferID != nil && *ref.OfferID != "" {
			sb.WriteString(`,"offer_id":"`)
			sb.WriteString(*ref.OfferID)
			sb.WriteByte('"')
		}
		sb.WriteByte('}')
	}
	sb.WriteByte(']')
	return []byte(sb.String())
}

// derefAIContext safely dereferences a *ports.AIContext, returning a zero
// value if nil. Used when passing the context to CatalogBatchController
// which expects a value (not a pointer).
func derefAIContext(ctx *ports.AIContext) ports.AIContext {
	if ctx == nil {
		return ports.AIContext{}
	}
	return *ctx
}

// extractItemIDs/extractVariantIDs/extractOfferIDs pull the evidence IDs from
// the AIContext so the ValidationPipeline can verify per contract ⑥ §10.
func extractItemIDs(ctx *ports.AIContext) []string {
	if ctx == nil {
		return nil
	}
	out := make([]string, 0, len(ctx.CatalogEvidence))
	for _, e := range ctx.CatalogEvidence {
		out = append(out, e.Reference)
	}
	return out
}
func extractVariantIDs(ctx *ports.AIContext) []string {
	if ctx == nil {
		return nil
	}
	out := make([]string, 0, len(ctx.VariantEvidence))
	for _, e := range ctx.VariantEvidence {
		out = append(out, e.Reference)
	}
	return out
}
func extractOfferIDs(ctx *ports.AIContext) []string {
	if ctx == nil {
		return nil
	}
	out := make([]string, 0, len(ctx.OfferEvidence))
	for _, e := range ctx.OfferEvidence {
		out = append(out, e.Reference)
	}
	return out
}

func uuidStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil
	}
	return &value
}

func pointerTo(value time.Time) *time.Time { return &value }

// truncate shortens a string for logging, appending "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

var _ commands.AutoReplyHandler = AutoReplyService{}
