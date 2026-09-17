--
-- PostgreSQL database dump
--

\restrict eN7N1as40eswpAC4QbARumJSWY0BZNkobeeIqgvKtrqONGbzgcf4EWyc7hMRDKV

-- Dumped from database version 16.15
-- Dumped by pg_dump version 16.15

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: reject_audit_event_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_audit_event_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: ai_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_decisions (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    conversation_id uuid,
    source_message_reference text,
    intent_base text NOT NULL,
    domain_context text,
    entities jsonb DEFAULT '{}'::jsonb NOT NULL,
    evidence_references jsonb DEFAULT '[]'::jsonb NOT NULL,
    requested_action text NOT NULL,
    confidence_value numeric(5,4),
    confidence_band text NOT NULL,
    requires_human boolean NOT NULL,
    missing_information jsonb DEFAULT '[]'::jsonb NOT NULL,
    reason_codes jsonb DEFAULT '[]'::jsonb NOT NULL,
    policy_reference text,
    policy_version text NOT NULL,
    knowledge_version text,
    model_reference text,
    schema_version integer NOT NULL,
    lifecycle text NOT NULL,
    policy_decision text,
    outcome text,
    execution_reference text,
    correlation_id uuid,
    causation_id uuid,
    expires_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    human_review_reason text,
    human_review_requested_at timestamp with time zone,
    human_review_requested_by text,
    decided_at timestamp with time zone,
    CONSTRAINT ai_decisions_action_chk CHECK ((requested_action = ANY (ARRAY['answer'::text, 'ask_clarification'::text, 'create_lead'::text, 'update_lead'::text, 'create_transaction_draft'::text, 'request_availability_check'::text, 'request_price_check'::text, 'request_human'::text, 'no_action'::text]))),
    CONSTRAINT ai_decisions_confidence_band_chk CHECK ((confidence_band = ANY (ARRAY['unknown'::text, 'low'::text, 'medium'::text, 'high'::text]))),
    CONSTRAINT ai_decisions_confidence_value_chk CHECK (((confidence_value IS NULL) OR ((confidence_value >= (0)::numeric) AND (confidence_value <= (1)::numeric)))),
    CONSTRAINT ai_decisions_conversation_ref_chk CHECK (((source_message_reference IS NULL) OR (length(btrim(source_message_reference)) > 0))),
    CONSTRAINT ai_decisions_decided_at_chk CHECK (((decided_at IS NULL) OR (decided_at >= created_at))),
    CONSTRAINT ai_decisions_entities_object_chk CHECK ((jsonb_typeof(entities) = 'object'::text)),
    CONSTRAINT ai_decisions_evidence_array_chk CHECK ((jsonb_typeof(evidence_references) = 'array'::text)),
    CONSTRAINT ai_decisions_expiry_chk CHECK (((expires_at IS NULL) OR (expires_at >= created_at))),
    CONSTRAINT ai_decisions_human_review_pair_chk CHECK ((((human_review_requested_at IS NULL) AND (human_review_requested_by IS NULL)) OR ((human_review_requested_at IS NOT NULL) AND (human_review_requested_by IS NOT NULL)))),
    CONSTRAINT ai_decisions_human_review_reason_chk CHECK (((human_review_reason IS NULL) OR (length(btrim(human_review_reason)) > 0))),
    CONSTRAINT ai_decisions_human_review_requested_by_chk CHECK (((human_review_requested_by IS NULL) OR (length(btrim(human_review_requested_by)) > 0))),
    CONSTRAINT ai_decisions_intent_chk CHECK ((length(btrim(intent_base)) > 0)),
    CONSTRAINT ai_decisions_lifecycle_chk CHECK ((lifecycle = ANY (ARRAY['proposed'::text, 'validated'::text, 'policy_evaluated'::text, 'expired'::text, 'rejected'::text]))),
    CONSTRAINT ai_decisions_missing_array_chk CHECK ((jsonb_typeof(missing_information) = 'array'::text)),
    CONSTRAINT ai_decisions_policy_decision_chk CHECK (((policy_decision IS NULL) OR (policy_decision = ANY (ARRAY['allowed'::text, 'requires_approval'::text, 'denied'::text])))),
    CONSTRAINT ai_decisions_reason_codes_array_chk CHECK ((jsonb_typeof(reason_codes) = 'array'::text)),
    CONSTRAINT ai_decisions_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT ai_decisions_schema_version_chk CHECK ((schema_version > 0))
);


--
-- Name: attribute_definitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.attribute_definitions (
    id uuid NOT NULL,
    schema_id uuid NOT NULL,
    attribute_key text NOT NULL,
    label text NOT NULL,
    data_type text NOT NULL,
    is_required boolean NOT NULL,
    is_searchable boolean NOT NULL,
    validation_rules jsonb DEFAULT '{}'::jsonb NOT NULL,
    display_order integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT attribute_definitions_key_chk CHECK ((length(btrim(attribute_key)) > 0)),
    CONSTRAINT attribute_definitions_label_chk CHECK ((length(btrim(label)) > 0)),
    CONSTRAINT attribute_definitions_order_chk CHECK ((display_order >= 0)),
    CONSTRAINT attribute_definitions_type_chk CHECK ((data_type = ANY (ARRAY['text'::text, 'number'::text, 'boolean'::text, 'date'::text, 'datetime'::text, 'select'::text, 'multi_select'::text, 'location'::text, 'money'::text]))),
    CONSTRAINT attribute_definitions_validation_object_chk CHECK ((jsonb_typeof(validation_rules) = 'object'::text))
);


--
-- Name: attribute_schemas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.attribute_schemas (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    name text NOT NULL,
    version integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT attribute_schemas_name_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT attribute_schemas_version_chk CHECK ((version > 0))
);


--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    actor_type text NOT NULL,
    actor_reference text,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    correlation_id uuid,
    causation_id uuid,
    occurred_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    decision_reference text,
    result text,
    reason_code text,
    before_reference text,
    after_reference text,
    schema_version integer DEFAULT 1 NOT NULL,
    redaction_version integer DEFAULT 1 NOT NULL,
    CONSTRAINT audit_events_action_chk CHECK ((length(btrim(action)) > 0)),
    CONSTRAINT audit_events_actor_type_chk CHECK ((actor_type = ANY (ARRAY['customer'::text, 'human_agent'::text, 'ai'::text, 'automation'::text, 'system'::text, 'provider'::text]))),
    CONSTRAINT audit_events_after_reference_chk CHECK (((after_reference IS NULL) OR (length(btrim(after_reference)) > 0))),
    CONSTRAINT audit_events_before_reference_chk CHECK (((before_reference IS NULL) OR (length(btrim(before_reference)) > 0))),
    CONSTRAINT audit_events_decision_reference_chk CHECK (((decision_reference IS NULL) OR (length(btrim(decision_reference)) > 0))),
    CONSTRAINT audit_events_metadata_object_chk CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT audit_events_reason_code_chk CHECK (((reason_code IS NULL) OR (length(btrim(reason_code)) > 0))),
    CONSTRAINT audit_events_redaction_version_chk CHECK ((redaction_version > 0)),
    CONSTRAINT audit_events_resource_type_chk CHECK ((length(btrim(resource_type)) > 0)),
    CONSTRAINT audit_events_result_chk CHECK (((result IS NULL) OR (result = ANY (ARRAY['accepted'::text, 'rejected'::text, 'failed'::text, 'completed'::text, 'skipped'::text])))),
    CONSTRAINT audit_events_schema_version_chk CHECK ((schema_version > 0))
);


--
-- Name: automation_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_executions (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    rule_id uuid NOT NULL,
    inbound_event_id uuid NOT NULL,
    result text NOT NULL,
    reason_code text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT automation_executions_reason_not_blank_chk CHECK ((length(btrim(reason_code)) > 0)),
    CONSTRAINT automation_executions_result_chk CHECK ((result = ANY (ARRAY['processing'::text, 'executed'::text, 'skipped'::text, 'failed'::text])))
);


--
-- Name: automation_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.automation_rules (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    name text NOT NULL,
    status text NOT NULL,
    trigger_kind text NOT NULL,
    conditions jsonb DEFAULT '{}'::jsonb NOT NULL,
    action_kind text NOT NULL,
    action_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" integer DEFAULT 100 NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT automation_rules_action_kind_chk CHECK ((action_kind = ANY (ARRAY['add_label'::text, 'set_priority'::text, 'assign_human'::text]))),
    CONSTRAINT automation_rules_action_payload_object_chk CHECK ((jsonb_typeof(action_payload) = 'object'::text)),
    CONSTRAINT automation_rules_conditions_object_chk CHECK ((jsonb_typeof(conditions) = 'object'::text)),
    CONSTRAINT automation_rules_name_not_blank_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT automation_rules_position_positive_chk CHECK (("position" > 0)),
    CONSTRAINT automation_rules_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT automation_rules_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT automation_rules_trigger_kind_chk CHECK ((trigger_kind = 'inbound_message'::text))
);


--
-- Name: business_memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_memberships (
    business_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    role text NOT NULL,
    permissions jsonb DEFAULT '[]'::jsonb NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT business_memberships_permissions_array_chk CHECK ((jsonb_typeof(permissions) = 'array'::text)),
    CONSTRAINT business_memberships_role_chk CHECK ((role = ANY (ARRAY['owner'::text, 'admin'::text, 'manager'::text, 'agent'::text, 'analyst'::text, 'viewer'::text]))),
    CONSTRAINT business_memberships_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'revoked'::text])))
);


--
-- Name: business_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_policies (
    business_id uuid NOT NULL,
    ai_mode text NOT NULL,
    default_human_review boolean NOT NULL,
    allow_auto_reply boolean NOT NULL,
    allow_auto_lead_creation boolean NOT NULL,
    allow_auto_transaction_draft boolean NOT NULL,
    allow_auto_confirmation boolean NOT NULL,
    business_hours_reference text,
    escalation_policy_reference text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT business_policies_ai_mode_chk CHECK ((ai_mode = ANY (ARRAY['disabled'::text, 'assist'::text, 'approval'::text, 'restricted_auto'::text]))),
    CONSTRAINT business_policies_resource_version_positive_chk CHECK ((resource_version > 0))
);


--
-- Name: business_policy_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_policy_versions (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    policy_key text NOT NULL,
    category text NOT NULL,
    title text NOT NULL,
    summary text NOT NULL,
    rules jsonb NOT NULL,
    authority text NOT NULL,
    status text NOT NULL,
    version integer NOT NULL,
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT business_policy_versions_authority_chk CHECK ((authority = 'merchant'::text)),
    CONSTRAINT business_policy_versions_category_chk CHECK ((category = ANY (ARRAY['hours'::text, 'returns'::text, 'delivery'::text, 'pricing'::text, 'availability'::text, 'ai_mode'::text, 'channel'::text, 'general'::text]))),
    CONSTRAINT business_policy_versions_key_chk CHECK ((length(btrim(policy_key)) > 0)),
    CONSTRAINT business_policy_versions_rules_object_chk CHECK ((jsonb_typeof(rules) = 'object'::text)),
    CONSTRAINT business_policy_versions_status_chk CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'archived'::text]))),
    CONSTRAINT business_policy_versions_summary_chk CHECK ((length(btrim(summary)) > 0)),
    CONSTRAINT business_policy_versions_title_chk CHECK ((length(btrim(title)) > 0)),
    CONSTRAINT business_policy_versions_validity_chk CHECK (((valid_until IS NULL) OR (valid_until > valid_from))),
    CONSTRAINT business_policy_versions_version_chk CHECK ((version > 0))
);


--
-- Name: businesses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.businesses (
    id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    status text NOT NULL,
    vertical_type text NOT NULL,
    timezone text NOT NULL,
    default_currency character(3) NOT NULL,
    locale text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT businesses_currency_chk CHECK ((default_currency ~ '^[A-Z]{3}$'::text)),
    CONSTRAINT businesses_locale_not_blank_chk CHECK ((length(btrim(locale)) > 0)),
    CONSTRAINT businesses_name_not_blank_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT businesses_resource_version_positive_chk CHECK ((resource_version > 0)),
    CONSTRAINT businesses_slug_not_blank_chk CHECK ((length(btrim(slug)) > 0)),
    CONSTRAINT businesses_status_chk CHECK ((status = ANY (ARRAY['pending_setup'::text, 'active'::text, 'suspended'::text, 'archived'::text]))),
    CONSTRAINT businesses_vertical_type_chk CHECK ((vertical_type = ANY (ARRAY['retail'::text, 'travel'::text, 'services'::text, 'restaurant'::text, 'clinic'::text, 'hospitality'::text, 'education'::text, 'real_estate'::text, 'other'::text])))
);


--
-- Name: canned_replies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.canned_replies (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    title text NOT NULL,
    shortcut text NOT NULL,
    body text NOT NULL,
    status text NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT canned_replies_body_not_blank_chk CHECK ((length(btrim(body)) > 0)),
    CONSTRAINT canned_replies_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT canned_replies_shortcut_normalized_chk CHECK ((shortcut = lower(btrim(shortcut)))),
    CONSTRAINT canned_replies_shortcut_not_blank_chk CHECK ((length(btrim(shortcut)) > 0)),
    CONSTRAINT canned_replies_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'archived'::text]))),
    CONSTRAINT canned_replies_title_not_blank_chk CHECK ((length(btrim(title)) > 0))
);


