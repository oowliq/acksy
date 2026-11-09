package store

import (
	"context"
	"testing"
)

func TestBackfillSearchText(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, "sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	p, err := db.migrator()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	// A notification written before search_text was maintained.
	for _, stmt := range []string{
		`INSERT INTO users (id, username, display_name, email, role, password_hash, created_at, updated_at)
			VALUES ('u1', 'alice', '', '', 'user', '', 0, 0)`,
		`INSERT INTO notifications (id, user_id, message_id, title, body, priority, tags, source, created_at, updated_at)
			VALUES ('n1', 'u1', 'm1', 'Бэкап', 'OK', 3, '["Prod"]', 'acksy', 0, 0)`,
	} {
		if _, err := db.SQL.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.SQL.QueryRowContext(ctx, `SELECT search_text FROM notifications WHERE id = 'n1'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "бэкап ok prod" {
		t.Fatalf("search_text = %q", got)
	}
}
