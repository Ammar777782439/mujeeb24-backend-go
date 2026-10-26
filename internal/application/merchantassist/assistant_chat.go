package merchantassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

var (
	ErrInvalidModelPlan = errors.New("merchant assistant requested invalid or unauthorized capability")
	ErrModelRouting     = errors.New("merchant assistant native tool request failed")
	ErrModelAnswer      = errors.New("merchant assistant final answer failed")
	assistantUUID       = regexp.MustCompile("(?i)\\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\\b")
)

const (
	assistMaxToolEvidence = 65000
)

type nativeReadArgs struct {
	Search   string `json:"search"`
	RecordID string `json:"record_id"`
}

func decodeReadArgs(spec ToolDefinition, raw json.RawMessage) (nativeReadArgs, error) {
	var args nativeReadArgs
	if len(raw) == 0 || !json.Valid(raw) || raw[0] != '{' {
		return args, ErrInvalidModelPlan
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil || params == nil {
		return args, ErrInvalidModelPlan
	}
	props, ok := spec.Parameters["properties"].(map[string]any)
	if !ok {
		return args, ErrInvalidModelPlan
	}
	required, ok := spec.Parameters["required"].([]string)
	if !ok {
		return args, ErrInvalidModelPlan
	}
	for _, name := range required {
		if _, found := params[name]; !found {
			return args, ErrInvalidModelPlan
		}
	}
	for key, value := range params {
		if _, allowed := props[key]; !allowed {
			return args, ErrInvalidModelPlan
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return args, ErrInvalidModelPlan
		}
		switch key {
		case "search":
			args.Search = text
		case "record_id":
			args.RecordID = text
		default:
			return args, ErrInvalidModelPlan
		}
	}
	if len(args.Search) > 80 || strings.ContainsAny(args.Search, "\r\n") ||
		len(args.RecordID) > 128 || strings.ContainsAny(args.RecordID, "\r\n") {
		return args, ErrInvalidModelPlan
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return args, ErrInvalidModelPlan
	}
	var tail any
	if err := decoder.Decode(&tail); err != io.EOF {
		return args, ErrInvalidModelPlan
	}
	if (spec.Name == "published_policies" || spec.Name == "published_knowledge") && strings.TrimSpace(args.Search) == "" {
		return args, ErrInvalidModelPlan
	}
	return args, nil
}

func rememberIDs(known map[string]map[string]bool, tool string, rows any) {
	var kind string
	switch tool {
	case "find_customers":
		kind = "customer"
	case "list_conversations":
		kind = "conversation"
	case "list_leads":
		kind = "lead"
	case "list_transactions":
		kind = "transaction"
	default:
		return
	}
	data, ok := rows.(map[string]any)
	if !ok {
		return
	}
	sample, ok := data["sample"].([]map[string]any)
	if !ok {
		return
	}
	for _, row := range sample {
		id, _ := row["id"].(string)
		if id != "" {
			known[kind][id] = true
		}
		// Each sample row came from a tenant-checked Go read. Follow its
		// customer reference rather than forcing the model to guess an ID.
		// The destination getter still enforces actor and business scope.
		if tool == "list_transactions" || tool == "list_leads" || tool == "list_conversations" {
			customerID, _ := row["customer_id"].(string)
			if customerID != "" {
				known["customer"][customerID] = true
			}
		}
	}
}

func (a *Assistant) assistConversational(ctx context.Context, req AssistRequest, question string) (AssistResponse, error) {
	var empty AssistResponse
	if a.Native == nil || a.Discovery == nil {
		return empty, ErrAssistantUnavailable
	}
	if !authorization.Has(req.Actor, authorization.PermissionMerchantAIUse) ||
		string(req.Actor.BusinessID) != req.Business || strings.TrimSpace(string(req.Actor.PrincipalID)) == "" {
		return empty, ErrReadDenied
	}
	available := a.Discovery.Available(req.Actor)
	add := func(name, permission string, ready bool) {
		if ready && authorization.Has(req.Actor, permission) {
			available = append(available, name)
		}
	}
	add("customer_activity", authorization.PermissionCustomersRead, a.Discovery.GetCustomer != nil)
	add("published_policies", authorization.PermissionBusinessRead, a.Published != nil)
	add("published_knowledge", authorization.PermissionBusinessRead, a.Published != nil)
	add("conversation_history", authorization.PermissionConversationsRead, a.History != nil)
	add("customer_record", authorization.PermissionCustomersRead, a.Records != nil)
	add("transaction_record", authorization.PermissionSalesRead, a.Records != nil)
	add("lead_evidence", authorization.PermissionSalesRead, a.Leads != nil)
	if len(available) == 0 {
		return empty, ErrReadDenied
	}
	defs := toolDefinitions(available)
	allowed := make(map[string]ToolDefinition, len(defs))
	for _, def := range defs {
		allowed[def.Name] = def
	}
	known := map[string]map[string]bool{
		"customer": {}, "conversation": {}, "transaction": {}, "lead": {},
	}
	if req.RecordID != "" && len(req.RecordID) <= 128 {
		switch req.Search {
		case "screen:customers":
			known["customer"][req.RecordID] = true
		case "screen:chat":
			known["conversation"][req.RecordID] = true
		case "screen:transactions":
			known["transaction"][req.RecordID] = true
		case "screen:leads":
			known["lead"][req.RecordID] = true
		}
	}
	messages := []ToolMessage{{Role: "user", Content: question}}
	links := make([]DiscoveryLink, 0, 12)
	coverages := make([]string, 0, 8)
	// Repeated model reads are served from the verified result cache. Repeating
	// a query must not burn database resources or invalidate a merchant request.
	readCache := map[string]struct {
		callID   string
		coverage string
	}{}
	seenIDs := map[string]bool{}
	pinned := ""
	toolCount := 0
	evidenceBytes := 0
	for {
		// Read-only tools continue until the model naturally returns a final
		// answer. Context cancellation and existing payload-size checks still
		// protect the server; no arbitrary call or round count ends the task.
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		reply, err := a.Native.CompleteTools(ctx, messages, defs, pinned)
		if err != nil {
			return empty, fmt.Errorf("%w: %w", ErrModelRouting, err)
		}
		if reply.Provider == "" || (pinned != "" && pinned != reply.Provider) {
			return empty, ErrInvalidSourceEvidence
		}
		pinned = reply.Provider
		if len(reply.Calls) == 0 {
			if reply.FinishReason != "stop" {
				return empty, ErrInvalidModelPlan
			}
			answer := strings.TrimSpace(assistantUUID.ReplaceAllString(reply.Content, "السجل"))
			if len(answer) == 0 || len(answer) > 4000 {
				return empty, ErrInvalidSourceEvidence
			}
			if len(links) > 15 {
				links = links[:15]
			}
			raw, err := json.Marshal(map[string]any{"links": links, "coverage": coverages})
			if err != nil {
				return empty, ErrInvalidSourceEvidence
			}
			return AssistResponse{Answer: answer, ProjectionID: "auto.v1", Evidence: raw, ReviewRequired: true}, nil
		}
		if reply.FinishReason != "tool_calls" || len(reply.Calls) == 0 {
			log.Printf("[MerchantAssist] stage=invalid_tool_batch finish=%s count=%d", reply.FinishReason, len(reply.Calls))
			return empty, ErrInvalidModelPlan
		}
		// Validate the complete untrusted batch before executing any read.
		// Record references must come from verified reads; sequential calls
		// inside the same batch may discover them for later calls in that batch.
		decoded := make([]nativeReadArgs, len(reply.Calls))
		for i, call := range reply.Calls {
			def, present := allowed[call.Name]
			if !present {
				log.Printf("[MerchantAssist] stage=tool_contract_rejected reason=unknown_or_unauthorized_tool batch_index=%d", i+1)
				return empty, ErrInvalidModelPlan
			}
			if call.ID == "" || seenIDs[call.ID] {
				log.Printf("[MerchantAssist] stage=tool_contract_rejected reason=missing_or_duplicate_call_id tool=%s batch_index=%d", call.Name, i+1)
				return empty, ErrInvalidModelPlan
			}
			args, err := decodeReadArgs(def, call.Arguments)
			if err != nil {
				log.Printf("[MerchantAssist] stage=tool_contract_rejected reason=invalid_arguments tool=%s batch_index=%d", call.Name, i+1)
				return empty, err
			}
			seenIDs[call.ID] = true
			decoded[i] = args
		}
		log.Printf("[MerchantAssist] stage=tool_batch_validated count=%d previously_executed=%d", len(reply.Calls), toolCount)
		// DeepSeek Chat Completions expects one assistant turn containing all
		// calls and one tool message per call, in its original ID order.
		results := make([]ToolMessage, 0, len(reply.Calls))
		for i, call := range reply.Calls {
			args := decoded[i]
			signature := call.Name + "|" + args.Search + "|" + args.RecordID
			kind := map[string]string{
				"customer_activity": "customer", "customer_record": "customer",
				"conversation_history": "conversation", "transaction_record": "transaction",
				"transaction_customer_details": "transaction", "lead_evidence": "lead",
			}[call.Name]
			if kind != "" && (args.RecordID == "" || !known[kind][args.RecordID]) {
				log.Printf("[MerchantAssist] stage=tool_contract_rejected reason=unverified_record_reference tool=%s record_kind=%s batch_index=%d",
					call.Name, kind, i+1)
				return empty, ErrInvalidModelPlan
			}
			if prior, ok := readCache[signature]; ok {
				// The earlier tool message remains in the conversation history.
				// Return a small reference rather than re-reading or duplicating
				// evidence (which could exhaust the context window).
				cached, _ := json.Marshal(map[string]any{
					"cached": true, "previous_tool_call_id": prior.callID,
					"coverage": prior.coverage,
				})
				results = append(results, ToolMessage{Role: "tool", ToolCallID: call.ID, Content: string(cached)})
				log.Printf("[MerchantAssist] stage=tool_read_cache_hit tool=%s batch_index=%d", call.Name, i+1)
				continue
			}
			log.Printf("[MerchantAssist] stage=tool_read_start tool=%s batch_index=%d", call.Name, i+1)
			var data any
			var coverage string
			switch call.Name {
			case "published_policies", "published_knowledge":
				kind := PublishedPolicies
				if call.Name == "published_knowledge" {
					kind = PublishedKnowledge
				}
				read, err := a.Published.Read(ctx, PublishedReadRequest{
					Actor: req.Actor, Business: req.Business, Kind: kind,
					Task: contracts.TaskExplain, Search: args.Search, Limit: 3,
				})
				if err != nil {
					logAssistReadFailure(call.Name, err)
					return empty, err
				}
				for i := range read.Knowledge {
					safe, err := a.Sanitizer.Sanitize(ctx, ContentMessage, read.Knowledge[i].Value.Content)
					if err != nil {
						return empty, ErrInvalidSourceEvidence
					}
					read.Knowledge[i].Value.Content = safe
				}
				for i := range read.Policies {
					safe, err := a.Sanitizer.Sanitize(ctx, ContentMessage, read.Policies[i].Value.Summary)
					if err != nil {
						return empty, ErrInvalidSourceEvidence
					}
					read.Policies[i].Value.Summary = safe
				}
				data, coverage = read, "published_matches_only"
			case "conversation_history":
				read, err := a.History.Read(ctx, ConversationHistoryRequest{
					Actor: req.Actor, Business: req.Business, ConversationID: args.RecordID,
					Task: contracts.TaskSummarize,
				})
				if err != nil {
					logAssistReadFailure(call.Name, err)
					return empty, err
				}
				data, coverage = read, read.Coverage
				links = append(links, DiscoveryLink{Title: "فتح المحادثة", URL: "/chat?conversation_id=" + args.RecordID})
			case "customer_record", "transaction_record":
				recordKind := RecordCustomer
				if call.Name == "transaction_record" {
					recordKind = RecordTransaction
				}
				read, err := a.Records.Read(ctx, RecordReadRequest{
					Actor: req.Actor, Business: req.Business, Kind: recordKind,
					RecordID: args.RecordID, Task: contracts.TaskExplain,
				})
				if err != nil {
					logAssistReadFailure(call.Name, err)
					return empty, err
				}
				data, coverage = read, "single_record_verified"
			case "lead_evidence":
				read, err := a.Leads.Read(ctx, LeadEvidenceRequest{
					Actor: req.Actor, Business: req.Business,
					LeadID: args.RecordID, Task: contracts.TaskExplain,
				})
				if err != nil {
					logAssistReadFailure(call.Name, err)
					return empty, err
				}
				data, coverage = read, "single_lead_verified"
			default:
				read, err := a.Discovery.Read(ctx, req.Actor, req.Business, DiscoveryCall{
					Name: call.Name, Search: args.Search, RecordID: args.RecordID,
				})
				if err != nil {
					logAssistReadFailure(call.Name, err)
					return empty, err
				}
				rememberIDs(known, call.Name, read.Rows)
				if call.Name == "customer_activity" {
					if related, ok := read.Rows.(map[string]any); ok {
						for _, pair := range []struct {
							key  string
							kind string
						}{
							{"conversations", "conversation"}, {"transactions", "transaction"},
						} {
							if block, ok := related[pair.key].(map[string]any); ok {
								if sample, ok := block["sample"].([]map[string]any); ok {
									for _, row := range sample {
										if id, ok := row["id"].(string); ok {
											known[pair.kind][id] = true
										}
									}
								}
							}
						}
					}
				}
				data = map[string]any{
					"tool": read.Tool, "rows": read.Rows, "has_more": read.HasMore, "coverage": read.Coverage,
				}
				coverage = read.Coverage
				links = append(links, read.Links...)
			}
			output, err := json.Marshal(data)
			if err != nil || len(output) == 0 || len(output) > 28000 ||
				evidenceBytes+len(output) > assistMaxToolEvidence {
				return empty, ErrInvalidSourceEvidence
			}
			evidenceBytes += len(output)
			coverages = append(coverages, coverage)
			results = append(results, ToolMessage{Role: "tool", ToolCallID: call.ID, Content: string(output)})
			readCache[signature] = struct {
				callID   string
				coverage string
			}{callID: call.ID, coverage: coverage}
			toolCount++
			log.Printf("[MerchantAssist] stage=tool_read_complete tool=%s batch_index=%d bytes=%d", call.Name, i+1, len(output))
		}
		messages = append(messages, ToolMessage{Role: "assistant", ToolCalls: reply.Calls})
		messages = append(messages, results...)

	}
}