--
-- Name: catalog_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.catalog_items (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    catalog_id uuid NOT NULL,
    attribute_schema_id uuid,
    attribute_schema_version integer,
    item_type text NOT NULL,
    name text NOT NULL,
    short_description text,
    long_description text,
    status text NOT NULL,
    pricing_mode text NOT NULL,
    availability_mode text NOT NULL,
    fulfillment_mode text NOT NULL,
    requires_confirmation boolean NOT NULL,
    attributes jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT catalog_items_attributes_object_chk CHECK ((jsonb_typeof(attributes) = 'object'::text)),
    CONSTRAINT catalog_items_availability_mode_chk CHECK ((availability_mode = ANY (ARRAY['stock'::text, 'schedule'::text, 'supplier_check'::text, 'always_available'::text, 'unknown'::text]))),
    CONSTRAINT catalog_items_fulfillment_mode_chk CHECK ((fulfillment_mode = ANY (ARRAY['delivery'::text, 'pickup'::text, 'digital'::text, 'appointment'::text, 'travel'::text, 'manual'::text]))),
    CONSTRAINT catalog_items_name_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT catalog_items_pricing_mode_chk CHECK ((pricing_mode = ANY (ARRAY['fixed'::text, 'starting_from'::text, 'per_unit'::text, 'per_person'::text, 'per_day'::text, 'quote_required'::text, 'dynamic'::text]))),
    CONSTRAINT catalog_items_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT catalog_items_schema_version_pair_chk CHECK ((((attribute_schema_id IS NULL) AND (attribute_schema_version IS NULL)) OR ((attribute_schema_id IS NOT NULL) AND (attribute_schema_version IS NOT NULL) AND (attribute_schema_version > 0)))),
    CONSTRAINT catalog_items_status_chk CHECK ((status = ANY (ARRAY['draft'::text, 'active'::text, 'inactive'::text, 'archived'::text]))),
    CONSTRAINT catalog_items_type_chk CHECK ((length(btrim(item_type)) > 0))
);


--
-- Name: catalogs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.catalogs (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    name text NOT NULL,
    description text,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT catalogs_name_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT catalogs_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT catalogs_status_chk CHECK ((status = ANY (ARRAY['draft'::text, 'active'::text, 'archived'::text])))
);


--
-- Name: channel_connection_capabilities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.channel_connection_capabilities (
    connection_id uuid NOT NULL,
    capability text NOT NULL,
    enabled boolean NOT NULL,
    checked_at timestamp with time zone NOT NULL,
    evidence_source text,
    CONSTRAINT channel_connection_capabilities_capability_chk CHECK ((capability = ANY (ARRAY['receive_messages'::text, 'send_messages'::text, 'receive_comments'::text, 'reply_comments'::text, 'private_reply'::text, 'media_inbound'::text, 'media_outbound'::text, 'interactive_messages'::text, 'templates'::text, 'delivery_status'::text, 'read_status'::text]))),
    CONSTRAINT channel_connection_capabilities_evidence_chk CHECK (((evidence_source IS NULL) OR (length(btrim(evidence_source)) > 0)))
);


--
-- Name: channel_connection_state_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.channel_connection_state_events (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    action text NOT NULL,
    from_status text NOT NULL,
    to_status text NOT NULL,
    reason text NOT NULL,
    actor_reference text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT channel_connection_state_events_action_chk CHECK ((action = ANY (ARRAY['reconnect_requested'::text, 'disconnect_requested'::text]))),
    CONSTRAINT channel_connection_state_events_actor_chk CHECK ((length(btrim(actor_reference)) > 0)),
    CONSTRAINT channel_connection_state_events_reason_chk CHECK ((length(btrim(reason)) > 0))
);


--
-- Name: channel_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.channel_connections (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    provider_ref text NOT NULL,
    channel text NOT NULL,
    provider_account_ref text,
    provider_connection_ref text NOT NULL,
    status text NOT NULL,
    secret_reference text NOT NULL,
    last_health_check_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT channel_connections_channel_chk CHECK ((channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text]))),
    CONSTRAINT channel_connections_connection_ref_chk CHECK ((length(btrim(provider_connection_ref)) > 0)),
    CONSTRAINT channel_connections_provider_ref_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT channel_connections_resource_version_positive_chk CHECK ((resource_version > 0)),
    CONSTRAINT channel_connections_secret_ref_chk CHECK ((length(btrim(secret_reference)) > 0)),
    CONSTRAINT channel_connections_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'active'::text, 'disconnected'::text, 'failed'::text, 'reconnect_required'::text, 'archived'::text])))
);


--
-- Name: channel_provisioning_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.channel_provisioning_sessions (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    provider_ref text NOT NULL,
    channel text NOT NULL,
    display_name text NOT NULL,
    status text NOT NULL,
    oauth_state text,
    authorization_url text,
    provider_account_ref text,
    provider_connection_ref text,
    chatwoot_account_id text,
    chatwoot_inbox_id text,
    channel_connection_id uuid,
    failure_code text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT channel_provisioning_authorization_url_chk CHECK (((authorization_url IS NULL) OR (authorization_url ~~ 'https://%'::text))),
    CONSTRAINT channel_provisioning_channel_chk CHECK ((channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text]))),
    CONSTRAINT channel_provisioning_display_name_chk CHECK ((length(btrim(display_name)) > 0)),
    CONSTRAINT channel_provisioning_failure_chk CHECK (((failure_code IS NULL) OR (length(btrim(failure_code)) > 0))),
    CONSTRAINT channel_provisioning_provider_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT channel_provisioning_state_chk CHECK (((oauth_state IS NULL) OR (length(btrim(oauth_state)) > 0))),
    CONSTRAINT channel_provisioning_status_chk CHECK ((status = ANY (ARRAY['pending_authorization'::text, 'provisioning'::text, 'connected'::text, 'failed'::text, 'reconnect_required'::text])))
);


--
-- Name: chatwoot_contact_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chatwoot_contact_links (
    id uuid NOT NULL,
    binding_id uuid NOT NULL,
    business_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    external_user_id text NOT NULL,
    chatwoot_contact_id text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT chatwoot_contact_links_contact_chk CHECK (((chatwoot_contact_id IS NULL) OR (length(btrim(chatwoot_contact_id)) > 0))),
    CONSTRAINT chatwoot_contact_links_user_chk CHECK ((length(btrim(external_user_id)) > 0))
);


--
-- Name: chatwoot_mirror_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chatwoot_mirror_jobs (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    communication_message_id uuid NOT NULL,
    status text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    lease_owner text,
    lease_token text,
    lease_expires_at timestamp with time zone,
    failure_code text,
    result_code text,
    chatwoot_contact_id text,
    chatwoot_conversation_id text,
    chatwoot_message_id text,
    completed_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT chatwoot_mirror_jobs_attempt_count_chk CHECK ((attempt_count >= 0)),
    CONSTRAINT chatwoot_mirror_jobs_failure_chk CHECK (((failure_code IS NULL) OR (length(btrim(failure_code)) > 0))),
    CONSTRAINT chatwoot_mirror_jobs_lease_chk CHECK ((((status = 'processing'::text) AND (lease_owner IS NOT NULL) AND (lease_token IS NOT NULL) AND (lease_expires_at IS NOT NULL)) OR ((status <> 'processing'::text) AND (lease_owner IS NULL) AND (lease_token IS NULL) AND (lease_expires_at IS NULL)))),
    CONSTRAINT chatwoot_mirror_jobs_result_chk CHECK (((result_code IS NULL) OR (length(btrim(result_code)) > 0))),
    CONSTRAINT chatwoot_mirror_jobs_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'completed'::text, 'dead_letter'::text])))
);


--
-- Name: chatwoot_workspace_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chatwoot_workspace_bindings (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    route_key text NOT NULL,
    account_id text NOT NULL,
    inbox_id text NOT NULL,
    channel text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT chatwoot_workspace_bindings_account_chk CHECK ((length(btrim(account_id)) > 0)),
    CONSTRAINT chatwoot_workspace_bindings_channel_chk CHECK ((channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text, 'other'::text]))),
    CONSTRAINT chatwoot_workspace_bindings_inbox_chk CHECK ((length(btrim(inbox_id)) > 0)),
    CONSTRAINT chatwoot_workspace_bindings_route_chk CHECK ((length(btrim(route_key)) > 0))
);


--
-- Name: commercial_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.commercial_transactions (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    lead_id uuid,
    transaction_type text NOT NULL,
    state text NOT NULL,
    source_conversation_reference_id uuid,
    currency character(3),
    total_amount numeric(20,4),
    schema_version integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    requires_human_review boolean DEFAULT false NOT NULL,
    cancellation_reason text,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT commercial_transactions_amount_chk CHECK (((total_amount IS NULL) OR (total_amount >= (0)::numeric))),
    CONSTRAINT commercial_transactions_amount_currency_chk CHECK (((total_amount IS NULL) OR (currency IS NOT NULL))),
    CONSTRAINT commercial_transactions_cancellation_reason_chk CHECK (((cancellation_reason IS NULL) OR (length(btrim(cancellation_reason)) > 0))),
    CONSTRAINT commercial_transactions_currency_chk CHECK (((currency IS NULL) OR (currency ~ '^[A-Z]{3}$'::text))),
    CONSTRAINT commercial_transactions_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT commercial_transactions_schema_version_chk CHECK ((schema_version > 0)),
    CONSTRAINT commercial_transactions_state_chk CHECK ((state = ANY (ARRAY['draft'::text, 'needs_information'::text, 'awaiting_confirmation'::text, 'confirmed'::text, 'in_fulfillment'::text, 'completed'::text, 'cancelled'::text, 'expired'::text, 'rejected'::text]))),
    CONSTRAINT commercial_transactions_type_chk CHECK ((transaction_type = ANY (ARRAY['order'::text, 'booking'::text, 'appointment'::text, 'service_request'::text, 'reservation'::text, 'quote'::text, 'subscription'::text])))
);


--
-- Name: communication_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.communication_messages (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    conversation_reference_id uuid NOT NULL,
    inbound_event_id uuid,
    outbound_message_id uuid,
    direction text NOT NULL,
    origin text NOT NULL,
    transport text NOT NULL,
    provider_message_id text,
    chatwoot_message_id text,
    content_type text NOT NULL,
    text_content text,
    content_reference text NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    visibility text DEFAULT 'public'::text NOT NULL,
    CONSTRAINT communication_messages_chatwoot_reference_chk CHECK (((chatwoot_message_id IS NULL) OR (length(btrim(chatwoot_message_id)) > 0))),
    CONSTRAINT communication_messages_content_reference_chk CHECK ((length(btrim(content_reference)) > 0)),
    CONSTRAINT communication_messages_content_type_chk CHECK ((content_type = ANY (ARRAY['text'::text, 'image'::text, 'video'::text, 'audio'::text, 'file'::text, 'mixed'::text, 'unknown'::text]))),
    CONSTRAINT communication_messages_direction_chk CHECK ((direction = ANY (ARRAY['inbound'::text, 'outbound'::text]))),
    CONSTRAINT communication_messages_external_reference_chk CHECK (((provider_message_id IS NULL) OR (length(btrim(provider_message_id)) > 0))),
    CONSTRAINT communication_messages_origin_chk CHECK ((origin = ANY (ARRAY['customer'::text, 'ai'::text, 'human'::text, 'automation'::text, 'system'::text]))),
    CONSTRAINT communication_messages_text_content_chk CHECK (((content_type <> 'text'::text) OR (text_content IS NOT NULL))),
    CONSTRAINT communication_messages_transport_chk CHECK ((transport = ANY (ARRAY['provider'::text, 'chatwoot'::text, 'mujeeb'::text]))),
    CONSTRAINT communication_messages_visibility_chk CHECK ((visibility = ANY (ARRAY['public'::text, 'private'::text])))
);


--
-- Name: conversation_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_labels (
    business_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    label text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT conversation_labels_normalized_chk CHECK ((label = lower(btrim(label)))),
    CONSTRAINT conversation_labels_not_blank_chk CHECK ((length(btrim(label)) > 0))
);


--
-- Name: conversation_read_cursors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_read_cursors (
    business_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    last_read_message_id uuid,
    read_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);


--
-- Name: conversation_references; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_references (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    system text NOT NULL,
    provider_ref text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL,
    connection_id uuid,
    conversation_kind text,
    is_current boolean NOT NULL,
    mapping_status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    chatwoot_account_id text,
    chatwoot_inbox_id text,
    chatwoot_conversation_id text,
    CONSTRAINT conversation_references_chatwoot_binding_chk CHECK (((system <> 'provider'::text) OR (((chatwoot_account_id IS NULL) AND (chatwoot_inbox_id IS NULL) AND (chatwoot_conversation_id IS NULL)) OR ((length(btrim(chatwoot_account_id)) > 0) AND (length(btrim(chatwoot_inbox_id)) > 0) AND (length(btrim(chatwoot_conversation_id)) > 0))))),
    CONSTRAINT conversation_references_kind_chk CHECK (((conversation_kind IS NULL) OR (conversation_kind = ANY (ARRAY['dm'::text, 'comment'::text, 'story_reply'::text, 'mention'::text, 'review'::text, 'other'::text])))),
    CONSTRAINT conversation_references_mapping_status_chk CHECK ((mapping_status = ANY (ARRAY['pending'::text, 'active'::text, 'stale'::text, 'failed'::text]))),
    CONSTRAINT conversation_references_provider_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT conversation_references_provider_connection_chk CHECK ((((system = 'provider'::text) AND (connection_id IS NOT NULL)) OR (system = 'chatwoot'::text))),
    CONSTRAINT conversation_references_resource_id_chk CHECK ((length(btrim(resource_id)) > 0)),
    CONSTRAINT conversation_references_resource_type_chk CHECK ((length(btrim(resource_type)) > 0)),
    CONSTRAINT conversation_references_system_chk CHECK ((system = ANY (ARRAY['provider'::text, 'chatwoot'::text])))
);


