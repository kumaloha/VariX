package compilev2

import (
	"context"
	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
	"strings"
	"testing"
)

func graphNodesContainID(nodes []graphNode, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func TestClientCompileRecordsStageMetrics(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"Driver A"},{"id":"n2","text":"Target B"},{"id":"n3","text":"Driver C"}],"edges":[{"from":"n1","to":"n2"},{"from":"n3","to":"n2"}],"off_graph":[]}`, Model: "compilev2-model"},
		{Text: `{"replacements":[]}`, Model: "compilev2-model"},
		{Text: `{"aggregates":[]}`, Model: "compilev2-model"},
		{Text: `{"support_edges":[]}`, Model: "compilev2-model"},
		{Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"Driver A drives Target B","reason":"The quote directly states the relation."},{"from":"n3","to":"n2","source_quote":"Driver C drives Target B","reason":"The quote directly states the relation."}]}`, Model: "compilev2-model"},
		{Text: `{"translations":[{"id":"n1","text":"驱动A"},{"id":"n2","text":"目标B"},{"id":"n3","text":"驱动C"}]}`, Model: "compilev2-model"},
		{Text: `{"summary":"驱动A和驱动C推动目标B"}`, Model: "compilev2-model"},
	}}
	client := &Client{runtime: rt, model: "compilev2-model", projectRoot: ""}
	record, err := client.Compile(context.Background(), compile.Bundle{
		UnitID:     "web:v2-metrics",
		Source:     "web",
		ExternalID: "v2-metrics",
		Content:    "driver paragraph\n\ntarget paragraph",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if record.Metrics.CompileElapsedMS <= 0 {
		t.Fatalf("CompileElapsedMS = %d, want positive total metric", record.Metrics.CompileElapsedMS)
	}
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "relations", "classify", "render"} {
		if record.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("CompileStageElapsedMS = %#v, want positive duration for %q", record.Metrics.CompileStageElapsedMS, stage)
		}
	}
	for _, retired := range []string{"validate", "mainline", "reclassify", "cluster", "evidence", "explanation", "supplement"} {
		if _, ok := record.Metrics.CompileStageElapsedMS[retired]; ok {
			t.Fatalf("CompileStageElapsedMS = %#v, want no retired metric %q", record.Metrics.CompileStageElapsedMS, retired)
		}
	}
	if rt.calls == 0 {
		t.Fatal("expected runtime to be called at least once")
	}
}

func TestClientCompileNeverUsesValidateSearch(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"Driver A"},{"id":"n2","text":"Target B"},{"id":"n3","text":"Driver C"}],"edges":[{"from":"n1","to":"n2"},{"from":"n3","to":"n2"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"aggregates":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"Driver A drives Target B","reason":"The quote directly states the relation."},{"from":"n3","to":"n2","source_quote":"Driver C drives Target B","reason":"The quote directly states the relation."}]}`},
		{Text: `{"translations":[{"id":"n1","text":"驱动A"},{"id":"n2","text":"目标B"},{"id":"n3","text":"驱动C"}]}`},
		{Text: `{"summary":"驱动A和驱动C推动目标B"}`},
	}}
	client := &Client{runtime: rt, model: "unused-fallback", projectRoot: ""}
	_, err := client.Compile(context.Background(), compile.Bundle{
		UnitID:     "web:v2-routing",
		Source:     "web",
		ExternalID: "v2-routing",
		Content:    "driver paragraph\n\ntarget paragraph",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(rt.requests) == 0 {
		t.Fatal("expected requests to be recorded")
	}
	for i, req := range rt.requests {
		if req.Search {
			t.Fatalf("request %d uses search=true; compile must not run validate/search requests", i)
		}
		if req.Model != "unused-fallback" {
			t.Fatalf("request %d search=false model = %q, want %q", i, req.Model, "unused-fallback")
		}
	}
}

func TestStage1ExtractPreservesArticleFormAndNodeRoles(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"article_form":"main_narrative_plus_investment_implication","nodes":[{"id":"n1","text":"War desensitization drives US stock highs","source_quote":"markets are desensitized and stocks hit highs","role":"thesis"},{"id":"n2","text":"Oil prices erode consumer confidence","source_quote":"oil prices erode consumer confidence","role":"mechanism"},{"id":"n3","text":"FactSet reports earnings growth","source_quote":"FactSet reports earnings growth","role":"evidence"}],"off_graph":[]}`,
	}}}
	state, err := stage1Extract(context.Background(), rt, "compilev2-model", compile.Bundle{
		UnitID:  "web:v2-extract-form-role",
		Content: "markets are desensitized and stocks hit highs. oil prices erode consumer confidence. FactSet reports earnings growth.",
	})
	if err != nil {
		t.Fatalf("stage1Extract() error = %v", err)
	}
	if state.ArticleForm != "main_narrative_plus_investment_implication" {
		t.Fatalf("ArticleForm = %q", state.ArticleForm)
	}
	if got := state.Nodes[0].DiscourseRole; got != "thesis" {
		t.Fatalf("node role = %q, want thesis", got)
	}
	if got := state.Nodes[2].DiscourseRole; got != "evidence" {
		t.Fatalf("evidence node role = %q, want evidence", got)
	}
}

