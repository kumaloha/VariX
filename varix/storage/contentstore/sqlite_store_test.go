package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
)

func TestSQLiteStore_MarkProcessedAndIsProcessed(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	ok, err := store.IsProcessed(context.Background(), "twitter", "12345")
	if err != nil {
		t.Fatalf("IsProcessed() error = %v", err)
	}
	if ok {
		t.Fatal("expected item to be absent before MarkProcessed")
	}

	err = store.MarkProcessed(context.Background(), types.ProcessedRecord{
		Platform:    "twitter",
		ExternalID:  "12345",
		URL:         "https://x.com/a/status/12345",
		Author:      "alice",
		ProcessedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	ok, err = store.IsProcessed(context.Background(), "twitter", "12345")
	if err != nil {
		t.Fatalf("IsProcessed() error = %v", err)
	}
	if !ok {
		t.Fatal("expected item to be present after MarkProcessed")
	}
}

func TestSQLiteStore_RegisterListAndUpdateFollow(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	target := types.FollowTarget{
		Kind:       types.KindNative,
		Platform:   "weibo",
		PlatformID: "123456",
		Locator:    "https://weibo.com/123456",
		URL:        "https://weibo.com/123456",
		FollowedAt: time.Now().UTC(),
	}
	if err := store.RegisterFollow(context.Background(), target); err != nil {
		t.Fatalf("RegisterFollow() error = %v", err)
	}

	items, warnings, err := store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 1 {
		t.Fatalf("len(ListFollows()) = %d, want 1", len(items))
	}

	now := time.Now().UTC()
	if err := store.UpdateFollowPolled(context.Background(), types.KindNative, "weibo", "https://weibo.com/123456", now); err != nil {
		t.Fatalf("UpdateFollowPolled() error = %v", err)
	}

	items, warnings, err = store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if items[0].LastPolledAt.IsZero() {
		t.Fatal("LastPolledAt was not updated")
	}
}

func TestSQLiteStore_RemoveFollow(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	target := types.FollowTarget{
		Kind:       types.KindRSS,
		Platform:   "rss",
		Locator:    "https://feeds.example.test/feed.xml",
		URL:        "https://feeds.example.test/feed.xml",
		FollowedAt: time.Now().UTC(),
	}
	if err := store.RegisterFollow(context.Background(), target); err != nil {
		t.Fatalf("RegisterFollow() error = %v", err)
	}
	if err := store.RemoveFollow(context.Background(), types.KindRSS, "rss", "https://feeds.example.test/feed.xml"); err != nil {
		t.Fatalf("RemoveFollow() error = %v", err)
	}

	items, warnings, err := store.ListFollows(context.Background())
	if err != nil {
		t.Fatalf("ListFollows() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("len(ListFollows() warnings) = %d, want 0", len(warnings))
	}
	if len(items) != 0 {
		t.Fatalf("len(ListFollows()) = %d, want 0", len(items))
	}
}

func TestSQLiteStore_RegisterFollowRejectsInvalidRecord(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	err = store.RegisterFollow(context.Background(), types.FollowTarget{
		Kind:     types.KindSearch,
		Platform: "twitter",
	})
	if err == nil {
		t.Fatal("expected RegisterFollow() to reject invalid follow target")
	}
}

func TestSQLiteStore_RecordPollReportPersistsRunAndTargets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	report := types.PollReport{
		StartedAt:         time.Now().UTC().Add(-time.Minute),
		FinishedAt:        time.Now().UTC(),
		TargetCount:       2,
		DiscoveredCount:   3,
		FetchedCount:      2,
		SkippedCount:      1,
		StoreWarningCount: 0,
		PollWarningCount:  1,
		Targets: []types.TargetPollReport{
			{Target: "search:twitter:nvda", DiscoveredCount: 2, FetchedCount: 1, SkippedCount: 1, WarningCount: 0, Status: "ok"},
			{Target: "rss:rss:https://example.com/feed.xml", DiscoveredCount: 1, FetchedCount: 1, SkippedCount: 0, WarningCount: 1, Status: "warning", ErrorDetail: "hydrate failed"},
		},
	}

	if err := store.RecordPollReport(context.Background(), report); err != nil {
		t.Fatalf("RecordPollReport() error = %v", err)
	}

	var runCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM poll_runs`).Scan(&runCount); err != nil {
		t.Fatalf("QueryRow(poll_runs) error = %v", err)
	}
	if runCount != 1 {
		t.Fatalf("poll_runs count = %d, want 1", runCount)
	}

	var targetCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM poll_target_runs`).Scan(&targetCount); err != nil {
		t.Fatalf("QueryRow(poll_target_runs) error = %v", err)
	}
	if targetCount != 2 {
		t.Fatalf("poll_target_runs count = %d, want 2", targetCount)
	}
}
