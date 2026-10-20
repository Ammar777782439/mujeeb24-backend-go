-- Start consent is not transaction confirmation; no product data or payment fields are stored.
CREATE TABLE sales_start_requests (
 id UUID PRIMARY KEY,
 business_id UUID NOT NULL,
 customer_id UUID NOT NULL,
 conversation_id UUID NOT NULL,
 outbound_message_id UUID NOT NULL,
 source_message_id UUID NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','declined','cancelled','expired','superseded')),
 accept_token_hash TEXT NOT NULL UNIQUE,
 decline_token_hash TEXT NOT NULL UNIQUE,
 expires_at TIMESTAMPTZ NOT NULL,
 resolved_at TIMESTAMPTZ,
 decision_message_id UUID,
 workflow_id UUID,
 continuation_state TEXT NOT NULL DEFAULT 'not_required' CHECK (continuation_state IN ('not_required','pending','completed')),
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE (business_id,id),
 UNIQUE (business_id,outbound_message_id),
 FOREIGN KEY (business_id,customer_id) REFERENCES customers(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,conversation_id) REFERENCES conversations(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,outbound_message_id) REFERENCES outbound_messages(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,source_message_id) REFERENCES communication_messages(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,decision_message_id) REFERENCES communication_messages(business_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (business_id,workflow_id) REFERENCES sales_workflows(business_id,id) ON DELETE RESTRICT,
 CHECK (expires_at > created_at),
 CHECK ((status='pending' AND resolved_at IS NULL AND decision_message_id IS NULL)
        OR (status<>'pending' AND resolved_at IS NOT NULL)),
 CHECK (status<>'accepted' OR decision_message_id IS NOT NULL)
);
CREATE UNIQUE INDEX sales_start_one_pending_per_conversation
 ON sales_start_requests(business_id,conversation_id) WHERE status='pending';
CREATE INDEX sales_start_customer_history
 ON sales_start_requests(business_id,customer_id,conversation_id,created_at DESC);
