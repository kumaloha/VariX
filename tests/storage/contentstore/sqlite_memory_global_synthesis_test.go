package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func TestSQLiteStore_RunGlobalMemorySynthesisPersistsOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	seedCompiledRecordForMemory(t, store)
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if out.UserID != "u-synthesis" {
		t.Fatalf("UserID = %q, want u-synthesis", out.UserID)
	}
	if out.GeneratedAt.IsZero() {
		t.Fatalf("GeneratedAt = zero, want persisted timestamp")
	}

	got, err := store.GetLatestGlobalMemorySynthesisOutput(context.Background(), "u-synthesis")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemorySynthesisOutput() error = %v", err)
	}
	if got.OutputID == 0 {
		t.Fatalf("OutputID = 0, want persisted row id")
	}
	if got.UserID != "u-synthesis" {
		t.Fatalf("latest UserID = %q, want u-synthesis", got.UserID)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisIncludesCardTopItemsWhenNoConclusion(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	seedCompiledRecordForMemory(t, store)
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-card-only",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-card-only", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CognitiveCards) == 0 {
		t.Fatalf("CognitiveCards = %#v, want a relation-detail card", out.CognitiveCards)
	}
	if len(out.TopMemoryItems) == 0 {
		t.Fatalf("TopMemoryItems = %#v, want first-layer card item", out.TopMemoryItems)
	}
	if got := out.TopMemoryItems[0].ItemType; got != "card" {
		t.Fatalf("first ItemType = %q, want card when no conclusion is available", got)
	}
	if got, want := out.TopMemoryItems[0].BackingObjectID, out.CognitiveCards[0].CardID; got != want {
		t.Fatalf("BackingObjectID = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisSurfacesConflictSets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	records := []model.Record{
		{
			UnitID:         "weibo:C1",
			Source:         "weibo",
			ExternalID:     "C1",
			RootExternalID: "C1",
			Model:          "qwen3.6-plus",
			Output: model.Output{
				Summary: "summary",
				Graph: model.ReasoningGraph{
					Nodes: []model.GraphNode{
						{ID: "n1", Kind: model.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: model.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
				},
				Details:    model.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
		},
		{
			UnitID:         "twitter:C2",
			Source:         "twitter",
			ExternalID:     "C2",
			RootExternalID: "C2",
			Model:          "qwen3.6-plus",
			Output: model.Output{
				Summary: "summary",
				Graph: model.ReasoningGraph{
					Nodes: []model.GraphNode{
						{ID: "n1", Kind: model.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: model.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
				},
				Details:    model.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
		},
	}
	for _, record := range records {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-synthesis-conflict", SourcePlatform: "weibo", SourceExternalID: "C1", NodeIDs: []string{"n2"}},
		{UserID: "u-synthesis-conflict", SourcePlatform: "twitter", SourceExternalID: "C2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-conflict", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.ConflictSets) == 0 {
		t.Fatalf("ConflictSets = %#v, want at least one blocked conflict", out.ConflictSets)
	}
	if got := out.ConflictSets[0].SideAWhy; len(got) == 0 || got[0] != "需求走弱" {
		t.Fatalf("SideAWhy = %#v, want graph-backed support text for side A", got)
	}
	if got := out.ConflictSets[0].SideBWhy; len(got) == 0 || got[0] != "供给趋紧" {
		t.Fatalf("SideBWhy = %#v, want graph-backed support text for side B", got)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisPrefersFactSupportFirstInConflictWhy(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := model.Record{
		UnitID:         "weibo:CF1",
		Source:         "weibo",
		ExternalID:     "CF1",
		RootExternalID: "CF1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeExplicitCondition, Text: "若供给持续收缩"},
					{ID: "n2", Kind: model.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n3", Kind: model.EdgePresets},
					{From: "n2", To: "n3", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := model.Record{
		UnitID:         "twitter:CF2",
		Source:         "twitter",
		ExternalID:     "CF2",
		RootExternalID: "CF2",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []model.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-synthesis-conflict-rank", SourcePlatform: "weibo", SourceExternalID: "CF1", NodeIDs: []string{"n3"}},
		{UserID: "u-synthesis-conflict-rank", SourcePlatform: "twitter", SourceExternalID: "CF2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-conflict-rank", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.ConflictSets) == 0 {
		t.Fatalf("ConflictSets = %#v, want at least one blocked conflict", out.ConflictSets)
	}
	conflict := out.ConflictSets[0]
	var got []string
	if len(conflict.SideASourceRefs) > 0 && conflict.SideASourceRefs[0] == "weibo:CF1" {
		got = conflict.SideAWhy
	} else {
		got = conflict.SideBWhy
	}
	if len(got) < 2 || got[0] != "供给趋紧" || got[1] != "若供给持续收缩" {
		t.Fatalf("graph-backed why = %#v, want fact before condition", got)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisIncludesIndirectSupportBehindCondition(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := model.Record{
		UnitID:         "weibo:CF3",
		Source:         "weibo",
		ExternalID:     "CF3",
		RootExternalID: "CF3",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "中东运输扰动扩大", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeExplicitCondition, Text: "若霍尔木兹海峡未能恢复通航秩序"},
					{ID: "n3", Kind: model.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n2", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgePresets},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := model.Record{
		UnitID:         "twitter:CF4",
		Source:         "twitter",
		ExternalID:     "CF4",
		RootExternalID: "CF4",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []model.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-synthesis-conflict-depth", SourcePlatform: "weibo", SourceExternalID: "CF3", NodeIDs: []string{"n3"}},
		{UserID: "u-synthesis-conflict-depth", SourcePlatform: "twitter", SourceExternalID: "CF4", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-conflict-depth", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.ConflictSets) == 0 {
		t.Fatalf("ConflictSets = %#v, want at least one blocked conflict", out.ConflictSets)
	}
	conflict := out.ConflictSets[0]
	var got []string
	if len(conflict.SideASourceRefs) > 0 && conflict.SideASourceRefs[0] == "weibo:CF3" {
		got = conflict.SideAWhy
	} else {
		got = conflict.SideBWhy
	}
	if len(got) < 2 || got[0] != "若霍尔木兹海峡未能恢复通航秩序" || got[1] != "中东运输扰动扩大" {
		t.Fatalf("graph-backed why = %#v, want direct condition first, then upstream fact", got)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisBuildsCausalThesesAndCards(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "weibo:T1",
		Source:         "weibo",
		ExternalID:     "T1",
		RootExternalID: "T1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "风险资产承压", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n2", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-causal",
		SourcePlatform:   "weibo",
		SourceExternalID: "T1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-causal", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CausalTheses) == 0 {
		t.Fatalf("CausalTheses = %#v, want at least one causal thesis", out.CausalTheses)
	}
	if len(out.CognitiveCards) == 0 {
		t.Fatalf("CognitiveCards = %#v, want at least one card", out.CognitiveCards)
	}
	if len(out.CognitiveConclusions) == 0 {
		t.Fatalf("CognitiveConclusions = %#v, want at least one conclusion", out.CognitiveConclusions)
	}
	if len(out.TopMemoryItems) == 0 || out.TopMemoryItems[0].ItemType != "conclusion" {
		t.Fatalf("TopMemoryItems = %#v, want first-layer conclusion item", out.TopMemoryItems)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisBuildsRelationFirstTransmissionProjection(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	predictionStart := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	predictionDue := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	record := model.Record{
		UnitID:         "weibo:R1",
		Source:         "weibo",
		ExternalID:     "R1",
		RootExternalID: "R1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "美元流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeExplicitCondition, Text: "若再融资窗口继续关闭"},
					{ID: "n3", Kind: model.NodeImplicitCondition, Text: "信用利差扩大并向高收益市场传导", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: model.NodeConclusion, Text: "企业融资压力上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n5", Kind: model.NodePrediction, Text: "未来三个月违约波动加大", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n2", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgePresets},
					{From: "n3", To: "n4", Kind: model.EdgeDerives},
					{From: "n4", To: "n5", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-relation-first",
		SourcePlatform:   "weibo",
		SourceExternalID: "R1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4", "n5"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-relation-first", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CanonicalEntities) != 2 {
		t.Fatalf("len(CanonicalEntities) = %d, want 2 synthetic endpoints", len(out.CanonicalEntities))
	}
	if len(out.Relations) != 1 {
		t.Fatalf("len(Relations) = %d, want 1", len(out.Relations))
	}
	if len(out.Mechanisms) != 1 {
		t.Fatalf("len(Mechanisms) = %d, want 1", len(out.Mechanisms))
	}
	if len(out.MechanismNodes) != 5 {
		t.Fatalf("len(MechanismNodes) = %d, want 5 ordered transmission nodes", len(out.MechanismNodes))
	}
	if got := out.MechanismNodes[0].NodeType; got != memory.MechanismNodeDriver {
		t.Fatalf("MechanismNodes[0].NodeType = %q, want driver", got)
	}
	if got := out.MechanismNodes[1].NodeType; got != memory.MechanismNodeCondition {
		t.Fatalf("MechanismNodes[1].NodeType = %q, want condition", got)
	}
	if got := out.MechanismNodes[2].NodeType; got != memory.MechanismNodeMarketBehavior {
		t.Fatalf("MechanismNodes[2].NodeType = %q, want market_behavior", got)
	}
	if got := out.MechanismNodes[3].NodeType; got != memory.MechanismNodeTargetEffect {
		t.Fatalf("MechanismNodes[3].NodeType = %q, want target_effect", got)
	}
	if got := out.MechanismNodes[4].NodeType; got != memory.MechanismNodeTargetEffect {
		t.Fatalf("MechanismNodes[4].NodeType = %q, want terminal target_effect", got)
	}
	if len(out.MechanismEdges) != 4 {
		t.Fatalf("len(MechanismEdges) = %d, want 4 aligned path edges", len(out.MechanismEdges))
	}
	for i, edge := range out.MechanismEdges {
		if got, want := edge.PathOrder, i+1; got != want {
			t.Fatalf("MechanismEdges[%d].PathOrder = %d, want %d", i, got, want)
		}
	}
	if len(out.PathOutcomes) != 1 {
		t.Fatalf("len(PathOutcomes) = %d, want 1", len(out.PathOutcomes))
	}
	path := out.PathOutcomes[0]
	if len(path.NodePath) != 5 {
		t.Fatalf("len(NodePath) = %d, want 5", len(path.NodePath))
	}
	if len(path.EdgePath) != 4 {
		t.Fatalf("len(EdgePath) = %d, want 4", len(path.EdgePath))
	}
	if got, want := path.ConditionScope, "若再融资窗口继续关闭"; got != want {
		t.Fatalf("ConditionScope = %q, want %q", got, want)
	}
	if len(path.PredictionNodeIDs) != 1 || path.PredictionNodeIDs[0] != out.MechanismNodes[4].MechanismNodeID {
		t.Fatalf("PredictionNodeIDs = %#v, want terminal prediction node id", path.PredictionNodeIDs)
	}
	if !path.PredictionStartAt.Equal(predictionStart) {
		t.Fatalf("PredictionStartAt = %v, want %v", path.PredictionStartAt, predictionStart)
	}
	if !path.PredictionDueAt.Equal(predictionDue) {
		t.Fatalf("PredictionDueAt = %v, want %v", path.PredictionDueAt, predictionDue)
	}
	if len(out.DriverAggregates) != 1 || len(out.TargetAggregates) != 1 {
		t.Fatalf("aggregates = (%d,%d), want one driver and one target aggregate", len(out.DriverAggregates), len(out.TargetAggregates))
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisCompressesPetrodollarPrivateCreditOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "weibo:PD1",
		Source:         "weibo",
		ExternalID:     "PD1",
		RootExternalID: "PD1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "1970年代美沙达成石油美元协议，形成中东石油收入回流购买美国金融资产的闭环", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeImplicitCondition, Text: "私募信贷基金通过监管套利进行期限错配，积累流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodeConclusion, Text: "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: model.NodePrediction, Text: "一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n3", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgeDerives},
					{From: "n3", To: "n4", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-petro",
		SourcePlatform:   "weibo",
		SourceExternalID: "PD1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-petro", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CandidateTheses) != 1 {
		t.Fatalf("len(CandidateTheses) = %d, want 1", len(out.CandidateTheses))
	}
	if got, want := out.CandidateTheses[0].TopicLabel, "关于「石油美元与私募信贷流动性风险」的判断"; got != want {
		t.Fatalf("TopicLabel = %q, want %q", got, want)
	}
	if len(out.CognitiveConclusions) != 1 {
		t.Fatalf("len(CognitiveConclusions) = %d, want 1", len(out.CognitiveConclusions))
	}
	if got, want := out.CognitiveConclusions[0].Headline, "石油美元与私募信贷流动性风险正在推高美国资产脆弱性"; got != want {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisAbstractsOilShockOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "weibo:O1",
		Source:         "weibo",
		ExternalID:     "O1",
		RootExternalID: "O1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "乌克兰战争、伊朗冲突及中东地缘紧张局势持续升级", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeExplicitCondition, Text: "若霍尔木兹海峡未能恢复通航秩序"},
					{ID: "n3", Kind: model.NodeConclusion, Text: "释放石油储备等舒缓性措施无法根本平抑油价，危机核心在于海峡封锁", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: model.NodePrediction, Text: "布伦特原油价格将攀升至每桶130-150美元甚至更高", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n3", Kind: model.EdgeDerives},
					{From: "n2", To: "n4", Kind: model.EdgePresets},
					{From: "n3", To: "n4", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-oil",
		SourcePlatform:   "weibo",
		SourceExternalID: "O1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-oil", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CognitiveConclusions) != 1 {
		t.Fatalf("len(CognitiveConclusions) = %d, want 1", len(out.CognitiveConclusions))
	}
	if got, want := out.CognitiveConclusions[0].Headline, "油价冲击与海峡封锁风险正在放大能源与市场压力"; got != want {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisAbstractsBankResilienceOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "web:JPM1",
		Source:         "web",
		ExternalID:     "JPM1",
		RootExternalID: "JPM1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "2025年摩根大通实现创纪录营收1856亿美元与净利润570亿美元，ROTCE达20%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeImplicitCondition, Text: "当前高资产价格环境在遭遇宏观负面冲击时将放大金融系统脆弱性", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodeConclusion, Text: "宏观高利率与资产价格风险正在累积，但摩根大通具备抵御波动的能力", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: model.NodePrediction, Text: "摩根大通将在复杂宏观环境下维持长期稳健增长与股东回报", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n3", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgePositive},
					{From: "n3", To: "n4", Kind: model.EdgeDerives},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-jpm",
		SourcePlatform:   "web",
		SourceExternalID: "JPM1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-jpm", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CognitiveConclusions) != 1 {
		t.Fatalf("len(CognitiveConclusions) = %d, want 1", len(out.CognitiveConclusions))
	}
	if got, want := out.CognitiveConclusions[0].Headline, "高利率与资产价格脆弱性并存，但头部银行仍展现经营韧性"; got != want {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
	if got, want := out.TopMemoryItems[0].SignalStrength, memory.SignalHigh; got != want {
		t.Fatalf("SignalStrength = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisAbstractsDebtPurchasingPowerOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "twitter:DEBT1",
		Source:         "twitter",
		ExternalID:     "DEBT1",
		RootExternalID: "DEBT1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "过去500年历史显示债务与资本市场周期反复导致财富大起大落，且当前主要经济体名义与实际利率均处于历史极低水平", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeExplicitCondition, Text: "若金融资产承诺规模远超实物财富支撑，且央行被迫大量印钞以缓解债务违约压力"},
					{ID: "n3", Kind: model.NodeConclusion, Text: "传统现金与债券资产的实际购买力将不可避免地下降，货币面临系统性贬值风险", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n3", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgePresets},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-debt",
		SourcePlatform:   "twitter",
		SourceExternalID: "DEBT1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-debt", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CognitiveConclusions) != 1 {
		t.Fatalf("len(CognitiveConclusions) = %d, want 1", len(out.CognitiveConclusions))
	}
	if got, want := out.CognitiveConclusions[0].Headline, "债务与货币贬值压力正在侵蚀现金与债券购买力"; got != want {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
	if got, want := out.TopMemoryItems[0].SignalStrength, memory.SignalHigh; got != want {
		t.Fatalf("SignalStrength = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisMergesCrossSourceConditionAndConclusionOnSameObject(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := model.Record{
		UnitID:         "weibo:XC1",
		Source:         "weibo",
		ExternalID:     "XC1",
		RootExternalID: "XC1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeExplicitCondition, Text: "若石油价格维持在每桶100美元"},
					{ID: "n2", Kind: model.NodeConclusion, Text: "释放石油储备等舒缓性措施无法根本平抑油价", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgePresets}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := model.Record{
		UnitID:         "twitter:XC2",
		Source:         "twitter",
		ExternalID:     "XC2",
		RootExternalID: "XC2",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "供应担忧升温", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []model.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-synthesis-cross-role", SourcePlatform: "weibo", SourceExternalID: "XC1", NodeIDs: []string{"n1", "n2"}},
		{UserID: "u-synthesis-cross-role", SourcePlatform: "twitter", SourceExternalID: "XC2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-cross-role", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CandidateTheses) != 1 {
		t.Fatalf("len(CandidateTheses) = %d, want 1", len(out.CandidateTheses))
	}
	if got, want := out.CandidateTheses[0].TopicLabel, "关于「油价」的判断"; got != want {
		t.Fatalf("TopicLabel = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisDoesNotMergeSameThemeDifferentQuestion(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := model.Record{
		UnitID:         "weibo:NF1",
		Source:         "weibo",
		ExternalID:     "NF1",
		RootExternalID: "NF1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "油价上涨", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "油价上涨会推升通胀压力", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := model.Record{
		UnitID:         "twitter:NF2",
		Source:         "twitter",
		ExternalID:     "NF2",
		RootExternalID: "NF2",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "高油价", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeConclusion, Text: "高油价会提升能源企业利润", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{{From: "n1", To: "n2", Kind: model.EdgeDerives}},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []model.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-synthesis-no-false-merge", SourcePlatform: "weibo", SourceExternalID: "NF1", NodeIDs: []string{"n1", "n2"}},
		{UserID: "u-synthesis-no-false-merge", SourcePlatform: "twitter", SourceExternalID: "NF2", NodeIDs: []string{"n1", "n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-no-false-merge", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CandidateTheses) != 2 {
		t.Fatalf("len(CandidateTheses) = %d, want 2 separate theses", len(out.CandidateTheses))
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisReattachesSameSourceSingletons(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := model.Record{
		UnitID:         "weibo:SR1",
		Source:         "weibo",
		ExternalID:     "SR1",
		RootExternalID: "SR1",
		Model:          "qwen3.6-plus",
		Output: model.Output{
			Summary: "summary",
			Graph: model.ReasoningGraph{
				Nodes: []model.GraphNode{
					{ID: "n1", Kind: model.NodeFact, Text: "1970年代美沙达成石油美元协议，形成中东石油收入回流购买美国金融资产的闭环", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: model.NodeImplicitCondition, Text: "私募信贷基金通过监管套利进行期限错配，积累流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: model.NodeConclusion, Text: "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: model.NodeExplicitCondition, Text: "若AI应用冲击导致SaaS企业现金流断裂"},
					{ID: "n5", Kind: model.NodePrediction, Text: "一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []model.GraphEdge{
					{From: "n1", To: "n3", Kind: model.EdgeDerives},
					{From: "n2", To: "n3", Kind: model.EdgeDerives},
					{From: "n3", To: "n5", Kind: model.EdgeDerives},
					{From: "n4", To: "n5", Kind: model.EdgePresets},
				},
			},
			Details:    model.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-synthesis-reattach",
		SourcePlatform:   "weibo",
		SourceExternalID: "SR1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4", "n5"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-synthesis-reattach", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CandidateTheses) != 1 {
		t.Fatalf("len(CandidateTheses) = %d, want 1 thesis after singleton reattach", len(out.CandidateTheses))
	}
	if len(out.CandidateTheses[0].NodeIDs) != 5 {
		t.Fatalf("NodeIDs = %#v, want all 5 nodes grouped", out.CandidateTheses[0].NodeIDs)
	}
	if len(out.CognitiveConclusions) != 1 {
		t.Fatalf("len(CognitiveConclusions) = %d, want 1", len(out.CognitiveConclusions))
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisReusesPersistedEventAndParadigmProjectionsWhenAvailable(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := model.ContentSubgraph{
		ID:               "reuse-synthesis",
		ArticleID:        "reuse-synthesis",
		SourcePlatform:   "twitter",
		SourceExternalID: "reuse-synthesis",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "reuse-synthesis", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "reuse-synthesis", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-reuse-synthesis", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunEventGraphProjection(context.Background(), "u-reuse-synthesis", now); err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	if _, err := store.RunParadigmProjection(context.Background(), "u-reuse-synthesis", now); err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}

	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-reuse-synthesis", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.DriverAggregates) == 0 || len(out.TargetAggregates) == 0 {
		t.Fatalf("global synthesis output = %#v, want aggregates derived from persisted event/paradigm projections", out)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisBuildsTopItemsFromPersistedParadigmsFallback(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := model.ContentSubgraph{
		ID:               "reuse-synthesis-top",
		ArticleID:        "reuse-synthesis-top",
		SourcePlatform:   "twitter",
		SourceExternalID: "reuse-synthesis-top",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "reuse-synthesis-top", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis-top", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "reuse-synthesis-top", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis-top", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-reuse-synthesis-top", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunParadigmProjection(context.Background(), "u-reuse-synthesis-top", now); err != nil {
		t.Fatalf("RunParadigmProjection() error = %v", err)
	}
	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-reuse-synthesis-top", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.TopMemoryItems) == 0 {
		t.Fatalf("TopMemoryItems = %#v, want fallback top item from paradigm", out.TopMemoryItems)
	}
	if out.TopMemoryItems[0].ItemType != memory.TopMemoryItemConclusion {
		t.Fatalf("TopMemoryItems[0].ItemType = %q, want conclusion", out.TopMemoryItems[0].ItemType)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisBuildsCardsFromPersistedEventGraphsFallback(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := model.ContentSubgraph{
		ID:               "reuse-synthesis-card",
		ArticleID:        "reuse-synthesis-card",
		SourcePlatform:   "twitter",
		SourceExternalID: "reuse-synthesis-card",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "reuse-synthesis-card", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis-card", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "reuse-synthesis-card", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis-card", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-reuse-synthesis-card", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunEventGraphProjection(context.Background(), "u-reuse-synthesis-card", now); err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-reuse-synthesis-card", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.CognitiveCards) == 0 {
		t.Fatalf("CognitiveCards = %#v, want fallback card from persisted event graphs", out.CognitiveCards)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisBuildsTopItemsFromPersistedEventGraphsFallback(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := model.ContentSubgraph{
		ID:               "reuse-synthesis-top-event",
		ArticleID:        "reuse-synthesis-top-event",
		SourcePlatform:   "twitter",
		SourceExternalID: "reuse-synthesis-top-event",
		CompileVersion:   model.CompileBridgeVersion,
		CompiledAt:       now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		Nodes: []model.ContentNode{
			{ID: "n1", SourceArticleID: "reuse-synthesis-top-event", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis-top-event", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
			{ID: "n2", SourceArticleID: "reuse-synthesis-top-event", SourcePlatform: "twitter", SourceExternalID: "reuse-synthesis-top-event", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"},
		},
	}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-reuse-synthesis-top-event", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.RunEventGraphProjection(context.Background(), "u-reuse-synthesis-top-event", now); err != nil {
		t.Fatalf("RunEventGraphProjection() error = %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM paradigms WHERE user_id = ?`, "u-reuse-synthesis-top-event"); err != nil {
		t.Fatalf("DELETE paradigms error = %v", err)
	}
	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-reuse-synthesis-top-event", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.TopMemoryItems) == 0 {
		t.Fatalf("TopMemoryItems = %#v, want fallback top item from event graph", out.TopMemoryItems)
	}
}

func TestSQLiteStore_RunGlobalMemorySynthesisRefreshesPersistedEventAndParadigmLayersFirst(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC)
	sg := model.ContentSubgraph{ID: "refresh-synthesis", ArticleID: "refresh-synthesis", SourcePlatform: "twitter", SourceExternalID: "refresh-synthesis", CompileVersion: model.CompileBridgeVersion, CompiledAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), Nodes: []model.ContentNode{{ID: "n1", SourceArticleID: "refresh-synthesis", SourcePlatform: "twitter", SourceExternalID: "refresh-synthesis", RawText: "美联储加息0.25%", SubjectText: "美联储", ChangeText: "加息0.25%", Kind: model.NodeKindObservation, GraphRole: model.GraphRoleDriver, IsPrimary: true, VerificationStatus: model.VerificationPending, TimeBucket: "1w"}, {ID: "n2", SourceArticleID: "refresh-synthesis", SourcePlatform: "twitter", SourceExternalID: "refresh-synthesis", RawText: "未来一周美股承压", SubjectText: "美股", ChangeText: "承压", Kind: model.NodeKindPrediction, GraphRole: model.GraphRoleTarget, IsPrimary: true, VerificationStatus: model.VerificationProved, TimeBucket: "1w"}}}
	if err := store.PersistMemoryContentGraph(context.Background(), "u-refresh-synthesis", sg, now); err != nil {
		t.Fatalf("PersistMemoryContentGraph() error = %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM event_graphs WHERE user_id = ?`, "u-refresh-synthesis"); err != nil {
		t.Fatalf("DELETE event_graphs error = %v", err)
	}
	if _, err := store.db.Exec(`DELETE FROM paradigms WHERE user_id = ?`, "u-refresh-synthesis"); err != nil {
		t.Fatalf("DELETE paradigms error = %v", err)
	}
	out, err := store.RunGlobalMemorySynthesis(context.Background(), "u-refresh-synthesis", now)
	if err != nil {
		t.Fatalf("RunGlobalMemorySynthesis() error = %v", err)
	}
	if len(out.DriverAggregates) == 0 || len(out.TopMemoryItems) == 0 {
		t.Fatalf("output = %#v, want regenerated projections included", out)
	}
	var eventCount, paradigmCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM event_graphs WHERE user_id = ?`, "u-refresh-synthesis").Scan(&eventCount); err != nil {
		t.Fatalf("QueryRow(event_graphs) error = %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM paradigms WHERE user_id = ?`, "u-refresh-synthesis").Scan(&paradigmCount); err != nil {
		t.Fatalf("QueryRow(paradigms) error = %v", err)
	}
	if eventCount == 0 || paradigmCount == 0 {
		t.Fatalf("eventCount/paradigmCount = %d/%d, want regenerated rows", eventCount, paradigmCount)
	}
}