--
-- Name: conversation_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_state (
    business_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    focus jsonb,
    previous jsonb DEFAULT '[]'::jsonb NOT NULL,
    comparison jsonb,
    preferences jsonb DEFAULT '[]'::jsonb NOT NULL,
    constraints jsonb DEFAULT '[]'::jsonb NOT NULL,
    pending jsonb DEFAULT '[]'::jsonb NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT conversation_state_comparison_object_chk CHECK (((comparison IS NULL) OR (jsonb_typeof(comparison) = 'object'::text))),
    CONSTRAINT conversation_state_constraints_array_chk CHECK ((jsonb_typeof(constraints) = 'array'::text)),
    CONSTRAINT conversation_state_focus_object_chk CHECK (((focus IS NULL) OR (jsonb_typeof(focus) = 'object'::text))),
    CONSTRAINT conversation_state_pending_array_chk CHECK ((jsonb_typeof(pending) = 'array'::text)),
    CONSTRAINT conversation_state_preferences_array_chk CHECK ((jsonb_typeof(preferences) = 'array'::text)),
    CONSTRAINT conversation_state_previous_array_chk CHECK ((jsonb_typeof(previous) = 'array'::text)),
    CONSTRAINT conversation_state_version_chk CHECK ((version > 0))
);


--
-- Name: conversations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversations (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    state text NOT NULL,
    ownership text NOT NULL,
    ai_mode_override text,
    priority text NOT NULL,
    assignment_reference text,
    last_activity_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT conversations_ai_mode_chk CHECK (((ai_mode_override IS NULL) OR (ai_mode_override = ANY (ARRAY['allowed'::text, 'draft_only'::text, 'disabled'::text])))),
    CONSTRAINT conversations_assignment_ref_chk CHECK (((assignment_reference IS NULL) OR (length(btrim(assignment_reference)) > 0))),
    CONSTRAINT conversations_ownership_chk CHECK ((ownership = ANY (ARRAY['none'::text, 'ai'::text, 'human'::text]))),
    CONSTRAINT conversations_priority_chk CHECK ((priority = ANY (ARRAY['low'::text, 'normal'::text, 'high'::text, 'urgent'::text]))),
    CONSTRAINT conversations_resource_version_positive_chk CHECK ((resource_version > 0)),
    CONSTRAINT conversations_state_chk CHECK ((state = ANY (ARRAY['open'::text, 'ai_handling'::text, 'waiting_customer'::text, 'waiting_human'::text, 'human_handling'::text, 'closed'::text])))
);


--
-- Name: customers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customers (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    profile jsonb DEFAULT '{}'::jsonb NOT NULL,
    contact_points jsonb DEFAULT '[]'::jsonb NOT NULL,
    locale_preference text,
    status text NOT NULL,
    merged_into_customer_id uuid,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT customers_contact_points_array_chk CHECK ((jsonb_typeof(contact_points) = 'array'::text)),
    CONSTRAINT customers_merged_target_chk CHECK (((status <> 'merged'::text) OR (merged_into_customer_id IS NOT NULL))),
    CONSTRAINT customers_profile_object_chk CHECK ((jsonb_typeof(profile) = 'object'::text)),
    CONSTRAINT customers_resource_version_positive_chk CHECK ((resource_version > 0)),
    CONSTRAINT customers_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'merged'::text, 'archived'::text])))
);


--
-- Name: external_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.external_identities (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    customer_id uuid,
    provider_ref text NOT NULL,
    channel text NOT NULL,
    external_account_ref text,
    external_user_id text NOT NULL,
    profile_snapshot_reference text,
    link_status text NOT NULL,
    first_seen_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT external_identities_channel_chk CHECK ((channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text]))),
    CONSTRAINT external_identities_link_consistency_chk CHECK ((((link_status = 'linked'::text) AND (customer_id IS NOT NULL)) OR (link_status = ANY (ARRAY['unresolved'::text, 'revoked'::text])))),
    CONSTRAINT external_identities_link_status_chk CHECK ((link_status = ANY (ARRAY['unresolved'::text, 'linked'::text, 'revoked'::text]))),
    CONSTRAINT external_identities_provider_ref_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT external_identities_user_ref_chk CHECK ((length(btrim(external_user_id)) > 0))
);


--
-- Name: identity_matches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_matches (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    left_identity_id uuid NOT NULL,
    right_identity_id uuid NOT NULL,
    method text NOT NULL,
    evidence_references jsonb DEFAULT '[]'::jsonb NOT NULL,
    confidence_band text NOT NULL,
    decision text NOT NULL,
    decided_by text NOT NULL,
    decided_at timestamp with time zone,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_matches_confidence_chk CHECK ((confidence_band = ANY (ARRAY['unknown'::text, 'low'::text, 'medium'::text, 'high'::text]))),
    CONSTRAINT identity_matches_decided_by_chk CHECK ((decided_by = ANY (ARRAY['system'::text, 'ai'::text, 'human'::text, 'automation'::text]))),
    CONSTRAINT identity_matches_decision_chk CHECK ((decision = ANY (ARRAY['match'::text, 'no_match'::text, 'needs_review'::text, 'revoked'::text]))),
    CONSTRAINT identity_matches_evidence_array_chk CHECK ((jsonb_typeof(evidence_references) = 'array'::text)),
    CONSTRAINT identity_matches_lifecycle_chk CHECK ((((status = 'pending'::text) AND (decision = 'needs_review'::text) AND (decided_at IS NULL)) OR ((status = 'decided'::text) AND (decision = ANY (ARRAY['match'::text, 'no_match'::text])) AND (decided_at IS NOT NULL)) OR ((status = 'revoked'::text) AND (decision = 'revoked'::text) AND (decided_at IS NOT NULL)))),
    CONSTRAINT identity_matches_method_chk CHECK ((method = ANY (ARRAY['explicit_link'::text, 'verified_phone'::text, 'verified_email'::text, 'provider_link'::text, 'manual_confirmation'::text, 'candidate_similarity'::text]))),
    CONSTRAINT identity_matches_order_chk CHECK ((left_identity_id < right_identity_id)),
    CONSTRAINT identity_matches_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'decided'::text, 'revoked'::text])))
);


--
-- Name: inbound_event_ledger; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inbound_event_ledger (
    id uuid NOT NULL,
    provider_ref text NOT NULL,
    provider_connection_ref text NOT NULL,
    provider_event_id text NOT NULL,
    dedupe_strategy text NOT NULL,
    business_id uuid,
    connection_id uuid,
    event_type text NOT NULL,
    interaction_kind text,
    provider_message_id text,
    provider_conversation_id text,
    external_user_id text,
    content_reference text,
    external_created_at timestamp with time zone,
    received_at timestamp with time zone NOT NULL,
    raw_payload_reference text NOT NULL,
    payload_hash text NOT NULL,
    signature_verified boolean NOT NULL,
    processing_state text NOT NULL,
    processing_owner text,
    lease_expires_at timestamp with time zone,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error_code text,
    next_attempt_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    processing_lease_token uuid,
    processing_result_code text,
    processed_at timestamp with time zone,
    CONSTRAINT inbound_event_attempt_count_chk CHECK ((attempt_count >= 0)),
    CONSTRAINT inbound_event_connection_ref_chk CHECK ((length(btrim(provider_connection_ref)) > 0)),
    CONSTRAINT inbound_event_dedupe_strategy_chk CHECK ((dedupe_strategy = ANY (ARRAY['provider_event_id'::text, 'provider_specific_fallback'::text, 'dedupe_uncertain'::text]))),
    CONSTRAINT inbound_event_dedupe_strategy_consistency_chk CHECK (((dedupe_strategy <> 'dedupe_uncertain'::text) OR (processing_state = ANY (ARRAY['unresolved'::text, 'retryable_failed'::text, 'dead_letter'::text, 'rejected'::text])))),
    CONSTRAINT inbound_event_id_chk CHECK ((length(btrim(provider_event_id)) > 0)),
    CONSTRAINT inbound_event_interaction_kind_chk CHECK (((interaction_kind IS NULL) OR (interaction_kind = ANY (ARRAY['dm'::text, 'comment'::text, 'story_reply'::text, 'mention'::text, 'review'::text, 'postback'::text, 'other'::text])))),
    CONSTRAINT inbound_event_lease_chk CHECK ((((processing_state = 'processing'::text) AND (processing_owner IS NOT NULL) AND (lease_expires_at IS NOT NULL)) OR ((processing_state <> 'processing'::text) AND (processing_owner IS NULL) AND (lease_expires_at IS NULL)))),
    CONSTRAINT inbound_event_lease_token_chk CHECK ((((processing_state = 'processing'::text) AND (processing_lease_token IS NOT NULL)) OR ((processing_state <> 'processing'::text) AND (processing_lease_token IS NULL)))),
    CONSTRAINT inbound_event_processed_at_chk CHECK ((((processing_state = 'processed'::text) AND (processed_at IS NOT NULL)) OR ((processing_state <> 'processed'::text) AND (processed_at IS NULL)))),
    CONSTRAINT inbound_event_processing_state_chk CHECK ((processing_state = ANY (ARRAY['received'::text, 'unresolved'::text, 'processing'::text, 'processed'::text, 'retryable_failed'::text, 'dead_letter'::text, 'rejected'::text]))),
    CONSTRAINT inbound_event_provider_ref_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT inbound_event_resolution_pair_chk CHECK ((((business_id IS NULL) AND (connection_id IS NULL)) OR ((business_id IS NOT NULL) AND ((connection_id IS NOT NULL) OR (provider_ref = 'chatwoot'::text))))),
    CONSTRAINT inbound_event_result_code_chk CHECK (((processing_result_code IS NULL) OR (length(btrim(processing_result_code)) > 0))),
    CONSTRAINT inbound_event_signature_chk CHECK ((signature_verified OR (processing_state = ANY (ARRAY['unresolved'::text, 'rejected'::text])))),
    CONSTRAINT inbound_event_type_chk CHECK ((event_type = ANY (ARRAY['interaction_received'::text, 'interaction_updated'::text, 'delivery_status_changed'::text, 'conversation_updated'::text, 'account_status_changed'::text])))
);


--
-- Name: inbound_webhook_payloads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.inbound_webhook_payloads (
    id uuid NOT NULL,
    provider_ref text NOT NULL,
    delivery_id text NOT NULL,
    payload bytea NOT NULL,
    payload_hash text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT inbound_webhook_payloads_delivery_chk CHECK ((length(btrim(delivery_id)) > 0)),
    CONSTRAINT inbound_webhook_payloads_hash_chk CHECK ((length(btrim(payload_hash)) = 64)),
    CONSTRAINT inbound_webhook_payloads_payload_chk CHECK ((octet_length(payload) > 0)),
    CONSTRAINT inbound_webhook_payloads_provider_chk CHECK ((length(btrim(provider_ref)) > 0))
);


--
-- Name: knowledge_documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.knowledge_documents (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    knowledge_key text NOT NULL,
    title text NOT NULL,
    content text NOT NULL,
    content_type text NOT NULL,
    source_reference text NOT NULL,
    authority text NOT NULL,
    status text NOT NULL,
    version integer NOT NULL,
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT knowledge_documents_authority_chk CHECK ((authority = ANY (ARRAY['merchant'::text, 'system'::text]))),
    CONSTRAINT knowledge_documents_content_chk CHECK ((length(btrim(content)) > 0)),
    CONSTRAINT knowledge_documents_content_type_chk CHECK ((content_type = ANY (ARRAY['faq'::text, 'hours'::text, 'location'::text, 'service'::text, 'general'::text]))),
    CONSTRAINT knowledge_documents_key_chk CHECK ((length(btrim(knowledge_key)) > 0)),
    CONSTRAINT knowledge_documents_source_chk CHECK ((length(btrim(source_reference)) > 0)),
    CONSTRAINT knowledge_documents_status_chk CHECK ((status = ANY (ARRAY['draft'::text, 'published'::text, 'archived'::text]))),
    CONSTRAINT knowledge_documents_title_chk CHECK ((length(btrim(title)) > 0)),
    CONSTRAINT knowledge_documents_validity_chk CHECK (((valid_until IS NULL) OR (valid_until > valid_from))),
    CONSTRAINT knowledge_documents_version_chk CHECK ((version > 0))
);


--
-- Name: lead_attributions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lead_attributions (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    source_conversation_id uuid,
    source_channel text,
    source_interaction_reference text,
    catalog_item_id uuid,
    offer_id uuid,
    campaign_reference text,
    captured_at timestamp with time zone NOT NULL,
    CONSTRAINT lead_attributions_source_channel_chk CHECK (((source_channel IS NULL) OR (source_channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text]))))
);


