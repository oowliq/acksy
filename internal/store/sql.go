package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/oowliq/acksy/internal/domain"
)

// SQLStore implements Store over SQLite or PostgreSQL using one set of
// portable SQL statements (see docs/adr/0001-portable-sql.md).
type SQLStore struct {
	queries
	x *sqlx.DB
}

var _ Store = (*SQLStore)(nil)

// New returns a Store backed by db.
func New(db *DB) *SQLStore {
	driver := "sqlite"
	if db.Dialect == DialectPostgres {
		driver = "pgx"
	}
	x := sqlx.NewDb(db.SQL, driver)
	return &SQLStore{queries: queries{x: x, dialect: db.Dialect}, x: x}
}

// InTx implements Store.
func (s *SQLStore) InTx(ctx context.Context, fn func(q Querier) error) (err error) {
	tx, err := s.x.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(queries{x: tx, dialect: s.dialect}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// queries runs statements on a DB or a Tx.
type queries struct {
	x       sqlx.ExtContext
	dialect Dialect
}

func (q queries) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return q.x.ExecContext(ctx, q.x.Rebind(query), args...)
}

func (q queries) get(ctx context.Context, dst any, query string, args ...any) error {
	return sqlx.GetContext(ctx, q.x, dst, q.x.Rebind(query), args...)
}

func (q queries) sel(ctx context.Context, dst any, query string, args ...any) error {
	return sqlx.SelectContext(ctx, q.x, dst, q.x.Rebind(query), args...)
}

// execOne runs a statement that must affect exactly one row.
func (q queries) execOne(ctx context.Context, what, query string, args ...any) error {
	res, err := q.exec(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.NotFound(what)
	}
	return nil
}

// notFound maps sql.ErrNoRows to a domain not-found error.
func notFound(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NotFound(what)
	}
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// placeholders returns "?, ?, ?" for n arguments.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func anys[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// Time is stored as Unix milliseconds.
func ms(t time.Time) int64 { return t.UnixMilli() }

func fromMs(v int64) time.Time { return time.UnixMilli(v).UTC() }

func msPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.UnixMilli()
	return &v
}

func fromMsPtr(v *int64) *time.Time {
	if v == nil {
		return nil
	}
	t := fromMs(*v)
	return &t
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
