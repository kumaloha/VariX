package contentstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStore_CreateAndUpdateCompilePreviewRun(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	startedAt := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	runID, err := store.CreateCompilePreviewRun(context.Background(), CompilePreviewRun{
		Pipeline:    "compile",
		SampleScope: "all",
		SampleCount: 22,
		WorkerCount: 5,
		Status:      "running",
		StartedAt:   startedAt.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("CreateCompilePreviewRun() error = %v", err)
	}
	if runID <= 0 {
		t.Fatalf("runID = %d, want positive", runID)
	}

	finishedAt := startedAt.Add(2 * time.Minute)
	if err := store.UpdateCompilePreviewRunStatus(context.Background(), runID, "finished", "", finishedAt); err != nil {
		t.Fatalf("UpdateCompilePreviewRunStatus() error = %v", err)
	}

	run, err := store.GetCompilePreviewRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetCompilePreviewRun() error = %v", err)
	}
	if run.Status != "finished" {
		t.Fatalf("run.Status = %q, want finished", run.Status)
	}
	if run.FinishedAt != finishedAt.Format(time.RFC3339) {
		t.Fatalf("run.FinishedAt = %q, want %q", run.FinishedAt, finishedAt.Format(time.RFC3339))
	}
}

func TestSQLiteStore_UpsertAndListCompilePreviewRunItems(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	startedAt := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	runID, err := store.CreateCompilePreviewRun(context.Background(), CompilePreviewRun{
		Pipeline:    "compile",
		SampleScope: "all",
		SampleCount: 2,
		WorkerCount: 1,
		Status:      "running",
		StartedAt:   startedAt.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("CreateCompilePreviewRun() error = %v", err)
	}

	item := CompilePreviewRunItem{
		RunID:                   runID,
		Platform:                "twitter",
		ExternalID:              "abc123",
		URL:                     "https://x.com/example/status/abc123",
		Status:                  "finished",
		ExtractNodes:            18,
		RelationsNodes:          8,
		RelationsEdges:          4,
		ClassifyTargets:         1,
		AuthorValidationTargets: 1,
		RenderDrivers:           2,
		RenderTargets:           1,
		RenderPaths:             1,
		PayloadJSON:             `{"ok":true}`,
		MainlineMarkdown:        "# sample\n",
		StartedAt:               startedAt.Format(time.RFC3339),
		FinishedAt:              startedAt.Add(time.Minute).Format(time.RFC3339),
	}
	if err := store.UpsertCompilePreviewRunItem(context.Background(), item); err != nil {
		t.Fatalf("UpsertCompilePreviewRunItem() error = %v", err)
	}

	items, err := store.ListCompilePreviewRunItems(context.Background(), runID)
	if err != nil {
		t.Fatalf("ListCompilePreviewRunItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListCompilePreviewRunItems()) = %d, want 1", len(items))
	}
	got := items[0]
	if got.Platform != item.Platform || got.ExternalID != item.ExternalID {
		t.Fatalf("got item = %#v, want platform/external id preserved", got)
	}
	if got.MainlineMarkdown != item.MainlineMarkdown {
		t.Fatalf("got.MainlineMarkdown = %q, want %q", got.MainlineMarkdown, item.MainlineMarkdown)
	}
	if got.ClassifyTargets != 1 || got.RenderPaths != 1 {
		t.Fatalf("got counts = %#v, want classify_targets=1 render_paths=1", got)
	}
	if got.AuthorValidationTargets != 1 {
		t.Fatalf("got.AuthorValidationTargets = %d, want 1", got.AuthorValidationTargets)
	}
}

func TestSQLiteStore_MigratesLegacyCompilePreviewValidationColumns(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "data", "content.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	db, err := sql.Open("sqlite", sqliteStoreDSN(dbPath))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE compile_preview_runs (
		run_id INTEGER PRIMARY KEY AUTOINCREMENT,
		pipeline TEXT NOT NULL,
		sample_scope TEXT NOT NULL,
		sample_count INTEGER NOT NULL,
		worker_count INTEGER NOT NULL,
		skip_validate INTEGER NOT NULL DEFAULT 0,
		validate_paragraph_limit INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		error_detail TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		finished_at TEXT
	)`); err != nil {
		t.Fatalf("create legacy compile_preview_runs: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE compile_preview_run_items (
		item_id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER NOT NULL,
		platform TEXT NOT NULL,
		external_id TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		error_detail TEXT NOT NULL DEFAULT '',
		extract_nodes INTEGER NOT NULL DEFAULT 0,
		relations_nodes INTEGER NOT NULL DEFAULT 0,
		relations_edges INTEGER NOT NULL DEFAULT 0,
		classify_targets INTEGER NOT NULL DEFAULT 0,
		validate_targets INTEGER NOT NULL DEFAULT 0,
		render_drivers INTEGER NOT NULL DEFAULT 0,
		render_targets INTEGER NOT NULL DEFAULT 0,
		render_paths INTEGER NOT NULL DEFAULT 0,
		payload_json TEXT NOT NULL DEFAULT '',
		mainline_markdown TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		finished_at TEXT,
		UNIQUE(run_id, platform, external_id),
		FOREIGN KEY(run_id) REFERENCES compile_preview_runs(run_id)
	)`); err != nil {
		t.Fatalf("create legacy compile_preview_run_items: %v", err)
	}
	startedAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO compile_preview_runs(run_id, pipeline, sample_scope, sample_count, worker_count, skip_validate, validate_paragraph_limit, status, started_at)
		VALUES (1, 'compile-author-validate', 'legacy', 1, 1, 1, 3, 'finished', ?)`, startedAt); err != nil {
		t.Fatalf("insert legacy run: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO compile_preview_run_items(run_id, platform, external_id, status, classify_targets, validate_targets, render_paths, started_at)
		VALUES (1, 'twitter', 'legacy', 'finished', 2, 7, 1, ?)`, startedAt); err != nil {
		t.Fatalf("insert legacy item: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	runColumns, err := store.tableColumns("compile_preview_runs")
	if err != nil {
		t.Fatalf("tableColumns(compile_preview_runs) error = %v", err)
	}
	for _, removed := range []string{"skip_validate", "validate_paragraph_limit"} {
		if _, ok := runColumns[removed]; ok {
			t.Fatalf("legacy run column %q still exists: %#v", removed, runColumns)
		}
	}
	itemColumns, err := store.tableColumns("compile_preview_run_items")
	if err != nil {
		t.Fatalf("tableColumns(compile_preview_run_items) error = %v", err)
	}
	if _, ok := itemColumns["validate_targets"]; ok {
		t.Fatalf("legacy item column validate_targets still exists: %#v", itemColumns)
	}
	if _, ok := itemColumns["author_validation_targets"]; !ok {
		t.Fatalf("author_validation_targets missing after migration: %#v", itemColumns)
	}
	items, err := store.ListCompilePreviewRunItems(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListCompilePreviewRunItems() error = %v", err)
	}
	if len(items) != 1 || items[0].AuthorValidationTargets != 7 {
		t.Fatalf("migrated items = %#v, want author validation target count preserved", items)
	}
}
