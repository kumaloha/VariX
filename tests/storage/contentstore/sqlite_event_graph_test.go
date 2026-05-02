package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_RunEventGraphProjectionPersistsGroupedGraphs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	firstAt := now.Add(-24 * time.Hour)
	secondAt := now
	sg1 := model.ContentSubgraph{
		ID:               "sg1",
		ArticleID:        "unit-1",
		SourcePlatform:   "twitter",
		SourceExternalID: "e1",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-1", SourcePlatform: "twitter", SourceExternalID: "e1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", TimeStart: firstAt.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-1", SourcePlatform: "twitter", SourceExternalID: "e1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", TimeStart: firstAt.Format(time.RFC3339), Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
	}
	sg2 := model.ContentSubgraph{
		ID:               "sg2",
		ArticleID:        "unit-2",
		SourcePlatform:   "twitter",
		SourceExternalID: "e2",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d2", SourceArticleID: "unit-2", SourcePlatform: "twitter", SourceExternalID: "e2", RawText: "联储继续收紧", SubjectText: "美联储", ChangeText: "继续收紧", TimeStart: secondAt.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t2", SourceArticleID: "unit-2", SourcePlatform: "twitter", SourceExternalID: "e2", RawText: "最近一周美股回撤", SubjectText: "美股", ChangeText: "回撤", TimeStart: secondAt.Format(time.RFC3339), Kind: model.NodeKindObservation, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
	}
	for _, sg := range []model.ContentSubgraph{sg1, sg2} {
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
	if byScope["target"].TimeStart != firstAt.Format(time.RFC3339) || byScope["target"].TimeEnd != secondAt.Format(time.RFC3339) {
		t.Fatalf("target time window = %s..%s, want %s..%s", byScope["target"].TimeStart, byScope["target"].TimeEnd, firstAt.Format(time.RFC3339), secondAt.Format(time.RFC3339))
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

func TestSQLiteStore_RunEventGraphProjectionHonorsCanonicalAliasMapping(t *testing.T) {
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
		t.Fatalf("UpsertCanonicalEntity(driver) error = %v", err)
	}
	if err := store.UpsertCanonicalEntity(context.Background(), memory.CanonicalEntity{
		EntityID:      "target-us-equity",
		EntityType:    memory.CanonicalEntityTarget,
		CanonicalName: "美股",
		Aliases:       []string{"美国股市"},
		Status:        memory.CanonicalEntityActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("UpsertCanonicalEntity(target) error = %v", err)
	}
	sg := model.ContentSubgraph{
		ID:               "sg-canonical",
		ArticleID:        "unit-canonical",
		SourcePlatform:   "twitter",
		SourceExternalID: "canonical",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-canonical", SourcePlatform: "twitter", SourceExternalID: "canonical", RawText: "联储继续收紧", SubjectText: "联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-canonical", SourcePlatform: "twitter", SourceExternalID: "canonical", RawText: "未来一周美国股市承压", SubjectText: "美国股市", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-event-canonical", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	graphs, err := store.RunEventGraphProjection(context.Background(), "u-event-canonical", now)
	if err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	byScope := map[string]EventGraphRecord{}
	for _, graph := range graphs {
		byScope[graph.Scope] = graph
	}
	if byScope["driver"].AnchorSubject != "美联储" {
		t.Fatalf("driver AnchorSubject = %q, want canonical 美联储", byScope["driver"].AnchorSubject)
	}
	if byScope["target"].AnchorSubject != "美股" {
		t.Fatalf("target AnchorSubject = %q, want canonical 美股", byScope["target"].AnchorSubject)
	}
}

func TestSQLiteStore_RunEventGraphProjectionMergesAliasAndCanonicalSubjects(t *testing.T) {
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
		t.Fatalf("UpsertCanonicalEntity(driver) error = %v", err)
	}
	for _, sg := range []model.ContentSubgraph{
		{ID: "sg-alias", ArticleID: "unit-alias", SourcePlatform: "twitter", SourceExternalID: "alias", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "d1", SourceArticleID: "unit-alias", SourcePlatform: "twitter", SourceExternalID: "alias", RawText: "联储继续收紧", SubjectText: "联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}}},
		{ID: "sg-canonical", ArticleID: "unit-canonical", SourcePlatform: "twitter", SourceExternalID: "canonical", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "d2", SourceArticleID: "unit-canonical", SourcePlatform: "twitter", SourceExternalID: "canonical", RawText: "美联储维持紧缩", SubjectText: "美联储", ChangeText: "维持紧缩", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}}},
	} {
		if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
			t.Fatalf("UpsertContentSubgraph(%s) error = %v", sg.ID, err)
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-event-mixed-canonical", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	graphs, err := store.RunEventGraphProjection(context.Background(), "u-event-mixed-canonical", now)
	if err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	if len(graphs) != 1 {
		t.Fatalf("len(RunEventGraphProjection()) = %d, want 1 merged canonical graph", len(graphs))
	}
	if graphs[0].AnchorSubject != "美联储" || graphs[0].SourceSubgraphCount != 2 {
		t.Fatalf("graph = %#v, want canonical 美联储 with both source subgraphs merged", graphs[0])
	}
}

func TestSQLiteStore_ListEventGraphsBySubjectSupportsAliasLookup(t *testing.T) {
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
		t.Fatalf("UpsertCanonicalEntity(driver) error = %v", err)
	}
	sg := model.ContentSubgraph{
		ID:               "sg-event-lookup",
		ArticleID:        "unit-event-lookup",
		SourcePlatform:   "twitter",
		SourceExternalID: "event-lookup",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-event-lookup", SourcePlatform: "twitter", SourceExternalID: "event-lookup", RawText: "联储继续收紧", SubjectText: "联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-event-lookup", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunEventGraphProjection(context.Background(), "u-event-lookup", now); err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	items, err := store.ListEventGraphsBySubject(context.Background(), "u-event-lookup", "联储")
	if err != nil {
		t.Fatalf("ListEventGraphsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].AnchorSubject != "美联储" {
		t.Fatalf("items = %#v, want alias lookup to return canonical 美联储 event graph", items)
	}
}

func TestSQLiteStore_ProjectionSweepBuildsEventGraphsAfterAccept(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "unit-event-accept",
		Source:         "twitter",
		ExternalID:     "ea1",
		RootExternalID: "root-ea1",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: model.Output{
			Summary: "summary text",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePositive}},
			},
			Drivers: []string{"美联储加息0.25%"},
			Targets: []string{"未来一周美股承压"},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-auto-event", SourcePlatform: "twitter", SourceExternalID: "ea1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if _, err := store.RunProjectionDirtySweep(context.Background(), "u-auto-event", 100, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunProjectionDirtySweep() error = %v", err)
	}
	graphs, err := store.ListEventGraphs(context.Background(), "u-auto-event")
	if err != nil {
		t.Fatalf("ListEventGraphs() error = %v", err)
	}
	if len(graphs) != 2 {
		t.Fatalf("len(ListEventGraphs()) = %d, want 2 event graphs after projection sweep", len(graphs))
	}
}

func TestSQLiteStore_ApplyVerifyVerdictAlsoRefreshesEventGraphs(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "unit-event-refresh",
		Source:         "twitter",
		ExternalID:     "er1",
		RootExternalID: "root-er1",
		Model:          "qwen3.6-plus",
		CompiledAt:     time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
		Output: model.Output{
			Summary: "summary text",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePositive}},
			},
			Drivers: []string{"美联储加息0.25%"},
			Targets: []string{"未来一周美股承压"},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-event-refresh", SourcePlatform: "twitter", SourceExternalID: "er1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "er1", model.VerifyVerdict{ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", Verdict: model.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
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
		if graph.VerificationSummary[model.VerificationProved] != 1 {
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
	sg := model.ContentSubgraph{ID: "scope-eg", ArticleID: "scope-eg", SourcePlatform: "twitter", SourceExternalID: "scope-eg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "scope-eg", SourcePlatform: "twitter", SourceExternalID: "scope-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "scope-eg", SourcePlatform: "twitter", SourceExternalID: "scope-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
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
	sg := model.ContentSubgraph{ID: "subject-eg", ArticleID: "subject-eg", SourcePlatform: "twitter", SourceExternalID: "subject-eg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "subject-eg", SourcePlatform: "twitter", SourceExternalID: "subject-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "subject-eg", SourcePlatform: "twitter", SourceExternalID: "subject-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
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
	sg := model.ContentSubgraph{
		ID:               "evidence-eg",
		ArticleID:        "evidence-eg",
		SourcePlatform:   "twitter",
		SourceExternalID: "evidence-eg",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "evidence-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "evidence-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
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
	sg := model.ContentSubgraph{ID: "evidence-list-eg", ArticleID: "evidence-list-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-eg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "evidence-list-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-eg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "evidence-list-eg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-eg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
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
	for _, sg := range []model.ContentSubgraph{
		{ID: "ee-user-1", ArticleID: "ee-user-1", SourcePlatform: "twitter", SourceExternalID: "ee-user-1", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "ee-user-1", SourcePlatform: "twitter", SourceExternalID: "ee-user-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-user-1", SourcePlatform: "twitter", SourceExternalID: "ee-user-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
		{ID: "ee-user-2", ArticleID: "ee-user-2", SourcePlatform: "twitter", SourceExternalID: "ee-user-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "ee-user-2", SourcePlatform: "twitter", SourceExternalID: "ee-user-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "ee-user-2", SourcePlatform: "twitter", SourceExternalID: "ee-user-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
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
