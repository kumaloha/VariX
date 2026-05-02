package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_RunParadigmProjectionBuildsGroupedParadigms(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg1 := model.ContentSubgraph{
		ID:               "psg1",
		ArticleID:        "unit-p1",
		SourcePlatform:   "twitter",
		SourceExternalID: "p1",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-p1", SourcePlatform: "twitter", SourceExternalID: "p1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-p1", SourcePlatform: "twitter", SourceExternalID: "p1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	sg2 := model.ContentSubgraph{
		ID:               "psg2",
		ArticleID:        "unit-p2",
		SourcePlatform:   "twitter",
		SourceExternalID: "p2",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d2", SourceArticleID: "unit-p2", SourcePlatform: "twitter", SourceExternalID: "p2", RawText: "联储继续收紧", SubjectText: "美联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t2", SourceArticleID: "unit-p2", SourcePlatform: "twitter", SourceExternalID: "p2", RawText: "最近一周美股回撤", SubjectText: "美股", ChangeText: "回撤", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationDisproved, TimeBucket: "1w"},
		},
	}
	for _, sg := range []model.ContentSubgraph{sg1, sg2} {
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

func TestSQLiteStore_RunParadigmProjectionDeletesStaleParadigms(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := model.ContentSubgraph{
		ID:               "stale-paradigm",
		ArticleID:        "stale-paradigm",
		SourcePlatform:   "twitter",
		SourceExternalID: "stale-paradigm",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "stale-paradigm", SourcePlatform: "twitter", SourceExternalID: "stale-paradigm", RawText: "AI投资叙事降温", SubjectText: "AI投资叙事", ChangeText: "降温", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "day"},
			{ID: "t1", SourceArticleID: "stale-paradigm", SourcePlatform: "twitter", SourceExternalID: "stale-paradigm", RawText: "美股回落", SubjectText: "美股", ChangeText: "回落", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "day"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-stale-paradigm", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO paradigms(paradigm_id, user_id, driver_subject, target_subject, time_bucket, payload_json, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		buildParadigmID("u-stale-paradigm", "AI股票", "美股", "day"),
		"u-stale-paradigm",
		"AI股票",
		"美股",
		"day",
		`{"paradigm_id":"stale","user_id":"u-stale-paradigm","driver_subject":"AI股票","target_subject":"美股","time_bucket":"day"}`,
		now.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("insert stale paradigm error = %v", err)
	}

	if _, err := store.RunParadigmProjection(context.Background(), "u-stale-paradigm", now); err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	got, err := store.ListParadigmsBySubject(context.Background(), "u-stale-paradigm", "美股")
	if err != nil {
		t.Fatalf("ListParadigmsBySubject() error = %v", err)
	}
	if len(got) != 1 || got[0].DriverSubject != "AI投资叙事" {
		t.Fatalf("paradigms = %#v, want only current AI投资叙事 -> 美股 paradigm", got)
	}
}

func TestSQLiteStore_RunParadigmProjectionHonorsCanonicalAliasMapping(t *testing.T) {
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
		ID:               "psg-canonical",
		ArticleID:        "unit-p-canonical",
		SourcePlatform:   "twitter",
		SourceExternalID: "p-canonical",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-p-canonical", SourcePlatform: "twitter", SourceExternalID: "p-canonical", RawText: "联储继续收紧", SubjectText: "联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-p-canonical", SourcePlatform: "twitter", SourceExternalID: "p-canonical", RawText: "未来一周美国股市承压", SubjectText: "美国股市", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-canonical", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	paradigms, err := store.RunParadigmProjection(context.Background(), "u-paradigm-canonical", now)
	if err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	if len(paradigms) != 1 {
		t.Fatalf("len(RunParadigmProjection()) = %d, want 1", len(paradigms))
	}
	if paradigms[0].DriverSubject != "美联储" || paradigms[0].TargetSubject != "美股" {
		t.Fatalf("paradigm = %#v, want canonical 美联储 -> 美股", paradigms[0])
	}
}

func TestSQLiteStore_ListParadigmsBySubjectSupportsAliasLookup(t *testing.T) {
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
		ID:               "psg-alias-lookup",
		ArticleID:        "unit-p-lookup",
		SourcePlatform:   "twitter",
		SourceExternalID: "p-lookup",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-p-lookup", SourcePlatform: "twitter", SourceExternalID: "p-lookup", RawText: "联储继续收紧", SubjectText: "联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-p-lookup", SourcePlatform: "twitter", SourceExternalID: "p-lookup", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-lookup", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunParadigmProjection(context.Background(), "u-paradigm-lookup", now); err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	items, err := store.ListParadigmsBySubject(context.Background(), "u-paradigm-lookup", "联储")
	if err != nil {
		t.Fatalf("ListParadigmsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].DriverSubject != "美联储" {
		t.Fatalf("items = %#v, want alias lookup to return canonical 美联储 paradigm", items)
	}
}

func TestSQLiteStore_RunParadigmProjectionMergesAliasAndCanonicalSubjects(t *testing.T) {
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
		{ID: "psg-alias", ArticleID: "unit-p-alias", SourcePlatform: "twitter", SourceExternalID: "p-alias", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "d1", SourceArticleID: "unit-p-alias", SourcePlatform: "twitter", SourceExternalID: "p-alias", RawText: "联储继续收紧", SubjectText: "联储", ChangeText: "继续收紧", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "t1", SourceArticleID: "unit-p-alias", SourcePlatform: "twitter", SourceExternalID: "p-alias", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
		{ID: "psg-canonical", ArticleID: "unit-p-canonical", SourcePlatform: "twitter", SourceExternalID: "p-canonical", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "d2", SourceArticleID: "unit-p-canonical", SourcePlatform: "twitter", SourceExternalID: "p-canonical", RawText: "美联储维持紧缩", SubjectText: "美联储", ChangeText: "维持紧缩", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "t2", SourceArticleID: "unit-p-canonical", SourcePlatform: "twitter", SourceExternalID: "p-canonical", RawText: "未来一周美股波动", SubjectText: "美股", ChangeText: "波动", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationDisproved, TimeBucket: "1w"}}},
	} {
		if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
			t.Fatalf("UpsertContentSubgraph(%s) error = %v", sg.ID, err)
		}
		if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-mixed", sg, now); err != nil {
			t.Fatalf("PersistMemoryContentGraph(%s) error = %v", sg.ID, err)
		}
	}
	items, err := store.RunParadigmProjection(context.Background(), "u-paradigm-mixed", now)
	if err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(RunParadigmProjection()) = %d, want 1 merged canonical paradigm", len(items))
	}
	if items[0].DriverSubject != "美联储" || items[0].SupportingSubgraphCount != 2 {
		t.Fatalf("items = %#v, want canonical 美联储 paradigm with both subgraphs merged", items)
	}
}

