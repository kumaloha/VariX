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
		`CREATE TABLE IF NOT EXISTS memory_posterior_states (
			memory_id INTEGER PRIMARY KEY,
			node_id TEXT NOT NULL,
			node_kind TEXT NOT NULL,
			state TEXT NOT NULL,
			diagnosis_code TEXT,
			reason TEXT,
			blocked_by_node_ids_json TEXT NOT NULL DEFAULT '[]',
			last_evaluated_at TEXT,
			last_evidence_at TEXT,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(memory_id) REFERENCES user_memory_nodes(memory_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_posterior_states_state
			ON memory_posterior_states(state)`,
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
		`CREATE TABLE IF NOT EXISTS global_memory_organization_outputs (
			output_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL UNIQUE,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS global_memory_v2_outputs (
				output_id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id TEXT NOT NULL UNIQUE,
				payload_json TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS memory_canonical_entities (
				entity_id TEXT PRIMARY KEY,
				entity_type TEXT NOT NULL,
				canonical_name TEXT NOT NULL,
				status TEXT NOT NULL,
				merge_history_json TEXT NOT NULL DEFAULT '[]',
				split_history_json TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE TABLE IF NOT EXISTS memory_canonical_entity_aliases (
				alias_id INTEGER PRIMARY KEY AUTOINCREMENT,
				entity_id TEXT NOT NULL,
				alias_text TEXT NOT NULL,
				created_at TEXT NOT NULL,
				FOREIGN KEY(entity_id) REFERENCES memory_canonical_entities(entity_id)
			)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_entity_alias_unique
				ON memory_canonical_entity_aliases(alias_text)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_entity_alias_entity
				ON memory_canonical_entity_aliases(entity_id)`,
		`CREATE TABLE IF NOT EXISTS memory_relations (
				relation_id TEXT PRIMARY KEY,
				driver_entity_id TEXT NOT NULL,
				target_entity_id TEXT NOT NULL,
				status TEXT NOT NULL,
				retired_at TEXT,
				superseded_by_relation_id TEXT,
				merge_history_json TEXT NOT NULL DEFAULT '[]',
				split_history_json TEXT NOT NULL DEFAULT '[]',
				lifecycle_reason TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_relations_driver_target_active
				ON memory_relations(driver_entity_id, target_entity_id)
				WHERE status IN ('active','inactive')`,
		`CREATE INDEX IF NOT EXISTS idx_memory_relations_driver
				ON memory_relations(driver_entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_relations_target
				ON memory_relations(target_entity_id)`,
		`CREATE TABLE IF NOT EXISTS memory_mechanisms (
				mechanism_id TEXT PRIMARY KEY,
				relation_id TEXT NOT NULL,
				as_of TEXT NOT NULL,
				valid_from TEXT,
				valid_to TEXT,
				confidence REAL NOT NULL,
				status TEXT NOT NULL,
				source_refs_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_mechanisms_relation_asof
				ON memory_mechanisms(relation_id, as_of DESC)`,
		`CREATE TABLE IF NOT EXISTS memory_mechanism_nodes (
				mechanism_node_id TEXT PRIMARY KEY,
				mechanism_id TEXT NOT NULL,
				node_type TEXT NOT NULL,
				label TEXT NOT NULL,
				backing_accepted_node_ids_json TEXT NOT NULL DEFAULT '[]',
				sort_order INTEGER,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_mechanism_nodes_mechanism
				ON memory_mechanism_nodes(mechanism_id)`,
		`CREATE TABLE IF NOT EXISTS memory_mechanism_edges (
				mechanism_edge_id TEXT PRIMARY KEY,
				mechanism_id TEXT NOT NULL,
				from_node_id TEXT NOT NULL,
				to_node_id TEXT NOT NULL,
				edge_type TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_mechanism_edges_mechanism
				ON memory_mechanism_edges(mechanism_id)`,
		`CREATE TABLE IF NOT EXISTS memory_path_outcomes (
				path_outcome_id TEXT PRIMARY KEY,
				mechanism_id TEXT NOT NULL,
				node_path_json TEXT NOT NULL,
				outcome_polarity TEXT NOT NULL,
				outcome_label TEXT NOT NULL,
				condition_scope TEXT,
				confidence REAL NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_path_outcomes_mechanism
				ON memory_path_outcomes(mechanism_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_path_outcomes_polarity
				ON memory_path_outcomes(outcome_polarity)`,
		`CREATE TABLE IF NOT EXISTS memory_driver_aggregates (
				aggregate_id TEXT PRIMARY KEY,
				driver_entity_id TEXT NOT NULL,
				relation_ids_json TEXT NOT NULL,
				target_entity_ids_json TEXT NOT NULL,
				mechanism_labels_json TEXT NOT NULL DEFAULT '[]',
				coverage_score REAL NOT NULL,
				conflict_count INTEGER NOT NULL,
				active_conclusion_ids_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				as_of TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_driver_aggregates_driver_asof
				ON memory_driver_aggregates(driver_entity_id, as_of DESC)`,
		`CREATE TABLE IF NOT EXISTS memory_target_aggregates (
				aggregate_id TEXT PRIMARY KEY,
				target_entity_id TEXT NOT NULL,
				relation_ids_json TEXT NOT NULL,
				driver_entity_ids_json TEXT NOT NULL,
				mechanism_labels_json TEXT NOT NULL DEFAULT '[]',
				coverage_score REAL NOT NULL,
				conflict_count INTEGER NOT NULL,
				active_conclusion_ids_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				as_of TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_target_aggregates_target_asof
				ON memory_target_aggregates(target_entity_id, as_of DESC)`,
		`CREATE TABLE IF NOT EXISTS memory_conflict_views (
				conflict_id TEXT PRIMARY KEY,
				scope_type TEXT NOT NULL,
				scope_id TEXT NOT NULL,
				left_path_outcome_ids_json TEXT NOT NULL,
				right_path_outcome_ids_json TEXT NOT NULL,
				conflict_reason TEXT NOT NULL,
				conflict_topic TEXT,
				status TEXT NOT NULL,
				as_of TEXT NOT NULL,
				traceability_map_json TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_conflict_views_scope_asof
				ON memory_conflict_views(scope_type, scope_id, as_of DESC)`,
		`CREATE TABLE IF NOT EXISTS memory_cognitive_cards (
				card_id TEXT PRIMARY KEY,
				relation_id TEXT NOT NULL,
				as_of TEXT NOT NULL,
				title TEXT NOT NULL,
				summary TEXT NOT NULL,
				mechanism_chain_json TEXT NOT NULL DEFAULT '[]',
				key_evidence_json TEXT NOT NULL DEFAULT '[]',
				conditions_json TEXT NOT NULL DEFAULT '[]',
				predictions_json TEXT NOT NULL DEFAULT '[]',
				source_refs_json TEXT NOT NULL DEFAULT '[]',
				confidence_label TEXT NOT NULL,
				trace_entry_json TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_cognitive_cards_relation_asof
				ON memory_cognitive_cards(relation_id, as_of DESC)`,
		`CREATE TABLE IF NOT EXISTS memory_cognitive_conclusions (
				conclusion_id TEXT PRIMARY KEY,
				source_type TEXT NOT NULL,
				source_id TEXT NOT NULL,
				headline TEXT NOT NULL,
				subheadline TEXT,
				backing_card_ids_json TEXT NOT NULL,
				core_claims_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				blocked_by_conflict INTEGER NOT NULL,
				as_of TEXT NOT NULL,
				judge_model TEXT,
				judge_prompt_version TEXT,
				judge_scores_json TEXT NOT NULL DEFAULT '{}',
				judge_passed INTEGER,
				judged_at TEXT,
				created_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_cognitive_conclusions_source_asof
				ON memory_cognitive_conclusions(source_type, source_id, as_of DESC)`,
		`CREATE TABLE IF NOT EXISTS memory_top_items (
				item_id TEXT PRIMARY KEY,
				item_type TEXT NOT NULL,
				headline TEXT NOT NULL,
				subheadline TEXT,
				backing_object_id TEXT NOT NULL,
				signal_strength TEXT NOT NULL,
				as_of TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_top_items_type_asof
				ON memory_top_items(item_type, as_of DESC)`,
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
