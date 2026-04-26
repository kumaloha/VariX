package contentstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CompilePreviewRun struct {
	RunID                  int64  `json:"run_id"`
	Pipeline               string `json:"pipeline"`
	SampleScope            string `json:"sample_scope"`
	SampleCount            int    `json:"sample_count"`
	WorkerCount            int    `json:"worker_count"`
	SkipValidate           bool   `json:"skip_validate"`
	ValidateParagraphLimit int    `json:"validate_paragraph_limit"`
	Status                 string `json:"status"`
	ErrorDetail            string `json:"error_detail,omitempty"`
	StartedAt              string `json:"started_at"`
	FinishedAt             string `json:"finished_at,omitempty"`
}

type CompilePreviewRunItem struct {
	ItemID            int64  `json:"item_id"`
	RunID             int64  `json:"run_id"`
	Platform          string `json:"platform"`
	ExternalID        string `json:"external_id"`
	URL               string `json:"url,omitempty"`
	Status            string `json:"status"`
	ErrorDetail       string `json:"error_detail,omitempty"`
	ExtractNodes      int    `json:"extract_nodes"`
	RelationsNodes    int    `json:"relations_nodes"`
	RelationsEdges    int    `json:"relations_edges"`
	ClassifyTargets   int    `json:"classify_targets"`
	ValidateTargets   int    `json:"validate_targets"`
	RenderDrivers     int    `json:"render_drivers"`
	RenderTargets     int    `json:"render_targets"`
	RenderPaths       int    `json:"render_paths"`
	PayloadJSON       string `json:"payload_json,omitempty"`
	MainlineMarkdown  string `json:"mainline_markdown,omitempty"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at,omitempty"`
}

type RawCaptureRef struct {
	Platform   string `json:"platform"`
	ExternalID string `json:"external_id"`
	URL        string `json:"url,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

func (s *SQLiteStore) CreateCompilePreviewRun(ctx context.Context, run CompilePreviewRun) (int64, error) {
	startedAt, err := normalizeRequiredRunTime(run.StartedAt)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO compile_preview_runs(
		pipeline, sample_scope, sample_count, worker_count, skip_validate, validate_paragraph_limit,
		status, error_detail, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(run.Pipeline),
		strings.TrimSpace(run.SampleScope),
		run.SampleCount,
		run.WorkerCount,
		boolToSQLiteInt(run.SkipValidate),
		run.ValidateParagraphLimit,
		strings.TrimSpace(run.Status),
		strings.TrimSpace(run.ErrorDetail),
		startedAt.Format(time.RFC3339),
		nullIfBlank(run.FinishedAt),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) UpdateCompilePreviewRunStatus(ctx context.Context, runID int64, status, errorDetail string, finishedAt time.Time) error {
	if runID <= 0 {
		return fmt.Errorf("run id is required")
	}
	if finishedAt.IsZero() {
		finishedAt = normalizeNow(time.Time{})
	}
	_, err := s.db.ExecContext(ctx, `UPDATE compile_preview_runs
		SET status = ?, error_detail = ?, finished_at = ?
		WHERE run_id = ?`,
		strings.TrimSpace(status),
		strings.TrimSpace(errorDetail),
		finishedAt.UTC().Format(time.RFC3339),
		runID,
	)
	return err
}

func (s *SQLiteStore) GetCompilePreviewRun(ctx context.Context, runID int64) (CompilePreviewRun, error) {
	if runID <= 0 {
		return CompilePreviewRun{}, fmt.Errorf("run id is required")
	}
	var run CompilePreviewRun
	var skipValidate int
	if err := s.db.QueryRowContext(ctx, `SELECT run_id, pipeline, sample_scope, sample_count, worker_count, skip_validate, validate_paragraph_limit, status, error_detail, started_at, COALESCE(finished_at, '')
		FROM compile_preview_runs WHERE run_id = ?`, runID).
		Scan(&run.RunID, &run.Pipeline, &run.SampleScope, &run.SampleCount, &run.WorkerCount, &skipValidate, &run.ValidateParagraphLimit, &run.Status, &run.ErrorDetail, &run.StartedAt, &run.FinishedAt); err != nil {
		return CompilePreviewRun{}, err
	}
	run.SkipValidate = skipValidate != 0
	return run, nil
}

func (s *SQLiteStore) UpsertCompilePreviewRunItem(ctx context.Context, item CompilePreviewRunItem) error {
	if item.RunID <= 0 {
		return fmt.Errorf("run id is required")
	}
	if strings.TrimSpace(item.Platform) == "" || strings.TrimSpace(item.ExternalID) == "" {
		return fmt.Errorf("platform and external id are required")
	}
	startedAt, err := normalizeRequiredRunTime(item.StartedAt)
	if err != nil {
		return err
	}
	finishedAt := strings.TrimSpace(item.FinishedAt)
	if finishedAt != "" {
		if _, err := time.Parse(time.RFC3339, finishedAt); err != nil {
			return fmt.Errorf("finished_at must be RFC3339: %w", err)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO compile_preview_run_items(
		run_id, platform, external_id, url, status, error_detail,
		extract_nodes, relations_nodes, relations_edges, classify_targets, validate_targets,
		render_drivers, render_targets, render_paths, payload_json, mainline_markdown, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(run_id, platform, external_id) DO UPDATE SET
		url = excluded.url,
		status = excluded.status,
		error_detail = excluded.error_detail,
		extract_nodes = excluded.extract_nodes,
		relations_nodes = excluded.relations_nodes,
		relations_edges = excluded.relations_edges,
		classify_targets = excluded.classify_targets,
		validate_targets = excluded.validate_targets,
		render_drivers = excluded.render_drivers,
		render_targets = excluded.render_targets,
		render_paths = excluded.render_paths,
		payload_json = excluded.payload_json,
		mainline_markdown = excluded.mainline_markdown,
		started_at = excluded.started_at,
		finished_at = excluded.finished_at`,
		item.RunID,
		strings.TrimSpace(item.Platform),
		strings.TrimSpace(item.ExternalID),
		strings.TrimSpace(item.URL),
		strings.TrimSpace(item.Status),
		strings.TrimSpace(item.ErrorDetail),
		item.ExtractNodes,
		item.RelationsNodes,
		item.RelationsEdges,
		item.ClassifyTargets,
		item.ValidateTargets,
		item.RenderDrivers,
		item.RenderTargets,
		item.RenderPaths,
		item.PayloadJSON,
		item.MainlineMarkdown,
		startedAt.Format(time.RFC3339),
		nullIfBlank(finishedAt),
	)
	return err
}

