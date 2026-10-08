CREATE TABLE merchant_ai_proposals (
 business_id uuid NOT NULL,
 id uuid NOT NULL,
 principal_id uuid NOT NULL REFERENCES principals(id),
 session_id uuid NOT NULL,
 catalog_id uuid NOT NULL,
 payload jsonb NOT NULL,
 result jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 executed_at timestamptz,
 PRIMARY KEY (business_id,id),
 FOREIGN KEY (business_id,session_id) REFERENCES merchant_ai_sessions(business_id,id),
 FOREIGN KEY (business_id,catalog_id) REFERENCES catalogs(business_id,id),
 CHECK ((result IS NULL) = (executed_at IS NULL))
);
CREATE INDEX merchant_ai_proposals_session_idx ON merchant_ai_proposals(business_id,session_id,created_at DESC);
