package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"strings"
	"testing"
)

func TestSemanticCoverageKeepsManagementAnswersAsSpeakerClaims(t *testing.T) {
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

	state, err := stageSemanticCoverage(context.Background(), nil, "", Bundle{Source: "youtube", Content: text}, graphState{ArticleForm: "shareholder_meeting"})
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

func TestSemanticCoverageFallbacksDoNotDuplicateLLMCategories(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"semantic_units":[{"id":"llm-cyber","speaker":"Ajit Jain","speaker_role":"primary","subject":"网络保险承保策略","force":"reject","claim":"伯克希尔拒绝承保网络保险，因为聚合风险无法建模且价格不足。","source_quote":"cyber aggregation risk cannot be modeled","salience":0.9,"confidence":"high"}]}`,
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