func (s *SQLiteStore) ListCompilePreviewRunItems(ctx context.Context, runID int64) ([]CompilePreviewRunItem, error) {
	if runID <= 0 {
		return nil, fmt.Errorf("run id is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item_id, run_id, platform, external_id, url, status, error_detail,
		extract_nodes, relations_nodes, relations_edges, classify_targets, validate_targets,
		render_drivers, render_targets, render_paths, payload_json, mainline_markdown, started_at, COALESCE(finished_at, '')
		FROM compile_preview_run_items
		WHERE run_id = ?
		ORDER BY platform ASC, external_id ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CompilePreviewRunItem, 0)
	for rows.Next() {
		var item CompilePreviewRunItem
		if err := rows.Scan(&item.ItemID, &item.RunID, &item.Platform, &item.ExternalID, &item.URL, &item.Status, &item.ErrorDetail,
			&item.ExtractNodes, &item.RelationsNodes, &item.RelationsEdges, &item.ClassifyTargets, &item.ValidateTargets,
			&item.RenderDrivers, &item.RenderTargets, &item.RenderPaths, &item.PayloadJSON, &item.MainlineMarkdown, &item.StartedAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListRawCaptureRefs(ctx context.Context, limit int, platform string) ([]RawCaptureRef, error) {
	platform = strings.TrimSpace(platform)
	query := `SELECT platform, external_id, url, updated_at FROM raw_captures`
	args := make([]any, 0, 2)
	if platform != "" {
		query += ` WHERE platform = ?`
		args = append(args, platform)
	}
	query += ` ORDER BY updated_at DESC, created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RawCaptureRef, 0)
	for rows.Next() {
		var item RawCaptureRef
		if err := rows.Scan(&item.Platform, &item.ExternalID, &item.URL, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func normalizeRequiredRunTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("started_at is required")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("started_at must be RFC3339: %w", err)
	}
	return parsed.UTC(), nil
}

func boolToSQLiteInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
