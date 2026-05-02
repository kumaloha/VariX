package compile

import (
	"context"
	"github.com/kumaloha/forge/llm"
	"testing"
	"time"
)

func hasTransmissionPath(paths []TransmissionPath, driver, target string) bool {
	for _, path := range paths {
		if path.Driver == driver && path.Target == target {
			return true
		}
	}
	return false
}

func TestStage5RenderProjectsDriverTargetPathsFromSpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"政策冲击"},{"id":"n2","text":"流动性收缩"},{"id":"n3","text":"美股价格下跌"}]}`},
		{Text: `{"summary":"政策冲击通过流动性收缩压低美股。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "web:spine-render",
		Source:     "web",
		ExternalID: "spine-render",
		Content:    "Policy shock tightens liquidity and lowers equities.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "政策冲击"},
			{ID: "n2", Text: "流动性收缩"},
			{ID: "n3", Text: "美股价格下跌"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
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
		t.Fatalf("stage5Render() error = %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("rendered output Validate() error = %v", err)
	}
	if out.Graph.Nodes[0].OccurredAt.IsZero() {
		t.Fatalf("OccurredAt is zero for rendered observation node: %#v", out.Graph.Nodes[0])
	}
	if len(out.Drivers) != 1 || out.Drivers[0] != "政策冲击" {
		t.Fatalf("Drivers = %#v, want projected spine source", out.Drivers)
	}
	if len(out.Targets) != 1 || out.Targets[0] != "美股价格下跌" {
		t.Fatalf("Targets = %#v, want projected spine terminal", out.Targets)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want one spine path", out.TransmissionPaths)
	}
}

func TestStage5RenderFallsBackForLowStructureContent(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"作者感谢同事并宣布离开 IIF"}]}`},
		{Text: `{"summary":"作者感谢同事并宣布离开 IIF。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "twitter:low-structure",
		Source:     "twitter",
		ExternalID: "low-structure",
		Content:    "Today is my last day at the IIF. I'd like to thank my friends and colleagues.",
		PostedAt:   time.Date(2024, 1, 26, 12, 0, 0, 0, time.UTC),
	}, graphState{
		Nodes: []graphNode{{ID: "n1", Text: "作者感谢同事并宣布离开 IIF"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("low-structure fallback Validate() error = %v", err)
	}
	if out.Confidence != "low" {
		t.Fatalf("Confidence = %q, want low", out.Confidence)
	}
	if len(out.Graph.Nodes) != 2 || len(out.Graph.Edges) != 1 {
		t.Fatalf("Graph = %#v, want fallback graph with two nodes and one edge", out.Graph)
	}
	if !containsString(out.Details.Caveats, "low-structure content fallback") {
		t.Fatalf("Caveats = %#v, want low-structure fallback marker", out.Details.Caveats)
	}
}

func TestStage5RenderIncludesAttachedOffGraphPremisesInPathSteps(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"AI基础设施支出规模庞大"},{"id":"n2","text":"小型软件公司跑输现金充裕平台"},{"id":"o1","text":"信用利差走阔"}]}`},
		{Text: `{"summary":"AI支出与信用利差共同影响小型软件公司。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "web:offgraph-premise-render",
		Source:     "web",
		ExternalID: "offgraph-premise-render",
		Content:    "If credit spreads widen while AI capex accelerates, smaller software companies may underperform.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "AI capex accelerates"},
			{ID: "n2", Text: "Smaller software companies underperform", IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}},
		OffGraph: []offGraphItem{{
			ID:          "o1",
			Text:        "Credit spreads widen",
			Role:        "inference",
			AttachesTo:  "n2",
			SourceQuote: "credit spreads widen",
		}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "forecast_inference",
			Thesis:   "AI capex and credit spreads imply software underperformance",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "inference"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want one path", out.TransmissionPaths)
	}
	if !containsString(out.TransmissionPaths[0].Steps, "信用利差走阔") {
		t.Fatalf("TransmissionPath steps = %#v, want attached off-graph premise", out.TransmissionPaths[0].Steps)
	}
	if len(out.Branches) != 1 || len(out.Branches[0].TransmissionPaths) != 1 || !containsString(out.Branches[0].TransmissionPaths[0].Steps, "信用利差走阔") {
		t.Fatalf("Branch paths = %#v, want attached off-graph premise", out.Branches)
	}
}