--
-- Name: lead_scores; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.lead_scores (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    lead_id uuid NOT NULL,
    value numeric(5,2) NOT NULL,
    band text NOT NULL,
    factors jsonb NOT NULL,
    rule_version text,
    model_reference text,
    calculated_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT lead_scores_band_chk CHECK ((band = ANY (ARRAY['unknown'::text, 'low'::text, 'medium'::text, 'high'::text]))),
    CONSTRAINT lead_scores_factors_object_chk CHECK ((jsonb_typeof(factors) = 'object'::text)),
    CONSTRAINT lead_scores_value_chk CHECK (((value >= (0)::numeric) AND (value <= (100)::numeric)))
);


--
-- Name: leads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.leads (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    customer_id uuid NOT NULL,
    status text NOT NULL,
    current_score_value numeric(5,2),
    current_score_band text,
    score_rule_version text,
    score_model_reference text,
    qualification_context jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by text NOT NULL,
    qualified_by text,
    lost_reason text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    source_conversation_reference_id uuid,
    source_channel text,
    intent_reference text,
    assigned_ownership_reference text,
    next_action_at timestamp with time zone,
    qualification_reason text,
    qualification_evidence jsonb DEFAULT '[]'::jsonb NOT NULL,
    qualification_state text DEFAULT 'new'::text NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT leads_assignment_reference_chk CHECK (((assigned_ownership_reference IS NULL) OR (length(btrim(assigned_ownership_reference)) > 0))),
    CONSTRAINT leads_context_object_chk CHECK ((jsonb_typeof(qualification_context) = 'object'::text)),
    CONSTRAINT leads_created_by_chk CHECK ((created_by = ANY (ARRAY['ai'::text, 'human'::text, 'automation'::text, 'system'::text]))),
    CONSTRAINT leads_intent_reference_chk CHECK (((intent_reference IS NULL) OR (length(btrim(intent_reference)) > 0))),
    CONSTRAINT leads_lost_reason_chk CHECK (((status <> 'lost'::text) OR ((lost_reason IS NOT NULL) AND (length(btrim(lost_reason)) > 0)))),
    CONSTRAINT leads_qualification_evidence_array_chk CHECK ((jsonb_typeof(qualification_evidence) = 'array'::text)),
    CONSTRAINT leads_qualification_reason_chk CHECK (((qualification_reason IS NULL) OR (length(btrim(qualification_reason)) > 0))),
    CONSTRAINT leads_qualification_state_chk CHECK ((qualification_state = ANY (ARRAY['new'::text, 'qualified'::text, 'working'::text, 'converted'::text, 'lost'::text, 'disqualified'::text]))),
    CONSTRAINT leads_qualified_by_chk CHECK (((qualified_by IS NULL) OR (qualified_by = ANY (ARRAY['ai'::text, 'human'::text, 'automation'::text, 'system'::text, 'none'::text])))),
    CONSTRAINT leads_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT leads_score_band_chk CHECK (((current_score_band IS NULL) OR (current_score_band = ANY (ARRAY['unknown'::text, 'low'::text, 'medium'::text, 'high'::text])))),
    CONSTRAINT leads_score_value_chk CHECK (((current_score_value IS NULL) OR ((current_score_value >= (0)::numeric) AND (current_score_value <= (100)::numeric)))),
    CONSTRAINT leads_source_channel_chk CHECK (((source_channel IS NULL) OR (source_channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text])))),
    CONSTRAINT leads_status_chk CHECK ((status = ANY (ARRAY['new'::text, 'interested'::text, 'qualified'::text, 'won'::text, 'lost'::text])))
);


--
-- Name: offers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offers (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    catalog_item_id uuid NOT NULL,
    variant_id uuid,
    name text NOT NULL,
    pricing_mode text NOT NULL,
    amount numeric(20,4),
    currency character(3),
    pricing_unit text,
    price_source text,
    price_verification_status text NOT NULL,
    price_checked_at timestamp with time zone,
    availability_mode text NOT NULL,
    availability_status text NOT NULL,
    availability_source text,
    availability_checked_at timestamp with time zone,
    availability_valid_until timestamp with time zone,
    availability_evidence_ref text,
    fulfillment_mode text NOT NULL,
    validity_from timestamp with time zone,
    validity_until timestamp with time zone,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT offers_amount_chk CHECK (((amount IS NULL) OR (amount >= (0)::numeric))),
    CONSTRAINT offers_availability_mode_chk CHECK ((availability_mode = ANY (ARRAY['stock'::text, 'schedule'::text, 'supplier_check'::text, 'always_available'::text, 'unknown'::text]))),
    CONSTRAINT offers_availability_status_chk CHECK ((availability_status = ANY (ARRAY['available'::text, 'unavailable'::text, 'unknown'::text, 'requires_check'::text, 'stale'::text]))),
    CONSTRAINT offers_availability_validity_chk CHECK (((availability_valid_until IS NULL) OR (availability_checked_at IS NULL) OR (availability_valid_until >= availability_checked_at))),
    CONSTRAINT offers_currency_chk CHECK (((currency IS NULL) OR (currency ~ '^[A-Z]{3}$'::text))),
    CONSTRAINT offers_fulfillment_mode_chk CHECK ((fulfillment_mode = ANY (ARRAY['delivery'::text, 'pickup'::text, 'digital'::text, 'appointment'::text, 'travel'::text, 'manual'::text]))),
    CONSTRAINT offers_name_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT offers_price_verification_chk CHECK ((price_verification_status = ANY (ARRAY['unverified'::text, 'verified'::text, 'stale'::text, 'rejected'::text]))),
    CONSTRAINT offers_pricing_consistency_chk CHECK ((((pricing_mode = ANY (ARRAY['fixed'::text, 'starting_from'::text])) AND (amount IS NOT NULL) AND (currency IS NOT NULL)) OR ((pricing_mode = ANY (ARRAY['per_unit'::text, 'per_person'::text, 'per_day'::text])) AND (amount IS NOT NULL) AND (currency IS NOT NULL) AND (pricing_unit IS NOT NULL)) OR (pricing_mode = 'quote_required'::text) OR ((pricing_mode = 'dynamic'::text) AND ((amount IS NULL) OR (currency IS NOT NULL))))),
    CONSTRAINT offers_pricing_mode_chk CHECK ((pricing_mode = ANY (ARRAY['fixed'::text, 'starting_from'::text, 'per_unit'::text, 'per_person'::text, 'per_day'::text, 'quote_required'::text, 'dynamic'::text]))),
    CONSTRAINT offers_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT offers_status_chk CHECK ((status = ANY (ARRAY['draft'::text, 'active'::text, 'inactive'::text, 'expired'::text, 'archived'::text]))),
    CONSTRAINT offers_validity_chk CHECK (((validity_until IS NULL) OR (validity_from IS NULL) OR (validity_until >= validity_from)))
);


--
-- Name: order_lines; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_lines (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    catalog_item_id uuid NOT NULL,
    offer_id uuid,
    variant_id uuid,
    item_name_snapshot text NOT NULL,
    selected_attributes_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    quantity numeric(20,4) NOT NULL,
    unit_price_snapshot numeric(20,4),
    line_total_snapshot numeric(20,4),
    currency character(3),
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    pricing_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    availability_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    fulfillment_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    CONSTRAINT order_lines_attributes_object_chk CHECK ((jsonb_typeof(selected_attributes_snapshot) = 'object'::text)),
    CONSTRAINT order_lines_availability_snapshot_object_chk CHECK ((jsonb_typeof(availability_snapshot) = 'object'::text)),
    CONSTRAINT order_lines_currency_chk CHECK (((currency IS NULL) OR (currency ~ '^[A-Z]{3}$'::text))),
    CONSTRAINT order_lines_fulfillment_snapshot_object_chk CHECK ((jsonb_typeof(fulfillment_snapshot) = 'object'::text)),
    CONSTRAINT order_lines_item_name_chk CHECK ((length(btrim(item_name_snapshot)) > 0)),
    CONSTRAINT order_lines_price_currency_chk CHECK ((((unit_price_snapshot IS NULL) AND (line_total_snapshot IS NULL)) OR (currency IS NOT NULL))),
    CONSTRAINT order_lines_pricing_snapshot_object_chk CHECK ((jsonb_typeof(pricing_snapshot) = 'object'::text)),
    CONSTRAINT order_lines_quantity_chk CHECK ((quantity > (0)::numeric)),
    CONSTRAINT order_lines_total_price_chk CHECK (((line_total_snapshot IS NULL) OR (line_total_snapshot >= (0)::numeric))),
    CONSTRAINT order_lines_unit_price_chk CHECK (((unit_price_snapshot IS NULL) OR (unit_price_snapshot >= (0)::numeric)))
);


--
-- Name: outbound_delivery_status_updates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbound_delivery_status_updates (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    inbound_event_id uuid NOT NULL,
    outbound_message_id uuid,
    provider_ref text NOT NULL,
    provider_message_id text NOT NULL,
    delivery_status text NOT NULL,
    occurred_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT outbound_delivery_updates_message_chk CHECK ((length(btrim(provider_message_id)) > 0)),
    CONSTRAINT outbound_delivery_updates_provider_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT outbound_delivery_updates_status_chk CHECK ((delivery_status = ANY (ARRAY['accepted'::text, 'sent'::text, 'delivered'::text, 'read'::text, 'failed'::text])))
);


--
-- Name: outbound_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbound_messages (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    conversation_reference_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    provider_ref text NOT NULL,
    channel text NOT NULL,
    origin text NOT NULL,
    direction text NOT NULL,
    transport text NOT NULL,
    content_reference text NOT NULL,
    provider_idempotency_key text NOT NULL,
    status text NOT NULL,
    provider_message_id text,
    chatwoot_message_id text,
    failure_code text,
    attempt_count integer DEFAULT 0 NOT NULL,
    correlation_id uuid,
    causation_id uuid,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT outbound_messages_attempt_count_chk CHECK ((attempt_count >= 0)),
    CONSTRAINT outbound_messages_channel_chk CHECK ((channel = ANY (ARRAY['facebook'::text, 'instagram'::text, 'whatsapp'::text]))),
    CONSTRAINT outbound_messages_content_ref_chk CHECK ((length(btrim(content_reference)) > 0)),
    CONSTRAINT outbound_messages_direction_chk CHECK ((direction = 'outbound'::text)),
    CONSTRAINT outbound_messages_idempotency_key_chk CHECK ((length(btrim(provider_idempotency_key)) > 0)),
    CONSTRAINT outbound_messages_origin_chk CHECK ((origin = ANY (ARRAY['human'::text, 'ai'::text, 'automation'::text, 'system'::text]))),
    CONSTRAINT outbound_messages_provider_ref_chk CHECK ((length(btrim(provider_ref)) > 0)),
    CONSTRAINT outbound_messages_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'sending'::text, 'accepted'::text, 'sent'::text, 'delivered'::text, 'read'::text, 'failed'::text, 'unknown'::text]))),
    CONSTRAINT outbound_messages_transport_chk CHECK ((transport = ANY (ARRAY['provider'::text, 'chatwoot'::text])))
);


--
-- Name: outbox_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbox_entries (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    outbound_message_id uuid NOT NULL,
    command_type text NOT NULL,
    dedupe_key text NOT NULL,
    status text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    available_at timestamp with time zone NOT NULL,
    lease_owner text,
    lease_expires_at timestamp with time zone,
    last_error_code text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    lease_token uuid,
    result_code text,
    completed_at timestamp with time zone,
    CONSTRAINT outbox_entries_attempt_count_chk CHECK ((attempt_count >= 0)),
    CONSTRAINT outbox_entries_command_type_chk CHECK ((length(btrim(command_type)) > 0)),
    CONSTRAINT outbox_entries_completed_at_chk CHECK ((((status = 'completed'::text) AND (completed_at IS NOT NULL)) OR ((status <> 'completed'::text) AND (completed_at IS NULL)))),
    CONSTRAINT outbox_entries_dedupe_key_chk CHECK ((length(btrim(dedupe_key)) > 0)),
    CONSTRAINT outbox_entries_lease_chk CHECK ((((status = 'processing'::text) AND (lease_owner IS NOT NULL) AND (lease_expires_at IS NOT NULL)) OR ((status <> 'processing'::text) AND (lease_owner IS NULL) AND (lease_expires_at IS NULL)))),
    CONSTRAINT outbox_entries_lease_token_chk CHECK ((((status = 'processing'::text) AND (lease_token IS NOT NULL)) OR ((status <> 'processing'::text) AND (lease_token IS NULL)))),
    CONSTRAINT outbox_entries_result_code_chk CHECK (((result_code IS NULL) OR (length(btrim(result_code)) > 0))),
    CONSTRAINT outbox_entries_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'processing'::text, 'completed'::text, 'retryable_failed'::text, 'dead_letter'::text])))
);


--
-- Name: platform_super_admins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_super_admins (
    principal_id uuid NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    CONSTRAINT platform_super_admins_revocation_chk CHECK (((status = 'revoked'::text) = (revoked_at IS NOT NULL))),
    CONSTRAINT platform_super_admins_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'revoked'::text])))
);


--
-- Name: principals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.principals (
    id uuid NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL,
    password_hash text NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT principals_display_name_not_blank_chk CHECK ((length(btrim(display_name)) > 0)),
    CONSTRAINT principals_email_format_chk CHECK ((email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'::text)),
    CONSTRAINT principals_email_not_blank_chk CHECK ((length(btrim(email)) > 0)),
    CONSTRAINT principals_password_hash_not_blank_chk CHECK ((length(btrim(password_hash)) > 0)),
    CONSTRAINT principals_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'suspended'::text])))
);


