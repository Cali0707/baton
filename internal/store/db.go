package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// DB is the SQLite-backed store for inbox items and runs.
type DB struct {
	db  *sqlx.DB
	dir string // data directory for output files
}

// OpenDB opens (or creates) the SQLite database and runs pending migrations.
func OpenDB(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	sessionsDir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating sessions directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "baton.db")
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=ON", dbPath)

	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Set pragmas that can't be set via DSN for modernc.org/sqlite.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting %s: %w", pragma, err)
		}
	}

	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return &DB{db: db, dir: dataDir}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

// --- Inbox items ---

func (d *DB) UpsertItem(ctx context.Context, item *InboxItem) error {
	const q = `INSERT INTO inbox_items (source_type, source_id, kind, number, title, body, author, labels, owner, repo, metadata, source_state, status, worktree_path, source_updated_at, created_at, updated_at)
VALUES (:source_type, :source_id, :kind, :number, :title, :body, :author, :labels, :owner, :repo, :metadata, :source_state, :status, :worktree_path, :source_updated_at, :created_at, :updated_at)
ON CONFLICT(source_id) DO UPDATE SET
    title = excluded.title,
    body = excluded.body,
    author = excluded.author,
    labels = excluded.labels,
    metadata = excluded.metadata,
    source_state = excluded.source_state,
    source_updated_at = excluded.source_updated_at,
    updated_at = CURRENT_TIMESTAMP
RETURNING id`

	rows, err := d.db.NamedQueryContext(ctx, q, item)
	if err != nil {
		return fmt.Errorf("upserting item: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&item.ID); err != nil {
			return fmt.Errorf("scanning item id: %w", err)
		}
	}
	return rows.Err()
}

func (d *DB) GetItem(ctx context.Context, id int64) (*InboxItem, error) {
	var item InboxItem
	if err := d.db.GetContext(ctx, &item, `SELECT * FROM inbox_items WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("getting item %d: %w", id, err)
	}
	return &item, nil
}

func (d *DB) GetItemBySourceID(ctx context.Context, sourceID string) (*InboxItem, error) {
	var item InboxItem
	if err := d.db.GetContext(ctx, &item, `SELECT * FROM inbox_items WHERE source_id = ?`, sourceID); err != nil {
		return nil, fmt.Errorf("getting item by source_id %q: %w", sourceID, err)
	}
	return &item, nil
}

func (d *DB) ListItems(ctx context.Context, statuses []ItemStatus) ([]*InboxItem, error) {
	if len(statuses) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		placeholders[i] = "?"
		args[i] = string(s)
	}

	q := fmt.Sprintf(`SELECT * FROM inbox_items WHERE status IN (%s) ORDER BY source_updated_at DESC NULLS LAST, updated_at DESC`,
		strings.Join(placeholders, ","))

	var items []*InboxItem
	if err := d.db.SelectContext(ctx, &items, q, args...); err != nil {
		return nil, fmt.Errorf("listing items: %w", err)
	}
	return items, nil
}

func (d *DB) UpdateItemStatus(ctx context.Context, id int64, status ItemStatus) error {
	_, err := d.db.ExecContext(ctx, `UPDATE inbox_items SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, string(status), id)
	if err != nil {
		return fmt.Errorf("updating item %d status: %w", id, err)
	}
	return nil
}

func (d *DB) UpdateItemSourceState(ctx context.Context, id int64, sourceState string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE inbox_items SET source_state = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sourceState, id)
	if err != nil {
		return fmt.Errorf("updating item %d source_state: %w", id, err)
	}
	return nil
}

func (d *DB) ListItemsByRepoAndSourceState(ctx context.Context, owner, repo, sourceState string) ([]*InboxItem, error) {
	var items []*InboxItem
	if err := d.db.SelectContext(ctx, &items,
		`SELECT * FROM inbox_items WHERE owner = ? AND repo = ? AND source_state = ? ORDER BY source_updated_at DESC NULLS LAST`,
		owner, repo, sourceState); err != nil {
		return nil, fmt.Errorf("listing items for %s/%s with source_state %q: %w", owner, repo, sourceState, err)
	}
	return items, nil
}

