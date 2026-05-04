package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"strings"
	"testing"
)

func TestSemanticCoverageKeepsManagementAnswersAsSpeakerClaims(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"semantic_units":[
			{"id":"llm-portfolio","span":"speaker_answer","speaker":"Greg Abel","speaker_role":"primary","subject":"existing portfolio / circle of competence","force":"answer","claim":"Greg Abel 的回答是：现有组合由 Warren Buffett 建立，但集中在他也理解业务和经济前景的公司，所以他对组合很舒服；之后会持续评估业务演化和新风险。Apple 是例子，说明伯克希尔判断能力圈不按“科技股”标签，而是看产品价值、消费者依赖、耐久性和风险。","prompt_context":"股东询问 Greg Abel 如何在能力圈不同的情况下管理 Warren Buffett 建立的组合。","source_quote":"technology stock / consumer valued","salience":0.93,"confidence":"high"},
			{"id":"llm-technology","span":"operating_update","speaker":"Greg Abel","speaker_role":"primary","subject":"technology / AI operating plan","force":"explain","claim":"Greg Abel 表示伯克希尔正在从购买技术转向建设技术能力，并把 AI 能力用于 GEICO、BNSF 等运营业务。","source_quote":"builder of technology","salience":0.88,"confidence":"high"},
			{"id":"llm-cyber","span":"risk_boundary","speaker":"Greg Abel","speaker_role":"primary","subject":"cyber insurance underwriting boundary","force":"set_boundary","claim":"伯克希尔面对网络保险的做法是克制承保：如果累计风险不能被可靠建模、价格又不足，就宁可不写；只有在能理解累计敞口并拿到足够价格时才可能承保。","source_quote":"cyber aggregation premiums","salience":0.84,"confidence":"high"},
			{"id":"llm-tokyo","span":"management_disclosure","speaker":"Greg Abel","speaker_role":"primary","subject":"Tokyo Marine strategic transaction","force":"disclose","claim":"Greg Abel 披露伯克希尔与东京海上的交易不是单点投资，而是三部分：买入约2.5%股权、承接一部分财险业务组合，并签订战略合作协议。","source_quote":"tokyo marine strategic agreement","salience":0.8,"confidence":"high"},
			{"id":"llm-culture","span":"management_boundary","speaker":"Greg Abel","speaker_role":"primary","subject":"culture and succession","force":"set_boundary","claim":"Greg Abel 强调伯克希尔换届后文化和价值观不会改变，董事会也认真处理关键岗位继任计划。","source_quote":"culture values succession","salience":0.78,"confidence":"high"}
		]}`,
	}}}
	text := strings.Join([]string{
		"Chinese shareholder asked how Greg Abel would manage Warren Buffett's equity portfolio and whether his circle of competence differs.",
		"Greg Abel answered that he is comfortable with the existing portfolio.",
		"I would not say we invest in Apple because we view it as a technology stock.",
		"We invest in Apple because we understand the product, the value to consumers, durability, and the risk around that business.",
		"Greg also said GEICO is undergoing a technology transformation.",
		"We are becoming a builder of technology, not simply a buyer, and we can use narrow artificial intelligence and large logic models across the operating businesses.",
		"On cyber insurance, Greg said aggregation risk is difficult to model, premiums have been coming down, and supply is greater than demand.",
		"On Tokyo Marine, Greg disclosed Berkshire bought 2 and a half percent of the stock, took a piece of the property casualty business, and signed a strategic agreement.",
		"Greg said Berkshire's culture and values did not change and the board has a succession plan in place.",
	}, "\n")

	state, err := stageSemanticCoverage(context.Background(), rt, "compile-model", Bundle{Source: "youtube", Content: text}, graphState{ArticleForm: "shareholder_meeting"})
	if err != nil {
		t.Fatalf("stageSemanticCoverage() error = %v", err)
	}
	if len(state.SemanticUnits) < 5 {
		t.Fatalf("SemanticUnits = %#v, want broad management answer coverage", state.SemanticUnits)
	}

	apple := semanticUnitBySubject(state.SemanticUnits, "portfolio")
	if apple == nil {
		t.Fatalf("SemanticUnits = %#v, missing portfolio/circle unit", state.SemanticUnits)
	}
	if apple.SpeakerRole != "primary" || apple.Force != "answer" {
		t.Fatalf("Portfolio unit = %#v, want primary answer", *apple)
	}
	if !strings.Contains(apple.Claim, "现有组合") || !strings.Contains(apple.Claim, "持续评估") || !strings.Contains(apple.Claim, "不按“科技股”标签") {
		t.Fatalf("Portfolio claim = %q, want full answer plus Apple example", apple.Claim)
	}
	if !strings.Contains(apple.PromptContext, "股东") || strings.Contains(apple.Claim, "股东询问") {
		t.Fatalf("Apple prompt/claim separation failed: %#v", *apple)
	}

	technology := semanticUnitBySubject(state.SemanticUnits, "technology")
	if technology == nil {
		t.Fatalf("SemanticUnits = %#v, missing technology unit", state.SemanticUnits)
	}
	if !strings.Contains(technology.Claim, "建设技术能力") || !strings.Contains(technology.Claim, "AI") {
		t.Fatalf("Technology claim = %q, want builder/AI operating plan", technology.Claim)
	}
	cyber := semanticUnitBySubject(state.SemanticUnits, "cyber")
	if cyber == nil || cyber.Force != "set_boundary" || !strings.Contains(cyber.Claim, "宁可不写") || !strings.Contains(cyber.Claim, "足够价格") {
		t.Fatalf("Cyber unit = %#v, want underwriting boundary", cyber)
	}
	tokyo := semanticUnitBySubject(state.SemanticUnits, "tokyo")
	if tokyo == nil || tokyo.Force != "disclose" || !strings.Contains(tokyo.Claim, "三部分") {
		t.Fatalf("Tokyo unit = %#v, want transaction disclosure", tokyo)
	}
}

func TestSemanticCoverageUsesLLMSpeakerSweep(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"semantic_units":[{"id":"semantic-operating-plan","speaker":"Greg Abel","speaker_role":"primary","subject":"operating plan","force":"explain","claim":"Greg Abel 表示系统会输出经营计划，而不是只靠确定性锚点。","source_quote":"we will use the system to produce operating plan coverage","salience":0.91,"confidence":"high"}]}`,
	}}}
	state, err := stageSemanticCoverage(context.Background(), rt, "compile-model", Bundle{
		Source:  "youtube",
		Content: strings.Repeat("management Q&A content ", 400),
	}, graphState{ArticleForm: "shareholder_meeting"})
	if err != nil {
		t.Fatalf("stageSemanticCoverage() error = %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("runtime calls = %d, want LLM semantic sweep", rt.calls)
	}
	unit := semanticUnitBySubject(state.SemanticUnits, "operating plan")
	if unit == nil || !strings.Contains(unit.Claim, "系统会输出经营计划") {
		t.Fatalf("SemanticUnits = %#v, missing LLM unit", state.SemanticUnits)
	}
}

