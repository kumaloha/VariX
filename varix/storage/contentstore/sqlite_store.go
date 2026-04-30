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
	db, err := sql.Open("sqlite", sqliteStoreDSN(path))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL journal mode: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(journalMode)) != "wal" {
		db.Close()
		return nil, fmt.Errorf("enable WAL journal mode: got %q", journalMode)
	}
	store := &SQLiteStore{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func sqliteStoreDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=busy_timeout%3D5000&_pragma=foreign_keys%3DON&_pragma=journal_mode%28WAL%29"
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
	record.ProcessedAt = normalizeRecordedTime(record.ProcessedAt)
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
	target.FollowedAt = normalizeRecordedTime(target.FollowedAt)
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
	for _, stmt := range sqliteInitStatements {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := s.ensurePosteriorStateStorage(); err != nil {
		return err
	}
	for _, m := range sqliteColumnMigrations {
		if err := s.ensureColumn(m.table, m.column, m.definition); err != nil {
			return err
		}
	}
	if err := s.backfillMemoryContentGraphSubjects(); err != nil {
		return err
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

func (s *SQLiteStore) ensurePosteriorStateStorage() error {
	columns, err := s.tableColumns("memory_posterior_states")
	if err != nil {
		return err
	}
	if len(columns) == 0 {
		return nil
	}
	if _, ok := columns["source_platform"]; ok {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`CREATE TABLE memory_posterior_states_v2 (
		source_platform TEXT NOT NULL,
		source_external_id TEXT NOT NULL,
		node_id TEXT NOT NULL,
		node_kind TEXT NOT NULL,
		state TEXT NOT NULL,
		diagnosis_code TEXT,
		reason TEXT,
		blocked_by_node_ids_json TEXT NOT NULL DEFAULT '[]',
		last_evaluated_at TEXT,
		last_evidence_at TEXT,
		updated_at TEXT NOT NULL,
		PRIMARY KEY(source_platform, source_external_id, node_id)
	)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`WITH ranked AS (
		SELECT
			u.source_platform,
			u.source_external_id,
			p.node_id,
			p.node_kind,
			p.state,
			p.diagnosis_code,
			p.reason,
			p.blocked_by_node_ids_json,
			p.last_evaluated_at,
			p.last_evidence_at,
			p.updated_at,
			ROW_NUMBER() OVER (
				PARTITION BY u.source_platform, u.source_external_id, p.node_id
				ORDER BY p.updated_at DESC, p.memory_id DESC
			) AS rn
		FROM memory_posterior_states p
		INNER JOIN user_memory_nodes u ON u.memory_id = p.memory_id
	)
	INSERT INTO memory_posterior_states_v2(
		source_platform,
		source_external_id,
		node_id,
		node_kind,
		state,
		diagnosis_code,
		reason,
		blocked_by_node_ids_json,
		last_evaluated_at,
		last_evidence_at,
		updated_at
	)
	SELECT
		source_platform,
		source_external_id,
		node_id,
		node_kind,
		state,
		diagnosis_code,
		reason,
		blocked_by_node_ids_json,
		last_evaluated_at,
		last_evidence_at,
		updated_at
	FROM ranked
	WHERE rn = 1`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE memory_posterior_states`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE memory_posterior_states_v2 RENAME TO memory_posterior_states`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_posterior_states_state
		ON memory_posterior_states(state)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) tableColumns(table string) (map[string]struct{}, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
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
