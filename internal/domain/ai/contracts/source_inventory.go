package contracts

// SourceDisposition is a source-level audit status, NOT permission to
// send a row, column or payload to a model. ProjectionCandidate means
// review may begin; NO projection is approved by this inventory.
type SourceDisposition string

const (
	SourceProjectionCandidate SourceDisposition = "projection_candidate"
	SourceInternalOnly        SourceDisposition = "internal_only"
	SourceProhibited          SourceDisposition = "prohibited"
	SourceRetired             SourceDisposition = "retired"
)

// TableSourceRecord maps an actual migration-created SQL table to its domain
// and conservative AI source classification.
// No raw database rows are exposed through this type.
type TableSourceRecord struct {
	Table               string
	IntroducedMigration int
	RemovedMigration    int
	Domain              string
	Disposition         SourceDisposition
}

// sourceInventory is an explicit deny-by-default census from migrations
// 000001..000081 on backend main. 000053 drops legacy Chatwoot tables.
// Adding any CREATE TABLE in a future migration requires updating this list
// and auditing its columns before any AI projection becomes accessible.
var sourceInventory = []TableSourceRecord{
	{Table: "businesses", IntroducedMigration: 1, RemovedMigration: 0, Domain: "business", Disposition: SourceProjectionCandidate},
	{Table: "business_policies", IntroducedMigration: 2, RemovedMigration: 0, Domain: "policy", Disposition: SourceProjectionCandidate},
	{Table: "channel_connections", IntroducedMigration: 3, RemovedMigration: 0, Domain: "channels", Disposition: SourceProjectionCandidate},
	{Table: "channel_connection_capabilities", IntroducedMigration: 4, RemovedMigration: 0, Domain: "channels", Disposition: SourceProjectionCandidate},
	{Table: "customers", IntroducedMigration: 5, RemovedMigration: 0, Domain: "customer", Disposition: SourceProjectionCandidate},
	{Table: "external_identities", IntroducedMigration: 6, RemovedMigration: 0, Domain: "identity", Disposition: SourceProhibited},
	{Table: "identity_matches", IntroducedMigration: 7, RemovedMigration: 0, Domain: "identity", Disposition: SourceProhibited},
	{Table: "conversations", IntroducedMigration: 8, RemovedMigration: 0, Domain: "inbox", Disposition: SourceProjectionCandidate},
	{Table: "conversation_references", IntroducedMigration: 9, RemovedMigration: 0, Domain: "inbox", Disposition: SourceInternalOnly},
	{Table: "inbound_event_ledger", IntroducedMigration: 10, RemovedMigration: 0, Domain: "transport", Disposition: SourceInternalOnly},
	{Table: "outbound_messages", IntroducedMigration: 11, RemovedMigration: 0, Domain: "transport", Disposition: SourceInternalOnly},
	{Table: "outbox_entries", IntroducedMigration: 12, RemovedMigration: 0, Domain: "transport", Disposition: SourceInternalOnly},
	{Table: "catalogs", IntroducedMigration: 13, RemovedMigration: 0, Domain: "catalog", Disposition: SourceProjectionCandidate},
	{Table: "attribute_schemas", IntroducedMigration: 14, RemovedMigration: 0, Domain: "catalog", Disposition: SourceProjectionCandidate},
	{Table: "attribute_definitions", IntroducedMigration: 15, RemovedMigration: 0, Domain: "catalog", Disposition: SourceProjectionCandidate},
	{Table: "catalog_items", IntroducedMigration: 16, RemovedMigration: 0, Domain: "catalog", Disposition: SourceProjectionCandidate},
	{Table: "variants", IntroducedMigration: 17, RemovedMigration: 0, Domain: "catalog", Disposition: SourceProjectionCandidate},
	{Table: "offers", IntroducedMigration: 18, RemovedMigration: 0, Domain: "catalog", Disposition: SourceProjectionCandidate},
	{Table: "leads", IntroducedMigration: 19, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "lead_attributions", IntroducedMigration: 20, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "lead_scores", IntroducedMigration: 21, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "commercial_transactions", IntroducedMigration: 22, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "transaction_confirmations", IntroducedMigration: 23, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "transaction_reviews", IntroducedMigration: 24, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "order_lines", IntroducedMigration: 25, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "audit_events", IntroducedMigration: 26, RemovedMigration: 0, Domain: "audit", Disposition: SourceInternalOnly},
	{Table: "ai_decisions", IntroducedMigration: 27, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "communication_messages", IntroducedMigration: 28, RemovedMigration: 0, Domain: "inbox", Disposition: SourceProjectionCandidate},
	{Table: "chatwoot_contact_links", IntroducedMigration: 34, RemovedMigration: 53, Domain: "retired", Disposition: SourceRetired},
	{Table: "chatwoot_workspace_bindings", IntroducedMigration: 34, RemovedMigration: 53, Domain: "retired", Disposition: SourceRetired},
	{Table: "business_policy_versions", IntroducedMigration: 36, RemovedMigration: 0, Domain: "knowledge", Disposition: SourceProjectionCandidate},
	{Table: "knowledge_documents", IntroducedMigration: 36, RemovedMigration: 0, Domain: "knowledge", Disposition: SourceProjectionCandidate},
	{Table: "channel_provisioning_sessions", IntroducedMigration: 37, RemovedMigration: 0, Domain: "channels", Disposition: SourceProhibited},
	{Table: "inbound_webhook_payloads", IntroducedMigration: 38, RemovedMigration: 0, Domain: "transport", Disposition: SourceProhibited},
	{Table: "chatwoot_mirror_jobs", IntroducedMigration: 39, RemovedMigration: 53, Domain: "retired", Disposition: SourceRetired},
	{Table: "outbound_delivery_status_updates", IntroducedMigration: 40, RemovedMigration: 0, Domain: "transport", Disposition: SourceInternalOnly},
	{Table: "business_memberships", IntroducedMigration: 41, RemovedMigration: 0, Domain: "identity", Disposition: SourceInternalOnly},
	{Table: "principals", IntroducedMigration: 41, RemovedMigration: 0, Domain: "identity", Disposition: SourceProhibited},
	{Table: "refresh_sessions", IntroducedMigration: 41, RemovedMigration: 0, Domain: "identity", Disposition: SourceProhibited},
	{Table: "conversation_labels", IntroducedMigration: 44, RemovedMigration: 0, Domain: "inbox", Disposition: SourceProjectionCandidate},
	{Table: "channel_connection_state_events", IntroducedMigration: 45, RemovedMigration: 0, Domain: "channels", Disposition: SourceInternalOnly},
	{Table: "conversation_read_cursors", IntroducedMigration: 46, RemovedMigration: 0, Domain: "inbox", Disposition: SourceInternalOnly},
	{Table: "canned_replies", IntroducedMigration: 47, RemovedMigration: 0, Domain: "inbox", Disposition: SourceProjectionCandidate},
	{Table: "automation_executions", IntroducedMigration: 48, RemovedMigration: 0, Domain: "automation", Disposition: SourceInternalOnly},
	{Table: "automation_rules", IntroducedMigration: 48, RemovedMigration: 0, Domain: "automation", Disposition: SourceProjectionCandidate},
	{Table: "team_invitations", IntroducedMigration: 49, RemovedMigration: 0, Domain: "identity", Disposition: SourceProhibited},
	{Table: "platform_super_admins", IntroducedMigration: 50, RemovedMigration: 0, Domain: "identity", Disposition: SourceProhibited},
	{Table: "conversation_state", IntroducedMigration: 52, RemovedMigration: 0, Domain: "inbox", Disposition: SourceProjectionCandidate},
	{Table: "merchant_ai_messages", IntroducedMigration: 54, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "merchant_ai_sessions", IntroducedMigration: 54, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_catalog_batches", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_gemini_interactions", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_run_attempts", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_runs", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_stage_latencies", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_tool_calls", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_usage_telemetry", IntroducedMigration: 55, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "plans", IntroducedMigration: 61, RemovedMigration: 0, Domain: "billing", Disposition: SourceProjectionCandidate},
	{Table: "ai_provider_pricing_versions", IntroducedMigration: 62, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_usage_records", IntroducedMigration: 62, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "subscription_ai_usage", IntroducedMigration: 62, RemovedMigration: 0, Domain: "billing", Disposition: SourceProjectionCandidate},
	{Table: "subscription_payments", IntroducedMigration: 62, RemovedMigration: 0, Domain: "billing", Disposition: SourceProhibited},
	{Table: "subscriptions", IntroducedMigration: 62, RemovedMigration: 0, Domain: "billing", Disposition: SourceProjectionCandidate},
	{Table: "support_messages", IntroducedMigration: 63, RemovedMigration: 0, Domain: "support", Disposition: SourceInternalOnly},
	{Table: "support_tickets", IntroducedMigration: 63, RemovedMigration: 0, Domain: "support", Disposition: SourceInternalOnly},
	{Table: "platform_audit_events", IntroducedMigration: 64, RemovedMigration: 0, Domain: "audit", Disposition: SourceInternalOnly},
	{Table: "ai_configuration_versions", IntroducedMigration: 65, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "ai_provider_credentials", IntroducedMigration: 65, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceProhibited},
	{Table: "ai_provider_models", IntroducedMigration: 65, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "channel_provider_brands", IntroducedMigration: 66, RemovedMigration: 0, Domain: "channels", Disposition: SourceInternalOnly},
	{Table: "merchant_ai_proposals", IntroducedMigration: 69, RemovedMigration: 0, Domain: "ai_ops", Disposition: SourceInternalOnly},
	{Table: "channel_history_purges", IntroducedMigration: 70, RemovedMigration: 0, Domain: "channels", Disposition: SourceInternalOnly},
	{Table: "sales_action_jobs", IntroducedMigration: 74, RemovedMigration: 0, Domain: "sales", Disposition: SourceInternalOnly},
	{Table: "sales_command_results", IntroducedMigration: 74, RemovedMigration: 0, Domain: "sales", Disposition: SourceInternalOnly},
	{Table: "sales_confirmation_requests", IntroducedMigration: 74, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "sales_transaction_requirements", IntroducedMigration: 74, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "sales_workflows", IntroducedMigration: 74, RemovedMigration: 0, Domain: "sales", Disposition: SourceProjectionCandidate},
	{Table: "subscription_addon_payments", IntroducedMigration: 77, RemovedMigration: 0, Domain: "billing", Disposition: SourceProhibited},
	{Table: "subscription_addons", IntroducedMigration: 77, RemovedMigration: 0, Domain: "billing", Disposition: SourceProjectionCandidate},
	{Table: "subscription_channel_pass_capacity_approvals", IntroducedMigration: 78, RemovedMigration: 0, Domain: "billing", Disposition: SourceInternalOnly},
}

// SourceInventory returns a defensive copy to prevent accidental mutation
// of the canonical source classification.
func SourceInventory() []TableSourceRecord {
	return append([]TableSourceRecord(nil), sourceInventory...)
}

// LookupTableSource returns only classification metadata; it does not
// confer authorization or approval to read any row or field.
func LookupTableSource(table string) (TableSourceRecord, bool) {
	for _, entry := range sourceInventory {
		if entry.Table == table {
			return entry, true
		}
	}
	return TableSourceRecord{}, false
}
