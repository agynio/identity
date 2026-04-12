CREATE TABLE org_nicknames (
    org_id          UUID NOT NULL,
    identity_id     UUID NOT NULL REFERENCES identities (identity_id),
    installation_id UUID NULL,
    nickname        TEXT NOT NULL
);

CREATE UNIQUE INDEX org_nicknames_org_nickname_idx
    ON org_nicknames (org_id, nickname);

CREATE UNIQUE INDEX org_nicknames_org_identity_idx
    ON org_nicknames (org_id, identity_id)
    WHERE installation_id IS NULL;

CREATE UNIQUE INDEX org_nicknames_org_installation_idx
    ON org_nicknames (org_id, installation_id)
    WHERE installation_id IS NOT NULL;
