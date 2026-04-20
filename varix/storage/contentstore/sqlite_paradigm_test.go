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

func TestSQLiteStore_RunParadigmProjectionBuildsGroupedParadigms(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg1 := graphmodel.ContentSubgraph{
		ID:               "psg1",
		ArticleID:        "unit-p1",
		SourcePlatform:   "twitter",
		SourceExternalID: "p1",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "d1", SourceArticleID: "unit-p1", SourcePlatform: "twitter", SourceExternalID: "p1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-p1", SourcePlatform: "twitter", SourceExternalID: "p1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"},
		},
	}
	sg2 := graphmodel.ContentSubgraph{
		ID:               "psg2",
		ArticleID:        "unit-p2",
		SourcePlatform:   "twitter",
		SourceExternalID: "p2",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "d2", SourceArticleID: "unit-p2", SourcePlatform: "twitter", SourceExternalID: "p2", RawText: "联储继续收紧", SubjectText: "美联储", ChangeText: "继续收紧", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "t2", SourceArticleID: "unit-p2", SourcePlatform: "twitter", SourceExternalID: "p2", RawText: "最近一周美股回撤", SubjectText: "美股", ChangeText: "回撤", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationDisproved, TimeBucket: "1w"},
		},
	}
	for _, sg := range []graphmodel.ContentSubgraph{sg1, sg2} {
		if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
			t.Fatalf("UpsertContentSubgraph(%s) error = %v", sg.ID, err)
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}

	paradigms, err := store.RunParadigmProjection(context.Background(), "u-paradigm", now)
	if err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	if len(paradigms) != 1 {
		t.Fatalf("len(RunParadigmProjection()) = %d, want 1 grouped paradigm", len(paradigms))
	}
	got := paradigms[0]
	if got.DriverSubject != "美联储" || got.TargetSubject != "美股" {
		t.Fatalf("paradigm = %#v, want 美联储 -> 美股", got)
	}
	if got.SupportingSubgraphCount != 2 {
		t.Fatalf("SupportingSubgraphCount = %d, want 2", got.SupportingSubgraphCount)
	}
	if len(got.SupportingSubgraphIDs) != 2 {
		t.Fatalf("SupportingSubgraphIDs = %#v, want 2 ids", got.SupportingSubgraphIDs)
	}
	if got.SuccessCount != 1 || got.FailureCount != 1 {
		t.Fatalf("success/failure = %d/%d, want 1/1", got.SuccessCount, got.FailureCount)
	}
	if got.CredibilityState == "" {
		t.Fatalf("CredibilityState = empty, want non-empty")
	}
	if len(got.TraceabilityMap) != 2 {
		t.Fatalf("TraceabilityMap = %#v, want 2 source entries", got.TraceabilityMap)
	}
	if got.SupportingEventGraphCount != 1 {
		t.Fatalf("SupportingEventGraphCount = %d, want 1", got.SupportingEventGraphCount)
	}

	persisted, err := store.ListParadigms(context.Background(), "u-paradigm")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("len(ListParadigms()) = %d, want 1", len(persisted))
	}
}

func TestSQLiteStore_AcceptMemoryNodesAlsoProjectsParadigms(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-paradigm-accept",
		Source:         "twitter",
		ExternalID:     "pa1",
		RootExternalID: "root-pa1",
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
			Drivers:      []string{"美联储加息0.25%"},
			Targets:      []string{"未来一周美股承压"},
			Details:      compile.HiddenDetails{Caveats: []string{"detail"}},
			Verification: compile.Verification{NodeVerifications: []compile.NodeVerification{{NodeID: "n2", Status: compile.NodeVerificationProved}}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-auto-paradigm", SourcePlatform: "twitter", SourceExternalID: "pa1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	items, err := store.ListParadigms(context.Background(), "u-auto-paradigm")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListParadigms()) = %d, want 1 auto-projected paradigm", len(items))
	}
}

func TestSQLiteStore_ApplyVerifyVerdictAlsoRefreshesParadigms(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "unit-paradigm-refresh",
		Source:         "twitter",
		ExternalID:     "pr1",
		RootExternalID: "root-pr1",
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
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-refresh-paradigm", SourcePlatform: "twitter", SourceExternalID: "pr1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "pr1", graphmodel.VerifyVerdict{ObjectType: graphmodel.VerifyQueueObjectNode, ObjectID: "n2", Verdict: graphmodel.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph() error = %v", err)
	}
	items, err := store.ListParadigms(context.Background(), "u-refresh-paradigm")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListParadigms()) = %d, want 1", len(items))
	}
	if items[0].SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1 after verdict refresh", items[0].SuccessCount)
	}
}

