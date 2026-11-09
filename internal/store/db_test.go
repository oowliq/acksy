package store

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		in      string
		dialect Dialect
		dsn     string
		wantErr bool
	}{
		{"sqlite://data/acksy.db", DialectSQLite, "data/acksy.db", false},
		{"sqlite:///data/acksy.db", DialectSQLite, "/data/acksy.db", false},
		{"sqlite://:memory:", DialectSQLite, ":memory:", false},
		{"postgres://u:p@h/db", DialectPostgres, "postgres://u:p@h/db", false},
		{"postgresql://h/db", DialectPostgres, "postgresql://h/db", false},
		{"sqlite://", "", "", true},
		{"mysql://h/db", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d, dsn, err := ParseURL(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if d != tt.dialect || dsn != tt.dsn {
				t.Fatalf("got (%q, %q), want (%q, %q)", d, dsn, tt.dialect, tt.dsn)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	if got := redactURL("mysql://user:secret@host/db"); got != "mysql://***@host/db" {
		t.Fatalf("got %q", got)
	}
}

// Every migration must exist for both dialects with the same file name.
func TestMigrationsParity(t *testing.T) {
	names := func(dir string) map[string]bool {
		entries, err := fs.ReadDir(migrationsFS, "migrations/"+dir)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, e := range entries {
			m[e.Name()] = true
		}
		return m
	}
	sq, pg := names("sqlite"), names("postgres")
	for n := range sq {
		if !pg[n] {
			t.Errorf("migration %s missing for postgres", n)
		}
	}
	for n := range pg {
		if !sq[n] {
			t.Errorf("migration %s missing for sqlite", n)
		}
	}
}

func testMigrate(t *testing.T, url string) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	v, err := db.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v < 1 {
		t.Fatalf("version = %d, want >= 1", v)
	}
	// Idempotent.
	if _, err := db.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	st, err := db.MigrationStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if !s.Applied {
			t.Errorf("migration %s not applied", s.Name)
		}
	}
}

func TestMigrateSQLiteMemory(t *testing.T) {
	testMigrate(t, "sqlite://:memory:")
}

func TestMigrateSQLiteFileCreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "acksy.db")
	testMigrate(t, "sqlite://"+path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}

// Runs only when a Postgres URL is provided (make test-pg / CI service).
func TestMigratePostgres(t *testing.T) {
	url := os.Getenv("ACKSY_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("ACKSY_TEST_POSTGRES_URL not set")
	}
	testMigrate(t, url)
}
