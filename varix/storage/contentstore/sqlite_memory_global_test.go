package contentstore

import (
	"context"
	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSQLiteStore_RunGlobalMemoryOrganizationBuildsNeutralClustersAcrossSources(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:G1",
		Source:         "weibo",
		ExternalID:     "G1",
		RootExternalID: "G1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价会上升", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeExplicitCondition, Text: "若地缘冲突升级"},
					{ID: "n3", Kind: compile.NodePrediction, Text: "未来几年风险资产承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n1", To: "n2", Kind: compile.EdgePositive},
					{From: "n2", To: "n3", Kind: compile.EdgePresets},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:G2",
		Source:         "twitter",
		ExternalID:     "G2",
		RootExternalID: "G2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeFact, Text: "油价会下降", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodeImplicitCondition, Text: "供给收缩会改变油价走势", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{
					{From: "n2", To: "n1", Kind: compile.EdgeDerives},
				},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordA) error = %v", err)
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordB) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-global",
		SourcePlatform:   "weibo",
		SourceExternalID: "G1",
		NodeIDs:          []string{"n1", "n2", "n3"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes(weibo) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{
		UserID:           "u-global",
		SourcePlatform:   "twitter",
		SourceExternalID: "G2",
		NodeIDs:          []string{"n1", "n2"},
	}); err != nil {
		t.Fatalf("AcceptMemoryNodes(twitter) error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganization(context.Background(), "u-global", time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganization() error = %v", err)
	}
	if len(out.Clusters) == 0 {
		t.Fatalf("len(Clusters) = 0, want clusters")
	}
	foundContradictionCluster := false
	for _, cluster := range out.Clusters {
		if len(cluster.ConflictingNodeIDs) > 0 {
			foundContradictionCluster = true
			if cluster.RepresentativeNodeID == "" {
				t.Fatalf("cluster missing representative node: %#v", cluster)
			}
			if cluster.CanonicalProposition == "" || !strings.HasPrefix(cluster.CanonicalProposition, "关于「") {
				t.Fatalf("cluster proposition should be neutral rather than raw representative text: %#v", cluster)
			}
		}
	}
	if !foundContradictionCluster {
		t.Fatalf("Clusters = %#v, want contradiction cluster", out.Clusters)
	}
	got, err := store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), "u-global")
	if err != nil {
		t.Fatalf("GetLatestGlobalMemoryOrganizationOutput() error = %v", err)
	}
	if got.OutputID == 0 || len(got.Clusters) == 0 {
		t.Fatalf("latest global output = %#v", got)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationMergesCrossSourceSharedProposition(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:S1",
		Source:         "weibo",
		ExternalID:     "S1",
		RootExternalID: "S1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeConclusion, Text: "石油美元回流正在削弱", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来几年美国资产承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "twitter:S2",
		Source:         "twitter",
		ExternalID:     "S2",
		RootExternalID: "S2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeConclusion, Text: "石油美元回流面临断裂风险", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来几年金融资产承压", PredictionStartAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordA) error = %v", err)
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordB) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-merge", SourcePlatform: "weibo", SourceExternalID: "S1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes(weibo) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-merge", SourcePlatform: "twitter", SourceExternalID: "S2", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes(twitter) error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganization(context.Background(), "u-merge", time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganization() error = %v", err)
	}
	foundMerged := false
	for _, cluster := range out.Clusters {
		if strings.Contains(cluster.CanonicalProposition, "石油美元回流") {
			foundMerged = true
			if len(cluster.SupportingNodeIDs)+len(cluster.PredictiveNodeIDs) < 2 {
				t.Fatalf("cluster = %#v, want cross-source merged supporting/predictive members", cluster)
			}
		}
	}
	if !foundMerged {
		t.Fatalf("Clusters = %#v, want shared proposition cluster around 石油美元回流", out.Clusters)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationDerivesHigherLevelMacroTheme(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	recordA := compile.Record{
		UnitID:         "weibo:M1",
		Source:         "weibo",
		ExternalID:     "M1",
		RootExternalID: "M1",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeConclusion, Text: "石油美元循环面临断裂风险", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来几年美债美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
	}
	recordB := compile.Record{
		UnitID:         "weibo:M2",
		Source:         "weibo",
		ExternalID:     "M2",
		RootExternalID: "M2",
		Model:          "qwen3.6-plus",
		Output: compile.Output{
			Summary: "summary",
			Graph: compile.ReasoningGraph{
				Nodes: []compile.GraphNode{
					{ID: "n1", Kind: compile.NodeConclusion, Text: "私募信贷流动性风险正在上升", OccurredAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
					{ID: "n2", Kind: compile.NodePrediction, Text: "未来几年华尔街可能遭遇挤兑冲击", PredictionStartAt: time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)},
				},
				Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
			},
			Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
			Confidence: "medium",
		},
		CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordA); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordA) error = %v", err)
	}
	if err := store.UpsertCompiledOutput(context.Background(), recordB); err != nil {
		t.Fatalf("UpsertCompiledOutput(recordB) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-theme", SourcePlatform: "weibo", SourceExternalID: "M1", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes(M1) error = %v", err)
	}
	if _, err := store.AcceptMemoryNodes(context.Background(), memory.AcceptRequest{UserID: "u-theme", SourcePlatform: "weibo", SourceExternalID: "M2", NodeIDs: []string{"n1", "n2"}}); err != nil {
		t.Fatalf("AcceptMemoryNodes(M2) error = %v", err)
	}

	out, err := store.RunGlobalMemoryOrganization(context.Background(), "u-theme", time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganization() error = %v", err)
	}
	foundTheme := false
	for _, cluster := range out.Clusters {
		if cluster.CanonicalProposition == "关于「石油美元、油价与流动性风险」的判断" {
			foundTheme = true
			if len(cluster.SupportingNodeIDs) == 0 || len(cluster.PredictiveNodeIDs) == 0 {
				t.Fatalf("cluster = %#v, want merged supporting and predictive members", cluster)
			}
			if len(cluster.SupportingNodeIDs)+len(cluster.PredictiveNodeIDs) < 3 {
				t.Fatalf("cluster = %#v, want richer higher-level theme members", cluster)
			}
			if !strings.Contains(cluster.Summary, "支持信息包括") || !strings.Contains(cluster.Summary, "相关预测包括") {
				t.Fatalf("cluster summary = %q, want synthesized role-aware summary", cluster.Summary)
			}
			if len(cluster.CoreSupportingNodeIDs) == 0 || len(cluster.CorePredictiveNodeIDs) == 0 {
				t.Fatalf("cluster = %#v, want core skeleton node sets", cluster)
			}
			if len(cluster.SynthesizedEdges) == 0 {
				t.Fatalf("cluster = %#v, want synthesized edges", cluster)
			}
		}
	}
	if !foundTheme {
		t.Fatalf("Clusters = %#v, want higher-level macro theme cluster", out.Clusters)
	}
}

