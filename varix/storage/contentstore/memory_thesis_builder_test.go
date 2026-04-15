package contentstore

import (
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestBuildCandidateTheses_GroupsByCognitiveQuestion(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "石油美元回流正在削弱",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "石油美元回流面临断裂风险",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1", len(got))
	}
	if len(got[0].NodeIDs) != 2 {
		t.Fatalf("NodeIDs = %#v, want 2 grouped nodes", got[0].NodeIDs)
	}
}

func TestBuildCandidateTheses_DoesNotMergeSameThemeDifferentQuestion(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "油价上涨会推升通胀压力",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "高油价会提升能源企业利润",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 2 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 2 separate theses", len(got))
	}
}

func TestBuildCandidateTheses_GroupsBySharedMechanismNotJustExactPhrase(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "私募信贷会积累流动性错配风险",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "私募信贷繁荣正在让流动性脆弱性上升",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 shared-mechanism thesis", len(got))
	}
}

func TestBuildCandidateTheses_GroupsByMechanismSynonyms(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeImplicitCondition),
			NodeText:         "银行去监管会提升金融体系安全性",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeImplicitCondition),
			NodeText:         "监管松绑会让系统更安全",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 thesis for mechanism synonyms", len(got))
	}
}

func TestBuildCandidateTheses_UsesSharedObjectLabelForContradictions(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "油价会上升",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "油价会下降",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 contradiction thesis", len(got))
	}
	if got[0].TopicLabel == "油价会下降" || got[0].TopicLabel == "油价会上升" {
		t.Fatalf("TopicLabel = %q, want shared object label rather than one-sided raw sentence", got[0].TopicLabel)
	}
}

func TestBuildCandidateTheses_UsesFactAndConclusionForSingleSourceTopicLabel(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeFact),
			NodeText:         "流动性收紧",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n2",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "风险资产承压",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 thesis", len(got))
	}
	if got[0].TopicLabel == "流动性收紧" || got[0].TopicLabel == "风险资产承压" {
		t.Fatalf("TopicLabel = %q, want a synthesized fact+conclusion label", got[0].TopicLabel)
	}
	if got[0].ClusterReason != "same_source_causal_chain" {
		t.Fatalf("ClusterReason = %q, want same_source_causal_chain", got[0].ClusterReason)
	}
}

func TestBuildCandidateTheses_UsesContradictionPairReason(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "油价会上升",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "油价会下降",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1", len(got))
	}
	if got[0].ClusterReason != "contradiction_pair" {
		t.Fatalf("ClusterReason = %q, want contradiction_pair", got[0].ClusterReason)
	}
}

func TestBuildCandidateTheses_UsesSharedMechanismThemeReason(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeImplicitCondition),
			NodeText:         "私募信贷会积累流动性错配风险",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeImplicitCondition),
			NodeText:         "私募信贷繁荣正在让流动性脆弱性上升",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1", len(got))
	}
	if got[0].ClusterReason != "shared_mechanism_theme" {
		t.Fatalf("ClusterReason = %q, want shared_mechanism_theme", got[0].ClusterReason)
	}
}

func TestBuildCandidateTheses_DoesNotOvermergeLargeSingleSourceMemory(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "BIG1", NodeID: "n1", NodeKind: string(compile.NodeFact), NodeText: "流动性收紧", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "BIG1", NodeID: "n2", NodeKind: string(compile.NodeConclusion), NodeText: "风险资产承压", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "BIG1", NodeID: "n3", NodeKind: string(compile.NodePrediction), NodeText: "未来数月波动加大", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "BIG1", NodeID: "n4", NodeKind: string(compile.NodeFact), NodeText: "银行去监管推进", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "BIG1", NodeID: "n5", NodeKind: string(compile.NodeConclusion), NodeText: "金融体系更安全", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "BIG1", NodeID: "n6", NodeKind: string(compile.NodePrediction), NodeText: "经济增长改善", AcceptedAt: now},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) < 2 {
		t.Fatalf("len(buildCandidateTheses) = %d, want at least 2 theses instead of one giant source-wide thesis", len(got))
	}
}

