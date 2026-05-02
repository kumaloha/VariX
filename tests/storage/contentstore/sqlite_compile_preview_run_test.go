package contentstore

import (
	"context"
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
		Pipeline:               "compile",
		SampleScope:            "all",
		SampleCount:            22,
		WorkerCount:            5,
		SkipValidate:           true,
		ValidateParagraphLimit: 3,
		Status:                 "running",
		StartedAt:              startedAt.Format(time.RFC3339),
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
		RunID:            runID,
		Platform:         "twitter",
		ExternalID:       "abc123",
		URL:              "https://x.com/example/status/abc123",
		Status:           "finished",
		ExtractNodes:     18,
		RelationsNodes:   8,
		RelationsEdges:   4,
		ClassifyTargets:  1,
		ValidateTargets:  1,
		RenderDrivers:    2,
		RenderTargets:    1,
		RenderPaths:      1,
		PayloadJSON:      `{"ok":true}`,
		MainlineMarkdown: "# sample\n",
		StartedAt:        startedAt.Format(time.RFC3339),
		FinishedAt:       startedAt.Add(time.Minute).Format(time.RFC3339),
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
}
