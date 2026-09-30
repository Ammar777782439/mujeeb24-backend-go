-- E2E Catalog Fixture — deterministic seed for Gemini Catalog E2E test.
-- Build tag: gemini_e2e. Loaded by /internal/application/services/e2e_gemini_catalog_test.go.
-- Designed to match the EXACT migrations as of HEAD 107e892 (no schema invention).
--
-- Two businesses with two distinct catalogs. Business A carries the product the
-- test asks about ("Mujeeb CI Test Phone", 125000 YER, available). Business B
-- carries a different product at a different price/currency so the test can
-- assert tenant isolation (Business A's Gemini reply must NOT contain B's
-- facts, and vice-versa).
--
-- Deterministic UUIDs (low-collision well-known test identifiers) so the test
-- can resolve rows by ID without needing to query-and-discover.

-- ===== BUSINESS A: "Mujeeb CI Test Store" (the primary subject of the E2E) =====
INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111111',
    'Mujeeb CI Test Store',
    'mujeeb-ci-test-store',
    'active',
    'retail',
    'Asia/Aden',
    'YER',
    'ar',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    status = EXCLUDED.status,
    vertical_type = EXCLUDED.vertical_type,
    timezone = EXCLUDED.timezone,
    default_currency = EXCLUDED.default_currency,
    locale = EXCLUDED.locale,
    updated_at = now();

-- Customer A
INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111112',
    '11111111-1111-1111-1111-111111111111',
    '{"name":"Mujeeb CI Test Customer A"}'::jsonb,
    '[{"type":"whatsapp","value":"+967700000001"}]'::jsonb,
    'ar',
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    profile = EXCLUDED.profile,
    contact_points = EXCLUDED.contact_points,
    locale_preference = EXCLUDED.locale_preference,
    status = EXCLUDED.status,
    updated_at = now();

-- Channel Connection A (WhatsApp)
INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111113',
    '11111111-1111-1111-1111-111111111111',
    'whatsapp_test_provider_a',
    'whatsapp',
    'whatsapp_business_account_a',
    'wa_conn_a_test',
    'active',
    'ci-test-secret-ref-a',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    updated_at = now();

