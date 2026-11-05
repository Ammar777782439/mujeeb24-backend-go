# internal/application/services

## Overview
Core application use-case implementation layer orchestrating business transactions, event ingestion, outbound message scheduling, background processing, catalog management, sales workflows, and team governance.

## Interface
- `OutboxProcessor`: Worker engine that claims outbox leases, calls SocialAPI outside DB transactions, updates message statuses, and isolates failures into dead-letter ([outbox_processor.go](./outbox_processor.go#L16)). Worker cycles run under a timeout (default 10s). Current Code Behavior: In `cmd/worker/main.go`, cycle processing errors trigger `log.Fatal(err)`, terminating the entire OS process and requiring external supervisor restart (systemd/k8s). Proposed Improvement: Per-item error isolation without crashing the worker process.
- `SocialAPIWebhookService`: Ingests external webhook payloads, performs deduplication via `inbound_event_ledger`, and routes messages ([webhook_ingestion.go](./webhook_ingestion.go)). Inbound Execution Flow: `SocialAPIWebhookService` receives webhook payload -> deduplicates via `inbound_event_ledger` -> persists customer message -> evaluates `InboundAutomationService` rules -> dispatches customer conversation turn to `AutoReplyService`.
- `AutoReplyService`: Central conversational B2C sales engine (1,402 lines) orchestrating customer turns, capability decision dispatch (Contract v12), and outbound reply staging ([auto_reply.go](./auto_reply.go#L44)).
- `AIRunLifecycle` State Machine: ADR-031 mandatory 8-stage execution lifecycle (`RECEIVED -> CONTEXT_BUILT -> RUNNING -> VALIDATING -> AUTHORIZED -> EXECUTING -> COMPLETED/FAILED/CANCELLED`) managed via `AIRunRepository`.
- `CatalogBatchController`: Paged catalog candidate evaluation with token budget controls ([catalog_batch_controller.go](./catalog_batch_controller.go)).
  - Current Code Behavior (Issue 4): Loops through all catalog pages and batches without an enforced batch ceiling.
  - Known Operational Risk: For large catalogs, this unbounded loop risks token budget exhaustion and cycle timeouts.
  - Proposed Improvement: Enforce a hard ceiling on batch evaluations (e.g. max 3 batches / ~150 candidates) before transitioning to `handoff_to_human`.
- `ConversationSummaryService`: Generates periodic conversation summaries and context compressions ([conversation_summary.go](./conversation_summary.go)).
- `CustomerRuntimeService`: Manages customer lifecycle, contact points, and profile merging ([customer_services.go](./customer_services.go)).
- `ConversationRuntimeService`: Manages conversations, message threading, private notes, assignees, and read cursors ([conversation_services.go](./conversation_services.go)).
- `ManualOutboundMessageService`: Atomically creates outbound communication records and outbox entries ([manual_outbound.go](./manual_outbound.go)).
- `CatalogCommandServices`: Creates and updates catalogs, items, variants, offers, and attribute schema versions ([catalog_commands.go](./catalog_commands.go)).
- `CatalogImageUploadService.UploadBase64`: Authoritative server-verified base64 image upload verifying MIME types and byte bounds ([catalog_media_service.go](./catalog_media_service.go)). Presigned storage URLs are restricted to read-only (GET) retrieval.
- `CustomerSalesWorkflowService` / `SalesActionProcessor`: Executes sales capabilities, qualifications, leads, and transaction drafts ([customer_sales_workflow.go](./customer_sales_workflow.go) / [sales_action_processor.go](./sales_action_processor.go)).
- `TeamService`: Handles team invitations, role changes, and member revocations ([team_services.go](./team_services.go)).
- `BusinessManagementService`: Manages business profile, policies, and onboarding status ([business_management.go](./business_management.go)).
- `AutomationRuleService` / `InboundAutomationService`: Evaluates rules and auto-reply triggers ([automation_rule_services.go](./automation_rule_services.go)).

## Invariants
- Zero network calls inside DB transactions: All SocialAPI, LLM, or S3/B2 storage calls occur strictly outside SQL transactions.
- No blind retries: Unknown delivery outcomes move immediately to `dead_letter` for manual reconciliation. (Known Operational Gap: No automated background reconciliation daemon currently reprocesses dead-lettered records; manual SQL review required).
- Multi-tenancy: Every business query and command strictly verifies `business_id` scoping via `ScopeProvider`.
- Last Owner Protection: Business cannot revoke or demote its last active `owner`.
- Outbox pattern ensures at-least-once delivery for all outgoing channel messages.
- AI Provider Fallback Architecture (Issue 21):
  - Design Policy: Multi-provider architecture with Gemini as primary and DeepSeek as fallback on HTTP 503 at transport level.
  - Active Runtime Implementation: In `internal/bootstrap/integrations.go:102`, DeepSeek is currently wired directly with `standby=nil` (fallback router is a documented design policy, not yet active in runtime bootstrap on `main`).

## Side Effects
- Mutates PostgreSQL database state within transactions.
- Dispatches HTTP requests to external channel providers (`SocialAPI`) and AI models.
- Claims worker leases in `outbox_entries`.
- Emits real-time messages via `RealtimePublisher`.

## Verification
- Run: `go test ./internal/application/services`
