package contentstore

import (
	"context"
	"database/sql"
	"sync/atomic"
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

func TestSQLiteStore_HasProjectionDirtyMarkUsesExactOptionalDimensions(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 1, 0, 0, 0, time.UTC)
	if err := store.MarkProjectionDirty(ctx, ProjectionDirtyMark{UserID: "u-dirty-exists", Layer: "subject-horizon", Subject: "美股", Horizon: "1w"}, now); err != nil {
		t.Fatalf("MarkProjectionDirty() error = %v", err)
	}

	got, err := store.HasProjectionDirtyMark(ctx, "u-dirty-exists", "subject-horizon", "美股", "1w")
	if err != nil {
		t.Fatalf("HasProjectionDirtyMark(exact) error = %v", err)
	}
	if !got {
		t.Fatal("HasProjectionDirtyMark(exact) = false, want true")
	}
	got, err = store.HasProjectionDirtyMark(ctx, "u-dirty-exists", "subject-horizon", "美股", "")
	if err != nil {
		t.Fatalf("HasProjectionDirtyMark(any horizon) error = %v", err)
	}
	if !got {
		t.Fatal("HasProjectionDirtyMark(any horizon) = false, want true")
	}
	got, err = store.HasProjectionDirtyMark(ctx, "u-dirty-exists", "subject-horizon", "美股", "1m")
	if err != nil {
		t.Fatalf("HasProjectionDirtyMark(other horizon) error = %v", err)
	}
	if got {
		t.Fatal("HasProjectionDirtyMark(other horizon) = true, want false")
	}
}

