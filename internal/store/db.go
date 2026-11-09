// Package store owns persistence: connecting to SQLite or PostgreSQL and
// running schema migrations. Query code (sqlc) lives in dialect subpackages.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers "sqlite" driver
)

// Dialect identifies the SQL backend.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

//go:embed migrations
var migrationsFS embed.FS

// DB is an open database handle with its dialect.
type DB struct {
	SQL     *sql.DB
	Dialect Dialect
}

// ParseURL splits a database URL into dialect and driver DSN.
//
//	sqlite://data/acksy.db      -> relative path
//	sqlite:///data/acksy.db     -> absolute path
//	sqlite://:memory:           -> private in-memory database
//	postgres://user:pw@host/db  -> passed to pgx as is
func ParseURL(raw string) (Dialect, string, error) {
	switch {
	case strings.HasPrefix(raw, "sqlite://"):
		path := strings.TrimPrefix(raw, "sqlite://")
		if path == "" {
			return "", "", errors.New("sqlite url has no path")
		}
		return DialectSQLite, path, nil
	case strings.HasPrefix(raw, "postgres://"), strings.HasPrefix(raw, "postgresql://"):
		return DialectPostgres, raw, nil
	default:
		return "", "", fmt.Errorf("unsupported database url scheme in %q (want sqlite:// or postgres://)", redactURL(raw))
	}
}

// Open connects to the database described by url and verifies the connection.
func Open(ctx context.Context, url string) (*DB, error) {
	dialect, dsn, err := ParseURL(url)
	if err != nil {
		return nil, err
	}

	var db *sql.DB
	switch dialect {
	case DialectSQLite:
		db, err = openSQLite(dsn)
	case DialectPostgres:
		db, err = sql.Open("pgx", dsn)
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dialect, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", dialect, err)
	}
	return &DB{SQL: db, Dialect: dialect}, nil
}

func openSQLite(path string) (*sql.DB, error) {
	memory := path == ":memory:"
	if !memory {
		if dir := filepath.Dir(path); dir != "." {
			//nolint:gosec // path comes from operator configuration
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("create data dir: %w", err)
			}
		}
	}
	pragmas := "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate"
	if !memory {
		pragmas += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", "file:"+path+"?"+pragmas)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; one connection avoids SQLITE_BUSY storms
	// and keeps an in-memory database alive and shared.
	db.SetMaxOpenConns(1)
	return db, nil
}

// Close releases the connection pool.
func (d *DB) Close() error { return d.SQL.Close() }

// Ping checks the database is reachable.
func (d *DB) Ping(ctx context.Context) error { return d.SQL.PingContext(ctx) }

func (d *DB) migrator() (*goose.Provider, error) {
	sub, err := fs.Sub(migrationsFS, "migrations/"+string(d.Dialect))
	if err != nil {
		return nil, err
	}
	gd, bind := goose.DialectSQLite3, sqlx.QUESTION
	if d.Dialect == DialectPostgres {
		gd, bind = goose.DialectPostgres, sqlx.DOLLAR
	}
	return goose.NewProvider(gd, d.SQL, sub, goose.WithGoMigrations(searchTextMigration(bind)))
}

// Migrate applies all pending migrations and returns the resulting version.
func (d *DB) Migrate(ctx context.Context) (int64, error) {
	p, err := d.migrator()
	if err != nil {
		return 0, fmt.Errorf("init migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return 0, fmt.Errorf("migrate up: %w", err)
	}
	return p.GetDBVersion(ctx)
}

// MigrationStatus describes one migration.
type MigrationStatus struct {
	Version int64
	Name    string
	Applied bool
}

// MigrationStatus lists all known migrations and whether they are applied.
func (d *DB) MigrationStatus(ctx context.Context) ([]MigrationStatus, error) {
	p, err := d.migrator()
	if err != nil {
		return nil, fmt.Errorf("init migrations: %w", err)
	}
	res, err := p.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationStatus, 0, len(res))
	for _, r := range res {
		out = append(out, MigrationStatus{
			Version: r.Source.Version,
			Name:    filepath.Base(r.Source.Path),
			Applied: r.State == goose.StateApplied,
		})
	}
	return out, nil
}

func redactURL(raw string) string {
	if i := strings.Index(raw, "@"); i >= 0 {
		if j := strings.Index(raw, "://"); j >= 0 && j < i {
			return raw[:j+3] + "***" + raw[i:]
		}
	}
	return raw
}
