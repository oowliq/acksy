package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type notificationRow struct {
	ID           string  `db:"id"`
	UserID       string  `db:"user_id"`
	TopicID      *string `db:"topic_id"`
	TopicColor   *string `db:"topic_color"`
	TopicName    *string `db:"topic_name"`
	MessageID    string  `db:"message_id"`
	Title        string  `db:"title"`
	Body         string  `db:"body"`
	Priority     int     `db:"priority"`
	Tags         string  `db:"tags"`
	ClickURL     string  `db:"click_url"`
	Actions      string  `db:"actions"`
	GroupKey     *string `db:"group_key"`
	DedupKey     *string `db:"dedup_key"`
	RepeatCount  int     `db:"repeat_count"`
	Source       string  `db:"source"`
	TokenID      *string `db:"token_id"`
	CreatedAt    int64   `db:"created_at"`
	UpdatedAt    int64   `db:"updated_at"`
	ExpiresAt    *int64  `db:"expires_at"`
	ReadAt       *int64  `db:"read_at"`
	ArchivedAt   *int64  `db:"archived_at"`
	SnoozedUntil *int64  `db:"snoozed_until"`
}

const notificationSelect = `SELECT n.id, n.user_id, n.topic_id, t.color AS topic_color, t.name AS topic_name, n.message_id, n.title, n.body,
	n.priority, n.tags, n.click_url, n.actions, n.group_key, n.dedup_key, n.repeat_count, n.source, n.token_id,
	n.created_at, n.updated_at, n.expires_at, n.read_at, n.archived_at, n.snoozed_until
	FROM notifications n LEFT JOIN topics t ON t.id = n.topic_id`