func TestStage1ExtractPromotesMacroFrameworkFromLongFormMacroMarkers(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"article_form":"main_narrative_plus_investment_implication","nodes":[{"id":"n1","text":"法币是劳动财富的载体","source_quote":"法币是劳动财富的载体","role":"mechanism"},{"id":"n2","text":"信用承诺积累推高债务风险","source_quote":"信用承诺积累推高债务风险","role":"thesis"},{"id":"n3","text":"人口老龄化导致税基净减少","source_quote":"人口老龄化导致税基净减少","role":"mechanism"},{"id":"n4","text":"主权债务压力迫使内部金融压抑","source_quote":"主权债务压力迫使内部金融压抑","role":"implication"},{"id":"n5","text":"美元信用需要外部信任支撑","source_quote":"美元信用需要外部信任支撑","role":"mechanism"}],"off_graph":[]}`,
	}}}
	state, err := stage1Extract(context.Background(), rt, "compilev2-model", compile.Bundle{
		UnitID:  "youtube:v2-form-macro-framework",
		Source:  "youtube",
		Content: "法币、信用、债务、人口老龄化、税基、主权债务、美元信用。",
	})
	if err != nil {
		t.Fatalf("stage1Extract() error = %v", err)
	}
	if state.ArticleForm != "macro_framework" {
		t.Fatalf("ArticleForm = %q, want macro_framework", state.ArticleForm)
	}
}

func TestSerializeRelationNodesIncludesDiscourseRole(t *testing.T) {
	body := serializeRelationNodes([]graphNode{{
		ID:            "n1",
		Text:          "Oil prices erode consumer confidence",
		SourceQuote:   "oil prices erode consumer confidence",
		DiscourseRole: "mechanism",
	}})
	if !strings.Contains(body, "discourse_role=mechanism") {
		t.Fatalf("serialized nodes = %q, want discourse_role", body)
	}
}

func TestStage5SummaryPromptIncludesArticleForm(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"通过叙事包装游戏为公平"},{"id":"n2","text":"游戏结束后本金归管理者基金"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺叙事包装下的财富转移。"}`},
	}}
	_, err := stage5Render(context.Background(), rt, "compilev2-model", compile.Bundle{
		UnitID:  "bilibili:summary-article-form",
		Source:  "bilibili",
		Content: "A village lottery is a satire.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "通过叙事包装游戏为公平", Role: roleDriver},
			{ID: "n2", Text: "游戏结束后本金归管理者基金", Role: roleTransmission, IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	summaryPrompt := ""
	for _, req := range rt.requests {
		if req.JSONSchema != nil && req.JSONSchema.Name == "compile_summary" {
			for _, part := range req.UserParts {
				if part.Type == "text" {
					summaryPrompt += part.Text
				}
			}
		}
	}
	if !strings.Contains(summaryPrompt, `"article_form":"satirical_financial_commentary"`) {
		t.Fatalf("summary prompt missing article_form:\n%s", summaryPrompt)
	}
}

func TestRefineArticleFormDetectsSatiricalFinancialCommentaryFromRoles(t *testing.T) {
	got := refineArticleFormFromExtract(compile.Bundle{
		Source:  "bilibili",
		Content: "村长和新富设计幸运游戏，用叙事把不公平包装成公平，忽悠后面的人进来。",
	}, graphState{
		ArticleForm: "main_narrative_plus_investment_implication",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "游戏本质不公平但可包装成公平", DiscourseRole: "satire_target"},
			{ID: "n3", Text: "后75%参与者承担机会成本", DiscourseRole: "implication"},
		},
	})
	if got != "satirical_financial_commentary" {
		t.Fatalf("article form = %q, want satirical_financial_commentary", got)
	}
}

func TestRefineArticleFormPreservesPureInstitutionalSatire(t *testing.T) {
	got := refineArticleFormFromExtract(compile.Bundle{
		Source:  "bilibili",
		Content: "村长和新富设计幸运游戏，用叙事把不公平包装成公平，忽悠后面的人进来。",
	}, graphState{
		ArticleForm: "institutional_satire",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "游戏本质不公平但可包装成公平", DiscourseRole: "satire_target"},
			{ID: "n3", Text: "后75%参与者承担机会成本", DiscourseRole: "implication"},
		},
	})
	if got != "institutional_satire" {
		t.Fatalf("article form = %q, want institutional_satire", got)
	}
}

func TestClientCompilePassesArticleContextToAggregate(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"高利率维持高位","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n2","text":"股票价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n3","text":"债券价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n4","text":"房产价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"aggregates":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"高利率压低所有资产价格","reason":"The quote states the pressure."}]}`},
		{Text: `{"translations":[{"id":"n1","text":"高利率维持高位"},{"id":"n2","text":"股票价格被压低"}]}`},
		{Text: `{"summary":"高利率压低股票价格。"}`},
	}}
	client := &Client{runtime: rt, model: "compilev2-model", projectRoot: ""}
	_, err := client.Compile(context.Background(), compile.Bundle{
		UnitID:     "web:v2-aggregate-article",
		Source:     "web",
		ExternalID: "v2-aggregate-article",
		Content:    "High rates depress all asset prices. This article context belongs in aggregate.",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	aggregatePrompt := ""
	for _, req := range rt.requests {
		if req.JSONSchema != nil && req.JSONSchema.Name == "compile_aggregate" {
			for _, part := range req.UserParts {
				if part.Type == "text" {
					aggregatePrompt += part.Text
				}
			}
		}
	}
	if aggregatePrompt == "" {
		t.Fatal("aggregate request was not recorded")
	}
	if !strings.Contains(aggregatePrompt, "This article context belongs in aggregate") {
		t.Fatalf("aggregate prompt missing article context:\n%s", aggregatePrompt)
	}
}
