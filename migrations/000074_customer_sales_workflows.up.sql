-- Customer sales is an independently enabled executor, sharing the existing outbox.
ALTER TABLE business_policies
 ADD COLUMN sales_workflow_enabled BOOLEAN NOT NULL DEFAULT false,
 ADD COLUMN allow_sales_interactions BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE commercial_transactions ADD COLUMN agreement_version BIGINT NOT NULL DEFAULT 1 CHECK (agreement_version>0), ADD COLUMN transaction_data JSONB NOT NULL DEFAULT '{}'::jsonb
 CHECK (jsonb_typeof(transaction_data) = 'object');

CREATE TABLE sales_transaction_requirements (
 business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
 transaction_type TEXT NOT NULL CHECK (transaction_type IN ('order','booking','appointment','service_request','reservation','quote','subscription')),
 schema_version INTEGER NOT NULL CHECK (schema_version > 0),
 definitions JSONB NOT NULL CHECK (jsonb_typeof(definitions) = 'array'),
 PRIMARY KEY (business_id, transaction_type, schema_version)
);

CREATE TABLE sales_workflows (
 id UUID PRIMARY KEY,
 business_id UUID NOT NULL,
 customer_id UUID NOT NULL,
 conversation_id UUID,
 lead_id UUID,
 source_message_id UUID,
 pending_action TEXT,
 transaction_id UUID,
 state TEXT NOT NULL CHECK (state IN ('gathering','draft','awaiting_confirmation','awaiting_review','confirmed','declined','cancelled')),
 proposal JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(proposal) = 'object'),
 policy_version BIGINT NOT NULL,
 resource_version BIGINT NOT NULL DEFAULT 1 CHECK (resource_version > 0),
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE (business_id,id),
 FOREIGN KEY (business_id,customer_id) REFERENCES customers(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,conversation_id) REFERENCES conversations(business_id,id) ON DELETE SET NULL (conversation_id),
 FOREIGN KEY (business_id,source_message_id) REFERENCES communication_messages(business_id,id) ON DELETE SET NULL (source_message_id),
 FOREIGN KEY (business_id,lead_id) REFERENCES leads(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,transaction_id) REFERENCES commercial_transactions(business_id,id) ON DELETE RESTRICT
);
CREATE INDEX sales_workflows_conversation_idx ON sales_workflows(business_id,conversation_id,created_at DESC,id DESC);

CREATE TABLE sales_confirmation_requests (
 id UUID PRIMARY KEY,
 business_id UUID NOT NULL,
 workflow_id UUID NOT NULL,
 customer_id UUID NOT NULL,
 transaction_id UUID NOT NULL,
 transaction_version BIGINT NOT NULL,
 confirm_token_hash TEXT NOT NULL UNIQUE,
 decline_token_hash TEXT NOT NULL UNIQUE,
 status TEXT NOT NULL CHECK (status IN ('pending','accepted','declined','expired','superseded')),
 expires_at TIMESTAMPTZ NOT NULL,
 source_message_id UUID,
 response_message_id UUID,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE (business_id,id),
 FOREIGN KEY (business_id,workflow_id) REFERENCES sales_workflows(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,customer_id) REFERENCES customers(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,transaction_id) REFERENCES commercial_transactions(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,source_message_id) REFERENCES communication_messages(business_id,id) ON DELETE SET NULL (source_message_id),
 FOREIGN KEY (business_id,response_message_id) REFERENCES communication_messages(business_id,id) ON DELETE SET NULL (response_message_id)
);
CREATE UNIQUE INDEX sales_confirmation_pending_idx ON sales_confirmation_requests(business_id,workflow_id) WHERE status='pending';

CREATE TABLE sales_command_results (
 business_id UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
 operation TEXT NOT NULL,
 idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 250),
 request_hash TEXT NOT NULL,
 result JSONB NOT NULL CHECK (jsonb_typeof(result) = 'object'),
 created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY (business_id,operation,idempotency_key)
);

-- Button events survive process shutdown; enqueue is atomic with inbound materialization.
CREATE TABLE sales_action_jobs (
 id UUID PRIMARY KEY,
 business_id UUID NOT NULL,
 customer_id UUID NOT NULL,
 conversation_id UUID NOT NULL,
 message_id UUID NOT NULL,
 payload TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','processing','completed','dead_letter')),
 lease_token UUID,
 lease_until TIMESTAMPTZ,
 attempts INTEGER NOT NULL DEFAULT 0,
 error_code TEXT,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE(business_id,message_id),
 FOREIGN KEY(business_id,message_id) REFERENCES communication_messages(business_id,id) ON DELETE CASCADE,
 FOREIGN KEY(business_id,conversation_id) REFERENCES conversations(business_id,id) ON DELETE CASCADE,
 FOREIGN KEY(business_id,customer_id) REFERENCES customers(business_id,id) ON DELETE RESTRICT
);
CREATE INDEX sales_action_jobs_claim_idx ON sales_action_jobs(state,lease_until,created_at);

-- Capability is authoritative; legacy action may be empty for the new executor.
ALTER TABLE ai_decisions DROP CONSTRAINT ai_decisions_action_chk;
ALTER TABLE ai_decisions ADD CONSTRAINT ai_decisions_action_chk CHECK (
 requested_action IN ('answer','clarification','human_request','lead_draft','order_draft')
 OR (requested_capability IS NOT NULL AND requested_action = '')
);
ALTER TABLE ai_decisions DROP CONSTRAINT ai_decisions_effective_action_chk;
ALTER TABLE ai_decisions ADD CONSTRAINT ai_decisions_effective_action_chk CHECK (
 effective_action IS NULL OR effective_action IN ('answer','clarification','human_request','lead_draft','order_draft','no_action','blocked')
 OR (effective_capability IS NOT NULL AND effective_action = '')
);
