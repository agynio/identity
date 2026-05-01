DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'org_nicknames'
          AND column_name = 'org_id'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'org_nicknames'
          AND column_name = 'organization_id'
    ) THEN
        ALTER TABLE org_nicknames RENAME COLUMN org_id TO organization_id;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS org_nicknames_org_nickname_key
    ON org_nicknames (organization_id, nickname);

CREATE UNIQUE INDEX IF NOT EXISTS org_nicknames_org_identity_key
    ON org_nicknames (organization_id, identity_id)
    WHERE installation_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS org_nicknames_org_installation_key
    ON org_nicknames (organization_id, installation_id)
    WHERE installation_id IS NOT NULL;
