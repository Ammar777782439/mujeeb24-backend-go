# Customer AI Architectural Assessment Report

## A. Current Customer AI flow
1. **Trigger**: A customer message triggers `AutoReplyService.Handle()`.
2. **Context & Handoff**: The context builder checks `Ownership` and `State`. If the conversation is owned by a `human` or in `waiting_human` state, it immediately halts (`no_action`), correctly enforcing the Human Handoff rules.
3. **Execution**: `AutoReplyService` invokes `Runtime.Decide` (implemented by `gemini.Client`).
4. **Tool Provisioning (The Leak)**: `gemini.Client` fetches tools from the generic `capabilityRegistry`. **Critically, this registry is polluted** during bootstrap with `catalog_authoring` and all `merchant_*` capabilities. Thus, the Customer AI model is dangerously given tools to mutate the catalog and execute merchant operations.
5. **Tool Loop & Pagination**: The Gemini client runs a multi-turn loop (up to `safetyTurnBudget`). If the model calls `catalog_data`, it executes the tool. The client has **hardcoded logic** (`CatalogRetrievalSession`) that forces the model to fetch more pages if `HasMore=true` by injecting system prompts mid-loop.
6. **Hardcoded Output**: Once the model finally outputs `textContent`, the Gemini client **ignores the LLM's intent and reasoning**, forcing the response into a hardcoded `AIDecisionProposal` with `RequestedAction: "answer"`, `IntentBase: "information_request"`, and `RequiresHuman: false`. 
7. **Finalization**: `AutoReplyService` receives this rigid proposal, passes it to the `PolicyEvaluator`, saves it to PostgreSQL (`AIDecisionRepository`), and enqueues an `OutboundMessage`.

## B. Existing capabilities
| Capability | Location | Purpose | Customer AI? | Legacy/Merchant? |
|---|---|---|---|---|
| `catalog_data` | `ai_capabilities.go` | Retrieve catalog data deterministically (list, get, offers, variants) | **Yes** | No |
| `catalog_authoring` | `ai_capabilities.go` | Create/author catalog items, offers, variants. | **No (Leaked)** | Yes (Merchant logic) |
| `list_catalogs` | `merchant_ai_capabilities.go` | List merchant catalogs | **No (Leaked)** | Yes |
| `create_catalog` | `merchant_ai_capabilities.go` | Create new catalog | **No (Leaked)** | Yes |
| `author_catalog_item` | `merchant_ai_capabilities.go` | Mutate/Author items | **No (Leaked)** | Yes |
| `search_catalog` | `merchant_ai_capabilities.go` | Keyword search in catalog | **No (Leaked)** | Yes |
| `get_item_details` | `merchant_ai_capabilities.go` | Fetch item details | **No (Leaked)** | Yes |
| `create_variant` | `merchant_ai_capabilities.go` | Create a variant | **No (Leaked)** | Yes |
| `update_catalog_item` | `merchant_ai_capabilities.go` | Update item | **No (Leaked)** | Yes |
| `archive_catalog_item` | `merchant_ai_capabilities.go` | Archive item | **No (Leaked)** | Yes |
| `parse_file` | `merchant_ai_capabilities.go` | Parse CSV files | **No (Leaked)** | Yes |

*(Note: There are **zero** capabilities related to Sales (e.g. `create_order`, `draft_lead`). The model cannot transition to a Sales flow because no such capabilities exist in the codebase.)*

## C. AICapability contract assessment
The `AICapability`, `AICapabilityDispatcher`, and `AICapabilityRegistry` interfaces are architecturally sound as generic ports. 
**The problem is the wiring**: In `internal/bootstrap/api.go`, a single global `CapabilityRegistry` instance is populated with Customer AI tools, Catalog Authoring tools, AND Merchant AI tools, and then passed to both the Customer Gemini client and Merchant Gemini client. We must split this into isolated `CustomerAICapabilityRegistry` and `MerchantAICapabilityRegistry` instances.

