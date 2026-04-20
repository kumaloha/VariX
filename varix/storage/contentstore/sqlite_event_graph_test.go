package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_RunEventGraphProjectionPersistsGroupedGraphs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg1 := graphmodel.ContentSubgraph{
		ID:               "sg1",
		ArticleID:        "unit-1",
		SourcePlatform:   "twitter",
		SourceExternalID: "e1",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "d1", SourceArticleID: "unit-1", SourcePlatform: "twitter", SourceExternalID: "e1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-1", SourcePlatform: "twitter", SourceExternalID: "e1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
		},
	}
	sg2 := graphmodel.ContentSubgraph{
		ID:               "sg2",
		ArticleID:        "unit-2",
		SourcePlatform:   "twitter",
		SourceExternalID: "e2",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "d2", SourceArticleID: "unit-2", SourcePlatform: "twitter", SourceExternalID: "e2", RawText: "联储继续收紧", SubjectText: "美联储", ChangeText: "继续收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "t2", SourceArticleID: "unit-2", SourcePlatform: "twitter", SourceExternalID: "e2", RawText: "最近一周美股回撤", SubjectText: "美股", ChangeText: "回撤", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
		},
	}
	for _, sg := range []graphmodel.ContentSubgraph{sg1, sg2} {
		if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
			t.Fatalf("UpsertContentSubgraph(%s) error = %v", sg.ID, err)
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-proj", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}

	graphs, err := store.RunEventGraphProjection(context.Background(), "u-event-proj", now)
	if err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	if len(graphs) != 2 {
		t.Fatalf("len(RunEventGraphProjection()) = %d, want 2", len(graphs))
	}
	byScope := map[string]EventGraphRecord{}
	for _, graph := range graphs {
		byScope[graph.Scope] = graph
	}
	if byScope["driver"].AnchorSubject != "美联储" || len(byScope["driver"].SourceSubgraphIDs) != 2 {
		t.Fatalf("driver graph = %#v, want merged 美联储 graph", byScope["driver"])
	}
	if byScope["driver"].SourceSubgraphCount != 2 || byScope["driver"].PrimaryNodeCount != 2 {
		t.Fatalf("driver summary = %#v, want SourceSubgraphCount=2 PrimaryNodeCount=2", byScope["driver"])
	}
	if len(byScope["driver"].RepresentativeChanges) != 2 {
		t.Fatalf("driver RepresentativeChanges = %#v, want 2 distinct changes", byScope["driver"].RepresentativeChanges)
	}
	if len(byScope["driver"].TraceabilityMap) != 2 {
		t.Fatalf("driver TraceabilityMap = %#v, want 2 source entries", byScope["driver"].TraceabilityMap)
	}
	if byScope["target"].AnchorSubject != "美股" || len(byScope["target"].SourceSubgraphIDs) != 2 {
		t.Fatalf("target graph = %#v, want merged 美股 graph", byScope["target"])
	}
	if byScope["target"].SourceSubgraphCount != 2 || byScope["target"].PrimaryNodeCount != 2 {
		t.Fatalf("target summary = %#v, want SourceSubgraphCount=2 PrimaryNodeCount=2", byScope["target"])
	}
	if len(byScope["target"].RepresentativeChanges) != 2 {
		t.Fatalf("target RepresentativeChanges = %#v, want 2 distinct changes", byScope["target"].RepresentativeChanges)
	}
	if len(byScope["target"].TraceabilityMap) != 2 {
		t.Fatalf("target TraceabilityMap = %#v, want 2 source entries", byScope["target"].TraceabilityMap)
	}

	persisted, err := store.ListEventGraphs(context.Background(), "u-event-proj")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2", len(persisted))
	}
}

func TestSQLiteStore_AcceptMemoryNodesAlsoProjectsEventGraphs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-event-accept",
		Source:         "twitter",
		ExternalID:     "ea1",
		RootExternalID: "root-ea1",
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
			Drivers: []string{"美联储加息0.25%"},
			Targets: []string{"未来一周美股承压"},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-auto-event", SourcePlatform: "twitter", SourceExternalID: "ea1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	graphs, err := store.ListEventGraphs(context.Background(), "u-auto-event")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(graphs) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2 auto-projected event graphs", len(graphs))
	}
}