func (r notificationRow) toDomain() (domain.Notification, error) {
	n := domain.Notification{
		ID: r.ID, UserID: r.UserID, MessageID: r.MessageID, Title: r.Title, Body: r.Body,
		Priority: r.Priority, ClickURL: r.ClickURL, GroupKey: deref(r.GroupKey), DedupKey: deref(r.DedupKey),
		RepeatCount: r.RepeatCount, Source: r.Source, TokenID: deref(r.TokenID),
		CreatedAt: fromMs(r.CreatedAt), UpdatedAt: fromMs(r.UpdatedAt), ExpiresAt: fromMsPtr(r.ExpiresAt),
		ReadAt: fromMsPtr(r.ReadAt), ArchivedAt: fromMsPtr(r.ArchivedAt), SnoozedUntil: fromMsPtr(r.SnoozedUntil),
	}
	if r.TopicID != nil {
		n.Topic = &domain.TopicSummary{ID: *r.TopicID, Name: deref(r.TopicName), Color: deref(r.TopicColor)}
	}
	if err := json.Unmarshal([]byte(r.Tags), &n.Tags); err != nil {
		return n, fmt.Errorf("decode tags of %s: %w", r.ID, err)
	}
	if err := json.Unmarshal([]byte(r.Actions), &n.Actions); err != nil {
		return n, fmt.Errorf("decode actions of %s: %w", r.ID, err)
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	if n.Actions == nil {
		n.Actions = []domain.Action{}
	}
	return n, nil
}

func (q queries) UpsertNotification(ctx context.Context, n *domain.Notification) (bool, error) {
	tags, err := json.Marshal(nonNil(n.Tags))
	if err != nil {
		return false, err
	}
	actions, err := domain.ActionsForStorage(nonNil(n.Actions)) // keeps http secrets
	if err != nil {
		return false, err
	}
	var topicID *string
	if n.Topic != nil {
		topicID = &n.Topic.ID
	}

	var res struct {
		ID          string `db:"id"`
		RepeatCount int    `db:"repeat_count"`
	}
	err = q.get(ctx, &res, `INSERT INTO notifications (id, user_id, topic_id, message_id, title, body, priority, tags,
			click_url, actions, group_key, dedup_key, repeat_count, source, token_id, created_at, updated_at, expires_at,
			search_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, (COALESCE(topic_id, '')), dedup_key) WHERE dedup_key IS NOT NULL DO UPDATE SET
			message_id = excluded.message_id, title = excluded.title,
			body = excluded.body, priority = excluded.priority, tags = excluded.tags,
			search_text = excluded.search_text,
			click_url = excluded.click_url, actions = excluded.actions, group_key = excluded.group_key,
			source = excluded.source, token_id = excluded.token_id, updated_at = excluded.updated_at,
			expires_at = excluded.expires_at, repeat_count = notifications.repeat_count + 1,
			read_at = NULL, archived_at = NULL, snoozed_until = NULL
		RETURNING id, repeat_count`,
		n.ID, n.UserID, topicID, n.MessageID, n.Title, n.Body, n.Priority, string(tags),
		n.ClickURL, string(actions), strPtr(n.GroupKey), strPtr(n.DedupKey), n.Source, strPtr(n.TokenID),
		ms(n.CreatedAt), ms(n.UpdatedAt), msPtr(n.ExpiresAt), domain.SearchText(n.Title, n.Body, n.Tags))
	if err != nil {
		return false, err
	}
	n.ID = res.ID
	n.RepeatCount = res.RepeatCount
	return res.RepeatCount > 1, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (q queries) GetNotification(ctx context.Context, userID, id string) (*domain.Notification, error) {
	var r notificationRow
	if err := q.get(ctx, &r, notificationSelect+` WHERE n.user_id = ? AND n.id = ?`, userID, id); err != nil {
		return nil, notFound(err, "notification")
	}
	n, err := r.toDomain()
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// filterSQL builds the WHERE clause (without "WHERE") for f.
func filterSQL(f domain.NotificationFilter) (string, []any) {
	conds := []string{"n.user_id = ?"}
	args := []any{f.UserID}
	now := ms(f.Now)

	conds = append(conds, "(n.expires_at IS NULL OR n.expires_at > ?)")
	args = append(args, now)

	switch f.View {
	case domain.ViewInbox, "":
		conds = append(conds, "n.archived_at IS NULL", "(n.snoozed_until IS NULL OR n.snoozed_until <= ?)")
		args = append(args, now)
	case domain.ViewArchived:
		conds = append(conds, "n.archived_at IS NOT NULL")
	case domain.ViewSnoozed:
		conds = append(conds, "n.archived_at IS NULL", "n.snoozed_until > ?")
		args = append(args, now)
	case domain.ViewAll:
	}
	if f.UnreadOnly {
		conds = append(conds, "n.read_at IS NULL")
	}
	if f.TopicIDs != nil {
		if len(f.TopicIDs) == 0 {
			conds = append(conds, "1 = 0")
		} else {
			conds = append(conds, "n.topic_id IN ("+placeholders(len(f.TopicIDs))+")")
			args = append(args, anys(f.TopicIDs)...)
		}
	}
	if f.NoTopic {
		conds = append(conds, "n.topic_id IS NULL")
	}
	if f.MinPriority > 0 {
		conds = append(conds, "n.priority >= ?")
		args = append(args, f.MinPriority)
	}
	if f.GroupKey != "" {
		conds = append(conds, "n.group_key = ?")
		args = append(args, f.GroupKey)
	}
	if f.DedupKey != "" {
		conds = append(conds, "n.dedup_key = ?")
		args = append(args, f.DedupKey)
	}
	for _, term := range f.Search {
		conds = append(conds, `n.search_text LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(term)+"%")
	}
	if len(f.IDs) > 0 {
		conds = append(conds, "n.id IN ("+placeholders(len(f.IDs))+")")
		args = append(args, anys(f.IDs)...)
	}
	if f.Before != "" {
		conds = append(conds, "n.id < ?")
		args = append(args, f.Before)
	}
	return strings.Join(conds, " AND "), args
}

// likeEscaper makes LIKE wildcards in user input literal (ESCAPE '\').
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string { return likeEscaper.Replace(s) }

func (q queries) ListNotifications(ctx context.Context, f domain.NotificationFilter) ([]domain.Notification, error) {
	where, args := filterSQL(f)
	query := notificationSelect + ` WHERE ` + where + ` ORDER BY n.id DESC`
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	var rows []notificationRow
	if err := q.sel(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]domain.Notification, len(rows))
	for i, r := range rows {
		var err error
		if out[i], err = r.toDomain(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (q queries) ListNotificationIDs(ctx context.Context, f domain.NotificationFilter) ([]string, error) {
	where, args := filterSQL(f)
	query := `SELECT n.id FROM notifications n WHERE ` + where + ` ORDER BY n.id DESC`
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	ids := []string{}
	err := q.sel(ctx, &ids, query, args...)
	return ids, err
}

func (q queries) SetNotificationState(ctx context.Context, n *domain.Notification) error {
	return q.execOne(ctx, "notification", `UPDATE notifications SET read_at = ?, archived_at = ?, snoozed_until = ?,
		updated_at = ? WHERE user_id = ? AND id = ?`,
		msPtr(n.ReadAt), msPtr(n.ArchivedAt), msPtr(n.SnoozedUntil), ms(n.UpdatedAt), n.UserID, n.ID)
}

// bulkChunk bounds IN-list sizes (SQLite's default variable limit is 32766).
const bulkChunk = 500

func (q queries) BulkUpdateNotifications(ctx context.Context, userID string, ids []string, action domain.BulkAction, at time.Time) error {
	var set string
	var setArgs []any
	switch action {
	case domain.BulkRead:
		set, setArgs = "read_at = COALESCE(read_at, ?), updated_at = ?", []any{ms(at), ms(at)}
	case domain.BulkUnread:
		set, setArgs = "read_at = NULL, updated_at = ?", []any{ms(at)}
	case domain.BulkArchive:
		set, setArgs = "archived_at = COALESCE(archived_at, ?), read_at = COALESCE(read_at, ?), updated_at = ?", []any{ms(at), ms(at), ms(at)}
	case domain.BulkUnarchive:
		set, setArgs = "archived_at = NULL, updated_at = ?", []any{ms(at)}
	case domain.BulkDelete:
	default:
		return domain.Invalid("unknown bulk action %q", action)
	}

	for start := 0; start < len(ids); start += bulkChunk {
		chunk := ids[start:min(start+bulkChunk, len(ids))]
		in := "id IN (" + placeholders(len(chunk)) + ")"
		var err error
		if action == domain.BulkDelete {
			_, err = q.exec(ctx, `DELETE FROM notifications WHERE user_id = ? AND `+in,
				append([]any{userID}, anys(chunk)...)...)
		} else {
			args := append(append(setArgs[:len(setArgs):len(setArgs)], userID), anys(chunk)...)
			_, err = q.exec(ctx, `UPDATE notifications SET `+set+` WHERE user_id = ? AND `+in, args...)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Counts returns unread inbox counts (total and per topic) and the number of
// snoozed notifications.
func (q queries) Counts(ctx context.Context, userID string, topicIDs []string, now time.Time) (domain.Counts, error) {
	f := domain.NotificationFilter{UserID: userID, View: domain.ViewInbox, UnreadOnly: true, TopicIDs: topicIDs, Now: now}
	where, args := filterSQL(f)
	var rows []struct {
		TopicID *string `db:"topic_id"`
		N       int     `db:"n"`
	}
	err := q.sel(ctx, &rows, `SELECT n.topic_id, COUNT(*) AS n FROM notifications n WHERE `+where+` GROUP BY n.topic_id`, args...)
	if err != nil {
		return domain.Counts{}, err
	}
	c := domain.Counts{ByTopic: map[string]int{}}
	for _, r := range rows {
		c.Unread += r.N
		c.ByTopic[deref(r.TopicID)] = r.N
	}
	sf := domain.NotificationFilter{UserID: userID, View: domain.ViewSnoozed, TopicIDs: topicIDs, Now: now}
	where, args = filterSQL(sf)
	if err := q.get(ctx, &c.Snoozed, `SELECT COUNT(*) FROM notifications n WHERE `+where, args...); err != nil {
		return domain.Counts{}, err
	}
	return c, nil
}

// Events.

func (q queries) InsertEvent(ctx context.Context, e *domain.Event) error {
	_, err := q.exec(ctx, `INSERT INTO events (user_id, seq, type, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		e.UserID, e.Seq, string(e.Type), string(e.Data), ms(e.CreatedAt))
	return err
}

func (q queries) ListEvents(ctx context.Context, userID string, since int64, limit int) ([]domain.Event, error) {
	var rows []struct {
		Seq       int64  `db:"seq"`
		Type      string `db:"type"`
		Payload   string `db:"payload"`
		CreatedAt int64  `db:"created_at"`
	}
	err := q.sel(ctx, &rows, fmt.Sprintf(`SELECT seq, type, payload, created_at FROM events
		WHERE user_id = ? AND seq > ? ORDER BY seq LIMIT %d`, limit), userID, since)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Event, len(rows))
	for i, r := range rows {
		out[i] = domain.Event{UserID: userID, Seq: r.Seq, Type: domain.EventType(r.Type), Data: json.RawMessage(r.Payload), CreatedAt: fromMs(r.CreatedAt)}
	}
	return out, nil
}

func (q queries) EventSeqRange(ctx context.Context, userID string) (int64, int64, error) {
	var r struct {
		Current int64  `db:"current"`
		Oldest  *int64 `db:"oldest"`
	}
	err := q.get(ctx, &r, `SELECT u.event_seq AS current,
		(SELECT MIN(e.seq) FROM events e WHERE e.user_id = u.id) AS oldest
		FROM users u WHERE u.id = ?`, userID)
	if err != nil {
		return 0, 0, notFound(err, "user")
	}
	var oldest int64
	if r.Oldest != nil {
		oldest = *r.Oldest
	}
	return r.Current, oldest, nil
}

func (q queries) DeleteEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := q.exec(ctx, `DELETE FROM events WHERE created_at < ?`, ms(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q queries) ListNotificationRefsByKey(ctx context.Context, topicID, dedupKey string) ([]domain.NotificationRef, error) {
	var rows []struct {
		UserID string `db:"user_id"`
		ID     string `db:"id"`
	}
	err := q.sel(ctx, &rows, `SELECT user_id, id FROM notifications
		WHERE topic_id = ? AND dedup_key = ? ORDER BY user_id, id`, topicID, dedupKey)
	if err != nil {
		return nil, err
	}
	refs := make([]domain.NotificationRef, len(rows))
	for i, r := range rows {
		refs[i] = domain.NotificationRef{UserID: r.UserID, ID: r.ID}
	}
	return refs, nil
}
