package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_BuildSubjectTimelineGroupsChangesBySubject(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	graphs := []graphmodel.ContentSubgraph{
		subjectTimelineSubgraph("sg-1", now, []graphmodel.GraphNode{{
			ID:                 "n1",
			SourceArticleID:    "sg-1",
			SourcePlatform:     "twitter",
			SourceExternalID:   "sg-1",
			RawText:            "未来一周美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			TimeStart:          now.Add(-24 * time.Hour).Format(time.RFC3339),
			Kind:               graphmodel.NodeKindPrediction,
			GraphRole:          graphmodel.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		}}),
		subjectTimelineSubgraph("sg-2", now, []graphmodel.GraphNode{{
			ID:                 "n2",
			SourceArticleID:    "sg-2",
			SourcePlatform:     "twitter",
			SourceExternalID:   "sg-2",
			RawText:            "美股继续承压",
			SubjectText:        "美股",
			ChangeText:         "继续承压",
			TimeStart:          now.Format(time.RFC3339),
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationProved,
			VerificationReason: "observed drawdown",
		}}),
		subjectTimelineSubgraph("sg-3", now, []graphmodel.GraphNode{{
			ID:                 "n3",
			SourceArticleID:    "sg-3",
			SourcePlatform:     "twitter",
			SourceExternalID:   "sg-3",
			RawText:            "欧洲央行放缓缩表",
			SubjectText:        "欧洲央行",
			ChangeText:         "放缓缩表",
			TimeStart:          now.Format(time.RFC3339),
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleDriver,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		}}),
	}
	for _, graph := range graphs {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-timeline", graph, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", graph.ID, err)
		}
	}

	timeline, err := store.BuildSubjectTimeline(context.Background(), "u-timeline", "美股", now)
	if err != nil {
		t.Fatalf("BuildSubjectTimeline() error = %v", err)
	}
	if len(timeline.Entries) != 2 {
		t.Fatalf("len(timeline.Entries) = %d, want 2: %#v", len(timeline.Entries), timeline.Entries)
	}
	if timeline.Entries[0].ChangeText != "承压" || timeline.Entries[1].ChangeText != "继续承压" {
		t.Fatalf("timeline change order = %#v, want 承压 then 继续承压", timeline.Entries)
	}
	if timeline.Entries[1].VerificationStatus != string(graphmodel.VerificationProved) || timeline.Entries[1].VerificationReason != "observed drawdown" {
		t.Fatalf("timeline entry provenance/verdict = %#v", timeline.Entries[1])
	}
	if timeline.Entries[1].RelationToPrior != memory.SubjectChangeUpdates {
		t.Fatalf("second relation = %q, want updates", timeline.Entries[1].RelationToPrior)
	}
}

func TestSQLiteStore_BuildSubjectTimelineMarksLegacyMirroredNodeLowStructure(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("legacy", now, []graphmodel.GraphNode{{
		ID:                 "n1",
		SourceArticleID:    "legacy",
		SourcePlatform:     "twitter",
		SourceExternalID:   "legacy",
		RawText:            "未来一周美股承压",
		SubjectText:        "未来一周美股承压",
		ChangeText:         "未来一周美股承压",
		Kind:               graphmodel.NodeKindPrediction,
		GraphRole:          graphmodel.GraphRoleTarget,
		IsPrimary:          true,
		VerificationStatus: graphmodel.VerificationPending,
	}})
	if err := store.PersistMemoryContentGraph(context.Background(), "u-legacy-timeline", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}

	timeline, err := store.BuildSubjectTimeline(context.Background(), "u-legacy-timeline", "未来一周美股承压", now)
	if err != nil {
		t.Fatalf("BuildSubjectTimeline() error = %v", err)
	}
	if len(timeline.Entries) != 1 {
		t.Fatalf("len(timeline.Entries) = %d, want 1", len(timeline.Entries))
	}
	entry := timeline.Entries[0]
	if entry.Structure != memory.SubjectChangeLowStructure {
		t.Fatalf("entry.Structure = %q, want low_structure", entry.Structure)
	}
	if entry.RelationToPrior != memory.SubjectChangeRelationLowStructure {
		t.Fatalf("entry.RelationToPrior = %q, want low_structure", entry.RelationToPrior)
	}
}