--
-- Name: refresh_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.refresh_sessions (
    id uuid NOT NULL,
    principal_id uuid NOT NULL,
    token_hash character(64) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    CONSTRAINT refresh_sessions_expiry_after_create_chk CHECK ((expires_at > created_at))
);


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version bigint NOT NULL,
    name text NOT NULL,
    applied_at timestamp with time zone NOT NULL
);


--
-- Name: team_invitations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_invitations (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    email text NOT NULL,
    role text NOT NULL,
    permissions jsonb DEFAULT '[]'::jsonb NOT NULL,
    token_hash character(64) NOT NULL,
    status text NOT NULL,
    invited_by uuid NOT NULL,
    accepted_by uuid,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT team_invitations_acceptance_chk CHECK (((status = 'accepted'::text) = ((accepted_by IS NOT NULL) AND (accepted_at IS NOT NULL)))),
    CONSTRAINT team_invitations_email_format_chk CHECK ((email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'::text)),
    CONSTRAINT team_invitations_email_not_blank_chk CHECK ((length(btrim(email)) > 0)),
    CONSTRAINT team_invitations_expiry_after_create_chk CHECK ((expires_at > created_at)),
    CONSTRAINT team_invitations_permissions_array_chk CHECK ((jsonb_typeof(permissions) = 'array'::text)),
    CONSTRAINT team_invitations_revocation_chk CHECK (((status = 'revoked'::text) = (revoked_at IS NOT NULL))),
    CONSTRAINT team_invitations_role_chk CHECK ((role = ANY (ARRAY['owner'::text, 'admin'::text, 'manager'::text, 'agent'::text, 'analyst'::text, 'viewer'::text]))),
    CONSTRAINT team_invitations_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'revoked'::text, 'expired'::text])))
);


--
-- Name: transaction_confirmations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transaction_confirmations (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    status text NOT NULL,
    confirmed_by text,
    confirmed_at timestamp with time zone,
    evidence_reference text,
    policy_version text,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT transaction_confirmations_confirmed_by_chk CHECK (((confirmed_by IS NULL) OR (confirmed_by = ANY (ARRAY['customer'::text, 'human_agent'::text, 'system_policy'::text])))),
    CONSTRAINT transaction_confirmations_lifecycle_chk CHECK ((((status = ANY (ARRAY['not_required'::text, 'pending'::text])) AND (confirmed_by IS NULL) AND (confirmed_at IS NULL)) OR ((status = ANY (ARRAY['confirmed'::text, 'rejected'::text])) AND (confirmed_by IS NOT NULL) AND (confirmed_at IS NOT NULL)))),
    CONSTRAINT transaction_confirmations_status_chk CHECK ((status = ANY (ARRAY['not_required'::text, 'pending'::text, 'confirmed'::text, 'rejected'::text])))
);


--
-- Name: transaction_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.transaction_reviews (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    transaction_id uuid NOT NULL,
    required boolean NOT NULL,
    status text NOT NULL,
    reason_codes jsonb DEFAULT '[]'::jsonb NOT NULL,
    reviewer_reference text,
    decided_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    decision_reason text,
    CONSTRAINT transaction_reviews_decision_reason_chk CHECK (((decision_reason IS NULL) OR (length(btrim(decision_reason)) > 0))),
    CONSTRAINT transaction_reviews_lifecycle_chk CHECK ((((required = false) AND (status = 'bypassed'::text) AND (reviewer_reference IS NULL) AND (decided_at IS NULL)) OR ((required = true) AND (status = 'pending'::text) AND (reviewer_reference IS NULL) AND (decided_at IS NULL)) OR ((required = true) AND (status = ANY (ARRAY['approved'::text, 'rejected'::text])) AND (reviewer_reference IS NOT NULL) AND (decided_at IS NOT NULL)))),
    CONSTRAINT transaction_reviews_reason_codes_array_chk CHECK ((jsonb_typeof(reason_codes) = 'array'::text)),
    CONSTRAINT transaction_reviews_reviewer_chk CHECK (((reviewer_reference IS NULL) OR (length(btrim(reviewer_reference)) > 0))),
    CONSTRAINT transaction_reviews_status_chk CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'bypassed'::text])))
);


--
-- Name: variants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.variants (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    catalog_item_id uuid NOT NULL,
    name text NOT NULL,
    attributes jsonb NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT variants_attributes_object_chk CHECK ((jsonb_typeof(attributes) = 'object'::text)),
    CONSTRAINT variants_name_chk CHECK ((length(btrim(name)) > 0)),
    CONSTRAINT variants_resource_version_chk CHECK ((resource_version > 0)),
    CONSTRAINT variants_status_chk CHECK ((status = ANY (ARRAY['active'::text, 'inactive'::text, 'archived'::text])))
);


--
-- Name: ai_decisions ai_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_decisions
    ADD CONSTRAINT ai_decisions_pkey PRIMARY KEY (id);


--
-- Name: attribute_definitions attribute_definitions_key_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_definitions
    ADD CONSTRAINT attribute_definitions_key_uq UNIQUE (schema_id, attribute_key);


--
-- Name: attribute_definitions attribute_definitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_definitions
    ADD CONSTRAINT attribute_definitions_pkey PRIMARY KEY (id);


--
-- Name: attribute_schemas attribute_schemas_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_schemas
    ADD CONSTRAINT attribute_schemas_business_id_uq UNIQUE (business_id, id);


--
-- Name: attribute_schemas attribute_schemas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_schemas
    ADD CONSTRAINT attribute_schemas_pkey PRIMARY KEY (id);


--
-- Name: attribute_schemas attribute_schemas_version_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_schemas
    ADD CONSTRAINT attribute_schemas_version_uq UNIQUE (business_id, name, version);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: automation_executions automation_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_executions
    ADD CONSTRAINT automation_executions_pkey PRIMARY KEY (id);


--
-- Name: automation_executions automation_executions_rule_event_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_executions
    ADD CONSTRAINT automation_executions_rule_event_uq UNIQUE (business_id, rule_id, inbound_event_id);


--
-- Name: automation_rules automation_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rules
    ADD CONSTRAINT automation_rules_pkey PRIMARY KEY (id);


--
-- Name: business_memberships business_memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_pkey PRIMARY KEY (business_id, principal_id);


--
-- Name: business_policies business_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_policies
    ADD CONSTRAINT business_policies_pkey PRIMARY KEY (business_id);


--
-- Name: business_policy_versions business_policy_versions_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_policy_versions
    ADD CONSTRAINT business_policy_versions_business_id_uq UNIQUE (business_id, id);


--
-- Name: business_policy_versions business_policy_versions_key_version_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_policy_versions
    ADD CONSTRAINT business_policy_versions_key_version_uq UNIQUE (business_id, policy_key, version);


--
-- Name: business_policy_versions business_policy_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_policy_versions
    ADD CONSTRAINT business_policy_versions_pkey PRIMARY KEY (id);


--
-- Name: businesses businesses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.businesses
    ADD CONSTRAINT businesses_pkey PRIMARY KEY (id);


--
-- Name: businesses businesses_slug_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.businesses
    ADD CONSTRAINT businesses_slug_uq UNIQUE (slug);


--
-- Name: canned_replies canned_replies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.canned_replies
    ADD CONSTRAINT canned_replies_pkey PRIMARY KEY (id);


--
-- Name: catalog_items catalog_items_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_items
    ADD CONSTRAINT catalog_items_business_id_uq UNIQUE (business_id, id);


--
-- Name: catalog_items catalog_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_items
    ADD CONSTRAINT catalog_items_pkey PRIMARY KEY (id);


--
-- Name: catalogs catalogs_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogs
    ADD CONSTRAINT catalogs_business_id_uq UNIQUE (business_id, id);


--
-- Name: catalogs catalogs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogs
    ADD CONSTRAINT catalogs_pkey PRIMARY KEY (id);


--
-- Name: channel_connection_capabilities channel_connection_capabilities_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connection_capabilities
    ADD CONSTRAINT channel_connection_capabilities_pk PRIMARY KEY (connection_id, capability);


--
-- Name: channel_connection_state_events channel_connection_state_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connection_state_events
    ADD CONSTRAINT channel_connection_state_events_pkey PRIMARY KEY (id);


--
-- Name: channel_connections channel_connections_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connections
    ADD CONSTRAINT channel_connections_business_id_uq UNIQUE (business_id, id);


--
-- Name: channel_connections channel_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connections
    ADD CONSTRAINT channel_connections_pkey PRIMARY KEY (id);


--
-- Name: channel_connections channel_connections_provider_connection_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connections
    ADD CONSTRAINT channel_connections_provider_connection_uq UNIQUE (provider_ref, provider_connection_ref);


--
-- Name: channel_provisioning_sessions channel_provisioning_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_provisioning_sessions
    ADD CONSTRAINT channel_provisioning_business_id_uq UNIQUE (business_id, id);


--
-- Name: channel_provisioning_sessions channel_provisioning_idempotency_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_provisioning_sessions
    ADD CONSTRAINT channel_provisioning_idempotency_uq UNIQUE (business_id, idempotency_key);


--
-- Name: channel_provisioning_sessions channel_provisioning_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_provisioning_sessions
    ADD CONSTRAINT channel_provisioning_sessions_pkey PRIMARY KEY (id);


--
-- Name: chatwoot_contact_links chatwoot_contact_links_binding_user_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_binding_user_uq UNIQUE (binding_id, external_user_id);


--
-- Name: chatwoot_contact_links chatwoot_contact_links_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_business_id_uq UNIQUE (business_id, id);


--
-- Name: chatwoot_contact_links chatwoot_contact_links_chatwoot_contact_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_chatwoot_contact_uq UNIQUE (binding_id, chatwoot_contact_id);


--
-- Name: chatwoot_contact_links chatwoot_contact_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_pkey PRIMARY KEY (id);


--
-- Name: chatwoot_mirror_jobs chatwoot_mirror_jobs_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_mirror_jobs
    ADD CONSTRAINT chatwoot_mirror_jobs_business_id_uq UNIQUE (business_id, id);


--
-- Name: chatwoot_mirror_jobs chatwoot_mirror_jobs_message_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_mirror_jobs
    ADD CONSTRAINT chatwoot_mirror_jobs_message_uq UNIQUE (business_id, communication_message_id);


--
-- Name: chatwoot_mirror_jobs chatwoot_mirror_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_mirror_jobs
    ADD CONSTRAINT chatwoot_mirror_jobs_pkey PRIMARY KEY (id);


--
-- Name: chatwoot_workspace_bindings chatwoot_workspace_bindings_account_inbox_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_workspace_bindings
    ADD CONSTRAINT chatwoot_workspace_bindings_account_inbox_uq UNIQUE (account_id, inbox_id);


--
-- Name: chatwoot_workspace_bindings chatwoot_workspace_bindings_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_workspace_bindings
    ADD CONSTRAINT chatwoot_workspace_bindings_business_id_uq UNIQUE (business_id, id);


--
-- Name: chatwoot_workspace_bindings chatwoot_workspace_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_workspace_bindings
    ADD CONSTRAINT chatwoot_workspace_bindings_pkey PRIMARY KEY (id);


--
-- Name: chatwoot_workspace_bindings chatwoot_workspace_bindings_route_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_workspace_bindings
    ADD CONSTRAINT chatwoot_workspace_bindings_route_uq UNIQUE (route_key);


--
-- Name: commercial_transactions commercial_transactions_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.commercial_transactions
    ADD CONSTRAINT commercial_transactions_business_id_uq UNIQUE (business_id, id);


--
-- Name: commercial_transactions commercial_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.commercial_transactions
    ADD CONSTRAINT commercial_transactions_pkey PRIMARY KEY (id);


--
-- Name: communication_messages communication_messages_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_messages
    ADD CONSTRAINT communication_messages_business_id_uq UNIQUE (business_id, id);


--
-- Name: communication_messages communication_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_messages
    ADD CONSTRAINT communication_messages_pkey PRIMARY KEY (id);


--
-- Name: conversation_labels conversation_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_labels
    ADD CONSTRAINT conversation_labels_pkey PRIMARY KEY (business_id, conversation_id, label);


--
-- Name: conversation_read_cursors conversation_read_cursors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_read_cursors
    ADD CONSTRAINT conversation_read_cursors_pkey PRIMARY KEY (business_id, conversation_id, principal_id);


--
-- Name: conversation_references conversation_references_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_references
    ADD CONSTRAINT conversation_references_business_id_uq UNIQUE (business_id, id);


--
-- Name: conversation_references conversation_references_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_references
    ADD CONSTRAINT conversation_references_pkey PRIMARY KEY (id);


--
-- Name: conversation_state conversation_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_state
    ADD CONSTRAINT conversation_state_pkey PRIMARY KEY (business_id, conversation_id);


--
-- Name: conversations conversations_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_business_id_uq UNIQUE (business_id, id);


--
-- Name: conversations conversations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_pkey PRIMARY KEY (id);


--
-- Name: customers customers_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_business_id_uq UNIQUE (business_id, id);


--
-- Name: customers customers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_pkey PRIMARY KEY (id);


--
-- Name: external_identities external_identities_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_identities
    ADD CONSTRAINT external_identities_business_id_uq UNIQUE (business_id, id);


--
-- Name: external_identities external_identities_connection_user_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_identities
    ADD CONSTRAINT external_identities_connection_user_uq UNIQUE (connection_id, external_user_id);


