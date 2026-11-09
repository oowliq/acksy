package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type tokenRow struct {
	ID         string `db:"id"`
	UserID     string `db:"user_id"`
	Name       string `db:"name"`
	Prefix     string `db:"prefix"`
	SecretHash string `db:"secret_hash"`
	Scopes     string `db:"scopes"`
	ExpiresAt  *int64 `db:"expires_at"`
	LastUsedAt *int64 `db:"last_used_at"`
	CreatedAt  int64  `db:"created_at"`
}

const tokenCols = `id, user_id, name, prefix, secret_hash, scopes, expires_at, last_used_at, created_at` //nolint:gosec // column list, not a credential

func (r tokenRow) toDomain() domain.APIToken {
	return domain.APIToken{
		ID: r.ID, UserID: r.UserID, Name: r.Name, Prefix: r.Prefix, SecretHash: r.SecretHash,
		Scopes:     domain.ScopesFromString(r.Scopes),
		TopicIDs:   []string{},
		ExpiresAt:  fromMsPtr(r.ExpiresAt),
		LastUsedAt: fromMsPtr(r.LastUsedAt),
		CreatedAt:  fromMs(r.CreatedAt),
	}
}

func (q queries) CreateToken(ctx context.Context, t *domain.APIToken) error {
	_, err := q.exec(ctx, `INSERT INTO api_tokens (`+tokenCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.Name, t.Prefix, t.SecretHash, t.Scopes.String(),
		msPtr(t.ExpiresAt), msPtr(t.LastUsedAt), ms(t.CreatedAt))
	if err != nil {
		return err
	}
	for _, topicID := range t.TopicIDs {
		if _, err := q.exec(ctx, `INSERT INTO api_token_topics (token_id, topic_id) VALUES (?, ?)`, t.ID, topicID); err != nil {
			return err
		}
	}
	return nil
}

func (q queries) tokenTopics(ctx context.Context, tokenID string) ([]string, error) {
	ids := []string{}
	err := q.sel(ctx, &ids, `SELECT topic_id FROM api_token_topics WHERE token_id = ? ORDER BY topic_id`, tokenID)
	return ids, err
}

func (q queries) GetTokenByHash(ctx context.Context, hash string) (*domain.APIToken, error) {
	var r tokenRow
	if err := q.get(ctx, &r, `SELECT `+tokenCols+` FROM api_tokens WHERE secret_hash = ?`, hash); err != nil {
		return nil, notFound(err, "token")
	}
	t := r.toDomain()
	var err error
	if t.TopicIDs, err = q.tokenTopics(ctx, t.ID); err != nil {
		return nil, err
	}
	return &t, nil
}

func (q queries) ListTokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	var rows []tokenRow
	if err := q.sel(ctx, &rows, `SELECT `+tokenCols+` FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID); err != nil {
		return nil, err
	}
	out := make([]domain.APIToken, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
		var err error
		if out[i].TopicIDs, err = q.tokenTopics(ctx, r.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (q queries) DeleteToken(ctx context.Context, userID, id string) error {
	return q.execOne(ctx, "token", `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
}

func (q queries) TouchToken(ctx context.Context, id string, at time.Time) error {
	_, err := q.exec(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, ms(at), id)
	return err
}

// Topics.

type topicRow struct {
	ID              string  `db:"id"`
	OwnerID         string  `db:"owner_id"`
	OwnerUsername   string  `db:"owner_username"`
	Name            string  `db:"name"`
	Title           string  `db:"title"`
	Description     string  `db:"description"`
	DefaultPriority *int    `db:"default_priority"`
	RetentionDays   *int    `db:"retention_days"`
	MaxCount        *int    `db:"max_notifications"`
	Role            *string `db:"role"`
	MemberCount     int     `db:"member_count"`
	CreatedAt       int64   `db:"created_at"`
	UpdatedAt       int64   `db:"updated_at"`
}

func (r topicRow) toDomain() domain.Topic {
	return domain.Topic{
		ID: r.ID, OwnerID: r.OwnerID, OwnerUsername: r.OwnerUsername, Name: r.Name,
		Title: r.Title, Description: r.Description, DefaultPriority: r.DefaultPriority,
		RetentionDays: r.RetentionDays, MaxNotifications: r.MaxCount,
		Role: domain.MemberRole(deref(r.Role)), MemberCount: r.MemberCount,
		CreatedAt: fromMs(r.CreatedAt), UpdatedAt: fromMs(r.UpdatedAt),
	}
}

// topicSelect selects topics with owner username and member count. The
// caller's role is NULL unless the query joins a membership as "m".
const topicSelect = `SELECT t.id, t.owner_id, u.username AS owner_username, t.name, t.title, t.description,
	t.default_priority, t.retention_days, t.max_notifications, t.created_at, t.updated_at,
	(SELECT COUNT(*) FROM topic_members c WHERE c.topic_id = t.id) AS member_count`

