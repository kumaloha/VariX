package contentstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestSQLiteStore_RunGlobalMemoryOrganizationV2PersistsOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	seedCompiledRecordForMemory(t, store)
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2", now)
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if out.UserID != "u-v2" {
		t.Fatalf("UserID = %q, want u-v2", out.UserID)
	}
	if out.GeneratedAt.IsZero() {
		t.Fatalf("GeneratedAt = zero, want persisted timestamp")
	}

	got, err := store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), "u-v2")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemoryOrganizationV2Output() error = %v", err)
	}
	if got.OutputID == 0 {
		t.Fatalf("OutputID = 0, want persisted row id")
	}
	if got.UserID != "u-v2" {
		t.Fatalf("latest UserID = %q, want u-v2", got.UserID)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationV2IncludesCardTopItemsWhenNoConclusion(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	seedCompiledRecordForMemory(t, store)
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-card-only",
		SourcePlatform:   "weibo",
		SourceExternalID: "Q1",
		NodeIDs:          []string{"n1"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-card-only", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2SurfacesConflictSets(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	records := []compile.Record{
		{
			UnitID:         "weibo:C1",
			Source:         "weibo",
			ExternalID:     "C1",
			RootExternalID: "C1",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary: "summary",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
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
			Output: compile.Output{
				Summary: "summary",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
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
		{UserID: "u-v2-conflict", SourcePlatform: "weibo", SourceExternalID: "C1", NodeIDs: []string{"n2"}},
		{UserID: "u-v2-conflict", SourcePlatform: "twitter", SourceExternalID: "C2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-conflict", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2PrefersFactSupportFirstInConflictWhy(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:CF1",
		Source:         "weibo",
		ExternalID:     "CF1",
		RootExternalID: "CF1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeExplicitCondition, Text: "若供给持续收缩"},
					{ID: "n2", Kind: compile.NodeFact, Text: "供给趋紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgePresets},
					{From: "n2", To: "n3", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:CF2",
		Source:         "twitter",
		ExternalID:     "CF2",
		RootExternalID: "CF2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []compile.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-v2-conflict-rank", SourcePlatform: "weibo", SourceExternalID: "CF1", NodeIDs: []string{"n3"}},
		{UserID: "u-v2-conflict-rank", SourcePlatform: "twitter", SourceExternalID: "CF2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-conflict-rank", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2IncludesIndirectSupportBehindCondition(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:CF3",
		Source:         "weibo",
		ExternalID:     "CF3",
		RootExternalID: "CF3",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "中东运输扰动扩大", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "若霍尔木兹海峡未能恢复通航秩序"},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgePresets},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:CF4",
		Source:         "twitter",
		ExternalID:     "CF4",
		RootExternalID: "CF4",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "需求走弱", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []compile.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-v2-conflict-depth", SourcePlatform: "weibo", SourceExternalID: "CF3", NodeIDs: []string{"n3"}},
		{UserID: "u-v2-conflict-depth", SourcePlatform: "twitter", SourceExternalID: "CF4", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-conflict-depth", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2BuildsCausalThesesAndCards(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:T1",
		Source:         "weibo",
		ExternalID:     "T1",
		RootExternalID: "T1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "风险资产承压", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodePrediction, Text: "未来数月波动加大", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-causal",
		SourcePlatform:   "weibo",
		SourceExternalID: "T1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-causal", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2BuildsRelationFirstTransmissionProjection(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	predictionStart := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	predictionDue := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	record := compile.Record{
		UnitID:         "weibo:R1",
		Source:         "weibo",
		ExternalID:     "R1",
		RootExternalID: "R1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "美元流动性收紧", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "若再融资窗口继续关闭"},
					{ID: "n3", Kind: compile.NodeImplicitCondition, Text: "信用利差扩大并向高收益市场传导", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodeConclusion, Text: "企业融资压力上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n5", Kind: compile.NodePrediction, Text: "未来三个月违约波动加大", PredictionStartAt: predictionStart, PredictionDueAt: predictionDue},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgePresets},
					{From: "n3", To: "n4", Kind: compile.EdgeDerives},
					{From: "n4", To: "n5", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-relation-first",
		SourcePlatform:   "weibo",
		SourceExternalID: "R1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4", "n5"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-relation-first", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2CompressesPetrodollarPrivateCreditOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:PD1",
		Source:         "weibo",
		ExternalID:     "PD1",
		RootExternalID: "PD1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "1970年代美沙达成石油美元协议，形成中东石油收入回流购买美国金融资产的闭环", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeImplicitCondition, Text: "私募信贷基金通过监管套利进行期限错配，积累流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodePrediction, Text: "一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgeDerives},
					{From: "n3", To: "n4", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-petro",
		SourcePlatform:   "weibo",
		SourceExternalID: "PD1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-petro", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2AbstractsOilShockOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:O1",
		Source:         "weibo",
		ExternalID:     "O1",
		RootExternalID: "O1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "乌克兰战争、伊朗冲突及中东地缘紧张局势持续升级", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "若霍尔木兹海峡未能恢复通航秩序"},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "释放石油储备等舒缓性措施无法根本平抑油价，危机核心在于海峡封锁", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodePrediction, Text: "布伦特原油价格将攀升至每桶130-150美元甚至更高", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgeDerives},
					{From: "n2", To: "n4", Kind: compile.EdgePresets},
					{From: "n3", To: "n4", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-oil",
		SourcePlatform:   "weibo",
		SourceExternalID: "O1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-oil", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if len(out.CognitiveConclusions) != 1 {
		t.Fatalf("len(CognitiveConclusions) = %d, want 1", len(out.CognitiveConclusions))
	}
	if got, want := out.CognitiveConclusions[0].Headline, "油价冲击与海峡封锁风险正在放大能源与市场压力"; got != want {
		t.Fatalf("Headline = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationV2AbstractsBankResilienceOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "web:JPM1",
		Source:         "web",
		ExternalID:     "JPM1",
		RootExternalID: "JPM1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "2025年摩根大通实现创纪录营收1856亿美元与净利润570亿美元，ROTCE达20%", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeImplicitCondition, Text: "当前高资产价格环境在遭遇宏观负面冲击时将放大金融系统脆弱性", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "宏观高利率与资产价格风险正在累积，但摩根大通具备抵御波动的能力", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodePrediction, Text: "摩根大通将在复杂宏观环境下维持长期稳健增长与股东回报", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgePositive},
					{From: "n3", To: "n4", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-jpm",
		SourcePlatform:   "web",
		SourceExternalID: "JPM1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-jpm", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2AbstractsDebtPurchasingPowerOutput(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "twitter:DEBT1",
		Source:         "twitter",
		ExternalID:     "DEBT1",
		RootExternalID: "DEBT1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "过去500年历史显示债务与资本市场周期反复导致财富大起大落，且当前主要经济体名义与实际利率均处于历史极低水平", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "若金融资产承诺规模远超实物财富支撑，且央行被迫大量印钞以缓解债务违约压力"},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "传统现金与债券资产的实际购买力将不可避免地下降，货币面临系统性贬值风险", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgePresets},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-debt",
		SourcePlatform:   "twitter",
		SourceExternalID: "DEBT1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-debt", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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

func TestSQLiteStore_RunGlobalMemoryOrganizationV2MergesCrossSourceConditionAndConclusionOnSameObject(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:XC1",
		Source:         "weibo",
		ExternalID:     "XC1",
		RootExternalID: "XC1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeExplicitCondition, Text: "若石油价格维持在每桶100美元"},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "释放石油储备等舒缓性措施无法根本平抑油价", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgePresets}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:XC2",
		Source:         "twitter",
		ExternalID:     "XC2",
		RootExternalID: "XC2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "供应担忧升温", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []compile.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-v2-cross-role", SourcePlatform: "weibo", SourceExternalID: "XC1", NodeIDs: []string{"n1", "n2"}},
		{UserID: "u-v2-cross-role", SourcePlatform: "twitter", SourceExternalID: "XC2", NodeIDs: []string{"n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-cross-role", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if len(out.CandidateTheses) != 1 {
		t.Fatalf("len(CandidateTheses) = %d, want 1", len(out.CandidateTheses))
	}
	if got, want := out.CandidateTheses[0].TopicLabel, "关于「油价」的判断"; got != want {
		t.Fatalf("TopicLabel = %q, want %q", got, want)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationV2DoesNotMergeSameThemeDifferentQuestion(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:NF1",
		Source:         "weibo",
		ExternalID:     "NF1",
		RootExternalID: "NF1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价上涨", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "油价上涨会推升通胀压力", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:NF2",
		Source:         "twitter",
		ExternalID:     "NF2",
		RootExternalID: "NF2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "高油价", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeConclusion, Text: "高油价会提升能源企业利润", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	for _, record := range []compile.Record{recordA, recordB} {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-v2-no-false-merge", SourcePlatform: "weibo", SourceExternalID: "NF1", NodeIDs: []string{"n1", "n2"}},
		{UserID: "u-v2-no-false-merge", SourcePlatform: "twitter", SourceExternalID: "NF2", NodeIDs: []string{"n1", "n2"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-no-false-merge", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
	}
	if len(out.CandidateTheses) != 2 {
		t.Fatalf("len(CandidateTheses) = %d, want 2 separate theses", len(out.CandidateTheses))
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationV2ReattachesSameSourceSingletons(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	record := compile.Record{
		UnitID:         "weibo:SR1",
		Source:         "weibo",
		ExternalID:     "SR1",
		RootExternalID: "SR1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "1970年代美沙达成石油美元协议，形成中东石油收入回流购买美国金融资产的闭环", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeImplicitCondition, Text: "私募信贷基金通过监管套利进行期限错配，积累流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n3", Kind: compile.NodeConclusion, Text: "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n4", Kind: compile.NodeExplicitCondition, Text: "若AI应用冲击导致SaaS企业现金流断裂"},
					{ID: "n5", Kind: compile.NodePrediction, Text: "一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n3", Kind: compile.EdgeDerives},
					{From: "n2", To: "n3", Kind: compile.EdgeDerives},
					{From: "n3", To: "n5", Kind: compile.EdgeDerives},
					{From: "n4", To: "n5", Kind: compile.EdgePresets},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
		t.Fatalf("UpsertCompiledOutput() error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-v2-reattach",
		SourcePlatform:   "weibo",
		SourceExternalID: "SR1",
		NodeIDs:          []string{"n1", "n2", "n3", "n4", "n5"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes() error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), "u-v2-reattach", time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganizationV2() error = %v", err)
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
