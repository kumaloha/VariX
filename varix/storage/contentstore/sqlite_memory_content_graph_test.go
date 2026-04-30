package contentstore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_AcceptMemoryNodesPersistsGraphFirstContentMemorySnapshot(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-memory-graph",
		Source:         "twitter",
		ExternalID:     "mg1",
		RootExternalID: "root-mg1",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}

	gotAccept, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-graph",
		SourcePlatform:   "twitter",
		SourceExternalID: "mg1",
		NodeIDs:          []string{"n1", "n2"},
	})
	if err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if len(gotAccept.Nodes) != 2 {
		t.Fatalf("len(AcceptMemoryNodes().Nodes) = %d, want 2", len(gotAccept.Nodes))
	}

	var payload string
	if err := store.db.QueryRow(`SELECT payload_json FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ?`, "u-graph", "twitter", "mg1").Scan(&payload); err != nil {
		t.Fatalf("QueryRow(memory_content_graphs) error = %v", err)
	}
	var got graphmodel.ContentSubgraph
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("json.Unmarshal(memory_content_graphs payload) error = %v", err)
	}
	if got.ArticleID != "unit-memory-graph" {
		t.Fatalf("ArticleID = %q, want unit-memory-graph", got.ArticleID)
	}
	if len(got.Nodes) != 2 || len(got.Edges) != 1 {
		t.Fatalf("content graph snapshot = %#v, want 2 nodes and 1 edge", got)
	}
}

func TestSQLiteStore_AcceptMemoryNodesDefersProjectionRefreshAndMarksDirty(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-memory-defer",
		Source:         "twitter",
		ExternalID:     "mg-defer",
		RootExternalID: "root-mg-defer",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价上涨", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-accept-defer",
		SourcePlatform:   "twitter",
		SourceExternalID: "mg-defer",
		NodeIDs:          []string{"n1", "n2"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	var eventCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM event_graphs WHERE user_id = ?`, "u-accept-defer").Scan(&eventCount); err != nil {
		t.Fatalf("event graph count query error = %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("event graph count = %d, want accept to defer projection refresh", eventCount)
	}
	marks, err := store.ListProjectionDirtyMarks(context.Background(), "u-accept-defer", 100)
	if err != nil {
		t.Fatalf("ListProjectionDirtyMarks() error = %v", err)
	}
	if !hasDirtyMark(marks, "event", "", "") || !hasDirtyMark(marks, "paradigm", "", "") {
		t.Fatalf("dirty marks = %#v, want event and paradigm pending marks", marks)
	}
}

func TestSQLiteStore_AcceptMemoryNodesUpdatesExistingGraphFirstContentMemorySnapshot(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-memory-graph-2",
		Source:         "twitter",
		ExternalID:     "mg2",
		RootExternalID: "root-mg2",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph", SourcePlatform: "twitter", SourceExternalID: "mg2", NodeIDs: []string{"n1"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes(first) error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "mg2", graphmodel.VerifyVerdict{ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n2", Verdict: graphmodel.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph", SourcePlatform: "twitter", SourceExternalID: "mg2", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes(second) error = %v", err)
	}

	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ?`, "u-graph", "twitter", "mg2").Scan(&count); err != nil {
		t.Fatalf("QueryRow(memory_content_graphs count) error = %v", err)
	}
	if count != 1 {
		t.Fatalf("memory_content_graphs count = %d, want 1", count)
	}
	var payload string
	if err := store.db.QueryRow(`SELECT payload_json FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ?`, "u-graph", "twitter", "mg2").Scan(&payload); err != nil {
		t.Fatalf("QueryRow(memory_content_graphs payload) error = %v", err)
	}
	var got graphmodel.ContentSubgraph
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	var verdict graphmodel.VerificationStatus
	for _, node := range got.Nodes {
		if node.ID == "n2" {
			verdict = node.VerificationStatus
		}
	}
	if verdict != graphmodel.VerificationProved {
		t.Fatalf("n2 verification_status = %q, want proved", verdict)
	}
}

func TestSQLiteStore_ApplyVerifyVerdictAlsoRefreshesMemoryContentGraphSnapshot(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-memory-graph-3",
		Source:         "twitter",
		ExternalID:     "mg3",
		RootExternalID: "root-mg3",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph-refresh", SourcePlatform: "twitter", SourceExternalID: "mg3", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "mg3", graphmodel.VerifyVerdict{ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n2", Verdict: graphmodel.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph() error = %v", err)
	}

	var payload string
	if err := store.db.QueryRow(`SELECT payload_json FROM memory_content_graphs WHERE user_id = ? AND source_platform = ? AND source_external_id = ?`, "u-graph-refresh", "twitter", "mg3").Scan(&payload); err != nil {
		t.Fatalf("QueryRow(memory_content_graphs payload) error = %v", err)
	}
	var got graphmodel.ContentSubgraph
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	var verdict graphmodel.VerificationStatus
	for _, node := range got.Nodes {
		if node.ID == "n2" {
			verdict = node.VerificationStatus
		}
	}
	if verdict != graphmodel.VerificationProved {
		t.Fatalf("memory_content_graphs n2 verification_status = %q, want proved", verdict)
	}
}

