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
    ('00000000-0000-0000-0000-000000000001', 'Business A', 'business-a', 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()),
    ('00000000-0000-0000-0000-000000000002', 'Business B', 'business-b', 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', now(), now());

INSERT INTO channel_connections (
    id, business_id, provider_ref, channel, provider_account_ref,
    provider_connection_ref, status, secret_reference, created_at, updated_at
) VALUES
    ('10000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'socialapi', 'facebook', 'account-a', 'connection-a', 'active', 'secret://test/a', now(), now()),
    ('10000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'socialapi', 'facebook', 'account-b', 'connection-b', 'active', 'secret://test/b', now(), now());

INSERT INTO customers (
    id, business_id, status, created_at, updated_at
) VALUES
    ('20000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000001', 'active', now(), now()),
    ('20000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'active', now(), now());

INSERT INTO external_identities (
    id, business_id, connection_id, customer_id, provider_ref, channel,
    external_account_ref, external_user_id, link_status, first_seen_at,
    last_seen_at, created_at, updated_at
) VALUES (
    '30000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '10000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000001',
    'socialapi', 'facebook', 'account-a', 'external-user-1', 'linked',
    now(), now(), now(), now()
);

SELECT assert_raises(
    '23505',
    $$INSERT INTO external_identities (
        id, business_id, connection_id, customer_id, provider_ref, channel,
        external_account_ref, external_user_id, link_status, first_seen_at,
        last_seen_at, created_at, updated_at
    ) VALUES (
        '30000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000001',
        '20000000-0000-0000-0000-000000000001',
        'socialapi', 'facebook', 'account-a', 'external-user-1', 'linked',
        now(), now(), now(), now()
    )$$
);

SELECT assert_raises(
    '23503',
    $$INSERT INTO external_identities (
        id, business_id, connection_id, customer_id, provider_ref, channel,
        external_account_ref, external_user_id, link_status, first_seen_at,
        last_seen_at, created_at, updated_at
    ) VALUES (
        '30000000-0000-0000-0000-000000000003',
        '00000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000001',
        '20000000-0000-0000-0000-000000000002',
        'socialapi', 'facebook', 'account-a', 'external-user-2', 'linked',
        now(), now(), now(), now()
    )$$
);

INSERT INTO conversations (
    id, business_id, customer_id, state, ownership, ai_mode_override,
    priority, last_activity_at, created_at, updated_at
) VALUES (
    '40000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000001',
    'open', 'none', NULL, 'normal', now(), now(), now()
);

INSERT INTO conversation_references (
    id, business_id, conversation_id, system, provider_ref, resource_type,
    resource_id, connection_id, conversation_kind, is_current, mapping_status,
    created_at, updated_at
) VALUES (
    '50000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '40000000-0000-0000-0000-000000000001',
    'provider', 'socialapi', 'conversation', 'provider-conversation-a',
    '10000000-0000-0000-0000-000000000001', 'dm', true, 'active', now(), now()
);

SELECT assert_raises(
    '23503',
    $$INSERT INTO conversation_references (
        id, business_id, conversation_id, system, provider_ref, resource_type,
        resource_id, connection_id, conversation_kind, is_current, mapping_status,
        created_at, updated_at
    ) VALUES (
        '50000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000002',
        '40000000-0000-0000-0000-000000000001',
        'provider', 'socialapi', 'conversation', 'provider-conversation-cross-tenant',
        '10000000-0000-0000-0000-000000000002', 'dm', true, 'active', now(), now()
    )$$
);

INSERT INTO inbound_event_ledger (
    id, provider_ref, provider_connection_ref, provider_event_id,
    dedupe_strategy, event_type, received_at, raw_payload_reference,
    payload_hash, signature_verified, processing_state, created_at, updated_at
) VALUES (
    '60000000-0000-0000-0000-000000000001', 'socialapi', 'unresolved-connection',
    'provider-event-unresolved-1', 'provider_event_id', 'interaction_received',
    now(), 'payload://test/unresolved-1', 'hash-unresolved-1', true,
    'unresolved', now(), now()
);

SELECT assert_true(
    (SELECT count(*) = 1 FROM inbound_event_ledger
     WHERE provider_event_id = 'provider-event-unresolved-1'
       AND processing_state = 'unresolved'
       AND business_id IS NULL
       AND connection_id IS NULL),
    'unresolved event must be retained without tenant resolution'
);

SELECT assert_raises(
    '23505',
    $$INSERT INTO inbound_event_ledger (
        id, provider_ref, provider_connection_ref, provider_event_id,
        dedupe_strategy, event_type, received_at, raw_payload_reference,
        payload_hash, signature_verified, processing_state, created_at, updated_at
    ) VALUES (
        '60000000-0000-0000-0000-000000000002', 'socialapi', 'unresolved-connection',
        'provider-event-unresolved-1', 'provider_event_id', 'interaction_received',
        now(), 'payload://test/unresolved-duplicate', 'hash-unresolved-duplicate', true,
        'unresolved', now(), now()
    )$$
);

INSERT INTO outbound_messages (
    id, business_id, conversation_id, conversation_reference_id, connection_id,
    provider_ref, channel, origin, direction, transport, content_reference,
    provider_idempotency_key, status, attempt_count, created_at, updated_at
) VALUES (
    '70000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '40000000-0000-0000-0000-000000000001',
    '50000000-0000-0000-0000-000000000001',
    '10000000-0000-0000-0000-000000000001',
    'socialapi', 'facebook', 'human', 'outbound', 'provider',
    'content://test/message-1', 'logical-send-1', 'pending', 0, now(), now()
);

SELECT assert_raises(
    '23505',
    $$INSERT INTO outbound_messages (
        id, business_id, conversation_id, conversation_reference_id, connection_id,
        provider_ref, channel, origin, direction, transport, content_reference,
        provider_idempotency_key, status, attempt_count, created_at, updated_at
    ) VALUES (
        '70000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '40000000-0000-0000-0000-000000000001',
        '50000000-0000-0000-0000-000000000001',
        '10000000-0000-0000-0000-000000000001',
        'socialapi', 'facebook', 'human', 'outbound', 'provider',
        'content://test/message-duplicate', 'logical-send-1', 'pending', 0, now(), now()
    )$$
);

INSERT INTO outbox_entries (
    id, business_id, outbound_message_id, command_type, dedupe_key, status,
    attempt_count, available_at, created_at, updated_at
) VALUES (
    '80000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000001',
    '70000000-0000-0000-0000-000000000001',
    'send_outbound_message', 'logical-send-1', 'pending', 0, now(), now(), now()
);

SELECT assert_raises(
    '23505',
    $$INSERT INTO outbox_entries (
        id, business_id, outbound_message_id, command_type, dedupe_key, status,
        attempt_count, available_at, created_at, updated_at
    ) VALUES (
        '80000000-0000-0000-0000-000000000002',
        '00000000-0000-0000-0000-000000000001',
        '70000000-0000-0000-0000-000000000001',
        'send_outbound_message', 'logical-send-1', 'pending', 0, now(), now(), now()
    )$$
);

SELECT assert_raises(
    '23514',
    $$INSERT INTO outbox_entries (
        id, business_id, outbound_message_id, command_type, dedupe_key, status,
        attempt_count, available_at, lease_owner, lease_expires_at,
        created_at, updated_at
    ) VALUES (
        '80000000-0000-0000-0000-000000000003',
        '00000000-0000-0000-0000-000000000001',
        '70000000-0000-0000-0000-000000000001',
        'send_outbound_message', 'lease-invalid', 'processing', 0, now(),
        NULL, NULL, now(), now()
    )$$
);

UPDATE outbox_entries
SET status = 'processing', lease_owner = 'worker-a',
    lease_token = '81000000-0000-0000-0000-000000000001',
    lease_expires_at = now() + interval '5 minutes', updated_at = now()
WHERE id = '80000000-0000-0000-0000-000000000001';

SELECT assert_true(
    (SELECT status = 'processing' AND lease_owner = 'worker-a' AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL
     FROM outbox_entries WHERE id = '80000000-0000-0000-0000-000000000001'),
    'valid outbox lease must be persisted'
);

SELECT assert_raises(
    '23514',
    $$UPDATE outbox_entries
      SET lease_owner = NULL, lease_expires_at = NULL
      WHERE id = '80000000-0000-0000-0000-000000000001'$$
);

SELECT assert_raises(
    '23503',
    $$DELETE FROM businesses WHERE id = '00000000-0000-0000-0000-000000000001'$$
);

DROP FUNCTION assert_raises(text, text);
DROP FUNCTION assert_true(boolean, text);

\echo 'foundation_constraints=passed'
