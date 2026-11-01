-- Repair businesses created without their one-to-one policy record.
-- Only missing rows are initialized. Existing merchant settings, references,
-- resource versions and timestamps are preserved, including concurrent inserts.
-- Assist mode starts with all automation and sales execution flags disabled.
INSERT INTO business_policies (
    business_id, ai_mode, default_human_review,
    allow_auto_reply, allow_auto_lead_creation, allow_auto_transaction_draft,
    allow_auto_confirmation, sales_workflow_enabled, allow_sales_interactions,
    created_at, updated_at
)
SELECT b.id, 'assist', false, false, false, false, false, false, false,
       CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM businesses b
WHERE NOT EXISTS (
    SELECT 1 FROM business_policies p WHERE p.business_id = b.id
)
ON CONFLICT (business_id) DO NOTHING;