func TestSQLiteStore_BuildSubjectTimelineClassifiesContradictoryChange(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("contradict", now, []graphmodel.GraphNode{
		{
			ID:                 "n1",
			SourceArticleID:    "contradict",
			SourcePlatform:     "twitter",
			SourceExternalID:   "contradict",
			RawText:            "美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			TimeStart:          now.Add(-24 * time.Hour).Format(time.RFC3339),
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		},
		{
			ID:                 "n2",
			SourceArticleID:    "contradict",
			SourcePlatform:     "twitter",
			SourceExternalID:   "contradict",
			RawText:            "美股反弹",
			SubjectText:        "美股",
			ChangeText:         "反弹",
			TimeStart:          now.Format(time.RFC3339),
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		},
	})
	if err := store.PersistMemoryContentGraph(context.Background(), "u-contradict-timeline", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}

	timeline, err := store.BuildSubjectTimeline(context.Background(), "u-contradict-timeline", "美股", now)
	if err != nil {
		t.Fatalf("BuildSubjectTimeline() error = %v", err)
	}
	if len(timeline.Entries) != 2 {
		t.Fatalf("len(timeline.Entries) = %d, want 2", len(timeline.Entries))
	}
	if timeline.Entries[1].RelationToPrior != memory.SubjectChangeContradicts {
		t.Fatalf("second relation = %q, want contradicts", timeline.Entries[1].RelationToPrior)
	}
}

func TestSQLiteStore_BuildSubjectTimelineExcludesNonPrimaryAndContextNodes(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	graph := subjectTimelineSubgraph("roles", now, []graphmodel.GraphNode{
		{
			ID:                 "primary",
			SourceArticleID:    "roles",
			SourcePlatform:     "twitter",
			SourceExternalID:   "roles",
			RawText:            "美股承压",
			SubjectText:        "美股",
			ChangeText:         "承压",
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleTarget,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		},
		{
			ID:                 "context",
			SourceArticleID:    "roles",
			SourcePlatform:     "twitter",
			SourceExternalID:   "roles",
			RawText:            "美股历史估值偏高",
			SubjectText:        "美股",
			ChangeText:         "历史估值偏高",
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleContext,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		},
		{
			ID:                 "non-primary",
			SourceArticleID:    "roles",
			SourcePlatform:     "twitter",
			SourceExternalID:   "roles",
			RawText:            "美股成交量放大",
			SubjectText:        "美股",
			ChangeText:         "成交量放大",
			Kind:               graphmodel.NodeKindObservation,
			GraphRole:          graphmodel.GraphRoleTarget,
			IsPrimary:          false,
			VerificationStatus: graphmodel.VerificationPending,
		},
	})
	if err := store.PersistMemoryContentGraph(context.Background(), "u-role-filter", graph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}

	timeline, err := store.BuildSubjectTimeline(context.Background(), "u-role-filter", "美股", now)
	if err != nil {
		t.Fatalf("BuildSubjectTimeline() error = %v", err)
	}
	if len(timeline.Entries) != 1 || timeline.Entries[0].NodeID != "primary" {
		t.Fatalf("timeline entries = %#v, want only primary driver/target node", timeline.Entries)
	}
}

func newSubjectTimelineTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func subjectTimelineSubgraph(id string, now time.Time, nodes []graphmodel.GraphNode) graphmodel.ContentSubgraph {
	return graphmodel.ContentSubgraph{
		ID:               id,
		ArticleID:        id,
		SourcePlatform:   "twitter",
		SourceExternalID: id,
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes:            nodes,
	}
}
