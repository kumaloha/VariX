package contentstore

import (
	"context"
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStore_RunNextMemoryOrganizationJobPrefersGraphFirstVerdictsWhenAvailable(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:Q-graph-first-organizer",
		Source:         "twitter",
		ExternalID:     "Q-graph-first-organizer",
		RootExternalID: "Q-graph-first-organizer",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "美联储加息0.25%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePositive}},
			},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
		},
		CompiledAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph-first-organizer", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-organizer", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	if err := store.ApplyVerifyVerdictToContentSubgraph(context.Background(), "twitter", "Q-graph-first-organizer", model.VerifyVerdict{ObjectType: model.VerifyQueueObjectNode, ObjectID: "n2", Verdict: model.VerificationProved, Reason: "observed drawdown", AsOf: time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)}); err != nil {
		t.Fatalf("ApplyVerifyVerdictToContentSubgraph() error = %v", err)
	}
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-graph-first-organizer", time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	if len(out.PredictionStatuses) != 1 || out.PredictionStatuses[0].Status != string(model.PredictionStatusResolvedTrue) {
		t.Fatalf("PredictionStatuses = %#v, want resolved_true from graph-first verdict", out.PredictionStatuses)
	}
}

func TestSQLiteStore_RunNextMemoryOrganizationJobPrefersGraphFirstNodeTextByNodeID(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:Q-graph-first-text",
		Source:         "twitter",
		ExternalID:     "Q-graph-first-text",
		RootExternalID: "Q-graph-first-text",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{{ID: "n1", Kind: model.NodeFact, Text: "旧节点文本", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)}},
				Edges: []model.GraphEdge{{From: "n1", To: "n1", Kind: model.EdgeExplains}},
			},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
		},
		CompiledAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}
	// keep output valid with second node/edge
	record.Output.Graph.Nodes = append(record.Output.Graph.Nodes, model.GraphNode{ID: "n2", Kind: model.NodeConclusion, Text: "结论", ValidFrom: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)})
	record.Output.Graph.Edges = []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePositive}}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph-first-text", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-text", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	custom := model.ContentSubgraph{
		ID:               "twitter:Q-graph-first-text",
		ArticleID:        "twitter:Q-graph-first-text",
		SourcePlatform:   "twitter",
		SourceExternalID: "Q-graph-first-text",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		UpdatedAt:        time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "twitter:Q-graph-first-text", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-text", RawText: "更新后的节点文本", SubjectText: "更新后的节点文本", ChangeText: "更新后的节点文本", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending},
			{ID: "n2", SourceArticleID: "twitter:Q-graph-first-text", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-text", RawText: "结论", SubjectText: "结论", ChangeText: "结论", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending},
		},
		Edges: []model.ContentEdge{{ID: "e1", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending}},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-graph-first-text", custom, time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-graph-first-text", time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	found := false
	for _, node := range out.ActiveNodes {
		if node.NodeID == "n1" {
			found = true
			if node.NodeText != "更新后的节点文本" {
				t.Fatalf("node.NodeText = %q, want graph-first text", node.NodeText)
			}
		}
	}
	if !found {
		t.Fatalf("active nodes = %#v, want n1 present", out.ActiveNodes)
	}
}

func TestSQLiteStore_RunNextMemoryOrganizationJobPrefersGraphFirstValidityWindowByNodeID(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:Q-graph-first-validity",
		Source:         "twitter",
		ExternalID:     "Q-graph-first-validity",
		RootExternalID: "Q-graph-first-validity",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodePrediction, Text: "未来一周美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), PredictionDueAt: time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "结论", ValidFrom: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePositive}},
			},
			Details: model.HiddenDetails{Caveats: []string{"detail"}},
		},
		CompiledAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph-first-validity", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-validity", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	custom := model.ContentSubgraph{
		ID:               "twitter:Q-graph-first-validity",
		ArticleID:        "twitter:Q-graph-first-validity",
		SourcePlatform:   "twitter",
		SourceExternalID: "Q-graph-first-validity",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		UpdatedAt:        time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "twitter:Q-graph-first-validity", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-validity", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeStart: time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), TimeEnd: time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)},
			{ID: "n2", SourceArticleID: "twitter:Q-graph-first-validity", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-validity", RawText: "结论", SubjectText: "结论", ChangeText: "结论", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending},
		},
		Edges: []model.ContentEdge{{ID: "e1", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending}},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-graph-first-validity", custom, time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-graph-first-validity", time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	found := false
	for _, node := range out.ActiveNodes {
		if node.NodeID == "n1" {
			found = true
			if !node.ValidFrom.Equal(time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("node.ValidFrom = %s, want graph-first validity start", node.ValidFrom)
			}
			if !node.ValidTo.Equal(time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("node.ValidTo = %s, want graph-first validity end", node.ValidTo)
			}
		}
	}
	if !found {
		t.Fatalf("active nodes = %#v, want n1 present", out.ActiveNodes)
	}
}

func TestSQLiteStore_RunNextMemoryOrganizationJobPrefersGraphFirstEdgesForHierarchy(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:Q-graph-first-edges",
		Source:         "twitter",
		ExternalID:     "Q-graph-first-edges",
		RootExternalID: "Q-graph-first-edges",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "事实A", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeImplicitCondition, Text: "机制B", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodeConclusion, Text: "结论C", ValidFrom: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n3", Kind: model.EdgePositive}},
			},
			Details:      model.HiddenDetails{Caveats: []string{"detail"}},
			Verification: model.Verification{FactChecks: []model.FactCheck{{NodeID: "n1", Status: model.FactStatusClearlyTrue}}},
		},
		CompiledAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-graph-first-edges", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-edges", NodeIDs: []string{"n1", "n2", "n3"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}
	custom := model.ContentSubgraph{
		ID:               "twitter:Q-graph-first-edges",
		ArticleID:        "twitter:Q-graph-first-edges",
		SourcePlatform:   "twitter",
		SourceExternalID: "Q-graph-first-edges",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		UpdatedAt:        time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "twitter:Q-graph-first-edges", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-edges", RawText: "事实A", SubjectText: "事实A", ChangeText: "事实A", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending},
			{ID: "n2", SourceArticleID: "twitter:Q-graph-first-edges", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-edges", RawText: "机制B", SubjectText: "机制B", ChangeText: "机制B", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending},
			{ID: "n3", SourceArticleID: "twitter:Q-graph-first-edges", SourcePlatform: "twitter", SourceExternalID: "Q-graph-first-edges", RawText: "结论C", SubjectText: "结论C", ChangeText: "结论C", Kind: model.NodeKindObservation, IsPrimary: true, VerificationStatus: model.VerificationPending},
		},
		Edges: []model.ContentEdge{{ID: "e1", From: "n1", To: "n2", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending}, {ID: "e2", From: "n2", To: "n3", Type: model.EdgeTypeDrives, IsPrimary: true, VerificationStatus: model.VerificationPending}},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-graph-first-edges", custom, time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), "u-graph-first-edges", time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunNextMemoryOrganizationJob() error = %v", err)
	}
	want := map[string]bool{"n1->n2": false, "n2->n3": false}
	for _, link := range out.Hierarchy {
		key := link.ParentNodeID + "->" + link.ChildNodeID
		if _, ok := want[key]; ok {
			if link.Source != "graph_first" {
				t.Fatalf("hierarchy link %s source = %q, want graph_first", key, link.Source)
			}
			want[key] = true
		}
	}
	for key, ok := range want {
		if !ok {
			t.Fatalf("hierarchy = %#v, want graph-first edge %s", out.Hierarchy, key)
		}
	}
}