func TestSemanticCoverageChunksLongArticles(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"semantic_units":[{"id":"chunk-1","speaker":"Speaker A","speaker_role":"primary","subject":"capital allocation","force":"commit","claim":"第一段输出资本配置规则。","source_quote":"first chunk quote","salience":0.9,"confidence":"high"}]}`},
		{Text: `{"semantic_units":[{"id":"chunk-2","speaker":"Speaker A","speaker_role":"primary","subject":"AI governance","force":"set_boundary","claim":"第二段输出 AI 治理边界。","source_quote":"second chunk quote","salience":0.85,"confidence":"high"}]}`},
		{Text: `{"semantic_units":[]}`},
		{Text: `{"semantic_units":[]}`},
		{Text: `{"semantic_units":[]}`},
		{Text: `{"semantic_units":[]}`},
	}}
	firstParts := make([]string, 0, 700)
	secondParts := make([]string, 0, 700)
	for i := 0; i < 700; i++ {
		firstParts = append(firstParts, "first management answer about capital allocation unit "+string(rune('a'+(i%26)))+" .")
		secondParts = append(secondParts, "second management answer about AI governance unit "+string(rune('a'+(i%26)))+" .")
	}
	content := strings.Join(firstParts, " ") + strings.Join(secondParts, " ")
	state, err := stageSemanticCoverage(context.Background(), rt, "compile-model", Bundle{
		Source:  "youtube",
		Content: content,
	}, graphState{ArticleForm: "shareholder_meeting"})
	if err != nil {
		t.Fatalf("stageSemanticCoverage() error = %v", err)
	}
	if rt.calls < 2 {
		t.Fatalf("runtime calls = %d, want chunked semantic sweep", rt.calls)
	}
	if semanticUnitBySubject(state.SemanticUnits, "capital allocation") == nil || semanticUnitBySubject(state.SemanticUnits, "AI governance") == nil {
		t.Fatalf("SemanticUnits = %#v, want units from multiple chunks", state.SemanticUnits)
	}
}

func TestSemanticCoverageCompactsRepeatedTranscriptPhrases(t *testing.T) {
	raw := strings.Repeat("Greg said Greg said Greg said we will not write cyber insurance unless we understand the aggregation risk. ", 20)
	compacted := compactSemanticCoverageArticle(raw)
	if len([]rune(compacted)) >= len([]rune(raw))/2 {
		t.Fatalf("compacted transcript still too large: raw=%d compacted=%d", len([]rune(raw)), len([]rune(compacted)))
	}
	if !strings.Contains(compacted, "we will not write cyber insurance unless we understand the aggregation risk") {
		t.Fatalf("compacted transcript lost semantic content: %q", compacted)
	}
}

func TestSemanticCoverageRanksAndLimitsUnits(t *testing.T) {
	units := make([]SemanticUnit, 0, 14)
	for i := 0; i < 14; i++ {
		units = append(units, SemanticUnit{
			ID:          "u" + string(rune('a'+i)),
			SpeakerRole: "primary",
			Subject:     "subject " + string(rune('a'+i)),
			Force:       "explain",
			Claim:       "claim",
			Salience:    float64(i) / 20,
			Confidence:  "medium",
		})
	}
	got := rankSemanticUnits(units, "shareholder_meeting")
	if len(got) != 10 {
		t.Fatalf("len(rankSemanticUnits) = %d, want 10", len(got))
	}
	if got[0].Salience < got[len(got)-1].Salience {
		t.Fatalf("rankSemanticUnits not sorted by salience: %#v", got)
	}
}

func TestSemanticCoverageDedupeLLMCategories(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"semantic_units":[
			{"id":"llm-cyber-a","speaker":"Ajit Jain","speaker_role":"primary","subject":"网络保险承保策略","force":"reject","claim":"伯克希尔拒绝承保网络保险，因为聚合风险无法建模且价格不足。","source_quote":"cyber aggregation risk cannot be modeled","salience":0.9,"confidence":"high"},
			{"id":"llm-cyber-b","speaker":"Ajit Jain","speaker_role":"primary","subject":"cyber insurance underwriting boundary","force":"set_boundary","claim":"伯克希尔只有理解累计风险且价格足够时才会承保网络保险。","source_quote":"premiums have been coming down","salience":0.8,"confidence":"medium"}
		]}`,
	}}}
	text := strings.Join([]string{
		strings.Repeat("management Q&A content ", 300),
		"On cyber insurance, Ajit said aggregation risk is difficult to model, premiums have been coming down, and supply is greater than demand.",
	}, "\n")
	state, err := stageSemanticCoverage(context.Background(), rt, "compile-model", Bundle{Source: "youtube", Content: text}, graphState{ArticleForm: "shareholder_meeting"})
	if err != nil {
		t.Fatalf("stageSemanticCoverage() error = %v", err)
	}
	count := 0
	for _, unit := range state.SemanticUnits {
		if semanticCoverageCategory(unit) == "cyber_insurance" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("cyber semantic units = %d in %#v, want only LLM category", count, state.SemanticUnits)
	}
}