## D. AICapabilityResult assessment
Fields: `HasMore`, `NextCursor`, `Incomplete`, `Operation`, `StreamKey`
These fields are **domain-specific pagination details leaked into a generic contract**. They were added specifically to support the hardcoded `CatalogRetrievalSession` loop inside the Gemini Client. 
For a true Agentic flow, pagination should be handled by the LLM itself (by returning `has_more` and `next_cursor` inside the `Data` JSON block), rather than baking pagination controls into the core capability contract.

## E. AIDecisionProposal assessment
* **لماذا ما زال موجودًا؟** It acts as the established data schema for PostgreSQL persistence (`AIDecisionRepository`) and the Outbox worker pipeline.
* **هل هو مركز منطق Customer AI؟** No. Because `gemini.Client` hardcodes `RequestedAction: "answer"` and `IntentBase: "information_request"`, the AI does not meaningfully populate this struct. The LLM acts purely as a text generator.
* **هل يمكن أن يستمر كـ output/adapter؟** Yes. We do not need to delete it or create new tables. The Gemini client should be updated to use *Structured Outputs* (or a final `submit_decision` function call) allowing the LLM to populate `IntentBase`, `RequestedAction`, and `RequiresHuman` dynamically, which is then mapped into `AIDecisionProposal` as an adapter.

## F. AutoReply + Human Handoff flow
* **State & Ownership**: `AutoReplyService` perfectly respects the `human` ownership and `waiting_human` state, aborting auto-replies correctly.
* **The Flaw**: The LLM cannot actively request a human! Because `gemini.Client` hardcodes `RequiresHuman: false`, the model is trapped. The only way it triggers a handoff is if it exhausts its "safety budget" or if a hardcoded keyword trigger (`isSubscriptionHandoffIntent`) is hit on the hardcoded `information_request` intent.

## G. Detected legacy/mixed logic
1. **Severe Privilege Escalation (Tool Leak)**: Customer AI is provided with all `catalog_authoring` and `merchant_*` capabilities.
2. **Hardcoded Agent Logic**: The Gemini client strips the agent of agency. It forces a hardcoded `CatalogRetrievalSession` loop for pagination and forces the final output to always be a basic text response (`RequestedAction: "answer"`).
3. **No Sales Flow**: You cannot transition to a sales flow because there are no sales capabilities implemented for Customer AI to call.

## H. Recommended minimal changes
1. **Registry Isolation**: In `bootstrap/api.go`, instantiate two separate registries. Pass only the `CatalogDataCapability` to the Customer AI runtime.
2. **Restore LLM Agency (Gemini Client)**: 
   - Remove the hardcoded `CatalogRetrievalSession` forced-pagination loop. Let the model see `next_cursor` in the tool output and decide if it wants to call the tool again.
   - Stop hardcoding `AIDecisionProposal`. Provide the model with a `submit_decision` tool (or use Structured Outputs) so it can output its true intent (`information_request`, `request_human`, etc.), `RequiresHuman` flag, and `RequestedAction`.
3. **Contract Cleanup**: Remove `HasMore`, `NextCursor`, `Operation`, `StreamKey`, etc. from `AICapabilityResult` and embed them inside the `Data` map returned by the capabilities.

## I. Files that should be modified
* `internal/bootstrap/api.go` (Split registries)
* `internal/adapters/secondary/ai/gemini/client.go` (Rewrite `Decide` to respect model intent, remove hardcoded proposal and pagination loop)
* `internal/application/ports/ai_capability.go` (Clean up `AICapabilityResult`)
* `internal/application/services/ai_capabilities.go` (Adjust return format for `CatalogDataCapability`)

## J. Files that must NOT be touched
* `migrations/*.sql` (No DB schema changes)
* `internal/application/ports/ai_runtime.go` (`AIDecisionProposal` must remain as-is for the pipeline)
* `internal/application/services/auto_reply.go` (Handoff and Outbox logic works correctly)
* `internal/adapters/primary/http/handlers/*` (API boundary)