func TestSQLiteStore_RunGlobalMemoryOrganizationKeepsJPMStyleNodesInMacroClusters(t *testing.T) {
	root := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(root, "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	records := []compile.Record{
		{
			UnitID:         "twitter:D1",
			Source:         "twitter",
			ExternalID:     "D1",
			RootExternalID: "D1",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary:    "summary",
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeConclusion, Text: "传统现金与债券资产的实际购买力将不可避免地下降，货币面临系统性贬值风险", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodePrediction, Text: "未来几年未进行分散配置的投资者将面临财富缩水", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC),
		},
		{
			UnitID:         "weibo:O1",
			Source:         "weibo",
			ExternalID:     "O1",
			RootExternalID: "O1",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary:    "summary",
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeConclusion, Text: "石油美元循环面临断裂风险，且私募信贷正积累流动性隐患", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodePrediction, Text: "未来几年美债美股承压", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 5, 0, 0, time.UTC),
		},
		{
			UnitID:         "weibo:E1",
			Source:         "weibo",
			ExternalID:     "E1",
			RootExternalID: "E1",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary:    "summary",
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n1", Kind: compile.NodeFact, Text: "美国就业市场逼近衰退临界点，AI冲击白领就业，高收入家庭消费收缩", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n2", Kind: compile.NodeConclusion, Text: "美国经济面临衰退风险", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{{From: "n1", To: "n2", Kind: compile.EdgeDerives}},
				},
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 10, 0, 0, time.UTC),
		},
		{
			UnitID:         "web:J1",
			Source:         "web",
			ExternalID:     "J1",
			RootExternalID: "J1",
			Model:          "qwen3.6-plus",
			Output: compile.Output{
				Summary:    "summary",
				Details:    compile.HiddenDetails{Caveats: []string{"detail"}},
				Confidence: "medium",
				Graph: compile.ReasoningGraph{
					Nodes: []compile.GraphNode{
						{ID: "n4", Kind: compile.NodeExplicitCondition, Text: "若伊朗战争持续引发显著的大宗商品价格冲击与全球供应链重塑"},
						{ID: "n5", Kind: compile.NodeExplicitCondition, Text: "若银行去监管化政策能够被妥善设计与执行"},
						{ID: "n6", Kind: compile.NodeImplicitCondition, Text: "当前高资产价格环境在遭遇宏观负面冲击时将放大金融系统脆弱性", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n7", Kind: compile.NodeConclusion, Text: "宏观风险正在累积，金融体系安全取决于监管与资产价格韧性", OccurredAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
						{ID: "n9", Kind: compile.NodePrediction, Text: "妥善实施的银行去监管将提升金融体系安全性并更好支持经济增长", PredictionStartAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
					},
					Edges: []compile.GraphEdge{
						{From: "n6", To: "n7", Kind: compile.EdgePositive},
						{From: "n5", To: "n9", Kind: compile.EdgeDerives},
						{From: "n7", To: "n9", Kind: compile.EdgeDerives},
					},
				},
			},
			CompiledAt: time.Date(2026, 4, 14, 8, 15, 0, 0, time.UTC),
		},
	}
	for _, record := range records {
		if err := store.UpsertCompiledOutput(context.Background(), record); err != nil {
			t.Fatalf("UpsertCompiledOutput(%s) error = %v", record.UnitID, err)
		}
	}
	for _, req := range []memory.AcceptRequest{
		{UserID: "u-jpm", SourcePlatform: "twitter", SourceExternalID: "D1", NodeIDs: []string{"n1"}},
		{UserID: "u-jpm", SourcePlatform: "weibo", SourceExternalID: "O1", NodeIDs: []string{"n1"}},
		{UserID: "u-jpm", SourcePlatform: "weibo", SourceExternalID: "E1", NodeIDs: []string{"n1"}},
		{UserID: "u-jpm", SourcePlatform: "web", SourceExternalID: "J1", NodeIDs: []string{"n4", "n5", "n6", "n9"}},
	} {
		if _, err := store.AcceptMemoryNodes(context.Background(), req); err != nil {
			t.Fatalf("AcceptMemoryNodes(%s:%s) error = %v", req.SourcePlatform, req.SourceExternalID, err)
		}
	}

	out, err := store.RunGlobalMemoryOrganization(context.Background(), "u-jpm", time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunGlobalMemoryOrganization() error = %v", err)
	}

	find := func(proposition string) *memory.GlobalCluster {
		for i := range out.Clusters {
			if out.Clusters[i].CanonicalProposition == proposition {
				return &out.Clusters[i]
			}
		}
		return nil
	}
	debtCluster := find("关于「债务周期与金融资产实际回报」的判断")
	if debtCluster == nil || !slices.Contains(debtCluster.ConditionalNodeIDs, "web:J1:n6") {
		t.Fatalf("debtCluster = %#v, want web:J1:n6 to merge into debt cluster", debtCluster)
	}
	oilCluster := find("关于「石油美元、油价与流动性风险」的判断")
	if oilCluster == nil || !slices.Contains(oilCluster.ConditionalNodeIDs, "web:J1:n4") {
		t.Fatalf("oilCluster = %#v, want web:J1:n4 to merge into oil/liquidity cluster", oilCluster)
	}
	bankCluster := find("关于「银行监管与金融系统安全」的判断")
	if bankCluster == nil || !slices.Contains(bankCluster.ConditionalNodeIDs, "web:J1:n5") || !slices.Contains(bankCluster.PredictiveNodeIDs, "web:J1:n9") {
		t.Fatalf("bankCluster = %#v, want web:J1:n5 + web:J1:n9 to share regulation cluster", bankCluster)
	}
	employmentCluster := find("美国就业市场逼近衰退临界点，AI冲击白领就业，高收入家庭消费收缩")
	if employmentCluster != nil && slices.Contains(employmentCluster.ConditionalNodeIDs, "web:J1:n4") {
		t.Fatalf("employmentCluster = %#v, want web:J1:n4 excluded from employment cluster", employmentCluster)
	}
}