func TestSQLiteStore_ApplyVerifyVerdictAlsoRefreshesEventGraphs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-event-refresh",
		Source:         "twitter",
		ExternalID:     "er1",
		RootExternalID: "root-er1",
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
			Drivers: []string{"美联储加息0.25%"},
			Targets: []string{"未来一周美股承压"},
			Details: compile.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-event-refresh", SourcePlatform: "twitter", SourceExternalID: "er1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "er1", graphmodel.VerifyVerdict{ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n2", Verdict: graphmodel.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph() error = %v", err)
	}
	graphs, err := store.ListEventGraphs(context.Background(), "u-event-refresh")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(graphs) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2", len(graphs))
	}
	foundTarget := false
	for _, graph := range graphs {
		if graph.Scope != "target" {
			continue
		}
		foundTarget = true
		if graph.VerificationSummary[graphmodel.VerificationProved] != 1 {
			t.Fatalf("target VerificationSummary = %#v, want 1 proved node", graph.VerificationSummary)
		}
	}
	if !foundTarget {
		t.Fatal("target event graph not found")
	}
}

func TestSQLiteStore_ListEventGraphsByScopeFiltersPersistedRows(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{ID: "scope-eg", ArticleID: "scope-eg", SourcePlatform: "twitter", SourceExternalID: "scope-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "scope-eg", SourcePlatform: "twitter", SourceExternalID: "scope-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "scope-eg", SourcePlatform: "twitter", SourceExternalID: "scope-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-scope-eg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.ListEventGraphsByScope(context.Background(), "u-scope-eg", "target")
	if err != nil {
		t.Fatalf("ListEventGraphsByScope() error = %v", err)
	}
	if len(items) != 1 || items[0].Scope != "target" {
		t.Fatalf("filtered event graphs = %#v, want one target graph", items)
	}
}

func TestSQLiteStore_ListEventGraphsBySubjectFiltersPersistedRows(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{ID: "subject-eg", ArticleID: "subject-eg", SourcePlatform: "twitter", SourceExternalID: "subject-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-eg", SourcePlatform: "twitter", SourceExternalID: "subject-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "subject-eg", SourcePlatform: "twitter", SourceExternalID: "subject-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-eg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.ListEventGraphsBySubject(context.Background(), "u-subject-eg", "美联储")
	if err != nil {
		t.Fatalf("ListEventGraphsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].AnchorSubject != "美联储" {
		t.Fatalf("filtered event graphs = %#v, want one 美联储 graph", items)
	}
}

func TestSQLiteStore_RunEventGraphProjectionPersistsEvidenceLinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{
		ID:               "evidence-eg",
		ArticleID:        "evidence-eg",
		SourcePlatform:   "twitter",
		SourceExternalID: "evidence-eg",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "n1", SourceArticleID: "evidence-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "evidence-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-evidence-eg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.RunEventGraphProjection(context.Background(), "u-evidence-eg", now)
	if err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(RunEventGraphProjection()) = %d, want 2", len(items))
	}
	var linkCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM event_graph_evidence_links WHERE event_graph_id = ?`, items[0].EventGraphID).Scan(&linkCount); err != nil {
		t.Fatalf("QueryRow(event_graph_evidence_links) error = %v", err)
	}
	if linkCount == 0 {
		t.Fatalf("event graph evidence links count = %d, want > 0", linkCount)
	}
}

func TestSQLiteStore_ListEventGraphEvidenceLinksReturnsPersistedLinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{ID: "evidence-list-eg", ArticleID: "evidence-list-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-eg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "evidence-list-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "evidence-list-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-evidence-list-eg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	graphs, err := store.RunEventGraphProjection(context.Background(), "u-evidence-list-eg", now)
	if err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	links, err := store.ListEventGraphEvidenceLinks(context.Background(), graphs[0].EventGraphID)
	if err != nil {
		t.Fatalf("ListEventGraphEvidenceLinks() error = %v", err)
	}
	if len(links) == 0 {
		t.Fatalf("links = %#v, want non-empty", links)
	}
}

func TestSQLiteStore_ListEventGraphEvidenceLinksByUserReturnsAllUserLinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	for _, sg := range []graphmodel.ContentSubgraph{
		{ID: "ee-user-1", ArticleID: "ee-user-1", SourcePlatform: "twitter", SourceExternalID: "ee-user-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ee-user-1", SourcePlatform: "twitter", SourceExternalID: "ee-user-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-user-1", SourcePlatform: "twitter", SourceExternalID: "ee-user-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		{ID: "ee-user-2", ArticleID: "ee-user-2", SourcePlatform: "twitter", SourceExternalID: "ee-user-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "ee-user-2", SourcePlatform: "twitter", SourceExternalID: "ee-user-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-user-2", SourcePlatform: "twitter", SourceExternalID: "ee-user-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-ee-user", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	links, err := store.ListEventGraphEvidenceLinksByUser(context.Background(), "u-ee-user")
	if err != nil {
		t.Fatalf("ListEventGraphEvidenceLinksByUser() error = %v", err)
	}
	if len(links) < 4 {
		t.Fatalf("links = %#v, want combined evidence links across user graphs", links)
	}
}