func TestSQLiteStore_PersistMemoryContentGraphAlsoProjectsEventAndParadigmLayers(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	subgraph := graphmodel.ContentSubgraph{
		ID:               "mg-auto-1",
		ArticleID:        "mg-auto-1",
		SourcePlatform:   "twitter",
		SourceExternalID: "mg-auto-1",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "n1", SourceArticleID: "mg-auto-1", SourcePlatform: "twitter", SourceExternalID: "mg-auto-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "mg-auto-1", SourcePlatform: "twitter", SourceExternalID: "mg-auto-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-auto-project", subgraph, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	events, err := store.ListEventGraphs(context.Background(), "u-auto-project")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2 auto projected event graphs", len(events))
	}
	paradigms, err := store.ListParadigms(context.Background(), "u-auto-project")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(paradigms) != 1 {
		t.Fatalf("len(ListParadigms()) = %d, want 1 auto projected paradigm", len(paradigms))
	}
}

func TestSQLiteStore_PersistMemoryContentGraphFromCompiledOutputPersistsBySource(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-memory-graph-run",
		Source:         "twitter",
		ExternalID:     "mgrun1",
		RootExternalID: "root-mgrun1",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: compile.Output{
			Summary: "summary text",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{{ID: "n1", Kind: compile.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)}, {ID: "n2", Kind: compile.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)}},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePositive}},
			},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), "u-content-run", "twitter", "mgrun1", time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("PersistMemoryContentGraphFromCompiledOutput() error = %v", err)
	}
	items, err := store.ListMemoryContentGraphs(context.Background(), "u-content-run")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphs() error = %v", err)
	}
	if len(items) != 1 || items[0].SourceExternalID != "mgrun1" {
		t.Fatalf("content graphs = %#v, want mgrun1 snapshot", items)
	}
}

func TestSQLiteStore_ListMemoryContentGraphsBySubjectFiltersSnapshots(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	for _, sg := range []graphmodel.ContentSubgraph{
		{ID: "subject-cg-1", ArticleID: "subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-cg-1", SourcePlatform: "twitter", SourceExternalID: "subject-cg-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		{ID: "subject-cg-2", ArticleID: "subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-cg-2", SourcePlatform: "twitter", SourceExternalID: "subject-cg-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-cg", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	items, err := store.ListMemoryContentGraphsBySubject(context.Background(), "u-subject-cg", "美联储")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].SourceExternalID != "subject-cg-1" {
		t.Fatalf("filtered content graphs = %#v, want 美联储 snapshot only", items)
	}
}

func TestSQLiteStore_ListMemoryContentGraphsBySubjectSupportsAliasLookup(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
		EntityID:      "driver-fed",
		EntityType:    memory.CanonicalEntityDriver,
		CanonicalName: "美联储",
		Aliases:       []string{"联储"},
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("UpsertCanonicalEntity() error = %v", err)
	}
	sg := graphmodel.ContentSubgraph{
		ID:               "subject-alias-cg",
		ArticleID:        "subject-alias-cg",
		SourcePlatform:   "twitter",
		SourceExternalID: "subject-alias-cg",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{{
			ID:                 "n1",
			SourceArticleID:    "subject-alias-cg",
			SourcePlatform:     "twitter",
			SourceExternalID:   "subject-alias-cg",
			RawText:            "美联储加息0.25%",
			SubjectText:        "美联储",
			ChangeText:         "加息0.25%",
			Kind:               graphmodel.NodeKindObservation,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		}},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-alias-cg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.ListMemoryContentGraphsBySubject(context.Background(), "u-subject-alias-cg", "联储")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].SourceExternalID != "subject-alias-cg" {
		t.Fatalf("items = %#v, want alias lookup to return matching content graph", items)
	}
}

