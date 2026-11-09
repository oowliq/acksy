-- +goose Up
-- External (OIDC) identities linked to local users. A user may have several;
-- an identity (issuer + subject) belongs to exactly one user.
CREATE TABLE user_identities (
    issuer        TEXT   NOT NULL,
    subject       TEXT   NOT NULL,
    user_id       TEXT   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at    INTEGER NOT NULL,
    last_login_at INTEGER,
    PRIMARY KEY (issuer, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

-- +goose Down
DROP TABLE user_identities;
