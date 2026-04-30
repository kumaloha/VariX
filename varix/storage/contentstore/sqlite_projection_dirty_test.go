package contentstore

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
)

func TestSQLiteStore_ProjectionDirtyMarksCoalesceAndClear(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	firstAt := time.Date(2026, 4, 30, 1, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Hour)
	mark := ProjectionDirtyMark{
		UserID:  "u-dirty",
		Layer:   "subject-horizon",
		Subject: "美股",
		Horizon: "1w",
		Reason:  "content_graph_changed",
	}
	if err := store.MarkProjectionDirty(ctx, mark, firstAt); err != nil {
		t.Fatalf("first MarkProjectionDirty() error = %v", err)
	}
	mark.SourceRef = "twitter:2"
	if err := store.MarkProjectionDirty(ctx, mark, secondAt); err != nil {
		t.Fatalf("second MarkProjectionDirty() error = %v", err)
	}
	marks, err := store.ListProjectionDirtyMarks(ctx, "u-dirty", 10)
	if err != nil {
		t.Fatalf("ListProjectionDirtyMarks() error = %v", err)
	}
	if len(marks) != 1 {
		t.Fatalf("len(marks) = %d, want coalesced single mark", len(marks))
	}
	if marks[0].SourceRef != "twitter:2" || marks[0].DirtyAt != secondAt.Format(time.RFC3339Nano) {
		t.Fatalf("mark = %#v, want latest source and dirty time", marks[0])
	}
	if err := store.ClearProjectionDirtyMark(ctx, marks[0]); err != nil {
		t.Fatalf("ClearProjectionDirtyMark() error = %v", err)
	}
	marks, err = store.ListProjectionDirtyMarks(ctx, "u-dirty", 10)
	if err != nil {
		t.Fatalf("ListProjectionDirtyMarks(after clear) error = %v", err)
	}
	if len(marks) != 0 {
		t.Fatalf("len(marks after clear) = %d, want 0", len(marks))
	}
	if err := store.ClearProjectionDirtyMark(ctx, mark); err != sql.ErrNoRows {
		t.Fatalf("ClearProjectionDirtyMark(missing) error = %v, want sql.ErrNoRows", err)
	}
}

func TestSQLiteStore_PersistMemoryContentGraphDeferredMarksDirtyWithoutProjectionRefresh(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("deferred", now, []graphmodel.GraphNode{
		subjectHorizonNode("driver", "deferred", "油价", "继续上涨", now, graphmodel.GraphRoleDriver),
		subjectHorizonNode("target", "deferred", "美股", "从纪录高位回落", now, graphmodel.GraphRoleTarget),
	})

	if err := store.PersistMemoryContentGraphDeferred(ctx, "u-deferred", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraphDeferred() error = %v", err)
	}
	var contentCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_content_graphs WHERE user_id = ? AND source_external_id = ?`, "u-deferred", "deferred").Scan(&contentCount); err != nil {
		t.Fatalf("memory_content_graphs count query error = %v", err)
	}
	if contentCount != 1 {
		t.Fatalf("content graph count = %d, want 1", contentCount)
	}
	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_graphs WHERE user_id = ?`, "u-deferred").Scan(&eventCount); err != nil {
		t.Fatalf("event_graphs count query error = %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("event graph count = %d, want no synchronous projection", eventCount)
	}
	marks, err := store.ListProjectionDirtyMarks(ctx, "u-deferred", 100)
	if err != nil {
		t.Fatalf("ListProjectionDirtyMarks() error = %v", err)
	}
	if len(marks) == 0 {
		t.Fatal("dirty marks empty, want deferred projection marks")
	}
	if !hasDirtyMark(marks, "event", "", "") || !hasDirtyMark(marks, "subject-horizon", "美股", "1w") || !hasDirtyMark(marks, "subject-experience", "美股", "") {
		t.Fatalf("dirty marks = %#v, want event plus subject horizon/experience marks", marks)
	}
}