func TestSQLiteStore_ListParadigmsBySubjectSupportsTargetAliasLookup(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
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
		ID:               "psg-target-lookup",
		ArticleID:        "unit-p-target-lookup",
		SourcePlatform:   "twitter",
		SourceExternalID: "p-target-lookup",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "d1", SourceArticleID: "unit-p-target-lookup", SourcePlatform: "twitter", SourceExternalID: "p-target-lookup", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "t1", SourceArticleID: "unit-p-target-lookup", SourcePlatform: "twitter", SourceExternalID: "p-target-lookup", RawText: "未来一周美国股市承压", SubjectText: "美国股市", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.UpsertContentSubgraph(context.Background(), sg); err != nil {
		t.Fatalf("UpsertContentSubgraph() error = %v", err)
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-paradigm-target-lookup", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunParadigmProjection(context.Background(), "u-paradigm-target-lookup", now); err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	items, err := store.ListParadigmsBySubject(context.Background(), "u-paradigm-target-lookup", "美国股市")
	if err != nil {
		t.Fatalf("ListParadigmsBySubject() error = %v", err)
	}
	if len(items) != 1 || items[0].TargetSubject != "美股" {
		t.Fatalf("items = %#v, want target alias lookup to return canonical 美股 paradigm", items)
	}
}

func TestSQLiteStore_ProjectionSweepBuildsParadigmsAfterAccept(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "unit-paradigm-accept",
		Source:         "twitter",
		ExternalID:     "pa1",
		RootExternalID: "root-pa1",
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
			Drivers:      []string{"美联储加息0.25%"},
			Targets:      []string{"未来一周美股承压"},
			Details:      model.HiddenDetails{Caveats: []string{"detail"}},
			Verification: model.Verification{NodeVerifications: []model.NodeVerification{{NodeID: "n2", Status: model.NodeVerificationProved}}},
		},
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-auto-paradigm", SourcePlatform: "twitter", SourceExternalID: "pa1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if _, err := store.RunProjectionDirtySweep(context.Background(), "u-auto-paradigm", 100, time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RunProjectionDirtySweep() error = %v", err)
	}
	items, err := store.ListParadigms(context.Background(), "u-auto-paradigm")
	if err != nil {
		t.Fatalf("ListParadigms() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(ListParadigms()) = %d, want 1 paradigm after projection sweep", len(items))
	}
}

func TestSQLiteStore_ApplyVerifyVerdictAlsoRefreshesParadigms(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "unit-paradigm-refresh",
		Source:         "twitter",
		ExternalID:     "pr1",
		RootExternalID: "root-pr1",
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
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-refresh-paradigm", SourcePlatform: "twitter", SourceExternalID: "pr1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "pr1", model.VerifyVerdict{ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", Verdict: model.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
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
	sg := model.ContentSubgraph{ID: "subject-pg", ArticleID: "subject-pg", SourcePlatform: "twitter", SourceExternalID: "subject-pg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "subject-pg", SourcePlatform: "twitter", SourceExternalID: "subject-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "subject-pg", SourcePlatform: "twitter", SourceExternalID: "subject-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
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
	sg := model.ContentSubgraph{
		ID:               "evidence-pg",
		ArticleID:        "evidence-pg",
		SourcePlatform:   "twitter",
		SourceExternalID: "evidence-pg",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "evidence-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "evidence-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
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
	sg := model.ContentSubgraph{ID: "evidence-list-pg", ArticleID: "evidence-list-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-pg", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "evidence-list-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-pg", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "evidence-list-pg", SourcePlatform: "twitter", SourceExternalID: "evidence-list-pg", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
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
	for _, sg := range []model.ContentSubgraph{
		{ID: "pe-user-1", ArticleID: "pe-user-1", SourcePlatform: "twitter", SourceExternalID: "pe-user-1", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "pe-user-1", SourcePlatform: "twitter", SourceExternalID: "pe-user-1", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-user-1", SourcePlatform: "twitter", SourceExternalID: "pe-user-1", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
		{ID: "pe-user-2", ArticleID: "pe-user-2", SourcePlatform: "twitter", SourceExternalID: "pe-user-2", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "pe-user-2", SourcePlatform: "twitter", SourceExternalID: "pe-user-2", RawText: "欧洲央行放缓缩表", SubjectText: "欧洲央行", ChangeText: "放缓缩表", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "pe-user-2", SourcePlatform: "twitter", SourceExternalID: "pe-user-2", RawText: "欧股承压", SubjectText: "欧股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}},
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
