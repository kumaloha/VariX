package contentstore

import (
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func TestBuildCognitiveConclusion_AllowsSingleSourceCompleteChain(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CoreQuestion:      "关于「流动性收紧与风险资产」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险资产承压",
		Summary:         "流动性收紧 → 风险资产承压",
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got.Headline == "" {
		t.Fatalf("Headline = empty, want synthesized conclusion")
	}
	if len(got.BackingCardIDs) != 1 || got.BackingCardIDs[0] != "card-1" {
		t.Fatalf("BackingCardIDs = %#v, want card-1", got.BackingCardIDs)
	}
}

func TestBuildCognitiveConclusion_RejectsGenericHeadline(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CorePathNodeIDs:   []string{"n1", "n2"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险值得关注",
		Summary:         "市场可能发生变化",
		ConfidenceLabel: "strong",
	}}

	_, ok := buildCognitiveConclusion(thesis, cards)
	if ok {
		t.Fatalf("ok = true, want false for generic headline")
	}
}

func TestBuildTopMemoryItems_PrioritizesConflict(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	conflicts := []memory.ConflictSet{{
		ConflictID:     "conflict-1",
		ConflictStatus: "blocked",
		ConflictTopic:  "关于「油价方向」的矛盾",
		UpdatedAt:      now,
	}}
	conclusions := []memory.CognitiveConclusion{{
		ConclusionID: "conclusion-1",
		Headline:     "流动性收紧会压制风险资产",
	}}

	got := buildTopMemoryItems(conflicts, conclusions, now)
	if len(got) != 2 {
		t.Fatalf("len(buildTopMemoryItems) = %d, want 2", len(got))
	}
	if got[0].ItemType != "conflict" {
		t.Fatalf("first ItemType = %q, want conflict", got[0].ItemType)
	}
}

func TestBuildTopMemoryItems_SetsSignalStrength(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	conclusions := []memory.CognitiveConclusion{{
		ConclusionID: "conclusion-1",
		Headline:     "石油美元与私募信贷流动性风险正在推高美国资产脆弱性",
		Subheadline:  "石油美元闭环 → 私募信贷流动性隐患 → 美国资产更脆弱",
	}}

	got := buildTopMemoryItems(nil, conclusions, now)
	if len(got) != 1 {
		t.Fatalf("len(buildTopMemoryItems) = %d, want 1", len(got))
	}
	if got[0].SignalStrength != "high" {
		t.Fatalf("SignalStrength = %q, want high for strong abstract conclusion", got[0].SignalStrength)
	}
}

func TestBuildTopMemoryItems_HumanizesConflictReason(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	conflicts := []memory.ConflictSet{{
		ConflictID:     "conflict-1",
		ConflictStatus: "blocked",
		ConflictTopic:  "关于「油价」的判断",
		ConflictReason: "antonym contradiction",
		UpdatedAt:      now,
	}}

	got := buildTopMemoryItems(conflicts, nil, now)
	if len(got) != 1 {
		t.Fatalf("len(buildTopMemoryItems) = %d, want 1", len(got))
	}
	if got[0].Subheadline == "antonym contradiction" {
		t.Fatalf("Subheadline = %q, want human-readable conflict wording", got[0].Subheadline)
	}
}

