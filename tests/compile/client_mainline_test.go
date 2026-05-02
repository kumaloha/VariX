package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"strings"
	"testing"
)

func TestChooseClusterHeadPrefersMainlineRoles(t *testing.T) {
	nodeIndex := map[string]graphNode{
		"n1": {ID: "n1", Text: "FactSet reports earnings growth", DiscourseRole: "evidence"},
		"n2": {ID: "n2", Text: "Strong earnings drive US stock highs", DiscourseRole: "thesis"},
	}
	got := chooseClusterHead([]string{"n1", "n2"}, nil, nodeIndex)
	if got != "n2" {
		t.Fatalf("cluster head = %q, want thesis node n2", got)
	}
}

func TestMainlineCandidateEdgesSkipEvidenceAndExamples(t *testing.T) {
	nodes := []graphNode{
		{
			ID:            "n1",
			Text:          "earnings growth",
			SourceQuote:   "earnings growth drives stock highs",
			DiscourseRole: "evidence",
		},
		{
			ID:            "n2",
			Text:          "stock highs",
			SourceQuote:   "earnings growth drives stock highs",
			DiscourseRole: "thesis",
		},
	}
	got := serializeMainlineCandidateEdges("earnings growth drives stock highs", nodes)
	if got != "- (none)" {
		t.Fatalf("candidate edges = %q, want evidence/example nodes excluded from hints", got)
	}
}

func TestCollapseDemotesSupportingRolesWithoutAuxEdges(t *testing.T) {
	state := collapseClusters(graphState{Nodes: []graphNode{
		{ID: "n1", Text: "Strong earnings drive US stock highs", DiscourseRole: "thesis"},
		{ID: "n2", Text: "FactSet reports earnings growth", SourceQuote: "FactSet reports earnings growth", DiscourseRole: "evidence"},
		{ID: "n3", Text: "Intel is an illustrative case", SourceQuote: "Intel earnings improved", DiscourseRole: "example"},
	}})
	if got := strings.Join(state.BranchHeads, ","); got != "n1" {
		t.Fatalf("BranchHeads = %q, want only thesis node", got)
	}
	if len(state.Nodes) != 1 || state.Nodes[0].ID != "n1" {
		t.Fatalf("Nodes = %#v, want only thesis node retained", state.Nodes)
	}
	if len(state.OffGraph) != 2 {
		t.Fatalf("OffGraph = %#v, want evidence/example demoted", state.OffGraph)
	}
	for _, item := range state.OffGraph {
		if item.AttachesTo != "n1" {
			t.Fatalf("offgraph item = %#v, want attached to n1", item)
		}
	}
}

func TestCollapseKeepsEvidenceForEvidenceBackedForecast(t *testing.T) {
	state := collapseClusters(graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "World war is in early stages", DiscourseRole: "thesis"},
			{ID: "n2", Text: "Historical indicators resemble 1913-14 and 1938-39", SourceQuote: "roughly analogous to the 1913-14 and the 1938-39 periods", DiscourseRole: "evidence"},
			{ID: "n3", Text: "Some countries have less confidence the US will defend them", SourceQuote: "less confidence among some countries that the US will defend them", DiscourseRole: "evidence"},
			{ID: "n4", Text: "A side caveat is timing imprecision", SourceQuote: "there is nothing precise about these indicators", DiscourseRole: "caveat"},
		},
	})
	if !graphNodesContainID(state.Nodes, "n2") || !graphNodesContainID(state.Nodes, "n3") {
		t.Fatalf("Nodes = %#v, want forecast evidence retained for proof branches", state.Nodes)
	}
	if graphNodesContainID(state.Nodes, "n4") {
		t.Fatalf("Nodes = %#v, want caveat still demoted", state.Nodes)
	}
	if len(state.BranchHeads) != 3 {
		t.Fatalf("BranchHeads = %#v, want thesis plus retained evidence heads", state.BranchHeads)
	}
}

func TestCollapseKeepsInferentialConclusionAsHead(t *testing.T) {
	state := collapseClusters(graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "沃什主张大幅降息", SourceQuote: "沃什反复说利率应该大幅度下降", DiscourseRole: "evidence"},
			{ID: "n2", Text: "金融抑制可能正式开启", SourceQuote: "到那个时候金融抑制就是实打实开启新时代", DiscourseRole: "thesis"},
		},
		AuxEdges: []auxEdge{{
			From:        "n1",
			To:          "n2",
			Kind:        "inference",
			SourceQuote: "沃什反复说利率应该大幅度下降... 到那个时候金融抑制就是实打实开启新时代",
			Reason:      "The policy clue supports the inferred financial-repression conclusion.",
		}},
	})
	if len(state.Nodes) != 1 || state.Nodes[0].ID != "n2" {
		t.Fatalf("Nodes = %#v, want inferred conclusion retained as graph head", state.Nodes)
	}
	if len(state.OffGraph) != 1 || state.OffGraph[0].Role != "inference" || state.OffGraph[0].AttachesTo != "n2" {
		t.Fatalf("OffGraph = %#v, want inference premise attached to conclusion", state.OffGraph)
	}
	evidence, _, supplementary := renderOffGraph(state.OffGraph, func(_ string, fallback string) string { return fallback })
	if !containsString(evidence, "沃什主张大幅降息") {
		t.Fatalf("Evidence = %#v, want inference premise displayed as evidence", evidence)
	}
	if len(supplementary) != 0 {
		t.Fatalf("Supplementary = %#v, want inference excluded from supplementary", supplementary)
	}
}

