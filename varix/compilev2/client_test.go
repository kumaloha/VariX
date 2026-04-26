package compilev2

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/forge/llm"
)

type fakeRuntime struct {
	responses []llm.Response
	requests  []llm.ProviderRequest
	calls     int
}

func (f *fakeRuntime) Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error) {
	f.requests = append(f.requests, req)
	if f.calls >= len(f.responses) {
		return llm.Response{}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func TestClientCompileRecordsStageMetrics(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"Driver A"},{"id":"n2","text":"Target B"},{"id":"n3","text":"Driver C"}],"edges":[{"from":"n1","to":"n2"},{"from":"n3","to":"n2"}],"off_graph":[]}`, Model: "compilev2-model"},
		{Text: `{"replacements":[]}`, Model: "compilev2-model"},
		{Text: `{"aggregates":[]}`, Model: "compilev2-model"},
		{Text: `{"support_edges":[]}`, Model: "compilev2-model"},
		{Text: `{"drives_edges":[{"from":"n1","to":"n2","source_quote":"Driver A drives Target B","reason":"The quote directly states the relation."},{"from":"n3","to":"n2","source_quote":"Driver C drives Target B","reason":"The quote directly states the relation."}]}`, Model: "compilev2-model"},
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
	for _, stage := range []string{"extract", "refine", "aggregate", "support", "collapse", "mainline", "classify", "render"} {
		if record.Metrics.CompileStageElapsedMS[stage] <= 0 {
			t.Fatalf("CompileStageElapsedMS = %#v, want positive duration for %q", record.Metrics.CompileStageElapsedMS, stage)
		}
	}
	for _, retired := range []string{"validate", "relations", "reclassify", "cluster", "evidence", "explanation", "supplement"} {
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
		{Text: `{"drives_edges":[{"from":"n1","to":"n2","source_quote":"Driver A drives Target B","reason":"The quote directly states the relation."},{"from":"n3","to":"n2","source_quote":"Driver C drives Target B","reason":"The quote directly states the relation."}]}`},
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

func TestClientCompilePassesArticleContextToMainline(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"Middle East buys arms","source_quote":"they buy arms"},{"id":"n2","text":"Less money buys US bonds and stocks","source_quote":"less money buys US bonds and stocks"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"aggregates":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"drives_edges":[{"from":"n1","to":"n2","source_quote":"If they buy arms, less money buys US bonds and stocks","reason":"The article states the spending shift reduces capital for US assets."}]}`},
		{Text: `{"translations":[{"id":"n1","text":"中东买军火"},{"id":"n2","text":"买美债美股的钱减少"}]}`},
		{Text: `{"summary":"中东买军火导致买美债美股的钱减少。"}`},
	}}
	client := &Client{runtime: rt, model: "compilev2-model", projectRoot: ""}
	_, err := client.Compile(context.Background(), compile.Bundle{
		UnitID:     "web:v2-mainline-article",
		Source:     "web",
		ExternalID: "v2-mainline-article",
		Content:    "If they buy arms, less money buys US bonds and stocks. This context is the edge signal.",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	mainlinePrompt := ""
	for _, req := range rt.requests {
		if req.JSONSchema != nil && req.JSONSchema.Name == "compile_mainline" {
			for _, part := range req.UserParts {
				if part.Type == "text" {
					mainlinePrompt += part.Text
				}
			}
		}
	}
	if mainlinePrompt == "" {
		t.Fatal("mainline request was not recorded")
	}
	if !strings.Contains(mainlinePrompt, "This context is the edge signal") {
		t.Fatalf("mainline prompt missing article context:\n%s", mainlinePrompt)
	}
}

func TestClientCompilePassesArticleContextToAggregate(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"高利率维持高位","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n2","text":"股票价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n3","text":"债券价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n4","text":"房产价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"aggregates":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"drives_edges":[{"from":"n1","to":"n2","source_quote":"高利率压低所有资产价格","reason":"The quote states the pressure."}]}`},
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

func TestPreviewFlowStopAfterMainlineDoesNotRender(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"高利率维持高位","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n2","text":"股票价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n3","text":"债券价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n4","text":"房产价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"aggregates":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"drives_edges":[{"from":"n1","to":"n2","source_quote":"高利率压低所有资产价格","reason":"The quote states the pressure."}]}`},
	}}
	client := &Client{runtime: rt, model: "compilev2-model", projectRoot: ""}
	result, err := client.PreviewFlow(context.Background(), compile.Bundle{
		UnitID:     "web:v2-stop-mainline",
		Source:     "web",
		ExternalID: "v2-stop-mainline",
		Content:    "高利率压低所有资产价格。",
	}, FlowPreviewOptions{StopAfter: "mainline"})
	if err != nil {
		t.Fatalf("PreviewFlow() error = %v", err)
	}
	if len(result.Relations.Edges) != 1 {
		t.Fatalf("len(Relations.Edges) = %d, want 1", len(result.Relations.Edges))
	}
	if len(result.Classify.Nodes) != 0 {
		t.Fatalf("Classify populated despite stop-after mainline: %#v", result.Classify.Nodes)
	}
	if len(result.Render.Drivers) != 0 || len(result.Render.Targets) != 0 || len(result.Render.TransmissionPaths) != 0 {
		t.Fatalf("Render populated despite stop-after mainline: %#v", result.Render)
	}
	if rt.calls != 5 {
		t.Fatalf("runtime calls = %d, want 5 through mainline only", rt.calls)
	}
}

