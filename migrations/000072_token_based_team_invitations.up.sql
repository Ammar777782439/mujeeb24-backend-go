ALTER TABLE team_invitations
    ADD COLUMN display_name TEXT;

UPDATE team_invitations
SET display_name = split_part(email, '@', 1)
WHERE display_name IS NULL;

ALTER TABLE team_invitations
    ALTER COLUMN display_name SET NOT NULL,
    ADD CONSTRAINT team_invitations_display_name_not_blank_chk
        CHECK (length(btrim(display_name)) > 0),
    ADD CONSTRAINT team_invitations_non_owner_role_chk
        CHECK (role <> 'owner');

UPDATE team_invitations
SET status = 'expired',
    updated_at = now()
WHERE status = 'pending'
  AND expires_at <= now();

UPDATE business_memberships
SET permissions = CASE role
    WHEN 'owner' THEN '["*"]'::jsonb
    WHEN 'admin' THEN '["audit:read","automation:manage","automation:read","business:read","business:settings","catalog:read","catalog:write","channels:manage","channels:read","conversations:assign","conversations:read","conversations:reply","customers:read","customers:write","dashboard:read","merchant_ai:use","sales:approve","sales:read","sales:write","team:manage","team:read"]'::jsonb
    WHEN 'manager' THEN '["automation:manage","automation:read","business:read","catalog:read","catalog:write","channels:read","conversations:assign","conversations:read","conversations:reply","customers:read","customers:write","dashboard:read","merchant_ai:use","sales:read","sales:write","team:read"]'::jsonb
    WHEN 'agent' THEN '["business:read","catalog:read","conversations:assign","conversations:read","conversations:reply","customers:read","customers:write","dashboard:read","merchant_ai:use","sales:read","sales:write"]'::jsonb
    WHEN 'analyst' THEN '["audit:read","business:read","catalog:read","customers:read","dashboard:read","sales:read"]'::jsonb
    WHEN 'viewer' THEN '["business:read","catalog:read","conversations:read","customers:read","dashboard:read","sales:read"]'::jsonb
    ELSE '[]'::jsonb
END,
updated_at = now()
WHERE permissions = '[]'::jsonb;