func TestStage5RenderBuildsBranchLocalRolesFromSpines(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"战争前兆指标"},{"id":"n2","text":"长期世界大战"},{"id":"n3","text":"阵营对立"},{"id":"n4","text":"中俄相对赢家"}]}`},
		{Text: `{"summary":"战争前兆指标指向长期世界大战，阵营对立使中俄成为相对赢家。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "twitter:branch-render",
		Source:  "twitter",
		Content: "Long-form forecast with two branches.",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "战争前兆指标"},
			{ID: "n2", Text: "长期世界大战"},
			{ID: "n3", Text: "阵营对立"},
			{ID: "n4", Text: "中俄相对赢家"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Policy:   "forecast_inference",
				Thesis:   "战争前兆指标显示长期世界大战",
				NodeIDs:  []string{"n1", "n2"},
				Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "inference"}},
				Scope:    "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Policy:   "geopolitical_trade_realignment",
				Thesis:   "阵营对立使中俄相对受益",
				NodeIDs:  []string{"n3", "n4"},
				Edges:    []PreviewEdge{{From: "n3", To: "n4", Kind: "causal"}},
				Scope:    "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Branches) != 2 {
		t.Fatalf("Branches = %#v, want two branch-local views", out.Branches)
	}
	if len(out.Branches[0].Drivers) != 1 || out.Branches[0].Drivers[0] != "战争前兆指标" {
		t.Fatalf("primary branch drivers = %#v, want branch-local driver", out.Branches[0].Drivers)
	}
	if len(out.Branches[0].Targets) != 1 || out.Branches[0].Targets[0] != "长期世界大战" {
		t.Fatalf("primary branch targets = %#v, want branch-local target", out.Branches[0].Targets)
	}
	if len(out.Branches[0].TransmissionPaths) != 1 || out.Branches[0].TransmissionPaths[0].Driver != "战争前兆指标" || out.Branches[0].TransmissionPaths[0].Target != "长期世界大战" {
		t.Fatalf("primary branch paths = %#v, want local transmission path", out.Branches[0].TransmissionPaths)
	}
	if len(out.Branches[1].Drivers) != 1 || out.Branches[1].Drivers[0] != "阵营对立" {
		t.Fatalf("second branch drivers = %#v, want branch-local driver", out.Branches[1].Drivers)
	}
	if len(out.Branches[1].Targets) != 1 || out.Branches[1].Targets[0] != "中俄相对赢家" {
		t.Fatalf("second branch targets = %#v, want branch-local target", out.Branches[1].Targets)
	}
}

func TestStage5RenderSeparatesCommonAnchorFromBranchDrivers(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"长期世界大战"},{"id":"n2","text":"秩序转向强权逻辑"},{"id":"n3","text":"冲突增加"},{"id":"n4","text":"美国过度扩张"},{"id":"n5","text":"世界秩序被重塑"}]}`},
		{Text: `{"summary":"长期世界大战通过不同分支影响秩序与美国脆弱性。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "twitter:anchor-render",
		Source:  "twitter",
		Content: "A macro frame supports several local branches.",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "长期世界大战"},
			{ID: "n2", Text: "秩序转向强权逻辑"},
			{ID: "n3", Text: "冲突增加"},
			{ID: "n4", Text: "美国过度扩张"},
			{ID: "n5", Text: "世界秩序被重塑"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Policy:   "causal_mechanism",
				Thesis:   "长期世界大战推动秩序转向强权并增加冲突",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "causal"}, {From: "n2", To: "n3", Kind: "causal"}},
				Scope:    "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Policy:   "causal_mechanism",
				Thesis:   "长期世界大战暴露美国过度扩张并重塑秩序",
				NodeIDs:  []string{"n1", "n4", "n5"},
				Edges:    []PreviewEdge{{From: "n1", To: "n4", Kind: "causal"}, {From: "n4", To: "n5", Kind: "causal"}},
				Scope:    "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if len(out.Branches) != 2 {
		t.Fatalf("Branches = %#v, want two branches", out.Branches)
	}
	if len(out.Branches[0].Anchors) != 1 || out.Branches[0].Anchors[0] != "长期世界大战" {
		t.Fatalf("first branch anchors = %#v, want shared macro frame", out.Branches[0].Anchors)
	}
	if len(out.Branches[0].BranchDrivers) != 1 || out.Branches[0].BranchDrivers[0] != "秩序转向强权逻辑" {
		t.Fatalf("first branch drivers = %#v, want local mechanism", out.Branches[0].BranchDrivers)
	}
	if len(out.Branches[1].Anchors) != 1 || out.Branches[1].Anchors[0] != "长期世界大战" {
		t.Fatalf("second branch anchors = %#v, want shared macro frame", out.Branches[1].Anchors)
	}
	if len(out.Branches[1].BranchDrivers) != 1 || out.Branches[1].BranchDrivers[0] != "美国过度扩张" {
		t.Fatalf("second branch drivers = %#v, want local mechanism", out.Branches[1].BranchDrivers)
	}
}

