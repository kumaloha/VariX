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
	if got, want := out.TopMemoryItems[0].SignalStrength, "high"; got != want {
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
	if got, want := out.TopMemoryItems[0].SignalStrength, "high"; got != want {
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