func TestStage3MainlinePrunesShortcutWhenMechanismPathExists(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"drives_edges":[{"from":"n16","to":"n19","source_quote":"redemptions trigger caps","reason":"mechanism"},{"from":"n19","to":"n20","source_quote":"caps trigger suspension","reason":"mechanism"},{"from":"n20","to":"n23","source_quote":"suspension triggers run","reason":"mechanism"},{"from":"n16","to":"n23","source_quote":"redemptions trigger run","reason":"shortcut"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compilev2-model", compile.Bundle{
		UnitID:  "web:v2-mainline-shortcut",
		Content: "redemptions trigger caps, caps trigger suspension, suspension triggers run",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n16", Text: "高净值客户集中要求赎回私募信贷基金", SourceQuote: "redemptions trigger caps"},
			{ID: "n19", Text: "私募信贷基金设置季度赎回上限", SourceQuote: "caps trigger suspension"},
			{ID: "n20", Text: "私募信贷基金暂停当期赎回", SourceQuote: "suspension triggers run"},
			{ID: "n23", Text: "私募信贷基金发生挤兑概率大幅上升", SourceQuote: "run"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if hasEdge(state.Edges, "n16", "n23") {
		t.Fatalf("shortcut n16->n23 should be pruned when mechanism path exists: %#v", state.Edges)
	}
	for _, want := range [][2]string{{"n16", "n19"}, {"n19", "n20"}, {"n20", "n23"}} {
		if !hasEdge(state.Edges, want[0], want[1]) {
			t.Fatalf("mechanism edge %s->%s missing: %#v", want[0], want[1], state.Edges)
		}
	}
}

func TestStage3ClassifyMarksOnlyBranchTerminalMarketOutcomesAsTargets(t *testing.T) {
	state, err := stage3Classify(context.Background(), nil, "", compile.Bundle{}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "中东国家购买美债美股的资金减少"},
			{ID: "n2", Text: "美股面临资金流出压力"},
			{ID: "n3", Text: "美债面临资金流出压力"},
			{ID: "n4", Text: "高净值客户集中要求赎回私募信贷基金"},
			{ID: "n5", Text: "私募信贷基金设置季度赎回比例上限"},
			{ID: "n6", Text: "私募信贷基金触发季度赎回上限"},
			{ID: "n7", Text: "下季度集中赎回潮爆发"},
			{ID: "n8", Text: "私募信贷基金发生流动性危机概率上升"},
			{ID: "n9", Text: "AI交易资金流入减少"},
			{ID: "n10", Text: "全球流动性环境恶化"},
			{ID: "n11", Text: "孤立市场压力上升"},
			{ID: "n12", Text: "投资者恐慌性集中赎回"},
			{ID: "n13", Text: "私募信贷贷款面临坏账风险"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n1", To: "n3"},
			{From: "n4", To: "n5"},
			{From: "n5", To: "n6"},
			{From: "n6", To: "n7"},
			{From: "n6", To: "n12"},
			{From: "n7", To: "n8"},
			{From: "n1", To: "n9"},
			{From: "n1", To: "n10"},
			{From: "n1", To: "n13"},
			{From: "n13", To: "n12"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Classify() error = %v", err)
	}
	targets := map[string]bool{}
	roles := map[string]graphRole{}
	for _, node := range state.Nodes {
		targets[node.ID] = node.IsTarget
		roles[node.ID] = node.Role
	}
	ontology := map[string]string{}
	for _, node := range state.Nodes {
		ontology[node.ID] = node.Ontology
	}
	for _, id := range []string{"n2", "n3", "n7", "n8", "n9", "n12", "n13"} {
		if !targets[id] {
			t.Fatalf("%s should be target; targets=%#v", id, targets)
		}
	}
	for _, id := range []string{"n1", "n4", "n5", "n6", "n10", "n11"} {
		if targets[id] {
			t.Fatalf("%s should not be target; targets=%#v", id, targets)
		}
	}
	if ontology["n8"] != "flow" {
		t.Fatalf("n8 ontology = %q, want flow", ontology["n8"])
	}
	for _, id := range []string{"n1", "n4"} {
		if roles[id] != roleDriver {
			t.Fatalf("%s role = %s, want driver", id, roles[id])
		}
	}
}

func TestStage5RenderRecoversTargetFromOffGraphWhenMainlineHasOnlyDriver(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"石油美元闭环形成"},{"id":"off1","text":"私募信贷赎回门和流动性挤兑风险上升"},{"id":"off2","text":"中东资金减少购买美债美股"}]}`},
		{Text: `{"summary":"石油美元闭环受冲击，私募信贷赎回门和流动性挤兑风险上升。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compilev2-model", compile.Bundle{
		UnitID:     "weibo:QAJ0n0YGU",
		Source:     "weibo",
		ExternalID: "QAJ0n0YGU",
		Content:    "石油美元闭环变化可能导致私募信贷赎回门和流动性挤兑。",
	}, graphState{
		Nodes: []graphNode{{
			ID:   "n1",
			Text: "石油美元闭环形成",
			Role: roleDriver,
		}},
		OffGraph: []offGraphItem{
			{
				ID:         "off1",
				Text:       "私募信贷赎回门和流动性挤兑风险上升",
				Role:       "evidence",
				AttachesTo: "n1",
			},
			{
				ID:         "off2",
				Text:       "中东资金减少购买美债美股",
				Role:       "supplementary",
				AttachesTo: "n1",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Targets) == 0 {
		t.Fatalf("Targets = %#v, want fallback target recovered from off_graph", out.Targets)
	}
	if out.Targets[0] != "私募信贷赎回门和流动性挤兑风险上升" {
		t.Fatalf("Targets[0] = %q, want private-credit liquidity target", out.Targets[0])
	}
}

func TestStage5RenderDirectPathHasNonEmptySteps(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"驱动A"},{"id":"n2","text":"目标B"}]}`},
		{Text: `{"summary":"驱动A推动目标B。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compilev2-model", compile.Bundle{
		UnitID:     "web:direct-path",
		Source:     "web",
		ExternalID: "direct-path",
		Content:    "Driver A drives target B.",
		PostedAt:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Driver A", Role: roleDriver},
			{ID: "n2", Text: "Target B", Role: roleTransmission, IsTarget: true, Ontology: "flow"},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want one direct path", out.TransmissionPaths)
	}
	if len(out.TransmissionPaths[0].Steps) == 0 {
		t.Fatalf("TransmissionPaths[0].Steps = %#v, want non-empty direct-path fallback", out.TransmissionPaths[0].Steps)
	}
	if out.TransmissionPaths[0].Steps[0] != "驱动A" {
		t.Fatalf("TransmissionPaths[0].Steps[0] = %q, want driver text fallback", out.TransmissionPaths[0].Steps[0])
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Output.Validate() error = %v", err)
	}
}