--
-- Name: external_identities external_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_identities
    ADD CONSTRAINT external_identities_pkey PRIMARY KEY (id);


--
-- Name: identity_matches identity_matches_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_matches
    ADD CONSTRAINT identity_matches_business_id_uq UNIQUE (business_id, id);


--
-- Name: identity_matches identity_matches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_matches
    ADD CONSTRAINT identity_matches_pkey PRIMARY KEY (id);


--
-- Name: inbound_event_ledger inbound_event_event_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_event_ledger
    ADD CONSTRAINT inbound_event_event_uq UNIQUE (provider_ref, provider_connection_ref, provider_event_id);


--
-- Name: inbound_event_ledger inbound_event_ledger_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_event_ledger
    ADD CONSTRAINT inbound_event_ledger_business_id_uq UNIQUE (business_id, id);


--
-- Name: inbound_event_ledger inbound_event_ledger_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_event_ledger
    ADD CONSTRAINT inbound_event_ledger_pkey PRIMARY KEY (id);


--
-- Name: inbound_webhook_payloads inbound_webhook_payloads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_webhook_payloads
    ADD CONSTRAINT inbound_webhook_payloads_pkey PRIMARY KEY (id);


--
-- Name: inbound_webhook_payloads inbound_webhook_payloads_provider_delivery_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_webhook_payloads
    ADD CONSTRAINT inbound_webhook_payloads_provider_delivery_uq UNIQUE (provider_ref, delivery_id);


--
-- Name: knowledge_documents knowledge_documents_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_documents
    ADD CONSTRAINT knowledge_documents_business_id_uq UNIQUE (business_id, id);


--
-- Name: knowledge_documents knowledge_documents_key_version_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_documents
    ADD CONSTRAINT knowledge_documents_key_version_uq UNIQUE (business_id, knowledge_key, version);


--
-- Name: knowledge_documents knowledge_documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_documents
    ADD CONSTRAINT knowledge_documents_pkey PRIMARY KEY (id);


--
-- Name: lead_attributions lead_attributions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_attributions
    ADD CONSTRAINT lead_attributions_pkey PRIMARY KEY (id);


--
-- Name: lead_scores lead_scores_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_scores
    ADD CONSTRAINT lead_scores_pkey PRIMARY KEY (id);


--
-- Name: leads leads_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.leads
    ADD CONSTRAINT leads_business_id_uq UNIQUE (business_id, id);


--
-- Name: leads leads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.leads
    ADD CONSTRAINT leads_pkey PRIMARY KEY (id);


--
-- Name: offers offers_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_business_id_uq UNIQUE (business_id, id);


--
-- Name: offers offers_business_item_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_business_item_id_uq UNIQUE (business_id, catalog_item_id, id);


--
-- Name: offers offers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_pkey PRIMARY KEY (id);


--
-- Name: order_lines order_lines_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_business_id_uq UNIQUE (business_id, id);


--
-- Name: order_lines order_lines_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_pkey PRIMARY KEY (id);


--
-- Name: outbound_delivery_status_updates outbound_delivery_status_updates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_delivery_status_updates
    ADD CONSTRAINT outbound_delivery_status_updates_pkey PRIMARY KEY (id);


--
-- Name: outbound_delivery_status_updates outbound_delivery_updates_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_delivery_status_updates
    ADD CONSTRAINT outbound_delivery_updates_business_id_uq UNIQUE (business_id, id);


--
-- Name: outbound_delivery_status_updates outbound_delivery_updates_event_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_delivery_status_updates
    ADD CONSTRAINT outbound_delivery_updates_event_uq UNIQUE (business_id, inbound_event_id);


--
-- Name: outbound_messages outbound_messages_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_business_id_uq UNIQUE (business_id, id);


--
-- Name: outbound_messages outbound_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_pkey PRIMARY KEY (id);


--
-- Name: outbound_messages outbound_messages_provider_idempotency_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_provider_idempotency_uq UNIQUE (provider_ref, connection_id, provider_idempotency_key);


--
-- Name: outbox_entries outbox_entries_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_entries
    ADD CONSTRAINT outbox_entries_business_id_uq UNIQUE (business_id, id);


--
-- Name: outbox_entries outbox_entries_dedupe_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_entries
    ADD CONSTRAINT outbox_entries_dedupe_uq UNIQUE (business_id, command_type, dedupe_key);


--
-- Name: outbox_entries outbox_entries_outbound_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_entries
    ADD CONSTRAINT outbox_entries_outbound_id_uq UNIQUE (business_id, outbound_message_id);


--
-- Name: outbox_entries outbox_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_entries
    ADD CONSTRAINT outbox_entries_pkey PRIMARY KEY (id);


--
-- Name: platform_super_admins platform_super_admins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_super_admins
    ADD CONSTRAINT platform_super_admins_pkey PRIMARY KEY (principal_id);


--
-- Name: principals principals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.principals
    ADD CONSTRAINT principals_pkey PRIMARY KEY (id);


--
-- Name: refresh_sessions refresh_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_sessions
    ADD CONSTRAINT refresh_sessions_pkey PRIMARY KEY (id);


--
-- Name: refresh_sessions refresh_sessions_token_hash_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_sessions
    ADD CONSTRAINT refresh_sessions_token_hash_uq UNIQUE (token_hash);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: team_invitations team_invitations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_pkey PRIMARY KEY (id);


--
-- Name: team_invitations team_invitations_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_token_hash_key UNIQUE (token_hash);


--
-- Name: transaction_confirmations transaction_confirmations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_confirmations
    ADD CONSTRAINT transaction_confirmations_pkey PRIMARY KEY (id);


--
-- Name: transaction_confirmations transaction_confirmations_transaction_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_confirmations
    ADD CONSTRAINT transaction_confirmations_transaction_uq UNIQUE (business_id, transaction_id);


--
-- Name: transaction_reviews transaction_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_reviews
    ADD CONSTRAINT transaction_reviews_pkey PRIMARY KEY (id);


--
-- Name: transaction_reviews transaction_reviews_transaction_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_reviews
    ADD CONSTRAINT transaction_reviews_transaction_uq UNIQUE (business_id, transaction_id);


--
-- Name: variants variants_business_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variants
    ADD CONSTRAINT variants_business_id_uq UNIQUE (business_id, id);


--
-- Name: variants variants_business_item_id_uq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variants
    ADD CONSTRAINT variants_business_item_id_uq UNIQUE (business_id, catalog_item_id, id);


--
-- Name: variants variants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variants
    ADD CONSTRAINT variants_pkey PRIMARY KEY (id);


--
-- Name: business_memberships_principal_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX business_memberships_principal_active_idx ON public.business_memberships USING btree (principal_id, business_id) WHERE (status = 'active'::text);


--
-- Name: canned_replies_business_shortcut_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX canned_replies_business_shortcut_uq ON public.canned_replies USING btree (business_id, shortcut);


--
-- Name: idx_ai_decisions_business_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_decisions_business_conversation ON public.ai_decisions USING btree (business_id, conversation_id, created_at DESC) WHERE (conversation_id IS NOT NULL);


--
-- Name: idx_ai_decisions_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_decisions_business_created ON public.ai_decisions USING btree (business_id, created_at DESC, id DESC);


--
-- Name: idx_ai_decisions_business_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_decisions_business_expires ON public.ai_decisions USING btree (business_id, expires_at, updated_at DESC, id DESC) WHERE (expires_at IS NOT NULL);


--
-- Name: idx_ai_decisions_business_lifecycle; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_decisions_business_lifecycle ON public.ai_decisions USING btree (business_id, lifecycle, updated_at DESC);


--
-- Name: idx_ai_decisions_business_requires_human; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_decisions_business_requires_human ON public.ai_decisions USING btree (business_id, requires_human, updated_at DESC, id DESC);


--
-- Name: idx_ai_decisions_correlation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_decisions_correlation ON public.ai_decisions USING btree (business_id, correlation_id) WHERE (correlation_id IS NOT NULL);


--
-- Name: idx_attribute_definitions_schema_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attribute_definitions_schema_order ON public.attribute_definitions USING btree (schema_id, display_order, id);


--
-- Name: idx_attribute_schemas_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attribute_schemas_business ON public.attribute_schemas USING btree (business_id, name, version DESC);


--
-- Name: idx_audit_events_business_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_business_action ON public.audit_events USING btree (business_id, action, occurred_at DESC, id DESC);


--
-- Name: idx_audit_events_business_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_business_actor ON public.audit_events USING btree (business_id, actor_type, occurred_at DESC, id DESC);


--
-- Name: idx_audit_events_business_occurred; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_business_occurred ON public.audit_events USING btree (business_id, occurred_at DESC, id DESC);


--
-- Name: idx_audit_events_business_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_business_resource ON public.audit_events USING btree (business_id, resource_type, resource_id, occurred_at DESC) WHERE (resource_id IS NOT NULL);


--
-- Name: idx_audit_events_correlation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_events_correlation ON public.audit_events USING btree (business_id, correlation_id) WHERE (correlation_id IS NOT NULL);


--
-- Name: idx_automation_executions_business_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_executions_business_created ON public.automation_executions USING btree (business_id, created_at DESC, id DESC);


--
-- Name: idx_automation_rules_active_inbound; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_automation_rules_active_inbound ON public.automation_rules USING btree (business_id, trigger_kind, "position", id) WHERE (status = 'active'::text);


--
-- Name: idx_business_policy_versions_published_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_policy_versions_published_category ON public.business_policy_versions USING btree (business_id, category, valid_from DESC, id DESC) WHERE (status = 'published'::text);


--
-- Name: idx_businesses_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_status ON public.businesses USING btree (status);


--
-- Name: idx_canned_replies_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_canned_replies_business_status ON public.canned_replies USING btree (business_id, status, updated_at DESC, id DESC);


--
-- Name: idx_catalog_items_business_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_items_business_name ON public.catalog_items USING btree (business_id, catalog_id, name);


--
-- Name: idx_catalog_items_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalog_items_business_status ON public.catalog_items USING btree (business_id, catalog_id, status, updated_at DESC);


--
-- Name: idx_catalogs_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_catalogs_business_status ON public.catalogs USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_channel_connection_state_events_connection; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_connection_state_events_connection ON public.channel_connection_state_events USING btree (business_id, connection_id, created_at DESC);


--
-- Name: idx_channel_connections_business_channel; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_connections_business_channel ON public.channel_connections USING btree (business_id, channel);


--
-- Name: idx_channel_connections_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_connections_business_status ON public.channel_connections USING btree (business_id, status);


--
-- Name: idx_channel_provisioning_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_channel_provisioning_business_status ON public.channel_provisioning_sessions USING btree (business_id, status, updated_at DESC, id DESC);


--
-- Name: idx_chatwoot_contact_links_business_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chatwoot_contact_links_business_customer ON public.chatwoot_contact_links USING btree (business_id, customer_id);


--
-- Name: idx_chatwoot_mirror_jobs_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chatwoot_mirror_jobs_business_status ON public.chatwoot_mirror_jobs USING btree (business_id, status, updated_at DESC, id DESC);


--
-- Name: idx_chatwoot_mirror_jobs_claimable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chatwoot_mirror_jobs_claimable ON public.chatwoot_mirror_jobs USING btree (status, created_at, id) WHERE (status = 'pending'::text);


--
-- Name: idx_chatwoot_workspace_bindings_business_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chatwoot_workspace_bindings_business_active ON public.chatwoot_workspace_bindings USING btree (business_id, active);


--
-- Name: idx_commercial_transactions_business_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_commercial_transactions_business_customer ON public.commercial_transactions USING btree (business_id, customer_id, created_at DESC);


--
-- Name: idx_commercial_transactions_business_lead; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_commercial_transactions_business_lead ON public.commercial_transactions USING btree (business_id, lead_id, created_at DESC) WHERE (lead_id IS NOT NULL);


--
-- Name: idx_commercial_transactions_business_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_commercial_transactions_business_state ON public.commercial_transactions USING btree (business_id, state, updated_at DESC);


--
-- Name: idx_commercial_transactions_business_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_commercial_transactions_business_type ON public.commercial_transactions USING btree (business_id, transaction_type, updated_at DESC, id DESC);


--
-- Name: idx_communication_messages_inbound_event; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_messages_inbound_event ON public.communication_messages USING btree (business_id, inbound_event_id) WHERE (inbound_event_id IS NOT NULL);


--
-- Name: idx_communication_messages_outbound_message; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_messages_outbound_message ON public.communication_messages USING btree (business_id, outbound_message_id) WHERE (outbound_message_id IS NOT NULL);


--
-- Name: idx_communication_messages_timeline; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_messages_timeline ON public.communication_messages USING btree (business_id, conversation_reference_id, occurred_at DESC, created_at DESC, id DESC);


--
-- Name: idx_communication_messages_visibility; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_communication_messages_visibility ON public.communication_messages USING btree (business_id, conversation_reference_id, visibility, occurred_at DESC, id DESC);


--
-- Name: idx_conversation_labels_business_label; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversation_labels_business_label ON public.conversation_labels USING btree (business_id, label, conversation_id);


--
-- Name: idx_conversation_read_cursors_principal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversation_read_cursors_principal ON public.conversation_read_cursors USING btree (business_id, principal_id, updated_at DESC);


--
-- Name: idx_conversation_references_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversation_references_conversation ON public.conversation_references USING btree (business_id, conversation_id, updated_at DESC);