func TestStage5RenderProjectsSatiricalAnalogyFromRealMechanism(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"村长与新富设计幸运游戏"},{"id":"n2","text":"牌照化金融游戏把资金转移包装成公平规则"},{"id":"n3","text":"多数后续参与者承担机会成本与管理费"}]}`},
		{Text: `{"summary":"牌照化金融游戏把资金转移包装成公平规则，导致多数后续参与者承担机会成本与管理费。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-render",
		Source:  "bilibili",
		Content: "A village game satirizes institutional finance.",
	}, graphState{
		ArticleForm: "institutional_satire",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "牌照化金融游戏把资金转移包装成公平规则", DiscourseRole: "thesis"},
			{ID: "n3", Text: "多数后续参与者承担机会成本与管理费", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2", Kind: "illustration"},
			{From: "n2", To: "n3", Kind: "causal"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏讽刺牌照化金融规则转嫁成本",
			NodeIDs:  []string{"n1", "n2", "n3"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "illustration"},
				{From: "n2", To: "n3", Kind: "causal"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "村长与新富设计幸运游戏") {
		t.Fatalf("Drivers = %#v, want satire vehicle kept out of display drivers", out.Drivers)
	}
	if !containsString(out.Drivers, "牌照化金融游戏把资金转移包装成公平规则") {
		t.Fatalf("Drivers = %#v, want real institutional mechanism as display driver", out.Drivers)
	}
	if !containsString(out.Targets, "多数后续参与者承担机会成本与管理费") {
		t.Fatalf("Targets = %#v, want real-world transferred cost as display target", out.Targets)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "牌照化金融游戏把资金转移包装成公平规则", "多数后续参与者承担机会成本与管理费") {
		t.Fatalf("TransmissionPaths = %#v, want real mechanism path instead of allegory-source path", out.TransmissionPaths)
	}
	if !containsString(out.EvidenceNodes, "村长与新富设计幸运游戏") {
		t.Fatalf("EvidenceNodes = %#v, want satire vehicle displayed as evidence/illustration", out.EvidenceNodes)
	}
}

func TestStage5RenderKeepsAnalogySourceOutOfSatiricalDriversWithoutIllustrationKind(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"村长与新富设计幸运游戏"},{"id":"n2","text":"游戏本质不公平但可包装成公平"},{"id":"n3","text":"多数后续参与者承担机会成本与管理费"}]}`},
		{Text: `{"summary":"游戏本质不公平但可包装成公平，导致多数后续参与者承担机会成本与管理费。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-render-causal-edge",
		Source:  "bilibili",
		Content: "A village game satirizes institutional finance.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "村长与新富设计幸运游戏", DiscourseRole: "analogy"},
			{ID: "n2", Text: "游戏本质不公平但可包装成公平", DiscourseRole: "satire_target"},
			{ID: "n3", Text: "多数后续参与者承担机会成本与管理费", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2", Kind: "causal"},
			{From: "n2", To: "n3", Kind: "causal"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏讽刺规则包装后的成本转嫁",
			NodeIDs:  []string{"n1", "n2", "n3"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "causal"},
				{From: "n2", To: "n3", Kind: "causal"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "村长与新富设计幸运游戏") {
		t.Fatalf("Drivers = %#v, want analogy node kept out of display drivers", out.Drivers)
	}
	if !containsString(out.Drivers, "游戏本质不公平但可包装成公平") {
		t.Fatalf("Drivers = %#v, want satire target as display driver", out.Drivers)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "游戏本质不公平但可包装成公平", "多数后续参与者承担机会成本与管理费") {
		t.Fatalf("TransmissionPaths = %#v, want satire target path", out.TransmissionPaths)
	}
}

func TestStage5RenderDropsCyclicSpineProjectionPath(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"不公平游戏"},{"id":"n2","text":"包装资产"},{"id":"n3","text":"安全感下降"},{"id":"n4","text":"财务自由变远"}]}`},
		{Text: `{"summary":"不公平游戏通过包装资产和安全感下降拉长财务自由进程。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:cyclic-spines",
		Source:  "bilibili",
		Content: "Satire plus branches.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "不公平游戏", DiscourseRole: "thesis"},
			{ID: "n2", Text: "包装资产", DiscourseRole: "satire_target"},
			{ID: "n3", Text: "安全感下降", DiscourseRole: "mechanism"},
			{ID: "n4", Text: "财务自由变远", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n3", To: "n4"},
			{From: "n4", To: "n1"},
		},
		Spines: []PreviewSpine{
			{ID: "s1", Level: "primary", Priority: 1, Policy: "satirical_analogy", Thesis: "不公平游戏映射包装资产", NodeIDs: []string{"n1", "n2"}, Edges: []PreviewEdge{{From: "n1", To: "n2"}}, Scope: "article"},
			{ID: "s2", Level: "branch", Priority: 2, Policy: "causal_mechanism", Thesis: "包装资产降低安全感", NodeIDs: []string{"n2", "n3"}, Edges: []PreviewEdge{{From: "n2", To: "n3"}}, Scope: "section"},
			{ID: "s3", Level: "branch", Priority: 3, Policy: "investment_implication", Thesis: "安全感下降拉长自由进程", NodeIDs: []string{"n3", "n4"}, Edges: []PreviewEdge{{From: "n3", To: "n4"}}, Scope: "section"},
			{ID: "s4", Level: "branch", Priority: 4, Policy: "investment_implication", Thesis: "错误回环", NodeIDs: []string{"n4", "n1"}, Edges: []PreviewEdge{{From: "n4", To: "n1"}}, Scope: "section"},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if hasTransmissionPath(out.TransmissionPaths, "财务自由变远", "不公平游戏") {
		t.Fatalf("TransmissionPaths = %#v, want cycle-closing path dropped", out.TransmissionPaths)
	}
	if containsString(out.Targets, "不公平游戏") {
		t.Fatalf("Targets = %#v, want driver not reintroduced as target by cycle fallback", out.Targets)
	}
	if !containsString(out.Targets, "财务自由变远") {
		t.Fatalf("Targets = %#v, want final acyclic terminal retained", out.Targets)
	}
}

func TestStage5RenderDropsCycleClosingPathThroughIntermediateStep(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"初始驱动"},{"id":"n2","text":"中间机制"},{"id":"n3","text":"最终结果"}]}`},
		{Text: `{"summary":"初始驱动经由中间机制导致最终结果。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:multi-step-cycle",
		Source:  "web",
		Content: "A drives B, B drives C, with a bad B-to-A branch.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "初始驱动", Role: roleDriver},
			{ID: "n2", Text: "中间机制", Role: roleTransmission},
			{ID: "n3", Text: "最终结果", Role: roleTransmission, IsTarget: true},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Policy:   "causal_mechanism",
				Thesis:   "初始驱动经由中间机制导致最终结果",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2", Kind: "causal"},
					{From: "n2", To: "n3", Kind: "causal"},
				},
				Scope: "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Policy:   "causal_mechanism",
				Thesis:   "错误回指",
				NodeIDs:  []string{"n2", "n1"},
				Edges:    []PreviewEdge{{From: "n2", To: "n1", Kind: "causal"}},
				Scope:    "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "初始驱动", "最终结果") {
		t.Fatalf("TransmissionPaths = %#v, want multi-step primary path retained", out.TransmissionPaths)
	}
	if hasTransmissionPath(out.TransmissionPaths, "中间机制", "初始驱动") {
		t.Fatalf("TransmissionPaths = %#v, want back-edge through intermediate step dropped", out.TransmissionPaths)
	}
	if containsString(out.Targets, "初始驱动") {
		t.Fatalf("Targets = %#v, want driver not reintroduced as target through back-edge", out.Targets)
	}
}

