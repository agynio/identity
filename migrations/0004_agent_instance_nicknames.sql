ALTER TABLE org_nicknames
    ADD COLUMN instance_suffix TEXT;

DROP INDEX IF EXISTS org_nicknames_org_nickname_key;

CREATE UNIQUE INDEX org_nicknames_org_nickname_suffix_key
    ON org_nicknames (organization_id, nickname, COALESCE(instance_suffix, ''));

