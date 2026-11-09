package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"github.com/oowliq/acksy/internal/domain"
)

// backfillBatch bounds how many rows one backfill query reads.
const backfillBatch = 500

// searchTextMigration fills search_text for notifications written before
// migration 9. It runs in Go because SQLite's lower() only folds ASCII, and
// the text must match what the store writes (domain.SearchText) exactly.
func searchTextMigration(bind int) *goose.Migration {
	return goose.NewGoMigration(10, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		return backfillSearchText(ctx, tx, bind)
	}}, nil)
}

type backfillRow struct{ id, title, body, tags string }

func backfillSearchText(ctx context.Context, tx *sql.Tx, bind int) error {
	sel := sqlx.Rebind(bind, `SELECT id, title, body, tags FROM notifications
		WHERE search_text = '' AND id > ? ORDER BY id LIMIT ?`)
	upd := sqlx.Rebind(bind, `UPDATE notifications SET search_text = ? WHERE id = ?`)

	after := ""
	for {
		batch, err := readBackfillBatch(ctx, tx, sel, after)
		if err != nil {
			return fmt.Errorf("backfill search_text: %w", err)
		}
		for _, r := range batch {
			var tags []string
			_ = json.Unmarshal([]byte(r.tags), &tags) // malformed tags just aren't searchable
			if _, err := tx.ExecContext(ctx, upd, domain.SearchText(r.title, r.body, tags), r.id); err != nil {
				return fmt.Errorf("backfill search_text: %w", err)
			}
		}
		if len(batch) < backfillBatch {
			return nil
		}
		after = batch[len(batch)-1].id
	}
}

func readBackfillBatch(ctx context.Context, tx *sql.Tx, query, after string) ([]backfillRow, error) {
	rows, err := tx.QueryContext(ctx, query, after, backfillBatch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var batch []backfillRow
	for rows.Next() {
		var r backfillRow
		if err := rows.Scan(&r.id, &r.title, &r.body, &r.tags); err != nil {
			return nil, err
		}
		batch = append(batch, r)
	}
	return batch, rows.Err()
}