func TestStage5RenderDropsMultiStepPathClosingPriorEndpointCycle(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"初始驱动"},{"id":"n2","text":"中间机制"},{"id":"n3","text":"最终结果"}]}`},
		{Text: `{"summary":"错误回指不应再允许初始驱动形成回环路径。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:prior-endpoint-cycle",
		Source:  "web",
		Content: "C points back to A before a later A-B-C projection.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "初始驱动", Role: roleDriver},
			{ID: "n2", Text: "中间机制", Role: roleTransmission},
			{ID: "n3", Text: "最终结果", Role: roleTransmission, IsTarget: true},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "branch",
				Priority: 1,
				Policy:   "causal_mechanism",
				Thesis:   "错误回指先出现",
				NodeIDs:  []string{"n3", "n1"},
				Edges:    []PreviewEdge{{From: "n3", To: "n1", Kind: "causal"}},
				Scope:    "section",
			},
			{
				ID:       "s2",
				Level:    "primary",
				Priority: 2,
				Policy:   "causal_mechanism",
				Thesis:   "初始驱动经由中间机制导致最终结果",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2", Kind: "causal"},
					{From: "n2", To: "n3", Kind: "causal"},
				},
				Scope: "article",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "最终结果", "初始驱动") {
		t.Fatalf("TransmissionPaths = %#v, want earlier endpoint path retained", out.TransmissionPaths)
	}
	if hasTransmissionPath(out.TransmissionPaths, "初始驱动", "最终结果") {
		t.Fatalf("TransmissionPaths = %#v, want later multi-step cycle-closing path dropped", out.TransmissionPaths)
	}
}

