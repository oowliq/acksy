package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type userRow struct {
	ID           string  `db:"id"`
	Username     string  `db:"username"`
	DisplayName  string  `db:"display_name"`
	Email        *string `db:"email"`
	PasswordHash *string `db:"password_hash"`
	Role         string  `db:"role"`
	DisabledAt   *int64  `db:"disabled_at"`
	CreatedAt    int64   `db:"created_at"`
	UpdatedAt    int64   `db:"updated_at"`
}

const userCols = `id, username, display_name, email, password_hash, role, disabled_at, created_at, updated_at`

func (r userRow) toDomain() domain.User {
	return domain.User{
		ID:           r.ID,
		Username:     r.Username,
		DisplayName:  r.DisplayName,
		Email:        deref(r.Email),
		PasswordHash: deref(r.PasswordHash),
		Role:         domain.Role(r.Role),
		DisabledAt:   fromMsPtr(r.DisabledAt),
		CreatedAt:    fromMs(r.CreatedAt),
		UpdatedAt:    fromMs(r.UpdatedAt),
	}
}

func (q queries) CreateUser(ctx context.Context, u *domain.User) error {
	_, err := q.exec(ctx, `INSERT INTO users (`+userCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.DisplayName, strPtr(u.Email), strPtr(u.PasswordHash), string(u.Role),
		msPtr(u.DisabledAt), ms(u.CreatedAt), ms(u.UpdatedAt))
	if isUniqueViolation(err) {
		return domain.Conflict("username %q is already taken", u.Username)
	}
	return err
}

func (q queries) getUser(ctx context.Context, where string, arg any) (*domain.User, error) {
	var r userRow
	if err := q.get(ctx, &r, `SELECT `+userCols+` FROM users WHERE `+where, arg); err != nil {
		return nil, notFound(err, "user")
	}
	u := r.toDomain()
	return &u, nil
}

func (q queries) GetUser(ctx context.Context, id string) (*domain.User, error) {
	return q.getUser(ctx, "id = ?", id)
}

func (q queries) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	return q.getUser(ctx, "username = ?", username)
}

func (q queries) ListUsers(ctx context.Context) ([]domain.User, error) {
	var rows []userRow
	if err := q.sel(ctx, &rows, `SELECT `+userCols+` FROM users ORDER BY username`); err != nil {
		return nil, err
	}
	out := make([]domain.User, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

func (q queries) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := q.get(ctx, &n, `SELECT COUNT(*) FROM users`)
	return n, err
}

func (q queries) CountActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := q.get(ctx, &n, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL`)
	return n, err
}

func (q queries) UpdateUser(ctx context.Context, u *domain.User) error {
	err := q.execOne(ctx, "user", `UPDATE users SET display_name = ?, email = ?, password_hash = ?, role = ?,
		disabled_at = ?, updated_at = ? WHERE id = ?`,
		u.DisplayName, strPtr(u.Email), strPtr(u.PasswordHash), string(u.Role), msPtr(u.DisabledAt), ms(u.UpdatedAt), u.ID)
	return err
}

func (q queries) DeleteUser(ctx context.Context, id string) error {
	return q.execOne(ctx, "user", `DELETE FROM users WHERE id = ?`, id)
}

func (q queries) NextEventSeq(ctx context.Context, userID string) (int64, error) {
	var seq int64
	err := q.get(ctx, &seq, `UPDATE users SET event_seq = event_seq + 1 WHERE id = ? RETURNING event_seq`, userID)
	return seq, notFound(err, "user")
}

// Sessions.

type sessionRow struct {
	ID         string `db:"id"`
	UserID     string `db:"user_id"`
	UserAgent  string `db:"user_agent"`
	CreatedAt  int64  `db:"created_at"`
	ExpiresAt  int64  `db:"expires_at"`
	LastSeenAt int64  `db:"last_seen_at"`
}

func (q queries) CreateSession(ctx context.Context, s *domain.Session) error {
	_, err := q.exec(ctx, `INSERT INTO sessions (id, user_id, user_agent, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.UserAgent, ms(s.CreatedAt), ms(s.ExpiresAt), ms(s.LastSeenAt))
	return err
}

func (q queries) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	var r sessionRow
	err := q.get(ctx, &r, `SELECT id, user_id, user_agent, created_at, expires_at, last_seen_at
		FROM sessions WHERE id = ?`, id)
	if err != nil {
		return nil, notFound(err, "session")
	}
	return &domain.Session{
		ID: r.ID, UserID: r.UserID, UserAgent: r.UserAgent,
		CreatedAt: fromMs(r.CreatedAt), ExpiresAt: fromMs(r.ExpiresAt), LastSeenAt: fromMs(r.LastSeenAt),
	}, nil
}

func (q queries) TouchSession(ctx context.Context, id string, lastSeen, expiresAt time.Time) error {
	_, err := q.exec(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id = ?`,
		ms(lastSeen), ms(expiresAt), id)
	return err
}

func (q queries) DeleteSession(ctx context.Context, id string) error {
	_, err := q.exec(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (q queries) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := q.exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (q queries) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := q.exec(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, ms(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
