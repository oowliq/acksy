-- +goose Up
-- Pending invitations to shared topics. Sharing needs the invitee's consent:
-- only accepting an invitation makes them a member (a recipient).
CREATE TABLE topic_invites (
    topic_id   TEXT    NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    user_id    TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (topic_id, user_id)
);
CREATE INDEX topic_invites_user_idx ON topic_invites (user_id);

-- +goose Down
DROP TABLE topic_invites;