func TestStage5RenderKeepsSalientPathStepDisplayTargets(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"驱动"},{"id":"n2","text":"中间桥"},{"id":"n3","text":"最终结果"}]}`},
		{Text: `{"summary":"驱动通过中间桥导致最终结果。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "web:path-step-target",
		Source:  "web",
		Content: "A drives B, B drives C.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "驱动", Role: roleDriver},
			{ID: "n2", Text: "中间桥", Role: roleTransmission, IsTarget: true},
			{ID: "n3", Text: "最终结果", Role: roleTransmission, IsTarget: true},
		},
		Edges: []graphEdge{{From: "n1", To: "n2"}, {From: "n2", To: "n3"}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !containsString(out.Targets, "中间桥") {
		t.Fatalf("Targets = %#v, want salient path step target retained", out.Targets)
	}
	if !containsString(out.Targets, "最终结果") {
		t.Fatalf("Targets = %#v, want final path target retained", out.Targets)
	}
}

func TestStage5RenderUsesCritiqueMechanismForAllIllustrationSatireSpine(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"幸运游戏规则：2000人每人出资5万"},{"id":"n2","text":"村长贷回1亿本金"},{"id":"n3","text":"游戏结束后本金归管理者基金"},{"id":"n4","text":"通过叙事包装游戏为公平"},{"id":"n5","text":"村长必须控制叙事"}]}`},
		{Text: `{"summary":"通过叙事包装游戏为公平，最终让本金归管理者基金。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:all-illustration-satire",
		Source:  "bilibili",
		Content: "A village lottery is an allegory.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "幸运游戏规则：2000人每人出资5万", DiscourseRole: "mechanism"},
			{ID: "n2", Text: "村长贷回1亿本金", DiscourseRole: "mechanism"},
			{ID: "n3", Text: "游戏结束后本金归管理者基金", DiscourseRole: "mechanism"},
			{ID: "n4", Text: "通过叙事包装游戏为公平", DiscourseRole: "mechanism"},
			{ID: "n5", Text: "村长必须控制叙事", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2", Kind: "illustration"},
			{From: "n2", To: "n3", Kind: "illustration"},
			{From: "n3", To: "n4", Kind: "illustration"},
			{From: "n4", To: "n5", Kind: "illustration"},
		},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2", "n3", "n4", "n5"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "illustration"},
				{From: "n2", To: "n3", Kind: "illustration"},
				{From: "n3", To: "n4", Kind: "illustration"},
				{From: "n4", To: "n5", Kind: "illustration"},
			},
			Scope: "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "幸运游戏规则：2000人每人出资5万") {
		t.Fatalf("Drivers = %#v, want story mechanics kept out of display driver", out.Drivers)
	}
	if !containsString(out.Drivers, "通过叙事包装游戏为公平") {
		t.Fatalf("Drivers = %#v, want critique mechanism as display driver", out.Drivers)
	}
	if !containsString(out.Targets, "游戏结束后本金归管理者基金") {
		t.Fatalf("Targets = %#v, want wealth-transfer consequence as display target", out.Targets)
	}
	if containsString(out.Targets, "村长贷回1亿本金") {
		t.Fatalf("Targets = %#v, want intermediate bridge omitted from display targets", out.Targets)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "通过叙事包装游戏为公平", "游戏结束后本金归管理者基金") {
		t.Fatalf("TransmissionPaths = %#v, want synthetic satire display path", out.TransmissionPaths)
	}
}

