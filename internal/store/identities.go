package store

import (
	"context"
	"time"

	"github.com/oowliq/acksy/internal/domain"
)

type identityRow struct {
	Issuer      string `db:"issuer"`
	Subject     string `db:"subject"`
	UserID      string `db:"user_id"`
	CreatedAt   int64  `db:"created_at"`
	LastLoginAt *int64 `db:"last_login_at"`
}

const identityCols = `issuer, subject, user_id, created_at, last_login_at`

func (r identityRow) toDomain() domain.Identity {
	return domain.Identity{
		Issuer: r.Issuer, Subject: r.Subject, UserID: r.UserID,
		CreatedAt: fromMs(r.CreatedAt), LastLoginAt: fromMsPtr(r.LastLoginAt),
	}
}

func (q queries) CreateIdentity(ctx context.Context, i *domain.Identity) error {
	_, err := q.exec(ctx, `INSERT INTO user_identities (`+identityCols+`) VALUES (?, ?, ?, ?, ?)`,
		i.Issuer, i.Subject, i.UserID, ms(i.CreatedAt), msPtr(i.LastLoginAt))
	if isUniqueViolation(err) {
		return domain.Conflict("identity is already linked to a user")
	}
	return err
}

func (q queries) GetIdentity(ctx context.Context, issuer, subject string) (*domain.Identity, error) {
	var r identityRow
	err := q.get(ctx, &r, `SELECT `+identityCols+` FROM user_identities WHERE issuer = ? AND subject = ?`, issuer, subject)
	if err != nil {
		return nil, notFound(err, "identity")
	}
	i := r.toDomain()
	return &i, nil
}

func (q queries) ListUserIdentities(ctx context.Context, userID string) ([]domain.Identity, error) {
	var rows []identityRow
	err := q.sel(ctx, &rows, `SELECT `+identityCols+` FROM user_identities WHERE user_id = ? ORDER BY issuer, subject`, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Identity, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

func (q queries) TouchIdentity(ctx context.Context, issuer, subject string, at time.Time) error {
	return q.execOne(ctx, "identity", `UPDATE user_identities SET last_login_at = ? WHERE issuer = ? AND subject = ?`,
		ms(at), issuer, subject)
}

func (q queries) DeleteIdentity(ctx context.Context, userID, issuer, subject string) error {
	return q.execOne(ctx, "identity", `DELETE FROM user_identities WHERE user_id = ? AND issuer = ? AND subject = ?`,
		userID, issuer, subject)
}