func TestSQLiteStore_RunProjectionDirtySweepRefreshesDeferredMarks(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("sweep", now, []graphmodel.GraphNode{
		subjectHorizonNode("driver", "sweep", "油价", "继续上涨", now, graphmodel.GraphRoleDriver),
		subjectHorizonNode("target", "sweep", "美股", "从纪录高位回落", now, graphmodel.GraphRoleTarget),
	})

	if err := store.PersistMemoryContentGraphDeferred(ctx, "u-sweep", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraphDeferred() error = %v", err)
	}
	result, err := store.RunProjectionDirtySweep(ctx, "u-sweep", 100, now)
	if err != nil {
		t.Fatalf("RunProjectionDirtySweep() error = %v; result = %#v", err, result)
	}
	if result.Scanned == 0 || result.Completed != result.Scanned || result.Failed != 0 || result.Remaining != 0 {
		t.Fatalf("sweep result = %#v, want all scanned marks completed", result)
	}
	if result.Layers["event"] != 1 || result.Layers["subject-horizon"] == 0 || result.Layers["subject-experience"] == 0 {
		t.Fatalf("sweep layers = %#v, want event and subject cache layers refreshed", result.Layers)
	}
	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_graphs WHERE user_id = ?`, "u-sweep").Scan(&eventCount); err != nil {
		t.Fatalf("event_graphs count query error = %v", err)
	}
	if eventCount == 0 {
		t.Fatal("event graph count = 0, want refreshed projection")
	}
	var horizonCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subject_horizon_memories WHERE user_id = ? AND canonical_subject = ? AND horizon = ?`, "u-sweep", "美股", "1w").Scan(&horizonCount); err != nil {
		t.Fatalf("subject_horizon_memories count query error = %v", err)
	}
	if horizonCount != 1 {
		t.Fatalf("subject horizon cache count = %d, want 1", horizonCount)
	}
}

func TestSQLiteStore_RunProjectionDirtySweepAvoidsDuplicateBaseProjectionRefresh(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("dedupe", now, []graphmodel.GraphNode{
		subjectHorizonNode("driver", "dedupe", "油价", "继续上涨", now, graphmodel.GraphRoleDriver),
		subjectHorizonNode("target", "dedupe", "美股", "从纪录高位回落", now, graphmodel.GraphRoleTarget),
	})

	if err := store.PersistMemoryContentGraphDeferred(ctx, "u-sweep-dedupe", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraphDeferred() error = %v", err)
	}
	marks, err := store.ListProjectionDirtyMarks(ctx, "u-sweep-dedupe", 100)
	if err != nil {
		t.Fatalf("ListProjectionDirtyMarks() error = %v", err)
	}
	for _, mark := range marks {
		switch mark.Layer {
		case "event", "paradigm", "global-v2":
		default:
			if err := store.ClearProjectionDirtyMark(ctx, mark); err != nil {
				t.Fatalf("ClearProjectionDirtyMark(%s) error = %v", mark.Layer, err)
			}
		}
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TEMP TABLE projection_refresh_counts(kind TEXT NOT NULL)`); err != nil {
		t.Fatalf("create temp count table error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TEMP TRIGGER count_event_graph_updates AFTER UPDATE ON event_graphs BEGIN INSERT INTO projection_refresh_counts(kind) VALUES ('event_update'); END`); err != nil {
		t.Fatalf("create event update trigger error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TEMP TRIGGER count_paradigm_updates AFTER UPDATE ON paradigms BEGIN INSERT INTO projection_refresh_counts(kind) VALUES ('paradigm_update'); END`); err != nil {
		t.Fatalf("create paradigm update trigger error = %v", err)
	}

	result, err := store.RunProjectionDirtySweep(ctx, "u-sweep-dedupe", 100, now)
	if err != nil {
		t.Fatalf("RunProjectionDirtySweep() error = %v; result = %#v", err, result)
	}
	var duplicateRefreshes int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projection_refresh_counts WHERE kind IN ('event_update', 'paradigm_update')`).Scan(&duplicateRefreshes); err != nil {
		t.Fatalf("duplicate refresh count query error = %v", err)
	}
	if duplicateRefreshes != 0 {
		t.Fatalf("duplicate base projection refresh updates = %d, want 0", duplicateRefreshes)
	}
}

func hasDirtyMark(marks []ProjectionDirtyMark, layer, subject, horizon string) bool {
	for _, mark := range marks {
		if mark.Layer == layer && mark.Subject == subject && mark.Horizon == horizon {
			return true
		}
	}
	return false
}