func TestBuildCandidateTheses_DoesNotMergeBroadDebtThemeWithPetrodollarLiquidityTheme(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "D1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "传统现金与债券资产的实际购买力将不可避免地下降，货币面临系统性贬值风险",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "P1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 2 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 2 separate theses for broad debt theme vs petrodollar/liquidity theme", len(got))
	}
}

func TestBuildCandidateTheses_DoesNotTreatAnyFinancialAssetMentionAsDebtTheme(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "D1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "传统现金与债券资产的实际购买力将不可避免地下降",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "P1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeFact),
			NodeText:         "1970年代美沙达成石油美元协议，石油收入回流购买美国金融资产",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 2 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 2 theses; mentioning 美国金融资产 alone should not trigger debt-theme merge", len(got))
	}
}

func TestBuildCandidateTheses_MergesCrossSourceFactAndConclusionOnSameObject(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeFact),
			NodeText:         "1970年代美沙达成石油美元协议，石油收入回流购买美国金融资产",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "石油美元循环面临断裂风险",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 thesis for cross-source same-object fact+conclusion", len(got))
	}
	if got[0].TopicLabel != "关于「石油美元」的判断" {
		t.Fatalf("TopicLabel = %q, want shared object label", got[0].TopicLabel)
	}
}

func TestBuildCandidateTheses_MergesCrossSourceConditionAndConclusionOnSameObject(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{
			UserID:           "u-thesis",
			SourcePlatform:   "weibo",
			SourceExternalID: "S1",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeExplicitCondition),
			NodeText:         "若石油价格维持在每桶100美元",
			AcceptedAt:       now,
		},
		{
			UserID:           "u-thesis",
			SourcePlatform:   "twitter",
			SourceExternalID: "S2",
			NodeID:           "n1",
			NodeKind:         string(compile.NodeConclusion),
			NodeText:         "释放石油储备等舒缓性措施无法根本平抑油价",
			AcceptedAt:       now,
		},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 thesis for cross-source same-object condition+conclusion", len(got))
	}
	if got[0].TopicLabel != "关于「油价」的判断" {
		t.Fatalf("TopicLabel = %q, want shared object label", got[0].TopicLabel)
	}
}

func TestBuildCandidateTheses_CompressesLargePetrodollarPrivateCreditTopicLabel(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n1", NodeKind: string(compile.NodeFact), NodeText: "1970年代美沙达成石油美元协议，形成中东石油收入回流购买美国金融资产的闭环", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n2", NodeKind: string(compile.NodeImplicitCondition), NodeText: "私募信贷基金通过监管套利进行期限错配，积累流动性隐患", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n3", NodeKind: string(compile.NodeConclusion), NodeText: "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患", AcceptedAt: now},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1", len(got))
	}
	want := "关于「石油美元与私募信贷流动性风险」的判断"
	if got[0].TopicLabel != want {
		t.Fatalf("TopicLabel = %q, want %q", got[0].TopicLabel, want)
	}
}

func TestBuildCandidateTheses_ReattachesSameSourceSingletonConditionAndPrediction(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	nodes := []memory.AcceptedNode{
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n1", NodeKind: string(compile.NodeFact), NodeText: "1970年代美沙达成石油美元协议，形成中东石油收入回流购买美国金融资产的闭环", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n2", NodeKind: string(compile.NodeImplicitCondition), NodeText: "私募信贷基金通过监管套利进行期限错配，积累流动性隐患", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n3", NodeKind: string(compile.NodeConclusion), NodeText: "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n4", NodeKind: string(compile.NodeExplicitCondition), NodeText: "若AI应用冲击导致SaaS企业现金流断裂", AcceptedAt: now},
		{UserID: "u-thesis", SourcePlatform: "weibo", SourceExternalID: "Q1", NodeID: "n5", NodeKind: string(compile.NodePrediction), NodeText: "一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街", AcceptedAt: now},
	}

	got := buildCandidateTheses(nodes, now)
	if len(got) != 1 {
		t.Fatalf("len(buildCandidateTheses) = %d, want 1 thesis with same-source condition/prediction reattached", len(got))
	}
	if len(got[0].NodeIDs) != 5 {
		t.Fatalf("NodeIDs = %#v, want all 5 nodes in one thesis", got[0].NodeIDs)
	}
}