--
-- Name: idx_conversation_state_business_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversation_state_business_updated ON public.conversation_state USING btree (business_id, updated_at DESC);


--
-- Name: idx_conversations_business_activity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversations_business_activity ON public.conversations USING btree (business_id, last_activity_at DESC);


--
-- Name: idx_conversations_business_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversations_business_customer ON public.conversations USING btree (business_id, customer_id, updated_at DESC);


--
-- Name: idx_conversations_business_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conversations_business_state ON public.conversations USING btree (business_id, state, updated_at DESC);


--
-- Name: idx_customers_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_customers_business_status ON public.customers USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_external_identities_business_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_identities_business_customer ON public.external_identities USING btree (business_id, customer_id);


--
-- Name: idx_external_identities_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_external_identities_business_status ON public.external_identities USING btree (business_id, link_status, last_seen_at DESC);


--
-- Name: idx_identity_matches_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_identity_matches_business_status ON public.identity_matches USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_inbound_event_business_received; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inbound_event_business_received ON public.inbound_event_ledger USING btree (business_id, received_at DESC) WHERE (business_id IS NOT NULL);


--
-- Name: idx_inbound_event_claimable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inbound_event_claimable ON public.inbound_event_ledger USING btree (processing_state, next_attempt_at, received_at, id) WHERE (processing_state = ANY (ARRAY['received'::text, 'retryable_failed'::text]));


--
-- Name: idx_inbound_event_owner_lease; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inbound_event_owner_lease ON public.inbound_event_ledger USING btree (processing_owner, lease_expires_at, id) WHERE (processing_state = 'processing'::text);


--
-- Name: idx_inbound_event_processing; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inbound_event_processing ON public.inbound_event_ledger USING btree (processing_state, next_attempt_at, received_at) WHERE (processing_state = ANY (ARRAY['received'::text, 'retryable_failed'::text]));


--
-- Name: idx_inbound_event_unresolved; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inbound_event_unresolved ON public.inbound_event_ledger USING btree (provider_ref, provider_connection_ref, processing_state, received_at) WHERE (processing_state = 'unresolved'::text);


--
-- Name: idx_inbound_webhook_payloads_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_inbound_webhook_payloads_created ON public.inbound_webhook_payloads USING btree (created_at DESC);


--
-- Name: idx_knowledge_documents_published; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_knowledge_documents_published ON public.knowledge_documents USING btree (business_id, status, valid_from DESC, id DESC) WHERE (status = 'published'::text);


--
-- Name: idx_lead_attributions_business_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_attributions_business_conversation ON public.lead_attributions USING btree (business_id, source_conversation_id, captured_at DESC) WHERE (source_conversation_id IS NOT NULL);


--
-- Name: idx_lead_attributions_business_lead; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_attributions_business_lead ON public.lead_attributions USING btree (business_id, lead_id, captured_at DESC);


--
-- Name: idx_lead_scores_business_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lead_scores_business_history ON public.lead_scores USING btree (business_id, lead_id, calculated_at DESC, id DESC);


--
-- Name: idx_leads_business_customer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_business_customer ON public.leads USING btree (business_id, customer_id, updated_at DESC);


--
-- Name: idx_leads_business_qualification_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_business_qualification_state ON public.leads USING btree (business_id, qualification_state, updated_at DESC, id DESC);


--
-- Name: idx_leads_business_source_reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_business_source_reference ON public.leads USING btree (business_id, source_conversation_reference_id, created_at DESC, id DESC) WHERE (source_conversation_reference_id IS NOT NULL);


--
-- Name: idx_leads_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_leads_business_status ON public.leads USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_offers_business_item_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_business_item_status ON public.offers USING btree (business_id, catalog_item_id, status, updated_at DESC);


--
-- Name: idx_offers_business_validity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_business_validity ON public.offers USING btree (business_id, validity_from, validity_until);


--
-- Name: idx_order_lines_business_transaction; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_lines_business_transaction ON public.order_lines USING btree (business_id, transaction_id, id);


--
-- Name: idx_outbound_delivery_updates_message; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbound_delivery_updates_message ON public.outbound_delivery_status_updates USING btree (business_id, outbound_message_id, occurred_at DESC) WHERE (outbound_message_id IS NOT NULL);


--
-- Name: idx_outbound_messages_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbound_messages_business_status ON public.outbound_messages USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_outbound_messages_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbound_messages_conversation ON public.outbound_messages USING btree (business_id, conversation_id, created_at DESC);


--
-- Name: idx_outbound_messages_correlation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbound_messages_correlation ON public.outbound_messages USING btree (business_id, correlation_id) WHERE (correlation_id IS NOT NULL);


--
-- Name: idx_outbox_business_available; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbox_business_available ON public.outbox_entries USING btree (business_id, status, available_at, created_at, id) WHERE (status = ANY (ARRAY['pending'::text, 'retryable_failed'::text]));


--
-- Name: idx_outbox_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbox_business_status ON public.outbox_entries USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_outbox_claimable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbox_claimable ON public.outbox_entries USING btree (status, available_at, created_at) WHERE (status = ANY (ARRAY['pending'::text, 'retryable_failed'::text]));


--
-- Name: idx_outbox_owner_lease; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_outbox_owner_lease ON public.outbox_entries USING btree (lease_owner, lease_expires_at, id) WHERE (status = 'processing'::text);


--
-- Name: idx_provider_conversation_references_chatwoot_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_provider_conversation_references_chatwoot_lookup ON public.conversation_references USING btree (business_id, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id, updated_at DESC) WHERE ((system = 'provider'::text) AND is_current);


--
-- Name: idx_transaction_reviews_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_transaction_reviews_business_status ON public.transaction_reviews USING btree (business_id, status, updated_at DESC);


--
-- Name: idx_variants_business_item_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_variants_business_item_status ON public.variants USING btree (business_id, catalog_item_id, status, updated_at DESC);


--
-- Name: platform_super_admins_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX platform_super_admins_active_idx ON public.platform_super_admins USING btree (principal_id) WHERE (status = 'active'::text);


--
-- Name: principals_email_ci_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX principals_email_ci_uq ON public.principals USING btree (lower(email));


--
-- Name: refresh_sessions_principal_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX refresh_sessions_principal_active_idx ON public.refresh_sessions USING btree (principal_id, expires_at) WHERE (revoked_at IS NULL);


--
-- Name: team_invitations_business_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX team_invitations_business_status_idx ON public.team_invitations USING btree (business_id, status, created_at DESC);


--
-- Name: team_invitations_pending_email_uq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX team_invitations_pending_email_uq ON public.team_invitations USING btree (business_id, lower(email)) WHERE (status = 'pending'::text);


--
-- Name: uq_business_policy_versions_published_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_business_policy_versions_published_key ON public.business_policy_versions USING btree (business_id, policy_key) WHERE (status = 'published'::text);


--
-- Name: uq_channel_connections_provider_account; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_channel_connections_provider_account ON public.channel_connections USING btree (provider_ref, provider_account_ref) WHERE ((provider_account_ref IS NOT NULL) AND (length(btrim(provider_account_ref)) > 0));


--
-- Name: uq_channel_provisioning_active_channel; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_channel_provisioning_active_channel ON public.channel_provisioning_sessions USING btree (business_id, channel) WHERE (status = ANY (ARRAY['pending_authorization'::text, 'provisioning'::text, 'connected'::text]));


--
-- Name: uq_channel_provisioning_oauth_state; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_channel_provisioning_oauth_state ON public.channel_provisioning_sessions USING btree (oauth_state) WHERE ((oauth_state IS NOT NULL) AND (length(btrim(oauth_state)) > 0));


--
-- Name: uq_communication_messages_chatwoot_message; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_communication_messages_chatwoot_message ON public.communication_messages USING btree (business_id, transport, chatwoot_message_id) WHERE (chatwoot_message_id IS NOT NULL);


--
-- Name: uq_communication_messages_provider_message; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_communication_messages_provider_message ON public.communication_messages USING btree (business_id, transport, provider_message_id) WHERE (provider_message_id IS NOT NULL);


--
-- Name: uq_conversation_references_current_system; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_conversation_references_current_system ON public.conversation_references USING btree (business_id, conversation_id, system) WHERE is_current;


--
-- Name: uq_conversation_references_external_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_conversation_references_external_resource ON public.conversation_references USING btree (business_id, system, provider_ref, resource_type, resource_id);


--
-- Name: uq_identity_matches_pair; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_identity_matches_pair ON public.identity_matches USING btree (business_id, left_identity_id, right_identity_id);


--
-- Name: uq_knowledge_documents_published_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_knowledge_documents_published_key ON public.knowledge_documents USING btree (business_id, knowledge_key) WHERE (status = 'published'::text);


--
-- Name: uq_provider_conversation_references_chatwoot_binding; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_provider_conversation_references_chatwoot_binding ON public.conversation_references USING btree (business_id, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id) WHERE ((system = 'provider'::text) AND is_current AND (chatwoot_account_id IS NOT NULL) AND (chatwoot_inbox_id IS NOT NULL) AND (chatwoot_conversation_id IS NOT NULL));


--
-- Name: audit_events audit_events_append_only_trigger; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER audit_events_append_only_trigger BEFORE DELETE OR UPDATE ON public.audit_events FOR EACH ROW EXECUTE FUNCTION public.reject_audit_event_mutation();


--
-- Name: ai_decisions ai_decisions_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_decisions
    ADD CONSTRAINT ai_decisions_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: ai_decisions ai_decisions_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_decisions
    ADD CONSTRAINT ai_decisions_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: attribute_definitions attribute_definitions_schema_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_definitions
    ADD CONSTRAINT attribute_definitions_schema_fk FOREIGN KEY (schema_id) REFERENCES public.attribute_schemas(id) ON DELETE RESTRICT;


--
-- Name: attribute_schemas attribute_schemas_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attribute_schemas
    ADD CONSTRAINT attribute_schemas_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: audit_events audit_events_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: automation_executions automation_executions_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_executions
    ADD CONSTRAINT automation_executions_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: automation_executions automation_executions_inbound_event_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_executions
    ADD CONSTRAINT automation_executions_inbound_event_fk FOREIGN KEY (inbound_event_id) REFERENCES public.inbound_event_ledger(id) ON DELETE RESTRICT;


--
-- Name: automation_executions automation_executions_rule_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_executions
    ADD CONSTRAINT automation_executions_rule_fk FOREIGN KEY (rule_id) REFERENCES public.automation_rules(id) ON DELETE RESTRICT;


--
-- Name: automation_rules automation_rules_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.automation_rules
    ADD CONSTRAINT automation_rules_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: business_memberships business_memberships_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: business_memberships business_memberships_principal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_principal_id_fkey FOREIGN KEY (principal_id) REFERENCES public.principals(id) ON DELETE RESTRICT;


--
-- Name: business_policies business_policies_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_policies
    ADD CONSTRAINT business_policies_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: business_policy_versions business_policy_versions_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_policy_versions
    ADD CONSTRAINT business_policy_versions_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: canned_replies canned_replies_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.canned_replies
    ADD CONSTRAINT canned_replies_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: catalog_items catalog_items_attribute_schema_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_items
    ADD CONSTRAINT catalog_items_attribute_schema_fk FOREIGN KEY (business_id, attribute_schema_id) REFERENCES public.attribute_schemas(business_id, id) ON DELETE RESTRICT;


--
-- Name: catalog_items catalog_items_catalog_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalog_items
    ADD CONSTRAINT catalog_items_catalog_fk FOREIGN KEY (business_id, catalog_id) REFERENCES public.catalogs(business_id, id) ON DELETE RESTRICT;


--
-- Name: catalogs catalogs_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogs
    ADD CONSTRAINT catalogs_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: channel_connection_capabilities channel_connection_capabilities_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connection_capabilities
    ADD CONSTRAINT channel_connection_capabilities_connection_fk FOREIGN KEY (connection_id) REFERENCES public.channel_connections(id) ON DELETE RESTRICT;


--
-- Name: channel_connection_state_events channel_connection_state_events_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connection_state_events
    ADD CONSTRAINT channel_connection_state_events_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: channel_connection_state_events channel_connection_state_events_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connection_state_events
    ADD CONSTRAINT channel_connection_state_events_connection_fk FOREIGN KEY (business_id, connection_id) REFERENCES public.channel_connections(business_id, id) ON DELETE RESTRICT;


--
-- Name: channel_connections channel_connections_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_connections
    ADD CONSTRAINT channel_connections_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: channel_provisioning_sessions channel_provisioning_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_provisioning_sessions
    ADD CONSTRAINT channel_provisioning_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: channel_provisioning_sessions channel_provisioning_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_provisioning_sessions
    ADD CONSTRAINT channel_provisioning_connection_fk FOREIGN KEY (business_id, channel_connection_id) REFERENCES public.channel_connections(business_id, id) ON DELETE RESTRICT;


--
-- Name: chatwoot_contact_links chatwoot_contact_links_binding_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_binding_business_fk FOREIGN KEY (business_id, binding_id) REFERENCES public.chatwoot_workspace_bindings(business_id, id) ON DELETE RESTRICT;


--
-- Name: chatwoot_contact_links chatwoot_contact_links_binding_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_binding_fk FOREIGN KEY (binding_id) REFERENCES public.chatwoot_workspace_bindings(id) ON DELETE RESTRICT;