func topicsToDomain(rows []topicRow) []domain.Topic {
	out := make([]domain.Topic, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out
}

func (q queries) CreateTopic(ctx context.Context, t *domain.Topic) error {
	_, err := q.exec(ctx, `INSERT INTO topics (id, owner_id, name, title, description, default_priority,
		retention_days, max_notifications, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.OwnerID, t.Name, t.Title, t.Description, t.DefaultPriority, t.RetentionDays, t.MaxNotifications,
		ms(t.CreatedAt), ms(t.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Conflict("topic %q already exists", t.Name)
	}
	if err != nil {
		return err
	}
	return q.AddTopicMember(ctx, t.ID, t.OwnerID, domain.MemberOwner, t.CreatedAt)
}

func (q queries) getTopic(ctx context.Context, where string, args ...any) (*domain.Topic, error) {
	var r topicRow
	err := q.get(ctx, &r, topicSelect+`, NULL AS role FROM topics t JOIN users u ON u.id = t.owner_id WHERE `+where, args...)
	if err != nil {
		return nil, notFound(err, "topic")
	}
	t := r.toDomain()
	return &t, nil
}

func (q queries) GetTopic(ctx context.Context, id string) (*domain.Topic, error) {
	return q.getTopic(ctx, "t.id = ?", id)
}

func (q queries) GetTopicByOwnerAndName(ctx context.Context, ownerID, name string) (*domain.Topic, error) {
	return q.getTopic(ctx, "t.owner_id = ? AND t.name = ?", ownerID, name)
}

func (q queries) FindMemberTopicsByName(ctx context.Context, userID, name string) ([]domain.Topic, error) {
	var rows []topicRow
	err := q.sel(ctx, &rows, topicSelect+`, m.role AS role FROM topics t
		JOIN users u ON u.id = t.owner_id
		JOIN topic_members m ON m.topic_id = t.id AND m.user_id = ?
		WHERE t.name = ? ORDER BY t.id`, userID, name)
	return topicsToDomain(rows), err
}

func (q queries) ListTopicsForUser(ctx context.Context, userID string) ([]domain.Topic, error) {
	var rows []topicRow
	err := q.sel(ctx, &rows, topicSelect+`, m.role AS role FROM topics t
		JOIN users u ON u.id = t.owner_id
		JOIN topic_members m ON m.topic_id = t.id AND m.user_id = ?
		ORDER BY t.name, u.username`, userID)
	return topicsToDomain(rows), err
}

func (q queries) UpdateTopic(ctx context.Context, t *domain.Topic) error {
	return q.execOne(ctx, "topic", `UPDATE topics SET title = ?, description = ?, default_priority = ?,
		retention_days = ?, max_notifications = ?, updated_at = ? WHERE id = ?`,
		t.Title, t.Description, t.DefaultPriority, t.RetentionDays, t.MaxNotifications, ms(t.UpdatedAt), t.ID)
}

func (q queries) DeleteTopic(ctx context.Context, id string) error {
	// The topic's notifications become topic-less (ON DELETE SET NULL). Their
	// dedup keys belonged to the topic: drop them, so they neither collide
	// with nor get overwritten by the recipient's own topic-less keys.
	if _, err := q.exec(ctx, `UPDATE notifications SET dedup_key = NULL WHERE topic_id = ? AND dedup_key IS NOT NULL`, id); err != nil {
		return err
	}
	return q.execOne(ctx, "topic", `DELETE FROM topics WHERE id = ?`, id)
}

func (q queries) GetMemberRole(ctx context.Context, topicID, userID string) (domain.MemberRole, error) {
	var role string
	err := q.get(ctx, &role, `SELECT role FROM topic_members WHERE topic_id = ? AND user_id = ?`, topicID, userID)
	if err != nil {
		return "", notFound(err, "topic")
	}
	return domain.MemberRole(role), nil
}

func (q queries) AddTopicMember(ctx context.Context, topicID, userID string, role domain.MemberRole, at time.Time) error {
	_, err := q.exec(ctx, `INSERT INTO topic_members (topic_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		topicID, userID, string(role), ms(at))
	if isUniqueViolation(err) {
		return domain.Conflict("user is already a member of this topic")
	}
	return err
}

func (q queries) RemoveTopicMember(ctx context.Context, topicID, userID string) error {
	return q.execOne(ctx, "member", `DELETE FROM topic_members WHERE topic_id = ? AND user_id = ?`, topicID, userID)
}

func (q queries) ListTopicMembers(ctx context.Context, topicID string) ([]domain.TopicMember, error) {
	var rows []struct {
		UserID    string `db:"user_id"`
		Username  string `db:"username"`
		Role      string `db:"role"`
		CreatedAt int64  `db:"created_at"`
	}
	err := q.sel(ctx, &rows, `SELECT m.user_id, u.username, m.role, m.created_at FROM topic_members m
		JOIN users u ON u.id = m.user_id WHERE m.topic_id = ? ORDER BY m.role DESC, u.username`, topicID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TopicMember, len(rows))
	for i, r := range rows {
		out[i] = domain.TopicMember{UserID: r.UserID, Username: r.Username, Role: domain.MemberRole(r.Role), CreatedAt: fromMs(r.CreatedAt)}
	}
	return out, nil
}

func (q queries) ListRecipientIDs(ctx context.Context, topicID string) ([]string, error) {
	ids := []string{}
	err := q.sel(ctx, &ids, `SELECT m.user_id FROM topic_members m JOIN users u ON u.id = m.user_id
		WHERE m.topic_id = ? AND u.disabled_at IS NULL ORDER BY m.user_id`, topicID)
	return ids, err
}