func TestStage5RenderUsesOffGraphConsequenceForCompressedSatireSpine(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"该幸运游戏本质上极其不公平"},{"id":"n2","text":"追求高回报率容易掉入陷阱"},{"id":"o1","text":"游戏结束后一个亿本金归管理者基金"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺表面公平方案隐藏财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:compressed-satire",
		Source:  "bilibili",
		Content: "A compressed satire spine.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n2", Text: "追求高回报率容易掉入陷阱", DiscourseRole: "thesis"},
		},
		OffGraph: []offGraphItem{{
			ID:         "o1",
			Text:       "游戏结束后一个亿本金归管理者基金",
			Role:       "explanation",
			AttachesTo: "n1",
		}, {
			ID:         "o2",
			Text:       "无关零售客户承担高额手续费",
			Role:       "explanation",
			AttachesTo: "other_branch",
		}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "illustration"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !containsString(out.Targets, "游戏结束后一个亿本金归管理者基金") {
		t.Fatalf("Targets = %#v, want off-graph wealth-transfer consequence", out.Targets)
	}
	if containsString(out.Targets, "追求高回报率容易掉入陷阱") {
		t.Fatalf("Targets = %#v, want generic trap target replaced by concrete consequence", out.Targets)
	}
}

func TestStage5RenderDoesNotStealUnattachedOffGraphSatireTarget(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"该幸运游戏本质上极其不公平"},{"id":"n2","text":"追求高回报率容易掉入陷阱"},{"id":"o1","text":"无关零售客户承担高额手续费"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺表面公平方案隐藏财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:unattached-offgraph",
		Source:  "bilibili",
		Content: "A compressed satire spine.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n2", Text: "追求高回报率容易掉入陷阱", DiscourseRole: "thesis"},
		},
		OffGraph: []offGraphItem{{ID: "o1", Text: "无关零售客户承担高额手续费", Role: "explanation", AttachesTo: "other_branch"}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "illustration"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Targets, "无关零售客户承担高额手续费") {
		t.Fatalf("Targets = %#v, want unattached off-graph item ignored", out.Targets)
	}
}

func TestStage5RenderIgnoresBlankAttachOffGraphSatireTarget(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"该幸运游戏本质上极其不公平"},{"id":"n2","text":"追求高回报率容易掉入陷阱"},{"id":"o1","text":"空挂载零售客户承担高额手续费"}]}`},
		{Text: `{"summary":"作者借幸运游戏讽刺表面公平方案隐藏财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:blank-attach-offgraph",
		Source:  "bilibili",
		Content: "A compressed satire spine.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n2", Text: "追求高回报率容易掉入陷阱", DiscourseRole: "thesis"},
		},
		OffGraph: []offGraphItem{{ID: "o1", Text: "空挂载零售客户承担高额手续费", Role: "explanation"}},
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "幸运游戏说明表面公平方案隐藏财富转移",
			NodeIDs:  []string{"n1", "n2"},
			Edges:    []PreviewEdge{{From: "n1", To: "n2", Kind: "illustration"}},
			Scope:    "article",
		}},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Targets, "空挂载零售客户承担高额手续费") {
		t.Fatalf("Targets = %#v, want blank-attached off-graph item ignored", out.Targets)
	}
}

func TestStage5RenderProjectsSatiricalBranchAlongsideCausalPrimary(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"利率下行"},{"id":"n2","text":"投资被压制"},{"id":"n3","text":"该幸运游戏本质上极其不公平"},{"id":"n4","text":"游戏结束后本金归管理者基金"}]}`},
		{Text: `{"summary":"利率下行压制投资，同时讽刺不公平游戏中的财富转移。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "bilibili:satire-branch",
		Source:  "bilibili",
		Content: "Mixed causal essay with a satire branch.",
	}, graphState{
		ArticleForm: "satirical_financial_commentary",
		Nodes: []graphNode{
			{ID: "n1", Text: "利率下行", DiscourseRole: "mechanism"},
			{ID: "n2", Text: "投资被压制", DiscourseRole: "implication"},
			{ID: "n3", Text: "该幸运游戏本质上极其不公平", DiscourseRole: "thesis"},
			{ID: "n4", Text: "游戏结束后本金归管理者基金", DiscourseRole: "implication"},
		},
		Spines: []PreviewSpine{
			{ID: "s1", Level: "primary", Priority: 1, Policy: "causal_mechanism", Thesis: "利率压制投资", NodeIDs: []string{"n1", "n2"}, Edges: []PreviewEdge{{From: "n1", To: "n2"}}, Scope: "article"},
			{ID: "s2", Level: "branch", Priority: 2, Policy: "satirical_analogy", Thesis: "幸运游戏说明财富转移", NodeIDs: []string{"n3", "n4"}, Edges: []PreviewEdge{{From: "n3", To: "n4", Kind: "illustration"}}, Scope: "section"},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "利率下行", "投资被压制") {
		t.Fatalf("TransmissionPaths = %#v, want primary causal path retained", out.TransmissionPaths)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "该幸运游戏本质上极其不公平", "游戏结束后本金归管理者基金") {
		t.Fatalf("TransmissionPaths = %#v, want branch satire path projected", out.TransmissionPaths)
	}
}