func TestSQLiteStore_ProjectionDirtyMarksHasUserSweepIndex(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	var name string
	err := store.db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, "idx_projection_dirty_marks_user_pending").Scan(&name)
	if err != nil {
		t.Fatalf("user pending dirty mark index lookup error = %v", err)
	}
	if name != "idx_projection_dirty_marks_user_pending" {
		t.Fatalf("index name = %q, want idx_projection_dirty_marks_user_pending", name)
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
	if hasDirtyMark(marks, "subject-timeline", "美股", "") {
		t.Fatalf("dirty marks = %#v, want no subject-timeline mark because timelines are computed on demand", marks)
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

func TestSQLiteStore_RunProjectionDirtyMarkStoresRefreshedSubjectHorizonForExperience(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("state-horizon", now, []graphmodel.GraphNode{
		subjectHorizonNode("target", "state-horizon", "美股", "继续走强", now, graphmodel.GraphRoleTarget),
	})
	if err := store.PersistMemoryContentGraphDeferred(ctx, "u-state-horizon", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraphDeferred() error = %v", err)
	}
	state := &projectionDirtyUserState{}
	err := store.runProjectionDirtyMark(ctx, ProjectionDirtyMark{UserID: "u-state-horizon", Layer: "subject-horizon", Subject: "美股", Horizon: "1w"}, now, state)
	if err != nil {
		t.Fatalf("runProjectionDirtyMark(subject-horizon) error = %v", err)
	}
	preloaded := state.preloadedSubjectHorizons("美股", []string{"1w"})
	if got := preloaded["1w"].SampleCount; got != 1 {
		t.Fatalf("preloaded 1w SampleCount = %d, want refreshed horizon memory in sweep state", got)
	}
}

func TestRunProjectionDirtyMarkGroupsProcessesDifferentUsersConcurrently(t *testing.T) {
	marks := []ProjectionDirtyMark{
		{ID: 1, UserID: "u-concurrent-a", Layer: "subject-timeline", Subject: "美股"},
		{ID: 2, UserID: "u-concurrent-b", Layer: "subject-timeline", Subject: "黄金"},
	}
	started := make(chan struct{}, len(marks))
	release := make(chan struct{})
	var active int32
	var maxActive int32
	runner := func(ctx context.Context, mark ProjectionDirtyMark, state *projectionDirtyUserState) error {
		nowActive := atomic.AddInt32(&active, 1)
		for {
			previous := atomic.LoadInt32(&maxActive)
			if nowActive <= previous || atomic.CompareAndSwapInt32(&maxActive, previous, nowActive) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		atomic.AddInt32(&active, -1)
		return nil
	}
	clearer := func(context.Context, []ProjectionDirtyMark) error { return nil }
	done := make(chan ProjectionDirtySweepResult, 1)

	go func() {
		done <- runProjectionDirtyMarkGroups(context.Background(), marks, 2, runner, clearer)
	}()
	<-started
	<-started
	if got := atomic.LoadInt32(&maxActive); got < 2 {
		t.Fatalf("max active runners = %d, want different users processed concurrently", got)
	}
	close(release)
	result := <-done
	if result.Completed != 2 || result.Failed != 0 {
		t.Fatalf("result = %#v, want both marks completed", result)
	}
}

func TestRunProjectionDirtyMarkGroupProcessesSubjectHorizonsConcurrentlyBeforeExperience(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	marks := []ProjectionDirtyMark{
		{ID: 1, UserID: "u-subject-concurrent", Layer: "event"},
		{ID: 2, UserID: "u-subject-concurrent", Layer: "subject-horizon", Subject: "美股", Horizon: "1w"},
		{ID: 3, UserID: "u-subject-concurrent", Layer: "subject-horizon", Subject: "美股", Horizon: "1m"},
		{ID: 4, UserID: "u-subject-concurrent", Layer: "subject-experience", Subject: "美股"},
	}
	horizonStarted := make(chan struct{}, 2)
	releaseHorizons := make(chan struct{})
	done := make(chan ProjectionDirtySweepResult, 1)
	var eventDone int32
	var activeHorizons int32
	var maxActiveHorizons int32
	var experienceStarted int32
	runner := func(ctx context.Context, mark ProjectionDirtyMark, state *projectionDirtyUserState) error {
		switch mark.Layer {
		case "event":
			atomic.StoreInt32(&eventDone, 1)
			return nil
		case "subject-horizon":
			if atomic.LoadInt32(&eventDone) != 1 {
				t.Errorf("subject-horizon started before event completed")
			}
			nowActive := atomic.AddInt32(&activeHorizons, 1)
			for {
				previous := atomic.LoadInt32(&maxActiveHorizons)
				if nowActive <= previous || atomic.CompareAndSwapInt32(&maxActiveHorizons, previous, nowActive) {
					break
				}
			}
			horizonStarted <- struct{}{}
			select {
			case <-releaseHorizons:
			case <-ctx.Done():
				return ctx.Err()
			}
			atomic.AddInt32(&activeHorizons, -1)
			return nil
		case "subject-experience":
			if got := atomic.LoadInt32(&activeHorizons); got != 0 {
				t.Errorf("subject-experience started while %d horizons are still active", got)
			}
			atomic.StoreInt32(&experienceStarted, 1)
			return nil
		default:
			t.Fatalf("unexpected layer %q", mark.Layer)
			return nil
		}
	}
	clearer := func(context.Context, []ProjectionDirtyMark) error { return nil }

	go func() {
		done <- runProjectionDirtyMarkGroup(ctx, marks, runner, clearer)
	}()
	select {
	case <-horizonStarted:
	case <-time.After(200 * time.Millisecond):
		close(releaseHorizons)
		t.Fatal("first subject-horizon mark did not start")
	}
	select {
	case <-horizonStarted:
	case <-time.After(200 * time.Millisecond):
		close(releaseHorizons)
		t.Fatal("second subject-horizon mark did not start while first was active")
	}
	if got := atomic.LoadInt32(&maxActiveHorizons); got < 2 {
		t.Fatalf("max active subject-horizon runners = %d, want concurrent horizon refreshes", got)
	}
	if got := atomic.LoadInt32(&experienceStarted); got != 0 {
		t.Fatal("subject-experience started before subject-horizon refreshes were released")
	}
	close(releaseHorizons)
	result := <-done
	if result.Completed != 4 || result.Failed != 0 {
		t.Fatalf("result = %#v, want all marks completed", result)
	}
	if got := atomic.LoadInt32(&experienceStarted); got != 1 {
		t.Fatal("subject-experience did not run after subject horizons completed")
	}
}

func TestProjectionDirtyUserStateMemoryContentGraphsCoalescesConcurrentLoads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state := &projectionDirtyUserState{}
	loadStarted := make(chan struct{}, 2)
	releaseLoad := make(chan struct{})
	var loadCalls int32
	load := func(ctx context.Context, userID string) ([]graphmodel.ContentSubgraph, error) {
		atomic.AddInt32(&loadCalls, 1)
		loadStarted <- struct{}{}
		select {
		case <-releaseLoad:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []graphmodel.ContentSubgraph{{SourceExternalID: "source-1"}}, nil
	}
	done := make(chan []graphmodel.ContentSubgraph, 2)
	errs := make(chan error, 2)
	go func() {
		graphs, err := state.memoryContentGraphs(ctx, "u-graphs", load)
		done <- graphs
		errs <- err
	}()
	select {
	case <-loadStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("first content graph load did not start")
	}
	go func() {
		graphs, err := state.memoryContentGraphs(ctx, "u-graphs", load)
		done <- graphs
		errs <- err
	}()
	select {
	case <-loadStarted:
		t.Fatal("second concurrent content graph request started a duplicate load")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseLoad)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("memoryContentGraphs() error = %v", err)
		}
		if got := <-done; len(got) != 1 || got[0].SourceExternalID != "source-1" {
			t.Fatalf("graphs = %#v, want shared loaded graph", got)
		}
	}
	if got := atomic.LoadInt32(&loadCalls); got != 1 {
		t.Fatalf("load calls = %d, want one coalesced load", got)
	}
	graphs, err := state.memoryContentGraphs(ctx, "u-graphs", load)
	if err != nil {
		t.Fatalf("memoryContentGraphs(cached) error = %v", err)
	}
	if len(graphs) != 1 || atomic.LoadInt32(&loadCalls) != 1 {
		t.Fatalf("cached graphs/load calls = %#v/%d, want cached reuse", graphs, loadCalls)
	}
}

func TestProjectionDirtyUserStateCanonicalSubjectsReuseLookup(t *testing.T) {
	ctx := context.Background()
	state := &projectionDirtyUserState{}
	var calls int32
	resolve := func(context.Context, graphmodel.GraphNode, map[string]string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "美联储", nil
	}
	node := graphmodel.GraphNode{SubjectText: "联储"}
	first, err := state.canonicalGraphNodeSubject(ctx, node, resolve)
	if err != nil {
		t.Fatalf("canonicalGraphNodeSubject(first) error = %v", err)
	}
	second, err := state.canonicalGraphNodeSubject(ctx, node, resolve)
	if err != nil {
		t.Fatalf("canonicalGraphNodeSubject(second) error = %v", err)
	}
	if first != "美联储" || second != "美联储" {
		t.Fatalf("canonical subjects = %q/%q, want 美联储", first, second)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("resolve calls = %d, want shared canonical cache hit", got)
	}
}

func TestRunProjectionDirtyMarkGroupsClearsSuccessfulMarksInOneBatch(t *testing.T) {
	marks := []ProjectionDirtyMark{
		{ID: 1, UserID: "u-batch-clear", Layer: "subject-timeline", Subject: "美股"},
		{ID: 2, UserID: "u-batch-clear", Layer: "subject-horizon", Subject: "美股", Horizon: "1w"},
	}
	clearCalls := 0
	cleared := 0
	runner := func(context.Context, ProjectionDirtyMark, *projectionDirtyUserState) error { return nil }
	clearer := func(_ context.Context, marks []ProjectionDirtyMark) error {
		clearCalls++
		cleared += len(marks)
		return nil
	}

	result := runProjectionDirtyMarkGroups(context.Background(), marks, 1, runner, clearer)

	if result.Completed != 2 || result.Failed != 0 {
		t.Fatalf("result = %#v, want both marks completed", result)
	}
	if clearCalls != 1 || cleared != 2 {
		t.Fatalf("clearCalls/cleared = %d/%d, want one batch clear with both marks", clearCalls, cleared)
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