func TestClientCompilePassesArticleContextToMainline(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"Middle East buys arms","source_quote":"they buy arms"},{"id":"n2","text":"Less money buys US bonds and stocks","source_quote":"less money buys US bonds and stocks"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"If they buy arms, less money buys US bonds and stocks","reason":"The article states the spending shift reduces capital for US assets."}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Arms purchases reduce capital for US assets","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"primary relation"}]}`},
		{Text: `{"missing_nodes":[],"missing_edges":[],"misclassified":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"If they buy arms, less money buys US bonds and stocks","reason":"The article states the spending shift reduces capital for US assets."}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Arms purchases reduce capital for US assets","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"primary relation"}]}`},
		{Text: `{"summary":"中东买军火导致买美债美股的钱减少。"}`},
	}}
	client := &Client{runtime: rt, model: "compile-model", projectRoot: ""}
	_, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:mainline-article",
		Source:     "web",
		ExternalID: "mainline-article",
		Content:    "If they buy arms, less money buys US bonds and stocks. This context is the edge signal.",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	mainlinePrompt := ""
	for _, req := range rt.requests {
		if req.JSONSchema != nil && req.JSONSchema.Name == "compile_relations" {
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

func TestStage3MainlinePreservesInferenceRelationKind(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","kind":"inference","source_quote":"沃什主张大幅降息，因此金融抑制可能开启","reason":"The Warsh research clue is used to infer the financial repression forecast."},{"from":"n2","to":"n3","kind":"causal","source_quote":"金融抑制开启后现金购买力会贬值","reason":"The forecast regime drives the cash purchasing-power implication."}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"调研证据推导金融抑制并影响现金","node_ids":["n1","n2","n3"],"edge_indexes":[0,1],"scope":"article","why":"This is the proof-backed forecast spine."}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:inference-mainline",
		Source:  "web",
		Content: "沃什主张大幅降息，因此金融抑制可能开启。金融抑制开启后现金购买力会贬值。",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "沃什主张大幅降息", DiscourseRole: "evidence"},
			{ID: "n2", Text: "金融抑制可能开启", DiscourseRole: "thesis"},
			{ID: "n3", Text: "现金购买力会贬值", DiscourseRole: "implication"},
		},
		BranchHeads: []string{"n2", "n3"},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Edges) != 2 {
		t.Fatalf("Edges = %#v, want two edges", state.Edges)
	}
	if state.Edges[0].Kind != "inference" {
		t.Fatalf("Edges[0].Kind = %q, want inference", state.Edges[0].Kind)
	}
	if state.Edges[1].Kind != "causal" {
		t.Fatalf("Edges[1].Kind = %q, want causal", state.Edges[1].Kind)
	}
	if len(state.Spines) != 1 || len(state.Spines[0].Edges) != 2 || state.Spines[0].Edges[0].Kind != "inference" {
		t.Fatalf("Spines = %#v, want inference edge preserved in spine", state.Spines)
	}
}

func TestStage3MainlinePreservesMixedSpinePolicyAndIllustrationKind(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","kind":"illustration","source_quote":"村长和新富设计幸运游戏，这就像现实里用牌照包装金融游戏","reason":"The allegory illustrates the real institutional mechanism instead of mechanically causing it."},{"from":"n2","to":"n3","kind":"causal","source_quote":"规则包装后，后来的参与者承担机会成本和管理费","reason":"The quote states the packaged mechanism transfers costs to later participants."}],"spines":[{"id":"s1","level":"primary","priority":1,"policy":"satirical_analogy","thesis":"幸运游戏讽刺牌照化金融规则转嫁成本","node_ids":["n1","n2","n3"],"edge_indexes":[0,1],"scope":"article","why":"The article's primary argument is made through a satire/allegory."}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-mainline",
		Source:  "bilibili",
		Content: "村长和新富设计幸运游戏，这就像现实里用牌照包装金融游戏。规则包装后，后来的参与者承担机会成本和管理费。",
	}, graphState{
		ArticleForm: "institutional_satire",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "牌照化金融游戏把资金转移包装成公平规则", DiscourseRole: "thesis"},
			{ID: "n3", Text: "多数后续参与者承担机会成本与管理费", DiscourseRole: "implication"},
		},
		BranchHeads: []string{"n2", "n3"},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Edges) != 2 {
		t.Fatalf("Edges = %#v, want two edges", state.Edges)
	}
	if state.Edges[0].Kind != "illustration" {
		t.Fatalf("Edges[0].Kind = %q, want illustration", state.Edges[0].Kind)
	}
	if len(state.Spines) != 1 {
		t.Fatalf("Spines = %#v, want one satire spine", state.Spines)
	}
	if state.Spines[0].Policy != "satirical_analogy" {
		t.Fatalf("Spine policy = %q, want satirical_analogy", state.Spines[0].Policy)
	}
	if len(state.Spines[0].Edges) != 2 || state.Spines[0].Edges[0].Kind != "illustration" {
		t.Fatalf("Spine edges = %#v, want illustration edge preserved", state.Spines[0].Edges)
	}
}

func TestPreviewFlowStopAfterMainlineDoesNotRender(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"nodes":[{"id":"n1","text":"高利率维持高位","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n2","text":"股票价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n3","text":"债券价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"},{"id":"n4","text":"房产价格被压低","source_quote":"高利率压低所有资产价格（股票、债券、房产、私募）"}],"off_graph":[]}`},
		{Text: `{"replacements":[]}`},
		{Text: `{"aggregates":[]}`},
		{Text: `{"support_edges":[]}`},
		{Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"高利率压低所有资产价格","reason":"The quote states the pressure."}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"高利率压低股票价格","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"primary relation"}]}`},
	}}
	client := &Client{runtime: rt, model: "compile-model", projectRoot: ""}
	result, err := client.PreviewFlow(context.Background(), Bundle{
		UnitID:     "web:stop-mainline",
		Source:     "web",
		ExternalID: "stop-mainline",
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
		Text: `{"relations":[{"from":"n16","to":"n19","source_quote":"redemptions trigger caps","reason":"mechanism"},{"from":"n19","to":"n20","source_quote":"caps trigger suspension","reason":"mechanism"},{"from":"n20","to":"n23","source_quote":"suspension triggers run","reason":"mechanism"},{"from":"n16","to":"n23","source_quote":"redemptions trigger run","reason":"shortcut"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-shortcut",
		Content: "redemptions trigger caps, caps trigger suspension, suspension triggers run",
	}, graphState{
		ArticleForm: "main_narrative_plus_investment_implication",
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

func TestStage3MainlinePreservesLLMSpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"claims expand into stress","reason":"claims drive stress"},{"from":"n2","to":"n3","source_quote":"stress drives devaluation","reason":"stress drives devaluation"}],"spines":[{"id":"s_primary","level":"primary","priority":1,"thesis":"Claims expansion creates devaluation pressure","node_ids":["n1","n2","n3"],"edge_indexes":[0,1],"scope":"article","why":"This is the article-level causal spine."}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-spines",
		Content: "claims expand into stress; stress drives devaluation",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Claims expand", SourceQuote: "claims expand"},
			{ID: "n2", Text: "Promises cannot be met", SourceQuote: "stress"},
			{ID: "n3", Text: "Currency devalues", SourceQuote: "devaluation"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Edges) != 2 {
		t.Fatalf("Edges = %#v, want two drives edges", state.Edges)
	}
	if len(state.Spines) != 1 {
		t.Fatalf("Spines = %#v, want one LLM spine", state.Spines)
	}
	got := state.Spines[0]
	if got.ID != "s_primary" || got.Level != "primary" || got.Priority != 1 || got.Scope != "article" {
		t.Fatalf("spine metadata = %#v", got)
	}
	if strings.Join(got.NodeIDs, ",") != "n1,n2,n3" {
		t.Fatalf("spine NodeIDs = %#v", got.NodeIDs)
	}
	if len(got.Edges) != 2 || got.Edges[0].From != "n1" || got.Edges[1].To != "n3" {
		t.Fatalf("spine Edges = %#v", got.Edges)
	}
}

func TestStage3MainlinePreservesSingleNodeRiskFamilySpine(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n2","to":"n3","source_quote":"private credit stress drives selling","reason":"private credit stress drives selling"}],"spines":[{"id":"s_geopolitical","level":"branch","priority":1,"thesis":"Geopolitical tensions remain a major risk family","node_ids":["n1"],"edge_indexes":[],"scope":"section","why":"The article names this as a major risk family, but retains it as one causal proposition node."},{"id":"s_private_credit","level":"branch","priority":2,"thesis":"Private credit stress drives selling","node_ids":["n2","n3"],"edge_indexes":[0],"scope":"section","why":"This is a grounded two-node branch."}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-single-node-risk-spine",
		Content: "geopolitical tensions are a major risk; private credit stress drives selling",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Geopolitical tensions remain JPMorgan's primary risk"},
			{ID: "n2", Text: "Private credit stress rises"},
			{ID: "n3", Text: "Investors sell private credit exposure"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Spines) != 2 {
		t.Fatalf("Spines = %#v, want single-node risk spine plus private-credit spine", state.Spines)
	}
	got := state.Spines[0]
	if got.ID != "s_geopolitical" || got.Level != "branch" || strings.Join(got.NodeIDs, ",") != "n1" {
		t.Fatalf("single-node spine metadata = %#v", got)
	}
	if len(got.Edges) != 0 {
		t.Fatalf("single-node spine edges = %#v, want none", got.Edges)
	}
}

func TestStage3MainlineDemotesExtraPrimarySpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"a drives b","reason":"a drives b"},{"from":"n3","to":"n4","source_quote":"c drives d","reason":"c drives d"}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Article thesis","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"top thesis"},{"id":"s2","level":"primary","priority":2,"thesis":"Subargument mislabeled primary","node_ids":["n3","n4"],"edge_indexes":[1],"scope":"section","why":"branch mislabeled primary"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-extra-primary",
		Content: "a drives b; c drives d",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "A"},
			{ID: "n2", Text: "B"},
			{ID: "n3", Text: "C"},
			{ID: "n4", Text: "D"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Spines) != 2 {
		t.Fatalf("Spines = %#v, want two spines", state.Spines)
	}
	if state.Spines[0].Level != "primary" {
		t.Fatalf("first spine level = %q, want primary", state.Spines[0].Level)
	}
	if state.Spines[1].Level != "branch" {
		t.Fatalf("extra primary spine level = %q, want demoted branch", state.Spines[1].Level)
	}
}

func TestStage3MainlinePromotesBestSpineWhenPrimaryMissing(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n2","to":"n3","source_quote":"policy shock drives market repricing","reason":"policy shock drives repricing"}],"spines":[{"id":"s_risk","level":"branch","priority":1,"thesis":"Named risk family","node_ids":["n1"],"edge_indexes":[],"scope":"section","why":"single-node risk branch"},{"id":"s_policy","level":"branch","priority":2,"thesis":"Policy shock reprices markets","node_ids":["n2","n3"],"edge_indexes":[0],"scope":"section","why":"best available causal spine"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-missing-primary",
		Content: "risk family; policy shock drives market repricing",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Named risk family"},
			{ID: "n2", Text: "Policy shock"},
			{ID: "n3", Text: "Market repricing"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Spines) != 2 {
		t.Fatalf("Spines = %#v, want two spines", state.Spines)
	}
	if state.Spines[0].Level != "branch" {
		t.Fatalf("single-node risk spine level = %q, want branch", state.Spines[0].Level)
	}
	if state.Spines[1].Level != "primary" || state.Spines[1].Scope != "article" {
		t.Fatalf("best causal spine metadata = %#v, want promoted primary article spine", state.Spines[1])
	}
}

func TestStage3MainlineRiskListDoesNotPromotePrimary(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n2","to":"n3","source_quote":"private credit stress drives selling","reason":"private credit stress drives selling"}],"spines":[{"id":"s_geopolitical","level":"branch","priority":1,"thesis":"Geopolitical tensions remain a major risk family","node_ids":["n1"],"edge_indexes":[],"scope":"section","why":"The article names this as a major risk family."},{"id":"s_private_credit","level":"branch","priority":2,"thesis":"Private credit stress drives selling","node_ids":["n2","n3"],"edge_indexes":[0],"scope":"section","why":"This is a grounded risk branch."}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-risk-list-policy",
		Content: "Geopolitical tensions are a major risk. Private credit stress drives selling.",
	}, graphState{
		ArticleForm: "risk_list",
		Nodes: []graphNode{
			{ID: "n1", Text: "Geopolitical tensions remain a major risk family"},
			{ID: "n2", Text: "Private credit stress intensifies"},
			{ID: "n3", Text: "Private credit selling risk rises"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	for _, spine := range state.Spines {
		if spine.Level == "primary" {
			t.Fatalf("risk_list spine promoted to primary: %#v", state.Spines)
		}
	}
	if len(state.Spines) != 2 {
		t.Fatalf("Spines = %#v, want two branch risk families", state.Spines)
	}
}

func TestStage3MainlineAssignsSpineFamilyMetadata(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"security doubts reduce petrodollar flows","reason":"petrodollar outflow"},{"from":"n3","to":"n4","source_quote":"private credit fears drive redemptions","reason":"private credit liquidity stress"},{"from":"n5","to":"n6","source_quote":"debt promises force money printing","reason":"macro debt cycle"},{"from":"n7","to":"n8","source_quote":"AI power shortage constrains data centers","reason":"AI bottleneck"}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Petrodollar outflows pressure US assets","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"petrodollar branch"},{"id":"s2","level":"branch","priority":2,"thesis":"Private credit redemption pressure creates liquidity risk","node_ids":["n3","n4"],"edge_indexes":[1],"scope":"section","why":"private credit branch"},{"id":"s3","level":"branch","priority":3,"thesis":"Debt promises trigger money printing and currency devaluation","node_ids":["n5","n6"],"edge_indexes":[2],"scope":"section","why":"debt cycle branch"},{"id":"s4","level":"branch","priority":4,"thesis":"AI power shortage becomes a data center bottleneck","node_ids":["n7","n8"],"edge_indexes":[3],"scope":"section","why":"AI bottleneck branch"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-family-metadata",
		Content: "petrodollar private credit debt AI power",
	}, graphState{
		ArticleForm: "main_narrative_plus_investment_implication",
		Nodes: []graphNode{
			{ID: "n1", Text: "US security credibility weakens"},
			{ID: "n2", Text: "Petrodollar flows leave US assets"},
			{ID: "n3", Text: "Private credit loan fears rise"},
			{ID: "n4", Text: "Private credit redemption pressure rises"},
			{ID: "n5", Text: "Debt promises cannot be met"},
			{ID: "n6", Text: "Money printing devalues currency"},
			{ID: "n7", Text: "AI data center power demand surges"},
			{ID: "n8", Text: "Power shortage constrains data center growth"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	got := map[string]PreviewSpine{}
	for _, spine := range state.Spines {
		got[spine.ID] = spine
	}
	checks := map[string]struct {
		key   string
		label string
		scope string
	}{
		"s1": {key: "petrodollar_outflow", label: "石油美元流出", scope: "geopolitics"},
		"s2": {key: "private_credit_liquidity", label: "私募信贷流动性", scope: "credit"},
		"s3": {key: "macro_debt_cycle", label: "宏观债务周期", scope: "macro"},
		"s4": {key: "ai_power_bottleneck", label: "AI电力瓶颈", scope: "tech"},
	}
	for id, want := range checks {
		spine, ok := got[id]
		if !ok {
			t.Fatalf("missing spine %s in %#v", id, state.Spines)
		}
		if spine.FamilyKey != want.key || spine.FamilyLabel != want.label || spine.FamilyScope != want.scope {
			t.Fatalf("%s family = (%q,%q,%q), want (%q,%q,%q)", id, spine.FamilyKey, spine.FamilyLabel, spine.FamilyScope, want.key, want.label, want.scope)
		}
	}
}

func TestInferSpineFamilyDistinguishesRiskListFamilies(t *testing.T) {
	valid := map[string]graphNode{
		"reg": {ID: "reg", Text: "Post-2008 regulations fragmented the financial system and reduced productive lending"},
		"geo": {ID: "geo", Text: "Wars in Ukraine and Iran affect commodities, markets, tariffs, and global trade arrangements"},
		"ai":  {ID: "ai", Text: "AI adoption is unprecedented and will create second- and third-order societal impacts"},
	}
	checks := map[string]struct {
		spine PreviewSpine
		key   string
		label string
		scope string
	}{
		"regulation": {
			spine: PreviewSpine{Thesis: "Post-2008 regulations fragmented the financial system and reduced productive lending", NodeIDs: []string{"reg"}, Scope: "article"},
			key:   "bank_regulation_fragmentation",
			label: "银行监管碎片化",
			scope: "regulation",
		},
		"geopolitics": {
			spine: PreviewSpine{Thesis: "Ongoing wars and U.S. tariff policy disrupt commodities, markets, and global trade arrangements", NodeIDs: []string{"geo"}, Scope: "article"},
			key:   "geopolitical_trade_realignment",
			label: "地缘贸易重组",
			scope: "geopolitics",
		},
		"ai": {
			spine: PreviewSpine{Thesis: "AI adoption is unprecedented and will create second- and third-order societal impacts", NodeIDs: []string{"ai"}, Scope: "article"},
			key:   "ai_societal_shift",
			label: "AI社会影响",
			scope: "tech",
		},
	}
	for name, want := range checks {
		got := inferSpineFamily(want.spine, valid)
		if got.Key != want.key || got.Label != want.label || got.Scope != want.scope {
			t.Fatalf("%s family = (%q,%q,%q), want (%q,%q,%q)", name, got.Key, got.Label, got.Scope, want.key, want.label, want.scope)
		}
	}
}

func TestInferSpineFamilyRequiresSpecificAnchors(t *testing.T) {
	got := inferSpineFamily(PreviewSpine{
		ID:     "s1",
		Thesis: "AI will change how companies compete",
		Scope:  "article",
	}, nil)
	if got.Key != "general_ai_will_change_how_companies_compete" || got.Label != "AI will change how companies compete" || got.Scope != "article" {
		t.Fatalf("generic AI family = (%q,%q,%q), want fallback family", got.Key, got.Label, got.Scope)
	}
}

func TestFallbackSpineFamilyUsesStableKeysForNonASCIIFallbacks(t *testing.T) {
	first := inferSpineFamily(PreviewSpine{ID: "s1", Thesis: "盈利脱钩推动美股创新高", Scope: "article"}, nil)
	second := inferSpineFamily(PreviewSpine{ID: "s2", Thesis: "消费信心走弱引发债市下跌", Scope: "article"}, nil)
	if first.Key == "general_spine" || second.Key == "general_spine" {
		t.Fatalf("non-ASCII fallback key should not collapse to general_spine: first=%q second=%q", first.Key, second.Key)
	}
	if first.Key == second.Key {
		t.Fatalf("distinct non-ASCII fallback theses collided: %q", first.Key)
	}
}

func TestFallbackSpineFamilyAvoidsTruncatedSlugCollisions(t *testing.T) {
	first := inferSpineFamily(PreviewSpine{
		ID:     "s1",
		Thesis: "This unusually long fallback spine starts with identical wording but ends with alpha conclusion",
		Scope:  "article",
	}, nil)
	second := inferSpineFamily(PreviewSpine{
		ID:     "s2",
		Thesis: "This unusually long fallback spine starts with identical wording but ends with beta conclusion",
		Scope:  "article",
	}, nil)
	if first.Key == second.Key {
		t.Fatalf("distinct long fallback theses collided after slug truncation: %q", first.Key)
	}
}

func TestInferSpineFamilySeparatesAICreditContagionFromPowerBottleneck(t *testing.T) {
	got := inferSpineFamily(PreviewSpine{
		ID:     "s1",
		Thesis: "AI SaaS revenue disruption and off-balance-sheet data center financing impair private credit loans",
		Scope:  "article",
		Edges: []PreviewEdge{{
			SourceQuote: "software cash flows weaken, data center leases sit outside balance sheets, private credit financed the projects",
			Reason:      "AI-driven software defaults and data center financing impair private credit asset values",
		}},
	}, nil)
	if got.Key != "ai_credit_contagion" || got.Label != "AI信贷传染" || got.Scope != "credit" {
		t.Fatalf("AI credit family = (%q,%q,%q), want AI credit contagion", got.Key, got.Label, got.Scope)
	}
}

func TestStage3MainlineCompressesMacroFrameworkToSummarySpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"debt promises break","reason":"debt promises break"},{"from":"n3","to":"n4","source_quote":"money creation raises growth","reason":"money creation raises growth"},{"from":"n4","to":"n5","source_quote":"growth later raises inflation","reason":"growth later raises inflation"},{"from":"n6","to":"n7","source_quote":"rates fall and asset prices rise","reason":"rates affect asset pricing"},{"from":"n7","to":"n8","source_quote":"money supply raises asset demand","reason":"money supply raises asset demand"},{"from":"n9","to":"n10","source_quote":"credit creation causes unpayable promises","reason":"credit repeats the debt promise cycle"},{"from":"n10","to":"n11","source_quote":"unpayable promises trigger stock crashes","reason":"promise failures trigger crashes"},{"from":"n12","to":"n13","source_quote":"real returns turn negative so hard assets outperform","reason":"negative real returns drive hard asset preference"},{"from":"n13","to":"n14","source_quote":"hard money and hard assets outperform cash","reason":"hard assets outperform cash"},{"from":"n15","to":"n16","source_quote":"emotional trading causes underperformance","reason":"emotional trading hurts returns"}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Debt promises break and trigger currency devaluation","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"framework thesis"},{"id":"s2","level":"branch","priority":2,"thesis":"Central bank money and credit creation raises growth and later inflation","node_ids":["n3","n4","n5"],"edge_indexes":[1,2],"scope":"section","why":"core mechanism"},{"id":"s3","level":"branch","priority":3,"thesis":"Rates and money supply drive asset pricing and risk premiums","node_ids":["n6","n7","n8"],"edge_indexes":[3,4],"scope":"section","why":"asset pricing family"},{"id":"s4","level":"branch","priority":4,"thesis":"Credit creation repeats the debt-promise crash cycle","node_ids":["n9","n10","n11"],"edge_indexes":[5,6],"scope":"section","why":"duplicate debt cycle"},{"id":"s5","level":"branch","priority":5,"thesis":"Negative real returns make hard money and hard assets outperform cash","node_ids":["n12","n13","n14"],"edge_indexes":[7,8],"scope":"section","why":"portfolio implication"},{"id":"s6","level":"branch","priority":6,"thesis":"Emotional trading causes investors to underperform market returns","node_ids":["n15","n16"],"edge_indexes":[9],"scope":"section","why":"local behavior"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-macro-summary-budget",
		Content: "macro framework",
	}, graphState{
		ArticleForm: "macro_framework",
		Nodes: []graphNode{
			{ID: "n1", Text: "Debt promises cannot be met", DiscourseRole: "thesis"},
			{ID: "n2", Text: "Currency devaluation occurs", DiscourseRole: "mechanism"},
			{ID: "n3", Text: "Central banks create money and credit", DiscourseRole: "mechanism"},
			{ID: "n4", Text: "Growth rises", DiscourseRole: "mechanism"},
			{ID: "n5", Text: "Inflation rises", DiscourseRole: "mechanism"},
			{ID: "n6", Text: "Interest rates fall", DiscourseRole: "mechanism"},
			{ID: "n7", Text: "Asset prices rise", DiscourseRole: "market_move"},
			{ID: "n8", Text: "Risk premiums compress", DiscourseRole: "market_move"},
			{ID: "n9", Text: "Credit creation expands", DiscourseRole: "mechanism"},
			{ID: "n10", Text: "Unpayable promises increase", DiscourseRole: "mechanism"},
			{ID: "n11", Text: "Stock market crashes occur", DiscourseRole: "market_move"},
			{ID: "n12", Text: "Real returns turn negative", DiscourseRole: "mechanism"},
			{ID: "n13", Text: "Hard money outperforms cash", DiscourseRole: "market_move"},
			{ID: "n14", Text: "Hard assets outperform cash", DiscourseRole: "market_move"},
			{ID: "n15", Text: "Investors trade emotionally", DiscourseRole: "market_move"},
			{ID: "n16", Text: "Investor returns underperform market returns", DiscourseRole: "mechanism"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Spines) != 4 {
		t.Fatalf("Spines = %#v, want summary-level budget of four", state.Spines)
	}
	gotIDs := make([]string, 0, len(state.Spines))
	for _, spine := range state.Spines {
		gotIDs = append(gotIDs, spine.ID)
	}
	if strings.Join(gotIDs, ",") != "s1,s2,s3,s5" {
		t.Fatalf("Spines IDs = %v, want primary, core mechanism, asset pricing, hard-asset implication", gotIDs)
	}
}

func TestStage3ClassifyProjectsRolesFromSpines(t *testing.T) {
	state, err := stage3Classify(context.Background(), nil, "", Bundle{}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "政策冲击"},
			{ID: "n2", Text: "流动性收缩"},
			{ID: "n3", Text: "美股价格下跌"},
			{ID: "n4", Text: "孤立市场压力上升"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n2", To: "n4"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Thesis:   "政策冲击压低美股",
			NodeIDs:  []string{"n1", "n2", "n3"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2"},
				{From: "n2", To: "n3"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage3Classify() error = %v", err)
	}
	byID := map[string]graphNode{}
	for _, node := range state.Nodes {
		byID[node.ID] = node
	}
	if byID["n1"].Role != roleDriver {
		t.Fatalf("n1 role = %s, want driver", byID["n1"].Role)
	}
	if byID["n3"].IsTarget != true {
		t.Fatalf("n3 IsTarget = false, want spine terminal target")
	}
	if byID["n4"].IsTarget {
		t.Fatalf("n4 IsTarget = true, want non-spine terminal ignored")
	}
}

func TestInferSpineFamilyDoesNotTreatWarshAsWar(t *testing.T) {
	got := inferSpineFamily(PreviewSpine{
		ID:     "s1",
		Thesis: "Warsh rate cuts interact with inflation expectations",
		Scope:  "section",
	}, nil)
	if got.Key == "war_energy_inflation" {
		t.Fatalf("Warsh was misclassified as war-energy family: %#v", got)
	}
}

func TestStage3MainlineMergesCryptoSellPressureSpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"tight reserves weaken Bitcoin","reason":"tight reserves weaken Bitcoin"},{"from":"n3","to":"n2","source_quote":"ETF outflows worsen Bitcoin selling pressure","reason":"ETF outflows worsen Bitcoin selling pressure"},{"from":"n4","to":"n2","source_quote":"market makers sell into thin liquidity","reason":"market makers add selling pressure"},{"from":"n5","to":"n2","source_quote":"stablecoin supply contraction caused Bitcoin to fall","reason":"stablecoin contraction caused Bitcoin weakness"},{"from":"n6","to":"n7","source_quote":"TGA drawdown restores reserves","reason":"TGA drawdown restores reserves"},{"from":"n7","to":"n8","source_quote":"reserve recovery triggers a new Bitcoin trend","reason":"reserve recovery supports Bitcoin"}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Tight dollar liquidity weakens Bitcoin","node_ids":["n1","n2"],"edge_indexes":[0],"scope":"article","why":"article thesis"},{"id":"s2","level":"branch","priority":2,"thesis":"ETF outflows worsen Bitcoin selling pressure","node_ids":["n3","n2"],"edge_indexes":[1],"scope":"section","why":"sell-pressure branch"},{"id":"s3","level":"branch","priority":3,"thesis":"Market makers sell into thin crypto liquidity","node_ids":["n4","n2"],"edge_indexes":[2],"scope":"section","why":"sell-pressure branch"},{"id":"s4","level":"branch","priority":4,"thesis":"Stablecoin supply contraction caused Bitcoin to fall","node_ids":["n5","n2"],"edge_indexes":[3],"scope":"section","why":"sell-pressure branch"},{"id":"s5","level":"branch","priority":5,"thesis":"Future reserve recovery supports Bitcoin","node_ids":["n6","n7","n8"],"edge_indexes":[4,5],"scope":"section","why":"recovery branch"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-crypto-sell-pressure",
		Content: "tight reserves weaken Bitcoin; ETF outflows, market makers, and stablecoins add selling pressure; TGA drawdown can restore reserves",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Dollar reserves remain tight"},
			{ID: "n2", Text: "Bitcoin remains weak"},
			{ID: "n3", Text: "ETF outflows continue"},
			{ID: "n4", Text: "Market makers sell into thin liquidity"},
			{ID: "n5", Text: "Stablecoin supply contracts"},
			{ID: "n6", Text: "TGA drawdown begins"},
			{ID: "n7", Text: "Bank reserves recover"},
			{ID: "n8", Text: "Bitcoin trend resumes"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if len(state.Spines) != 3 {
		t.Fatalf("Spines = %#v, want primary, merged sell-pressure branch, and recovery branch", state.Spines)
	}
	var sellPressure PreviewSpine
	for _, spine := range state.Spines {
		if strings.Contains(strings.ToLower(spine.Thesis), "sell-pressure") {
			sellPressure = spine
			break
		}
	}
	if sellPressure.ID == "" {
		t.Fatalf("Spines = %#v, want merged crypto sell-pressure branch", state.Spines)
	}
	if sellPressure.Level != "branch" || len(sellPressure.Edges) != 3 {
		t.Fatalf("sell-pressure spine = %#v, want branch with three sell-pressure edges", sellPressure)
	}
	for _, want := range []string{"n3", "n4", "n5", "n2"} {
		if !containsString(sellPressure.NodeIDs, want) {
			t.Fatalf("sell-pressure node_ids = %#v, missing %s", sellPressure.NodeIDs, want)
		}
	}
}

func TestStage3SpinesDoNotKeepPrunedShortcutIndexes(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"relations":[{"from":"n1","to":"n2","source_quote":"a to b","reason":"a to b"},{"from":"n2","to":"n3","source_quote":"b to c","reason":"b to c"},{"from":"n1","to":"n3","source_quote":"a to c shortcut","reason":"shortcut"}],"spines":[{"id":"s1","level":"primary","priority":1,"thesis":"Shortcut should not survive pruning","node_ids":["n1","n2","n3"],"edge_indexes":[2],"scope":"article","why":"tests pruned index handling"}]}`,
	}}}
	state, err := stage3Mainline(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:mainline-pruned-spine-index",
		Content: "a to b, b to c, shortcut",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "A"},
			{ID: "n2", Text: "B"},
			{ID: "n3", Text: "C"},
		},
	})
	if err != nil {
		t.Fatalf("stage3Mainline() error = %v", err)
	}
	if hasEdge(state.Edges, "n1", "n3") {
		t.Fatalf("shortcut edge should be pruned: %#v", state.Edges)
	}
	if len(state.Spines) != 1 {
		t.Fatalf("Spines = %#v, want one spine rebuilt from final edges", state.Spines)
	}
	for _, edge := range state.Spines[0].Edges {
		if edge.From == "n1" && edge.To == "n3" {
			t.Fatalf("spine retained pruned shortcut edge: %#v", state.Spines[0].Edges)
		}
	}
	if len(state.Spines[0].Edges) != 2 {
		t.Fatalf("spine edges = %#v, want final mechanism edges", state.Spines[0].Edges)
	}
}

func TestStage3ClassifyMarksOnlyBranchTerminalMarketOutcomesAsTargets(t *testing.T) {
	state, err := stage3Classify(context.Background(), nil, "", Bundle{}, graphState{
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