func TestStage5RenderOmitsBridgeDriverTargetsFromDisplayTargets(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"沃什上任概率上升"},{"id":"n2","text":"大幅降息"},{"id":"n3","text":"金融抑制启动"},{"id":"n4","text":"现金购买力贬值"},{"id":"n5","text":"债券收益承压"}]}`},
		{Text: `{"summary":"沃什上任概率上升通过降息触发金融抑制，并导致现金购买力贬值。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:     "youtube:bridge-target",
		Source:     "youtube",
		ExternalID: "bridge-target",
		Content:    "Warsh appointment triggers financial repression, then cash purchasing power erodes.",
	}, graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "沃什上任概率上升"},
			{ID: "n2", Text: "大幅降息"},
			{ID: "n3", Text: "金融抑制启动"},
			{ID: "n4", Text: "现金购买力贬值"},
			{ID: "n5", Text: "债券收益承压"},
			{ID: "n6", Text: "金融抑制正式开启"},
			{ID: "n7", Text: "金融抑制的核心机制是维持负实际利率"},
			{ID: "n8", Text: "金融抑制通过存款利率上限与资本管制锁定资金购买国债"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n3", To: "n4"},
			{From: "n2", To: "n5"},
			{From: "n1", To: "n6"},
			{From: "n2", To: "n6"},
			{From: "n1", To: "n7"},
			{From: "n7", To: "n8"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Thesis:   "沃什触发金融抑制",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n3"},
				},
				Scope: "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Thesis:   "金融抑制侵蚀现金购买力",
				NodeIDs:  []string{"n3", "n4"},
				Edges: []PreviewEdge{
					{From: "n3", To: "n4"},
				},
				Scope: "section",
			},
			{
				ID:       "s3",
				Level:    "branch",
				Priority: 3,
				Thesis:   "降息冲击债券收益",
				NodeIDs:  []string{"n2", "n5"},
				Edges: []PreviewEdge{
					{From: "n2", To: "n5"},
				},
				Scope: "section",
			},
			{
				ID:       "s4",
				Level:    "branch",
				Priority: 4,
				Thesis:   "政策切换开启金融抑制",
				NodeIDs:  []string{"n1", "n2", "n6"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n6"},
				},
				Scope: "section",
			},
			{
				ID:       "s5",
				Level:    "branch",
				Priority: 5,
				Thesis:   "金融抑制机制说明",
				NodeIDs:  []string{"n1", "n7", "n8"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n7"},
					{From: "n7", To: "n8"},
				},
				Scope: "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Drivers, "金融抑制启动") {
		t.Fatalf("Drivers = %#v, want bridge target-driver omitted from display drivers", out.Drivers)
	}
	if containsString(out.Drivers, "大幅降息") {
		t.Fatalf("Drivers = %#v, want transmission-step driver omitted from display drivers", out.Drivers)
	}
	if !containsString(out.Drivers, "沃什上任概率上升") {
		t.Fatalf("Drivers = %#v, want upstream source driver retained", out.Drivers)
	}
	if containsString(out.Targets, "金融抑制的核心机制是维持负实际利率") || containsString(out.Targets, "金融抑制通过存款利率上限与资本管制锁定资金购买国债") {
		t.Fatalf("Targets = %#v, want mechanism-definition targets omitted from display targets", out.Targets)
	}
	if !containsString(out.Targets, "现金购买力贬值") {
		t.Fatalf("Targets = %#v, want downstream user-facing target retained", out.Targets)
	}
	if !containsString(out.Targets, "债券收益承压") {
		t.Fatalf("Targets = %#v, want branch terminal target retained", out.Targets)
	}
	if containsString(out.Targets, "金融抑制启动") {
		t.Fatalf("Targets = %#v, want bridge financial repression conclusion omitted from display targets", out.Targets)
	}
	if containsString(out.Targets, "金融抑制正式开启") {
		t.Fatalf("Targets = %#v, want process-state financial repression conclusion omitted from display targets", out.Targets)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "沃什上任概率上升", "金融抑制启动") {
		t.Fatalf("TransmissionPaths = %#v, want evidence path into financial repression bridge conclusion", out.TransmissionPaths)
	}
	if !hasTransmissionPath(out.TransmissionPaths, "沃什上任概率上升", "金融抑制正式开启") {
		t.Fatalf("TransmissionPaths = %#v, want evidence path into financial repression launch conclusion", out.TransmissionPaths)
	}
	if len(out.TransmissionPaths) != 5 {
		t.Fatalf("TransmissionPaths = %#v, want both spine paths retained", out.TransmissionPaths)
	}
}