--
-- Name: chatwoot_contact_links chatwoot_contact_links_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: chatwoot_contact_links chatwoot_contact_links_customer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_contact_links
    ADD CONSTRAINT chatwoot_contact_links_customer_fk FOREIGN KEY (business_id, customer_id) REFERENCES public.customers(business_id, id) ON DELETE RESTRICT;


--
-- Name: chatwoot_mirror_jobs chatwoot_mirror_jobs_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_mirror_jobs
    ADD CONSTRAINT chatwoot_mirror_jobs_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: chatwoot_mirror_jobs chatwoot_mirror_jobs_message_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_mirror_jobs
    ADD CONSTRAINT chatwoot_mirror_jobs_message_fk FOREIGN KEY (business_id, communication_message_id) REFERENCES public.communication_messages(business_id, id) ON DELETE RESTRICT;


--
-- Name: chatwoot_workspace_bindings chatwoot_workspace_bindings_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chatwoot_workspace_bindings
    ADD CONSTRAINT chatwoot_workspace_bindings_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: commercial_transactions commercial_transactions_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.commercial_transactions
    ADD CONSTRAINT commercial_transactions_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: commercial_transactions commercial_transactions_conversation_reference_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.commercial_transactions
    ADD CONSTRAINT commercial_transactions_conversation_reference_fk FOREIGN KEY (business_id, source_conversation_reference_id) REFERENCES public.conversation_references(business_id, id) ON DELETE RESTRICT;


--
-- Name: commercial_transactions commercial_transactions_customer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.commercial_transactions
    ADD CONSTRAINT commercial_transactions_customer_fk FOREIGN KEY (business_id, customer_id) REFERENCES public.customers(business_id, id) ON DELETE RESTRICT;


--
-- Name: commercial_transactions commercial_transactions_lead_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.commercial_transactions
    ADD CONSTRAINT commercial_transactions_lead_fk FOREIGN KEY (business_id, lead_id) REFERENCES public.leads(business_id, id) ON DELETE RESTRICT;


--
-- Name: communication_messages communication_messages_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_messages
    ADD CONSTRAINT communication_messages_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: communication_messages communication_messages_conversation_reference_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_messages
    ADD CONSTRAINT communication_messages_conversation_reference_fk FOREIGN KEY (business_id, conversation_reference_id) REFERENCES public.conversation_references(business_id, id) ON DELETE RESTRICT;


--
-- Name: communication_messages communication_messages_inbound_event_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_messages
    ADD CONSTRAINT communication_messages_inbound_event_fk FOREIGN KEY (business_id, inbound_event_id) REFERENCES public.inbound_event_ledger(business_id, id) ON DELETE RESTRICT;


--
-- Name: communication_messages communication_messages_outbound_message_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.communication_messages
    ADD CONSTRAINT communication_messages_outbound_message_fk FOREIGN KEY (business_id, outbound_message_id) REFERENCES public.outbound_messages(business_id, id) ON DELETE RESTRICT;


--
-- Name: conversation_labels conversation_labels_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_labels
    ADD CONSTRAINT conversation_labels_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: conversation_labels conversation_labels_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_labels
    ADD CONSTRAINT conversation_labels_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: conversation_read_cursors conversation_read_cursors_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_read_cursors
    ADD CONSTRAINT conversation_read_cursors_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: conversation_read_cursors conversation_read_cursors_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_read_cursors
    ADD CONSTRAINT conversation_read_cursors_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: conversation_read_cursors conversation_read_cursors_membership_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_read_cursors
    ADD CONSTRAINT conversation_read_cursors_membership_fk FOREIGN KEY (business_id, principal_id) REFERENCES public.business_memberships(business_id, principal_id) ON DELETE RESTRICT;


--
-- Name: conversation_references conversation_references_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_references
    ADD CONSTRAINT conversation_references_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: conversation_references conversation_references_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_references
    ADD CONSTRAINT conversation_references_connection_fk FOREIGN KEY (business_id, connection_id) REFERENCES public.channel_connections(business_id, id) ON DELETE RESTRICT;


--
-- Name: conversation_references conversation_references_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_references
    ADD CONSTRAINT conversation_references_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: conversation_state conversation_state_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_state
    ADD CONSTRAINT conversation_state_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: conversation_state conversation_state_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_state
    ADD CONSTRAINT conversation_state_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: conversations conversations_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: conversations conversations_customer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_customer_fk FOREIGN KEY (business_id, customer_id) REFERENCES public.customers(business_id, id) ON DELETE RESTRICT;


--
-- Name: customers customers_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: customers customers_merge_same_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_merge_same_business_fk FOREIGN KEY (business_id, merged_into_customer_id) REFERENCES public.customers(business_id, id) ON DELETE RESTRICT;


--
-- Name: external_identities external_identities_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_identities
    ADD CONSTRAINT external_identities_connection_fk FOREIGN KEY (business_id, connection_id) REFERENCES public.channel_connections(business_id, id) ON DELETE RESTRICT;


--
-- Name: external_identities external_identities_customer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.external_identities
    ADD CONSTRAINT external_identities_customer_fk FOREIGN KEY (business_id, customer_id) REFERENCES public.customers(business_id, id) ON DELETE RESTRICT;


--
-- Name: identity_matches identity_matches_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_matches
    ADD CONSTRAINT identity_matches_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: identity_matches identity_matches_left_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_matches
    ADD CONSTRAINT identity_matches_left_fk FOREIGN KEY (business_id, left_identity_id) REFERENCES public.external_identities(business_id, id) ON DELETE RESTRICT;


--
-- Name: identity_matches identity_matches_right_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_matches
    ADD CONSTRAINT identity_matches_right_fk FOREIGN KEY (business_id, right_identity_id) REFERENCES public.external_identities(business_id, id) ON DELETE RESTRICT;


--
-- Name: inbound_event_ledger inbound_event_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_event_ledger
    ADD CONSTRAINT inbound_event_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: inbound_event_ledger inbound_event_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_event_ledger
    ADD CONSTRAINT inbound_event_connection_fk FOREIGN KEY (business_id, connection_id) REFERENCES public.channel_connections(business_id, id) ON DELETE RESTRICT;


--
-- Name: knowledge_documents knowledge_documents_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.knowledge_documents
    ADD CONSTRAINT knowledge_documents_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: lead_attributions lead_attributions_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_attributions
    ADD CONSTRAINT lead_attributions_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: lead_attributions lead_attributions_catalog_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_attributions
    ADD CONSTRAINT lead_attributions_catalog_item_fk FOREIGN KEY (business_id, catalog_item_id) REFERENCES public.catalog_items(business_id, id) ON DELETE RESTRICT;


--
-- Name: lead_attributions lead_attributions_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_attributions
    ADD CONSTRAINT lead_attributions_conversation_fk FOREIGN KEY (business_id, source_conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: lead_attributions lead_attributions_lead_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_attributions
    ADD CONSTRAINT lead_attributions_lead_fk FOREIGN KEY (business_id, lead_id) REFERENCES public.leads(business_id, id) ON DELETE RESTRICT;


--
-- Name: lead_attributions lead_attributions_offer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_attributions
    ADD CONSTRAINT lead_attributions_offer_fk FOREIGN KEY (business_id, offer_id) REFERENCES public.offers(business_id, id) ON DELETE RESTRICT;


--
-- Name: lead_scores lead_scores_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_scores
    ADD CONSTRAINT lead_scores_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: lead_scores lead_scores_lead_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.lead_scores
    ADD CONSTRAINT lead_scores_lead_fk FOREIGN KEY (business_id, lead_id) REFERENCES public.leads(business_id, id) ON DELETE RESTRICT;


--
-- Name: leads leads_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.leads
    ADD CONSTRAINT leads_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: leads leads_customer_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.leads
    ADD CONSTRAINT leads_customer_fk FOREIGN KEY (business_id, customer_id) REFERENCES public.customers(business_id, id) ON DELETE RESTRICT;


--
-- Name: leads leads_source_conversation_reference_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.leads
    ADD CONSTRAINT leads_source_conversation_reference_fk FOREIGN KEY (business_id, source_conversation_reference_id) REFERENCES public.conversation_references(business_id, id) ON DELETE RESTRICT;


--
-- Name: offers offers_catalog_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_catalog_item_fk FOREIGN KEY (business_id, catalog_item_id) REFERENCES public.catalog_items(business_id, id) ON DELETE RESTRICT;


--
-- Name: offers offers_variant_same_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_variant_same_item_fk FOREIGN KEY (business_id, catalog_item_id, variant_id) REFERENCES public.variants(business_id, catalog_item_id, id) ON DELETE RESTRICT;


--
-- Name: order_lines order_lines_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: order_lines order_lines_catalog_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_catalog_item_fk FOREIGN KEY (business_id, catalog_item_id) REFERENCES public.catalog_items(business_id, id) ON DELETE RESTRICT;


--
-- Name: order_lines order_lines_offer_same_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_offer_same_item_fk FOREIGN KEY (business_id, catalog_item_id, offer_id) REFERENCES public.offers(business_id, catalog_item_id, id) ON DELETE RESTRICT;


--
-- Name: order_lines order_lines_transaction_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_transaction_fk FOREIGN KEY (business_id, transaction_id) REFERENCES public.commercial_transactions(business_id, id) ON DELETE RESTRICT;


--
-- Name: order_lines order_lines_variant_same_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_lines
    ADD CONSTRAINT order_lines_variant_same_item_fk FOREIGN KEY (business_id, catalog_item_id, variant_id) REFERENCES public.variants(business_id, catalog_item_id, id) ON DELETE RESTRICT;


--
-- Name: outbound_delivery_status_updates outbound_delivery_updates_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_delivery_status_updates
    ADD CONSTRAINT outbound_delivery_updates_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: outbound_delivery_status_updates outbound_delivery_updates_event_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_delivery_status_updates
    ADD CONSTRAINT outbound_delivery_updates_event_fk FOREIGN KEY (business_id, inbound_event_id) REFERENCES public.inbound_event_ledger(business_id, id) ON DELETE RESTRICT;


--
-- Name: outbound_delivery_status_updates outbound_delivery_updates_message_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_delivery_status_updates
    ADD CONSTRAINT outbound_delivery_updates_message_fk FOREIGN KEY (business_id, outbound_message_id) REFERENCES public.outbound_messages(business_id, id) ON DELETE RESTRICT;


--
-- Name: outbound_messages outbound_messages_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: outbound_messages outbound_messages_connection_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_connection_fk FOREIGN KEY (business_id, connection_id) REFERENCES public.channel_connections(business_id, id) ON DELETE RESTRICT;


--
-- Name: outbound_messages outbound_messages_conversation_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES public.conversations(business_id, id) ON DELETE RESTRICT;


--
-- Name: outbound_messages outbound_messages_reference_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_messages
    ADD CONSTRAINT outbound_messages_reference_fk FOREIGN KEY (business_id, conversation_reference_id) REFERENCES public.conversation_references(business_id, id) ON DELETE RESTRICT;


--
-- Name: outbox_entries outbox_entries_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_entries
    ADD CONSTRAINT outbox_entries_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: outbox_entries outbox_entries_outbound_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_entries
    ADD CONSTRAINT outbox_entries_outbound_fk FOREIGN KEY (business_id, outbound_message_id) REFERENCES public.outbound_messages(business_id, id) ON DELETE RESTRICT;


--
-- Name: platform_super_admins platform_super_admins_principal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_super_admins
    ADD CONSTRAINT platform_super_admins_principal_id_fkey FOREIGN KEY (principal_id) REFERENCES public.principals(id) ON DELETE RESTRICT;


--
-- Name: refresh_sessions refresh_sessions_principal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_sessions
    ADD CONSTRAINT refresh_sessions_principal_id_fkey FOREIGN KEY (principal_id) REFERENCES public.principals(id) ON DELETE RESTRICT;


--
-- Name: team_invitations team_invitations_accepted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_accepted_by_fkey FOREIGN KEY (accepted_by) REFERENCES public.principals(id) ON DELETE RESTRICT;


--
-- Name: team_invitations team_invitations_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: team_invitations team_invitations_invited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.principals(id) ON DELETE RESTRICT;


--
-- Name: transaction_confirmations transaction_confirmations_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_confirmations
    ADD CONSTRAINT transaction_confirmations_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: transaction_confirmations transaction_confirmations_transaction_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_confirmations
    ADD CONSTRAINT transaction_confirmations_transaction_fk FOREIGN KEY (business_id, transaction_id) REFERENCES public.commercial_transactions(business_id, id) ON DELETE RESTRICT;


--
-- Name: transaction_reviews transaction_reviews_business_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_reviews
    ADD CONSTRAINT transaction_reviews_business_fk FOREIGN KEY (business_id) REFERENCES public.businesses(id) ON DELETE RESTRICT;


--
-- Name: transaction_reviews transaction_reviews_transaction_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.transaction_reviews
    ADD CONSTRAINT transaction_reviews_transaction_fk FOREIGN KEY (business_id, transaction_id) REFERENCES public.commercial_transactions(business_id, id) ON DELETE RESTRICT;


--
-- Name: variants variants_catalog_item_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variants
    ADD CONSTRAINT variants_catalog_item_fk FOREIGN KEY (business_id, catalog_item_id) REFERENCES public.catalog_items(business_id, id) ON DELETE RESTRICT;


--
-- PostgreSQL database dump complete
--

\unrestrict eN7N1as40eswpAC4QbARumJSWY0BZNkobeeIqgvKtrqONGbzgcf4EWyc7hMRDKV