func TestSQLiteStore_ListParadigmsBySubjectFiltersPersistedRows(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{ID: "subject-pg", ArticleID: "subject-pg", SourcePlatform: "twitter", SourceExternalID: "subject-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "subject-pg", SourcePlatform: "twitter", SourceExternalID: "subject-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "subject-pg", SourcePlatform: "twitter", SourceExternalID: "subject-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-subject-pg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.ListParadigmsBySubject(context.Background(), "u-subject-pg", "美联储")
	if err != nil {
		t.Fatalf("ListParadigmsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].DriverSubject != "美联储" {
		t.Fatalf("filtered paradigms = %#v, want 美联储 paradigm", items)
	}
}

func TestSQLiteStore_RunParadigmProjectionPersistsEvidenceLinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{
		ID:               "evidence-pg",
		ArticleID:        "evidence-pg",
		SourcePlatform:   "twitter",
		SourceExternalID: "evidence-pg",
		CompileVersion:   graphmodel.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []graphmodel.GraphNode{
			{ID: "n1", SourceArticleID: "evidence-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "evidence-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-evidence-pg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.RunParadigmProjection(context.Background(), "u-evidence-pg", now)
	if err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(RunParadigmProjection()) = %d, want 1", len(items))
	}
	var linkCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM paradigm_evidence_links WHERE paradigm_id = ?`, items[0].ParadigmID).Scan(&linkCount); err != nil {
		t.Fatalf("QueryRow(paradigm_evidence_links) error = %v", err)
	}
	if linkCount == 0 {
		t.Fatalf("paradigm evidence links count = %d, want > 0", linkCount)
	}
}

func TestSQLiteStore_ListParadigmEvidenceLinksReturnsPersistedLinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := graphmodel.ContentSubgraph{ID: "evidence-list-pg", ArticleID: "evidence-list-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-pg", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "evidence-list-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "evidence-list-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-evidence-list-pg", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	items, err := store.RunParadigmProjection(context.Background(), "u-evidence-list-pg", now)
	if err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	links, err := store.ListParadigmEvidenceLinks(context.Background(), items[0].ParadigmID)
	if err != nil {
		t.Fatalf("ListParadigmEvidenceLinks() error = %v", err)
	}
	if len(links) == 0 {
		t.Fatalf("links = %#v, want non-empty", links)
	}
}

func TestSQLiteStore_ListParadigmEvidenceLinksByUserReturnsAllUserLinks(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	for _, sg := range []graphmodel.ContentSubgraph{
		{ID: "pe-user-1", ArticleID: "pe-user-1", SourcePlatform: "twitter", SourceExternalID: "pe-user-1", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pe-user-1", SourcePlatform: "twitter", SourceExternalID: "pe-user-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-user-1", SourcePlatform: "twitter", SourceExternalID: "pe-user-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
		{ID: "pe-user-2", ArticleID: "pe-user-2", SourcePlatform: "twitter", SourceExternalID: "pe-user-2", CompileVersion: graphmodel.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []graphmodel.GraphNode{{ID: "n1", SourceArticleID: "pe-user-2", SourcePlatform: "twitter", SourceExternalID: "pe-user-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: graphmodel.NodeKindObservation, GraphRole: graphmodel.GraphRoleDriver, IsPrimary: true, VerificationStatus: graphmodel.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-user-2", SourcePlatform: "twitter", SourceExternalID: "pe-user-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: graphmodel.NodeKindPrediction, GraphRole: graphmodel.GraphRoleTarget, IsPrimary: true, VerificationStatus: graphmodel.VerificationProved, TimeBucket: "1w"}}},
	} {
		if err := store.PersistMemoryContentGraph(context.Background(), "u-pe-user", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	links, err := store.ListParadigmEvidenceLinksByUser(context.Background(), "u-pe-user")
	if err != nil {
		t.Fatalf("ListParadigmEvidenceLinksByUser() error = %v", err)
	}
	if len(links) < 2 {
		t.Fatalf("links = %#v, want combined evidence links across user paradigms", links)
	}
}