func (d *DB) SetLastReviewedAt(ctx context.Context, id int64, t time.Time) error {
	res, err := d.db.ExecContext(ctx, `UPDATE inbox_items SET last_reviewed_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, t, id)
	if err != nil {
		return fmt.Errorf("setting last_reviewed_at for item %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for item %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("setting last_reviewed_at for item %d: item not found", id)
	}
	return nil
}

func (d *DB) DeleteItem(ctx context.Context, id int64) error {
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs WHERE inbox_item_id = ?`, id); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting runs for item %d: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM inbox_items WHERE id = ?`, id); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting item %d: %w", id, err)
	}
	return tx.Commit()
}

// --- Runs ---

func (d *DB) CreateRun(ctx context.Context, run *Run) error {
	const q = `INSERT INTO runs (inbox_item_id, workflow_type, agent_name, agent_session_id, worktree_path, resume_cmd, status, output_file, started_at, completed_at)
VALUES (:inbox_item_id, :workflow_type, :agent_name, :agent_session_id, :worktree_path, :resume_cmd, :status, :output_file, :started_at, :completed_at)
RETURNING id`

	rows, err := d.db.NamedQueryContext(ctx, q, run)
	if err != nil {
		return fmt.Errorf("creating run: %w", err)
	}
	if rows.Next() {
		if err := rows.Scan(&run.ID); err != nil {
			rows.Close()
			return fmt.Errorf("scanning run id: %w", err)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	// Set output file path now that we have the ID.
	run.OutputFile = d.outputPath(run.ID)
	_, err = d.db.ExecContext(ctx, `UPDATE runs SET output_file = ? WHERE id = ?`, run.OutputFile, run.ID)
	return err
}

func (d *DB) GetRun(ctx context.Context, id int64) (*Run, error) {
	var run Run
	if err := d.db.GetContext(ctx, &run, `SELECT * FROM runs WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("getting run %d: %w", id, err)
	}
	return &run, nil
}

func (d *DB) UpdateRun(ctx context.Context, run *Run) error {
	const q = `UPDATE runs SET
		agent_session_id = :agent_session_id,
		worktree_path = :worktree_path,
		resume_cmd = :resume_cmd,
		status = :status,
		output_file = :output_file,
		completed_at = :completed_at
	WHERE id = :id`

	_, err := d.db.NamedExecContext(ctx, q, run)
	if err != nil {
		return fmt.Errorf("updating run %d: %w", run.ID, err)
	}
	return nil
}

func (d *DB) ListRuns(ctx context.Context, statuses []SessionStatus) ([]*Run, error) {
	if len(statuses) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		placeholders[i] = "?"
		args[i] = string(s)
	}

	q := fmt.Sprintf(`SELECT * FROM runs WHERE status IN (%s) ORDER BY started_at DESC`,
		strings.Join(placeholders, ","))

	var runs []*Run
	if err := d.db.SelectContext(ctx, &runs, q, args...); err != nil {
		return nil, fmt.Errorf("listing runs: %w", err)
	}
	return runs, nil
}

func (d *DB) ListRunsForItem(ctx context.Context, itemID int64) ([]*Run, error) {
	var runs []*Run
	if err := d.db.SelectContext(ctx, &runs, `SELECT * FROM runs WHERE inbox_item_id = ? ORDER BY started_at DESC`, itemID); err != nil {
		return nil, fmt.Errorf("listing runs for item %d: %w", itemID, err)
	}
	return runs, nil
}

func (d *DB) DeleteRunsForItem(ctx context.Context, itemID int64) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM runs WHERE inbox_item_id = ?`, itemID)
	if err != nil {
		return fmt.Errorf("deleting runs for item %d: %w", itemID, err)
	}
	return nil
}

// --- Output entries (file-based) ---

func (d *DB) outputPath(runID int64) string {
	return filepath.Join(d.dir, "sessions", fmt.Sprintf("%d.log", runID))
}

func (d *DB) LoadEntries(runID int64) ([]OutputEntry, error) {
	data, err := os.ReadFile(d.outputPath(runID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading output for run %d: %w", runID, err)
	}
	var entries []OutputEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry OutputEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (d *DB) AppendEntry(runID int64, entry OutputEntry) error {
	f, err := os.OpenFile(d.outputPath(runID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
