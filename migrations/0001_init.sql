CREATE TABLE identities (
    identity_id   UUID        PRIMARY KEY,
    identity_type SMALLINT    NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