func TestBuildCognitiveConclusion_UsesPredictionToLiftHeadline(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CoreQuestion:      "关于「流动性收紧与风险资产」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险资产承压",
		Summary:         "流动性收紧 → 风险资产承压 → 未来数月波动加大",
		KeyEvidence:     []string{"流动性收紧"},
		Predictions:     []string{"未来数月波动加大"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got.Headline == "风险资产承压" {
		t.Fatalf("Headline = %q, want lifted conclusion rather than raw card title", got.Headline)
	}
	if !strings.Contains(got.Headline, "流动性收紧") {
		t.Fatalf("Headline = %q, want key driver included in headline", got.Headline)
	}
	if got.Headline == "" {
		t.Fatalf("Headline = empty, want synthesized lifted headline")
	}
}

func TestBuildCognitiveConclusion_ProducesMoreAbstractHeadlineForPressureAndVolatility(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-1",
		ThesisID:          "thesis-1",
		CoreQuestion:      "关于「流动性收紧与风险资产承压」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "conclusion", "n3": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-1",
		CausalThesisID:  "ct-1",
		Title:           "风险资产承压",
		Summary:         "流动性收紧 → 风险资产承压 → 未来数月波动加大",
		KeyEvidence:     []string{"流动性收紧"},
		Predictions:     []string{"未来数月波动加大"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := "流动性收紧正在把风险资产推向承压与更高波动"
	if got.Headline != want {
		t.Fatalf("Headline = %q, want %q", got.Headline, want)
	}
}

func TestBuildCognitiveConclusion_AbstractsDebtPurchasingPowerPattern(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-2",
		ThesisID:          "thesis-2",
		CoreQuestion:      "关于「债务」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3"},
		CompletenessScore: 0.8,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "condition", "n3": "conclusion"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-2",
		CausalThesisID:  "ct-2",
		Title:           "传统现金与债券资产的实际购买力将不可避免地下降，货币面临系统性贬值风险",
		Summary:         "过去500年历史显示债务与资本市场周期反复导致财富大起大落 → 若金融资产承诺规模远超实物财富支撑，且央行被迫大量印钞以缓解债务违约压力 → 传统现金与债券资产的实际购买力将不可避免地下降，货币面临系统性贬值风险",
		KeyEvidence:     []string{"过去500年历史显示债务与资本市场周期反复导致财富大起大落"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := "债务与货币贬值压力正在侵蚀现金与债券购买力"
	if got.Headline != want {
		t.Fatalf("Headline = %q, want %q", got.Headline, want)
	}
}

func TestBuildCognitiveConclusion_AbstractsPetrodollarPrivateCreditPattern(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-3",
		ThesisID:          "thesis-3",
		CoreQuestion:      "关于「石油美元与私募信贷流动性风险」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3", "n4"},
		CompletenessScore: 1.0,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "condition", "n3": "conclusion", "n4": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-3",
		CausalThesisID:  "ct-3",
		Title:           "石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患",
		Summary:         "1970年代美沙达成石油美元闭环 → 若AI应用冲击导致SaaS企业现金流断裂 → 石油美元循环面临断裂风险，且私募信贷正积累类似2008年次贷危机的流动性隐患 → 一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街",
		KeyEvidence:     []string{"私募信贷基金通过监管套利进行期限错配，大量资金投向AI数据中心租约及SaaS企业贷款"},
		Predictions:     []string{"一旦私募信贷触发季度赎回上限，下季度极大概率发生全面挤兑并波及华尔街"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := "石油美元与私募信贷流动性风险正在推高美国资产脆弱性"
	if got.Headline != want {
		t.Fatalf("Headline = %q, want %q", got.Headline, want)
	}
}

func TestBuildCognitiveConclusion_AbstractsOilShockPattern(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-4",
		ThesisID:          "thesis-4",
		CoreQuestion:      "关于「油价」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3", "n4"},
		CompletenessScore: 1.0,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "condition", "n3": "conclusion", "n4": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-4",
		CausalThesisID:  "ct-4",
		Title:           "释放石油储备等舒缓性措施无法根本平抑油价，危机核心在于海峡封锁",
		Summary:         "国际能源组织协调释放4亿桶原油储备，但布伦特原油价格仍突破100美元/桶 → 若石油价格维持在每桶100美元 → 释放石油储备等舒缓性措施无法根本平抑油价，危机核心在于海峡封锁 → 布伦特原油价格将攀升至每桶130-150美元甚至更高",
		KeyEvidence:     []string{"国际能源组织协调释放4亿桶原油储备，但布伦特原油价格仍突破100美元/桶"},
		Predictions:     []string{"布伦特原油价格将攀升至每桶130-150美元甚至更高"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := "油价冲击与海峡封锁风险正在放大能源与市场压力"
	if got.Headline != want {
		t.Fatalf("Headline = %q, want %q", got.Headline, want)
	}
}

func TestBuildCognitiveConclusion_AbstractsJPMResiliencePattern(t *testing.T) {
	thesis := memory.CausalThesis{
		CausalThesisID:    "ct-5",
		ThesisID:          "thesis-5",
		CoreQuestion:      "关于「银行监管与金融系统安全」的判断",
		CorePathNodeIDs:   []string{"n1", "n2", "n3", "n4"},
		CompletenessScore: 1.0,
		AbstractionReady:  true,
		NodeRoles:         map[string]string{"n1": "fact", "n2": "mechanism", "n3": "conclusion", "n4": "prediction"},
	}
	cards := []memory.CognitiveCard{{
		CardID:          "card-5",
		CausalThesisID:  "ct-5",
		Title:           "宏观高利率与资产价格风险正在累积，但摩根大通具备抵御波动的能力",
		Summary:         "2025年摩根大通实现创纪录营收1856亿美元与净利润570亿美元，ROTCE达20% → 当前高资产价格环境在遭遇宏观负面冲击时将放大金融系统脆弱性 → 宏观高利率与资产价格风险正在累积，但摩根大通具备抵御波动的能力 → 摩根大通将在复杂宏观环境下维持长期稳健增长与股东回报",
		KeyEvidence:     []string{"2025年摩根大通实现创纪录营收1856亿美元与净利润570亿美元，ROTCE达20%"},
		Predictions:     []string{"摩根大通将在复杂宏观环境下维持长期稳健增长与股东回报"},
		ConfidenceLabel: "strong",
	}}

	got, ok := buildCognitiveConclusion(thesis, cards)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	want := "高利率与资产价格脆弱性并存，但头部银行仍展现经营韧性"
	if got.Headline != want {
		t.Fatalf("Headline = %q, want %q", got.Headline, want)
	}
}