func TestSQLiteStore_MemoryContentGraphSubjectsIndexTracksPersistAndUpdate(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	first := graphmodel.ContentSubgraph{
		ID:               "indexed-cg",
		ArticleID:        "indexed-cg",
		SourcePlatform:   "twitter",
		SourceExternalID: "indexed-cg",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{{
			ID:                 "n1",
			SourceArticleID:    "indexed-cg",
			SourcePlatform:     "twitter",
			SourceExternalID:   "indexed-cg",
			RawText:            "美联储加息",
			SubjectText:        "美联储",
			SubjectCanonical:   "美联储",
			ChangeText:         "加息",
			Kind:               graphmodel.NodeKindObservation,
			IsPrimary:          true,
			VerificationStatus: graphmodel.VerificationPending,
		}},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-index-cg", first, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph(first) error = %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM memory_content_graph_subjects WHERE user_id = ? AND subject = ?`, "u-index-cg", "美联储").Scan(&count); err != nil {
		t.Fatalf("subject index query error = %v", err)
	}
	if count != 1 {
		t.Fatalf("美联储 subject index count = %d, want 1", count)
	}

	second := first
	second.Nodes = []graphmodel.GraphNode{{
		ID:                 "n2",
		SourceArticleID:    "indexed-cg",
		SourcePlatform:     "twitter",
		SourceExternalID:   "indexed-cg",
		RawText:            "欧洲央行放缓缩表",
		SubjectText:        "欧洲央行",
		SubjectCanonical:   "欧洲央行",
		ChangeText:         "放缓缩表",
		Kind:               graphmodel.NodeKindObservation,
		IsPrimary:          true,
		VerificationStatus: graphmodel.VerificationPending,
	}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-index-cg", second, now.Add(time.Hour)); err != nil {
		t.Fatalf("PersistMemoryContentGraph(second) error = %v", err)
	}
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM memory_content_graph_subjects WHERE user_id = ? AND subject = ?`, "u-index-cg", "美联储").Scan(&count); err != nil {
		t.Fatalf("old subject index query error = %v", err)
	}
	if count != 0 {
		t.Fatalf("old subject index count = %d, want removed on graph update", count)
	}
	items, err := store.ListMemoryContentGraphsBySubject(context.Background(), "u-index-cg", "欧洲央行")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].SourceExternalID != "indexed-cg" {
		t.Fatalf("indexed subject items = %#v, want updated graph", items)
	}
}

func TestSQLiteStore_ListMemoryContentGraphsBySourceAndSubjectUsesIntersection(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	for _, sg := range []graphmodel.ContentSubgraph{
		{ID: "cg-ss-1", ArticleID: "cg-ss-1", SourcePlatform: "twitter", SourceExternalID: "cg-ss-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "cg-ss-1", SourcePlatform: "twitter", SourceExternalID: "cg-ss-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		{ID: "cg-ss-2", ArticleID: "cg-ss-2", SourcePlatform: "twitter", SourceExternalID: "cg-ss-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "cg-ss-2", SourcePlatform: "twitter", SourceExternalID: "cg-ss-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-cg-ss", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	items, err := store.ListMemoryContentGraphsBySourceAndSubject(context.Background(), "u-cg-ss", "twitter", "cg-ss-2", "美联储")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphsBySourceAndSubject() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want empty intersection", items)
	}
}

func TestSQLiteStore_ListMemoryContentGraphsBySourceAndSubjectSupportsAliasLookup(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
		EntityID:      "driver-fed",
		EntityType:    memory.CanonicalEntityDriver,
		CanonicalName: "美联储",
		Aliases:       []string{"联储"},
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("UpsertCanonicalEntity() error = %v", err)
	}
	for _, sg := range []graphmodel.ContentSubgraph{
		{ID: "cg-alias-ss-1", ArticleID: "cg-alias-ss-1", SourcePlatform: "twitter", SourceExternalID: "cg-alias-ss-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "cg-alias-ss-1", SourcePlatform: "twitter", SourceExternalID: "cg-alias-ss-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
		{ID: "cg-alias-ss-2", ArticleID: "cg-alias-ss-2", SourcePlatform: "twitter", SourceExternalID: "cg-alias-ss-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "cg-alias-ss-2", SourcePlatform: "twitter", SourceExternalID: "cg-alias-ss-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending}}},
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-cg-alias-ss", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	items, err := store.ListMemoryContentGraphsBySourceAndSubject(context.Background(), "u-cg-alias-ss", "twitter", "cg-alias-ss-1", "联储")
	if err != nil {
		t.Fatalf("ListMemoryContentGraphsBySourceAndSubject() error = %v", err)
	}
	if len(items) != 1 || items[0].SourceExternalID != "cg-alias-ss-1" {
		t.Fatalf("items = %#v, want alias lookup to return cg-alias-ss-1 only", items)
	}
}
