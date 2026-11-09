package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type inviteRow struct {
	TopicID       string `db:"topic_id"`
	TopicName     string `db:"topic_name"`
	TopicTitle    string `db:"topic_title"`
	OwnerUsername string `db:"owner_username"`
	UserID        string `db:"user_id"`
	Username      string `db:"username"`
	CreatedAt     int64  `db:"created_at"`
}

func (r inviteRow) toDomain() domain.TopicInvite {
	return domain.TopicInvite{
		TopicID: r.TopicID, TopicName: r.TopicName, TopicTitle: r.TopicTitle, OwnerUsername: r.OwnerUsername,
		UserID: r.UserID, Username: r.Username, CreatedAt: fromMs(r.CreatedAt),
	}
}

const inviteSelect = `SELECT i.topic_id, t.name AS topic_name, t.title AS topic_title, o.username AS owner_username,
	i.user_id, u.username, i.created_at
	FROM topic_invites i
	JOIN topics t ON t.id = i.topic_id
	JOIN users o ON o.id = t.owner_id
	JOIN users u ON u.id = i.user_id`

func (q queries) listInvites(ctx context.Context, where string, args ...any) ([]domain.TopicInvite, error) {
	var rows []inviteRow
	if err := q.sel(ctx, &rows, inviteSelect+` WHERE `+where+` ORDER BY i.created_at, i.topic_id, i.user_id`, args...); err != nil {
		return nil, err
	}
	out := make([]domain.TopicInvite, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

func (q queries) CreateTopicInvite(ctx context.Context, topicID, userID string, at time.Time) error {
	_, err := q.exec(ctx, `INSERT INTO topic_invites (topic_id, user_id, created_at) VALUES (?, ?, ?)`,
		topicID, userID, ms(at))
	if isUniqueViolation(err) {
		return domain.Conflict("user is already invited to this topic")
	}
	return err
}

func (q queries) GetTopicInvite(ctx context.Context, topicID, userID string) (*domain.TopicInvite, error) {
	var r inviteRow
	if err := q.get(ctx, &r, inviteSelect+` WHERE i.topic_id = ? AND i.user_id = ?`, topicID, userID); err != nil {
		return nil, notFound(err, "invitation")
	}
	inv := r.toDomain()
	return &inv, nil
}

func (q queries) ListTopicInvites(ctx context.Context, topicID string) ([]domain.TopicInvite, error) {
	return q.listInvites(ctx, "i.topic_id = ?", topicID)
}

func (q queries) ListUserInvites(ctx context.Context, userID string) ([]domain.TopicInvite, error) {
	return q.listInvites(ctx, "i.user_id = ?", userID)
}

func (q queries) DeleteTopicInvite(ctx context.Context, topicID, userID string) error {
	return q.execOne(ctx, "invitation", `DELETE FROM topic_invites WHERE topic_id = ? AND user_id = ?`, topicID, userID)
}