func TestSemanticCoverageAssignsGlobalIDs(t *testing.T) {
	units := assignSemanticUnitIDs([]SemanticUnit{
		{ID: "semantic-001", Subject: "capital allocation", Claim: "Deploy cash when market dislocation creates a large opportunity.", Force: "commit"},
		{ID: "semantic-001", Subject: "AI governance", Claim: "Use AI in operations while preserving accountability.", Force: "set_boundary"},
	})
	if len(units) != 2 {
		t.Fatalf("len(assignSemanticUnitIDs) = %d, want 2", len(units))
	}
	seen := map[string]struct{}{}
	for _, unit := range units {
		if _, ok := seen[unit.ID]; ok {
			t.Fatalf("duplicate semantic unit id %q in %#v", unit.ID, units)
		}
		seen[unit.ID] = struct{}{}
	}
	if units[0].ID != "semantic-001" || units[1].ID != "semantic-002" {
		t.Fatalf("semantic unit ids = %#v, want global sequential IDs", units)
	}
}

func semanticUnitBySubject(units []SemanticUnit, needle string) *SemanticUnit {
	needle = strings.ToLower(needle)
	for i := range units {
		if strings.Contains(strings.ToLower(units[i].Subject), needle) {
			return &units[i]
		}
	}
	return nil
}