func TestStage5RenderOmitsLightForecastCaveatTargets(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"沃什主张大幅降低利率"},{"id":"n2","text":"降息政策旨在制造负实际利率"},{"id":"n3","text":"实际利率转负是金融抑制启动的关键监测指标"},{"id":"n4","text":"沃什认为AI将成为反通胀力量"},{"id":"n5","text":"AI技术发展预期将压制通胀"},{"id":"n6","text":"投资者需进行多元资产配置"}]}`},
		{Text: `{"summary":"沃什主张大幅降息使实际利率转负成为金融抑制的关键监测指标，并推动多元资产配置需求。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
		UnitID:  "youtube:forecast-ai-caveat",
		Source:  "youtube",
		Content: "Warsh rate cuts matter; AI disinflation is a side tension.",
	}, graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{
			{ID: "n1", Text: "沃什主张大幅降低利率"},
			{ID: "n2", Text: "降息政策旨在制造负实际利率"},
			{ID: "n3", Text: "实际利率转负是金融抑制启动的关键监测指标", DiscourseRole: "implication"},
			{ID: "n4", Text: "沃什认为AI将成为反通胀力量"},
			{ID: "n5", Text: "AI技术发展预期将压制通胀", DiscourseRole: "mechanism"},
			{ID: "n6", Text: "投资者需进行多元资产配置", DiscourseRole: "implication"},
			{ID: "n7", Text: "跨境税务结构可能大幅侵蚀资产实际收益", DiscourseRole: "implication"},
		},
		Edges: []graphEdge{
			{From: "n1", To: "n2"},
			{From: "n2", To: "n3"},
			{From: "n4", To: "n5"},
			{From: "n3", To: "n6"},
			{From: "n3", To: "n7"},
		},
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Thesis:   "沃什降息推动金融抑制判断",
				NodeIDs:  []string{"n1", "n2", "n3", "n6"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n3"},
					{From: "n3", To: "n6"},
				},
				Scope: "article",
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Thesis:   "AI反通胀是沃什叙事旁支",
				NodeIDs:  []string{"n4", "n5"},
				Edges: []PreviewEdge{
					{From: "n4", To: "n5"},
				},
				Scope: "section",
			},
			{
				ID:       "s3",
				Level:    "branch",
				Priority: 3,
				Thesis:   "跨境税务是配置执行旁支",
				NodeIDs:  []string{"n3", "n7"},
				Edges: []PreviewEdge{
					{From: "n3", To: "n7"},
				},
				Scope: "section",
			},
		},
	})
	if err != nil {
		t.Fatalf("stage5Render() error = %v", err)
	}
	if containsString(out.Targets, "AI技术发展预期将压制通胀") {
		t.Fatalf("Targets = %#v, want light AI disinflation caveat omitted", out.Targets)
	}
	if containsString(out.Targets, "跨境税务结构可能大幅侵蚀资产实际收益") {
		t.Fatalf("Targets = %#v, want cross-border tax side note omitted", out.Targets)
	}
	if !containsString(out.Targets, "投资者需进行多元资产配置") {
		t.Fatalf("Targets = %#v, want downstream policy implication retained", out.Targets)
	}
}

func TestStage5RenderRecoversTargetFromOffGraphWhenMainlineHasOnlyDriver(t *testing.T) {
	rt := &fakeRuntime{responses: []llm.Response{
		{Text: `{"translations":[{"id":"n1","text":"石油美元闭环形成"},{"id":"off1","text":"私募信贷赎回门和流动性挤兑风险上升"},{"id":"off2","text":"中东资金减少购买美债美股"}]}`},
		{Text: `{"summary":"石油美元闭环受冲击，私募信贷赎回门和流动性挤兑风险上升。"}`},
	}}
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
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
	out, err := stage5Render(context.Background(), rt, "compile-model", Bundle{
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
