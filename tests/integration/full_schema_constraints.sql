\set ON_ERROR_STOP on

CREATE OR REPLACE FUNCTION assert_raises(expected_sqlstate text, statement text)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    actual_sqlstate text;
    completed      boolean := false;
BEGIN
    BEGIN
        EXECUTE statement;
        completed := true;
    EXCEPTION WHEN OTHERS THEN
        GET STACKED DIAGNOSTICS actual_sqlstate = RETURNED_SQLSTATE;
    END;

    IF completed THEN
        RAISE EXCEPTION 'expected SQLSTATE %, but statement succeeded: %', expected_sqlstate, statement;
    END IF;

    IF actual_sqlstate <> expected_sqlstate THEN
        RAISE EXCEPTION 'expected SQLSTATE %, got % for statement: %', expected_sqlstate, actual_sqlstate, statement;
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION assert_true(condition boolean, message text)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT condition THEN
        RAISE EXCEPTION 'assertion failed: %', message;
    END IF;
END;
$$;

INSERT INTO businesses (
    id, name, slug, status, vertical_type, timezone, default_currency,
    locale, created_at, updated_at
) VALUES
    ('00000000-0000-0000-0000-000000000001', 'Business A', 'full-business-a', 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()),
    ('00000000-0000-0000-0000-000000000002', 'Business B', 'full-business-b', 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', now(), now());

INSERT INTO channel_connections (
    id, business_id, provider_ref, channel, provider_account_ref,
    provider_connection_ref, status, secret_reference, created_at, updated_at
) VALUES
    ('10000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'socialapi', 'facebook', 'full-account-a', 'full-connection-a', 'active', 'secret://test/full-a', now(), now()),
    ('10000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'socialapi', 'facebook', 'full-account-b', 'full-connection-b', 'active', 'secret://test/full-b', now(), now());

INSERT INTO customers (id, business_id, status, created_at, updated_at) VALUES
    ('20000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'active', now(), now()),
    ('20000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'active', now(), now());

INSERT INTO conversations (
    id, business_id, customer_id, state, ownership, priority,
    last_activity_at, created_at, updated_at
) VALUES
    ('40000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', 'open', 'none', 'normal', now(), now(), now()),
    ('40000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000002', 'open', 'none', 'normal', now(), now(), now());

INSERT INTO conversation_references (
    id, business_id, conversation_id, system, provider_ref, resource_type,
    resource_id, connection_id, conversation_kind, is_current, mapping_status,
    created_at, updated_at
) VALUES
    ('50000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', '40000000-0000-0000-0000-000000000001', 'provider', 'socialapi', 'conversation', 'full-conversation-a', '10000000-0000-0000-0000-000000000001', 'dm', true, 'active', now(), now());

INSERT INTO catalogs (id, business_id, name, status, created_at, updated_at) VALUES
    ('90000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'Electronics', 'active', now(), now()),
    ('90000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'Travel', 'active', now(), now());

INSERT INTO attribute_schemas (id, business_id, name, version, created_at, updated_at) VALUES
    ('91000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'phone', 1, now(), now()),
    ('91000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'route', 1, now(), now());

INSERT INTO attribute_definitions (
    id, schema_id, attribute_key, label, data_type, is_required,
    is_searchable, validation_rules, display_order, created_at, updated_at
) VALUES (
    '92000000-0000-0000-0000-000000000001',
    '91000000-0000-0000-0000-000000000001',
    'color', 'Color', 'select', false, true, '{"options":["black","white"]}', 0, now(), now()
);

INSERT INTO catalog_items (
    id, business_id, catalog_id, attribute_schema_id, attribute_schema_version,
    item_type, name, status, pricing_mode, availability_mode, fulfillment_mode,
    requires_confirmation, attributes, created_at, updated_at
) VALUES
    ('93000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', '90000000-0000-0000-0000-000000000001', '91000000-0000-0000-0000-000000000001', 1, 'physical_good', 'Phone A', 'active', 'fixed', 'stock', 'delivery', true, '{"color":"black"}', now(), now()),
    ('93000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', '90000000-0000-0000-0000-000000000002', '91000000-0000-0000-0000-000000000002', 1, 'travel_offer', 'Route B', 'active', 'quote_required', 'supplier_check', 'travel', true, '{}', now(), now());

INSERT INTO variants (
    id, business_id, catalog_item_id, name, attributes, status, created_at, updated_at
) VALUES
    ('94000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', '93000000-0000-0000-0000-000000000001', 'Black 256GB', '{"color":"black","storage_gb":256}', 'active', now(), now()),
    ('94000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', '93000000-0000-0000-0000-000000000002', 'Economy', '{"cabin":"economy"}', 'active', now(), now());

INSERT INTO offers (
    id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount,
    currency, pricing_unit, price_source, price_verification_status,
    price_checked_at, availability_mode, availability_status, availability_source,
    availability_checked_at, availability_valid_until, fulfillment_mode,
    status, created_at, updated_at
) VALUES (
    '95000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '93000000-0000-0000-0000-000000000001',
    '94000000-0000-0000-0000-000000000001',
    'Phone A Offer', 'fixed', 180000, 'YER', NULL, 'merchant_catalog', 'verified',
    now(), 'stock', 'available', 'merchant_inventory', now(), now() + interval '1 day',
    'delivery', 'active', now(), now()
), (
    '95000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000002',
    '93000000-0000-0000-0000-000000000002',
    '94000000-0000-0000-0000-000000000002',
    'Route B Offer', 'quote_required', NULL, NULL, NULL, NULL, 'unverified',
    NULL, 'supplier_check', 'requires_check', 'supplier', NULL, NULL,
    'travel', 'draft', now(), now()
);

SELECT assert_raises(
    '23503',
    $$INSERT INTO offers (
        id, business_id, catalog_item_id, variant_id, name, pricing_mode,
        amount, currency, price_verification_status, availability_mode,
        availability_status, fulfillment_mode, status, created_at, updated_at
    ) VALUES (
        '95000000-0000-0000-0000-000000000003',
        '00000000-0000-0000-0000-000000000001',
        '93000000-0000-0000-0000-000000000001',
        '94000000-0000-0000-0000-000000000002',
        'Invalid Cross-Tenant Offer', 'quote_required', NULL, NULL, 'unverified',
        'unknown', 'unknown', 'travel', 'draft', now(), now()
    )$$
);

INSERT INTO leads (
    id, business_id, customer_id, status, current_score_value, current_score_band,
    qualification_context, created_by, created_at, updated_at
) VALUES (
    '96000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000001',
    'interested', 70, 'medium', '{"signal":"buying_intent"}', 'ai', now(), now()
);

INSERT INTO lead_attributions (
    id, business_id, lead_id, source_conversation_id, source_channel,
    source_interaction_reference, catalog_item_id, offer_id, campaign_reference,
    captured_at
) VALUES (
    '97000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '96000000-0000-0000-0000-000000000001',
    '40000000-0000-0000-0000-000000000001', 'facebook', 'comment-1',
    '93000000-0000-0000-0000-000000000001', '95000000-0000-0000-0000-000000000001',
    'campaign-1', now()
);

INSERT INTO lead_scores (
    id, business_id, lead_id, value, band, factors, rule_version,
    calculated_at, created_at
) VALUES (
    '98000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '96000000-0000-0000-0000-000000000001',
    70, 'medium', '{"high_purchase_intent":true}', 'lead-score-v1', now(), now()
);

INSERT INTO commercial_transactions (
    id, business_id, customer_id, lead_id, transaction_type, state,
    source_conversation_reference_id, currency, total_amount, schema_version,
    created_at, updated_at
) VALUES (
    '99000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000001',
    '96000000-0000-0000-0000-000000000001',
    'order', 'awaiting_confirmation',
    '50000000-0000-0000-0000-000000000001', 'YER', 180000, 1, now(), now()
);

SELECT assert_raises(
    '23503',
    $$INSERT INTO commercial_transactions (
        id, business_id, customer_id, transaction_type, state, schema_version,
        created_at, updated_at
    ) VALUES (
        '99000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '20000000-0000-0000-0000-000000000002',
        'order', 'draft', 1, now(), now()
    )$$
);

INSERT INTO transaction_confirmations (
    id, business_id, transaction_id, status, created_at, updated_at
) VALUES (
    '9a000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '99000000-0000-0000-0000-000000000001',
    'pending', now(), now()
);

INSERT INTO transaction_reviews (
    id, business_id, transaction_id, required, status, reason_codes,
    created_at, updated_at
) VALUES (
    '9b000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '99000000-0000-0000-0000-000000000001',
    true, 'pending', '["high_value_transaction"]', now(), now()
);

INSERT INTO order_lines (
    id, business_id, transaction_id, catalog_item_id, offer_id, variant_id,
    item_name_snapshot, selected_attributes_snapshot, quantity,
    unit_price_snapshot, line_total_snapshot, currency, created_at, updated_at
) VALUES (
    '9c000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '99000000-0000-0000-0000-000000000001',
    '93000000-0000-0000-0000-000000000001',
    '95000000-0000-0000-0000-000000000001',
    '94000000-0000-0000-0000-000000000001',
    'Phone A snapshot', '{"color":"black","storage_gb":256}', 1,
    180000, 180000, 'YER', now(), now()
);

SELECT assert_raises(
    '23503',
    $$INSERT INTO order_lines (
        id, business_id, transaction_id, catalog_item_id, offer_id,
        item_name_snapshot, selected_attributes_snapshot, quantity,
        currency, created_at, updated_at
    ) VALUES (
        '9c000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '99000000-0000-0000-0000-000000000001',
        '93000000-0000-0000-0000-000000000001',
        '95000000-0000-0000-0000-000000000002',
        'Invalid cross-tenant snapshot', '{}', 1, 'YER', now(), now()
    )$$
);

UPDATE catalog_items
SET name = 'Phone A renamed after order', updated_at = now()
WHERE id = '93000000-0000-0000-0000-000000000001';

SELECT assert_true(
    (SELECT item_name_snapshot = 'Phone A snapshot' FROM order_lines
     WHERE id = '9c000000-0000-0000-0000-000000000001'),
    'order line snapshot must remain unchanged after catalog mutation'
);

INSERT INTO audit_events (
    id, business_id, actor_type, actor_reference, action, resource_type,
    resource_id, metadata, correlation_id, occurred_at, created_at
) VALUES (
    '9d000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    'human_agent', 'agent-1', 'transaction_created', 'commercial_transaction',
    '99000000-0000-0000-0000-000000000001', '{"safe":true}',
    'aa000000-0000-0000-0000-000000000001', now(), now()
);

INSERT INTO ai_decisions (
    id, business_id, conversation_id, intent_base, entities,
    evidence_references, requested_action, confidence_value, confidence_band,
    requires_human, missing_information, reason_codes, policy_version,
    schema_version, lifecycle, policy_decision, correlation_id,
    created_at, updated_at
) VALUES (
    '9e000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '40000000-0000-0000-0000-000000000001',
    'purchase_intent', '{"quantity":1}', '["message://m1","offer://95000000-0000-0000-0000-000000000001"]',
    'create_transaction_draft', 0.9200, 'high', true, '[]', '["confirmation_required"]',
    'policy-v1', 1, 'policy_evaluated', 'requires_approval',
    'aa000000-0000-0000-0000-000000000001', now(), now()
);

SELECT assert_raises(
    '23503',
    $$INSERT INTO ai_decisions (
        id, business_id, conversation_id, intent_base, entities,
        evidence_references, requested_action, confidence_band, requires_human,
        missing_information, reason_codes, policy_version, schema_version,
        lifecycle, created_at, updated_at
    ) VALUES (
        '9e000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '40000000-0000-0000-0000-000000000002',
        'purchase_intent', '{}', '[]', 'no_action', 'unknown', false,
        '[]', '[]', 'policy-v1', 1, 'proposed', now(), now()
    )$$
);

DROP FUNCTION assert_raises(text, text);
DROP FUNCTION assert_true(boolean, text);

\echo 'full_schema_constraints=passed'
