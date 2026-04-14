package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	store := &SQLiteStore{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) IsProcessed(ctx context.Context, platform, externalID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(
		ctx,
		`SELECT 1 FROM processed WHERE platform = ? AND external_id = ? LIMIT 1`,
		platform,
		externalID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) MarkProcessed(ctx context.Context, record types.ProcessedRecord) error {
	if !isValidProcessedRecord(record) {
		return fmt.Errorf("invalid processed record")
	}
	if record.ProcessedAt.IsZero() {
		record.ProcessedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO processed(platform, external_id, url, author, processed_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(platform, external_id) DO UPDATE SET
		   url = excluded.url,
		   author = excluded.author,
		   processed_at = excluded.processed_at`,
		record.Platform,
		record.ExternalID,
		record.URL,
		record.Author,
		record.ProcessedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) ListProcessed(ctx context.Context) ([]types.ProcessedRecord, []ScanWarning, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT platform, external_id, url, author, processed_at
		 FROM processed
		 ORDER BY processed_at DESC`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]types.ProcessedRecord, 0)
	for rows.Next() {
		var item types.ProcessedRecord
		var processedAt string
		if err := rows.Scan(&item.Platform, &item.ExternalID, &item.URL, &item.Author, &processedAt); err != nil {
			return nil, nil, err
		}
		item.ProcessedAt = parseSQLiteTime(processedAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, nil, nil
}

func (s *SQLiteStore) RegisterFollow(ctx context.Context, target types.FollowTarget) error {
	if !isValidFollowTarget(target) {
		return fmt.Errorf("invalid follow target")
	}
	if target.FollowedAt.IsZero() {
		target.FollowedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO follows(kind, platform, platform_id, locator, url, query, hydration_hint, author_name, followed_at, last_polled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(kind, platform, locator) DO UPDATE SET
		   platform_id = excluded.platform_id,
		   url = excluded.url,
		   query = excluded.query,
		   hydration_hint = excluded.hydration_hint,
		   author_name = excluded.author_name,
		   followed_at = excluded.followed_at,
		   last_polled_at = excluded.last_polled_at`,
		string(target.Kind),
		target.Platform,
		target.PlatformID,
		target.Locator,
		target.URL,
		target.Query,
		target.HydrationHint,
		target.AuthorName,
		target.FollowedAt.UTC().Format(time.RFC3339Nano),
		formatSQLiteTime(target.LastPolledAt),
	)
	return err
}

func (s *SQLiteStore) ListFollows(ctx context.Context) ([]types.FollowTarget, []ScanWarning, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT kind, platform, platform_id, locator, url, query, hydration_hint, author_name, followed_at, last_polled_at
		 FROM follows
		 ORDER BY followed_at ASC`,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]types.FollowTarget, 0)
	for rows.Next() {
		var item types.FollowTarget
		var kind string
		var followedAt string
		var lastPolled sql.NullString
		if err := rows.Scan(
			&kind,
			&item.Platform,
			&item.PlatformID,
			&item.Locator,
			&item.URL,
			&item.Query,
			&item.HydrationHint,
			&item.AuthorName,
			&followedAt,
			&lastPolled,
		); err != nil {
			return nil, nil, err
		}
		item.Kind = types.Kind(kind)
		item.FollowedAt = parseSQLiteTime(followedAt)
		if lastPolled.Valid {
			item.LastPolledAt = parseSQLiteTime(lastPolled.String)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return items, nil, nil
}

func (s *SQLiteStore) RemoveFollow(ctx context.Context, kind types.Kind, platform string, locator string) error {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM follows WHERE kind = ? AND platform = ? AND locator = ?`,
		string(kind),
		platform,
		locator,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("follow target not found: %s/%s/%s", kind, platform, locator)
	}
	return nil
}

func (s *SQLiteStore) UpdateFollowPolled(ctx context.Context, kind types.Kind, platform string, locator string, at time.Time) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE follows
		 SET last_polled_at = ?
		 WHERE kind = ? AND platform = ? AND locator = ?`,
		at.UTC().Format(time.RFC3339Nano),
		string(kind),
		platform,
		locator,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("follow target not found: %s/%s/%s", kind, platform, locator)
	}
	return nil
}

func (s *SQLiteStore) RecordPollReport(ctx context.Context, report types.PollReport) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO poll_runs(started_at, finished_at, target_count, discovered_count, fetched_count, skipped_count, store_warning_count, poll_warning_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		report.StartedAt.UTC().Format(time.RFC3339Nano),
		report.FinishedAt.UTC().Format(time.RFC3339Nano),
		report.TargetCount,
		report.DiscoveredCount,
		report.FetchedCount,
		report.SkippedCount,
		report.StoreWarningCount,
		report.PollWarningCount,
	)
	if err != nil {
		return err
	}
	runID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	for _, target := range report.Targets {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO poll_target_runs(run_id, target, discovered_count, fetched_count, skipped_count, warning_count, status, error_detail)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			runID,
			target.Target,
			target.DiscoveredCount,
			target.FetchedCount,
			target.SkippedCount,
			target.WarningCount,
			target.Status,
			target.ErrorDetail,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS processed (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			author TEXT NOT NULL DEFAULT '',
			processed_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
		`CREATE TABLE IF NOT EXISTS follows (
			kind TEXT NOT NULL,
			platform TEXT NOT NULL,
			platform_id TEXT NOT NULL DEFAULT '',
			locator TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			query TEXT NOT NULL DEFAULT '',
			hydration_hint TEXT NOT NULL DEFAULT '',
			author_name TEXT NOT NULL DEFAULT '',
			followed_at TEXT NOT NULL,
			last_polled_at TEXT,
			PRIMARY KEY(kind, platform, locator)
		)`,
		`CREATE TABLE IF NOT EXISTS poll_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			target_count INTEGER NOT NULL,
			discovered_count INTEGER NOT NULL,
			fetched_count INTEGER NOT NULL,
			skipped_count INTEGER NOT NULL,
			store_warning_count INTEGER NOT NULL,
			poll_warning_count INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS poll_target_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			target TEXT NOT NULL,
			discovered_count INTEGER NOT NULL,
			fetched_count INTEGER NOT NULL,
			skipped_count INTEGER NOT NULL,
			warning_count INTEGER NOT NULL,
			status TEXT NOT NULL,
			error_detail TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(run_id) REFERENCES poll_runs(id)
		)`,
		`CREATE TABLE IF NOT EXISTS raw_captures (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			author_name TEXT NOT NULL DEFAULT '',
			author_id TEXT NOT NULL DEFAULT '',
			posted_at TEXT,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
		`CREATE TABLE IF NOT EXISTS source_lookup_jobs (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id),
			FOREIGN KEY(platform, external_id) REFERENCES raw_captures(platform, external_id)
		)`,
		`CREATE TABLE IF NOT EXISTS compiled_outputs (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			compiled_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_memory_nodes (
			memory_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			node_id TEXT NOT NULL,
			node_kind TEXT NOT NULL,
			node_text TEXT NOT NULL,
			source_model TEXT NOT NULL,
			source_compiled_at TEXT NOT NULL,
			valid_from TEXT NOT NULL,
			valid_to TEXT NOT NULL,
			accepted_at TEXT NOT NULL,
			UNIQUE(user_id, source_platform, source_external_id, node_id)
		)`,
		`CREATE TABLE IF NOT EXISTS memory_acceptance_events (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			trigger_type TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			source_model TEXT NOT NULL,
			source_compiled_at TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			accepted_count INTEGER NOT NULL,
			accepted_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS memory_organization_jobs (
			job_id INTEGER PRIMARY KEY AUTOINCREMENT,
			trigger_event_id INTEGER NOT NULL,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS memory_organization_outputs (
			output_id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL UNIQUE,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	migrations := []struct {
		table      string
		column     string
		definition string
	}{
		{table: "user_memory_nodes", column: "valid_from", definition: "TEXT NOT NULL DEFAULT ''"},
		{table: "user_memory_nodes", column: "valid_to", definition: "TEXT NOT NULL DEFAULT ''"},
	}
	for _, m := range migrations {
		if err := s.ensureColumn(m.table, m.column, m.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func parseSQLiteTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func formatSQLiteTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