-- Conversation A (ai_handling ownership=ai)
INSERT INTO conversations (id, business_id, customer_id, state, ownership, ai_mode_override, priority, last_activity_at, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111115',
    '11111111-1111-1111-1111-111111111111',
    '11111111-1111-1111-1111-111111111112',
    'ai_handling',
    'ai',
    'allowed',
    'normal',
    now(),
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    state = EXCLUDED.state,
    ownership = EXCLUDED.ownership,
    ai_mode_override = EXCLUDED.ai_mode_override,
    priority = EXCLUDED.priority,
    last_activity_at = now(),
    updated_at = now();

-- Conversation Reference A (provider system, linked to Connection A)
INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111114',
    '11111111-1111-1111-1111-111111111111',
    '11111111-1111-1111-1111-111111111115',
    'provider',
    'wa_conv_ref_a_test',
    'conversation',
    'wa_resource_a_test',
    '11111111-1111-1111-1111-111111111113',
    'dm',
    true,
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    provider_ref = EXCLUDED.provider_ref,
    resource_id = EXCLUDED.resource_id,
    connection_id = EXCLUDED.connection_id,
    is_current = EXCLUDED.is_current,
    mapping_status = EXCLUDED.mapping_status,
    updated_at = now();

-- Catalog A
INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111116',
    '11111111-1111-1111-1111-111111111111',
    'Mujeeb CI Test Catalog A',
    'Deterministic CI catalog for E2E Business A',
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    updated_at = now();

-- Catalog Item A: "Mujeeb CI Test Phone" — the product the customer asks about.
INSERT INTO catalog_items (id, business_id, catalog_id, attribute_schema_id, attribute_schema_version, item_type, name, short_description, long_description, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111117',
    '11111111-1111-1111-1111-111111111111',
    '11111111-1111-1111-1111-111111111116',
    NULL, NULL,
    'product',
    'Mujeeb CI Test Phone',
    'A test product used only by the Gemini Catalog E2E test.',
    'A test product used only by the Gemini Catalog E2E test. Deterministic fixture so Gemini must ground its answer on this exact row.',
    'active',
    'fixed',
    'always_available',
    'delivery',
    false,
    '{}'::jsonb,
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    short_description = EXCLUDED.short_description,
    long_description = EXCLUDED.long_description,
    status = EXCLUDED.status,
    pricing_mode = EXCLUDED.pricing_mode,
    availability_mode = EXCLUDED.availability_mode,
    fulfillment_mode = EXCLUDED.fulfillment_mode,
    requires_confirmation = EXCLUDED.requires_confirmation,
    updated_at = now();

-- Offer A: 125000 YER, available, fixed price.
INSERT INTO offers (id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency, pricing_unit, price_source, price_verification_status, price_checked_at, availability_mode, availability_status, availability_source, availability_checked_at, availability_valid_until, availability_evidence_ref, fulfillment_mode, validity_from, validity_until, status, created_at, updated_at)
VALUES (
    '11111111-1111-1111-1111-111111111118',
    '11111111-1111-1111-1111-111111111111',
    '11111111-1111-1111-1111-111111111117',
    NULL,
    'Mujeeb CI Test Phone — Standard Offer',
    'fixed',
    125000,
    'YER',
    NULL,
    'fixture',
    'verified',
    now(),
    'always_available',
    'available',
    'fixture',
    now(),
    NULL,
    NULL,
    'delivery',
    NULL,
    NULL,
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    pricing_mode = EXCLUDED.pricing_mode,
    amount = EXCLUDED.amount,
    currency = EXCLUDED.currency,
    availability_mode = EXCLUDED.availability_mode,
    availability_status = EXCLUDED.availability_status,
    status = EXCLUDED.status,
    updated_at = now();


-- ===== BUSINESS B: cross-tenant sentinel product (different facts) =====
INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222222',
    'Mujeeb CI Tenant B',
    'mujeeb-ci-tenant-b',
    'active',
    'retail',
    'Asia/Aden',
    'SAR',   -- different currency from Business A
    'ar',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    status = EXCLUDED.status,
    vertical_type = EXCLUDED.vertical_type,
    timezone = EXCLUDED.timezone,
    default_currency = EXCLUDED.default_currency,
    locale = EXCLUDED.locale,
    updated_at = now();

-- Customer B
INSERT INTO customers (id, business_id, profile, contact_points, locale_preference, status, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222223',
    '22222222-2222-2222-2222-222222222222',
    '{"name":"Mujeeb CI Test Customer B"}'::jsonb,
    '[{"type":"whatsapp","value":"+967700000002"}]'::jsonb,
    'ar',
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    profile = EXCLUDED.profile,
    contact_points = EXCLUDED.contact_points,
    locale_preference = EXCLUDED.locale_preference,
    status = EXCLUDED.status,
    updated_at = now();

-- Channel Connection B
INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222224',
    '22222222-2222-2222-2222-222222222222',
    'whatsapp_test_provider_b',
    'whatsapp',
    'whatsapp_business_account_b',
    'wa_conn_b_test',
    'active',
    'ci-test-secret-ref-b',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    updated_at = now();

-- Conversation B
INSERT INTO conversations (id, business_id, customer_id, state, ownership, ai_mode_override, priority, last_activity_at, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222226',
    '22222222-2222-2222-2222-222222222222',
    '22222222-2222-2222-2222-222222222223',
    'ai_handling',
    'ai',
    'allowed',
    'normal',
    now(),
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    state = EXCLUDED.state,
    ownership = EXCLUDED.ownership,
    ai_mode_override = EXCLUDED.ai_mode_override,
    priority = EXCLUDED.priority,
    last_activity_at = now(),
    updated_at = now();

-- Conversation Reference B
INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222225',
    '22222222-2222-2222-2222-222222222222',
    '22222222-2222-2222-2222-222222222226',
    'provider',
    'wa_conv_ref_b_test',
    'conversation',
    'wa_resource_b_test',
    '22222222-2222-2222-2222-222222222224',
    'dm',
    true,
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    provider_ref = EXCLUDED.provider_ref,
    resource_id = EXCLUDED.resource_id,
    connection_id = EXCLUDED.connection_id,
    is_current = EXCLUDED.is_current,
    mapping_status = EXCLUDED.mapping_status,
    updated_at = now();

-- Catalog B
INSERT INTO catalogs (id, business_id, name, description, status, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222227',
    '22222222-2222-2222-2222-222222222222',
    'Mujeeb CI Test Catalog B',
    'Deterministic CI catalog for E2E Business B (tenant isolation sentinel)',
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    updated_at = now();

-- Catalog Item B: same display name "Mujeeb CI Test Phone" BUT different facts (price 999 SAR / unavailable).
-- This is the cross-tenant sentinel: Gemini serving Business A must NEVER mention 999 or SAR,
-- and Gemini serving Business B should produce different facts.
INSERT INTO catalog_items (id, business_id, catalog_id, attribute_schema_id, attribute_schema_version, item_type, name, short_description, long_description, status, pricing_mode, availability_mode, fulfillment_mode, requires_confirmation, attributes, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222228',
    '22222222-2222-2222-2222-222222222222',
    '22222222-2222-2222-2222-222222222227',
    NULL, NULL,
    'product',
    'Mujeeb CI Test Phone',   -- SAME name as Business A on purpose (tenant-isolation trap)
    'A different test product belonging to Business B.',
    'A different test product belonging to Business B. The price/currency/availability differ from Business A. If Business A''s reply contains these facts, tenant isolation is broken.',
    'active',
    'fixed',
    'supplier_check',
    'pickup',
    false,
    '{}'::jsonb,
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    short_description = EXCLUDED.short_description,
    long_description = EXCLUDED.long_description,
    status = EXCLUDED.status,
    pricing_mode = EXCLUDED.pricing_mode,
    availability_mode = EXCLUDED.availability_mode,
    fulfillment_mode = EXCLUDED.fulfillment_mode,
    requires_confirmation = EXCLUDED.requires_confirmation,
    updated_at = now();

-- Offer B: 999 SAR, unavailable. Different facts to make tenant-isolation violation observable.
INSERT INTO offers (id, business_id, catalog_item_id, variant_id, name, pricing_mode, amount, currency, pricing_unit, price_source, price_verification_status, price_checked_at, availability_mode, availability_status, availability_source, availability_checked_at, availability_valid_until, availability_evidence_ref, fulfillment_mode, validity_from, validity_until, status, created_at, updated_at)
VALUES (
    '22222222-2222-2222-2222-222222222229',
    '22222222-2222-2222-2222-222222222222',
    '22222222-2222-2222-2222-222222222228',
    NULL,
    'Mujeeb CI Test Phone — Tenant B Offer',
    'fixed',
    999,
    'SAR',
    NULL,
    'fixture',
    'verified',
    now(),
    'supplier_check',
    'unavailable',
    'fixture',
    now(),
    NULL,
    NULL,
    'pickup',
    NULL,
    NULL,
    'active',
    now(),
    now()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    pricing_mode = EXCLUDED.pricing_mode,
    amount = EXCLUDED.amount,
    currency = EXCLUDED.currency,
    availability_mode = EXCLUDED.availability_mode,
    availability_status = EXCLUDED.availability_status,
    status = EXCLUDED.status,
    updated_at = now();
