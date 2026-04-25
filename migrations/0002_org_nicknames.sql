CREATE TABLE org_nicknames (
    organization_id UUID        NOT NULL,
    identity_id     UUID        NOT NULL REFERENCES identities(identity_id) ON DELETE CASCADE,
    installation_id UUID,
    nickname        TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX org_nicknames_org_nickname_key
    ON org_nicknames (organization_id, nickname);

CREATE UNIQUE INDEX org_nicknames_org_identity_key
    ON org_nicknames (organization_id, identity_id)
    WHERE installation_id IS NULL;

CREATE UNIQUE INDEX org_nicknames_org_installation_key
    ON org_nicknames (organization_id, installation_id)
    WHERE installation_id IS NOT NULL;
