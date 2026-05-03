package compile

import "testing"

func TestStage3ClassifyPromptRejectsProcessAndWrapperTargets(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("classify_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(classify_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"should carry a `target` tag",
		"Independent outcome rule:",
		"Salience rule:",
		"Branch headline test:",
		"If only one sentence from this branch could survive as the branch title, prefer the node that best serves as that title.",
		"If large parts of the article are spent describing, justifying, or returning to one result, that result is more likely to deserve a target tag.",
		"A node may be a target even if it has downstream consequences",
		"A node is not a target if it is only valuable because it helps explain how something else happens",
		"Do not classify upstream drivers, transmission steps, process states, or narrative wrappers as market outcomes",
		"restatement, slogan, label, wrapper, or commentary",
		"not enough that the node sounds important or result-like",
	} {
		if !contains(body, want) {
			t.Fatalf("classify prompt missing %q", want)
		}
	}
}
func TestStage3MainlinePromptRequiresGroundedRelations(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("mainline_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(mainline_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Inputs have already been clustered.",
		"Each retained node may include a `discourse_role` hint",
		"Treat `evidence` and `example` nodes as supporting material",
		"Your job now is only to draw the retained relations and spines",
		"Every relation must include kind:",
		"`inference`: A is research evidence, historical precedent, policy signal, legal feasibility, or quantitative indicator",
		"`illustration`: A is an allegory, analogy, satirical scene, or example used to map onto B.",
		"Every relation must include:",
		"source_quote: the quote span that grounds the drives relation",
		"relations: article-grounded edges among retained nodes",
		"spines: grouped causal spines over relations",
		"level: primary, branch, or local",
		"policy: one of causal_mechanism, forecast_inference, investment_implication, satirical_analogy",
		"management_declaration",
		"capital_allocation_rule",
		"speaker, condition, action, scale, constraint, and non-action",
		"Do not force a management declaration into a driver -> target causal chain",
		"edge_indexes: zero-based indexes into relations",
		"Risk-list articles should usually produce several branch spines",
		"Keep grounded non-market upstream causes",
		"Geopolitics, war, policy, regulation, demographics, technology, social behavior, and institutional trust are valid upstream drivers",
		"Do not discard a non-market node merely because it is not itself a market outcome.",
		"Do not start the spine at C merely because C is the first market-policy node.",
		"Merge same-source sibling branches",
		"same upstream source family",
		"one branch spine with multiple outgoing grounded edges",
		"Use a user-facing spine budget",
		"soft ceiling, not a recall filter",
		"Do not drop major source families",
		"risk-list articles must preserve each major risk family",
		"single-node risk-family spine",
		"Merge same-function local spines",
		"crypto liquidity / sell-pressure branch",
		"AI bottleneck articles",
		"long-form macro/video essays should usually fit in 4-6 spines",
		"Before returning JSON, run a spine budget self-check",
		"Never emit more than one primary spine",
		"AI bottleneck and long-form macro/video outputs exceed 6 spines",
		"main article narrative + investment implication",
		"use at most one primary spine unless the article truly has two independent article-level theses",
		"For evidence-backed forecast articles",
		"kind=inference",
		"For satirical analogy articles",
		"kind=illustration",
		"do not chain every in-story operational detail",
		"preserve the benefit/loss allocation",
		"Do not output a spine like game rule -> bank loan -> fund receives principal",
		"lucky-game allegory -> insiders/organizers benefit while later participants bear costs -> real scams test risk recognition",
		"Mixed articles may contain multiple spine policies at once.",
		"Do not turn section order into causal order.",
		"do not pretend proof material is mechanical causality",
		"2-8 nodes",
		"Primary means the article-level thesis; branch means a major sub-argument",
		"The `source_quote` may be a local span across adjacent clauses or sentences from the Article",
		"article wording can ground an endpoint",
		"A candidate edge is only a recall hint",
		"Do not connect nodes just because they are nearby, dramatic, or sequential.",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline prompt missing %q", want)
		}
	}
}
func TestStage4PromptPreservesAdditionalDownstreamChains(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("validate_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(validate_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"If the paragraph supports additional downstream consequences that are not yet in the graph, add them instead of forcing everything into the first retained outcome.",
		"Do not treat an already-present endpoint as sufficient if the paragraph also contains other distinct market, funding, credit, or allocation consequences.",
	} {
		if !contains(body, want) {
			t.Fatalf("stage4 prompt missing %q", want)
		}
	}
}
func TestStage3UserPromptIncludesNeighborContext(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("classify_user.tmpl", map[string]any{
		"NodeText":     "Private credit funds face a redemption run",
		"SourceQuote":  "private credit funds face a redemption run",
		"Predecessors": "- Oil prices surge",
		"Successors":   "- Investors' money is locked and cannot be redeemed",
	})
	if err != nil {
		t.Fatalf("render(classify_user.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Mainline predecessors:",
		"- Oil prices surge",
		"Mainline successors:",
		"- Investors' money is locked and cannot be redeemed",
	} {
		if !contains(body, want) {
			t.Fatalf("stage3 user prompt missing %q", want)
		}
	}
}
func TestMainlineCandidateEdgesIncludeRatePressureBridge(t *testing.T) {
	body := serializeMainlineCandidateEdges("", []graphNode{
		{ID: "n1", Text: "利率维持高位", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
		{ID: "n2", Text: "高利率对所有资产价格形成下行压力", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
	})

	for _, want := range []string{
		"n1 [利率维持高位] -> n2 [高利率对所有资产价格形成下行压力]",
		"rate-state bridge",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline candidate edges missing %q in:\n%s", want, body)
		}
	}
}
func TestMainlineSchemaIncludesOptionalSpines(t *testing.T) {
	schema := stageJSONSchema("mainline")
	if schema == nil {
		t.Fatal("mainline schema is nil")
	}
	if _, ok := schema.Properties["relations"]; !ok {
		t.Fatalf("mainline schema missing relations: %#v", schema.Properties)
	}
	if _, ok := schema.Properties["spines"]; !ok {
		t.Fatalf("mainline schema missing spines: %#v", schema.Properties)
	}
	if !containsString(schema.Required, "relations") {
		t.Fatalf("mainline schema required = %#v, want relations", schema.Required)
	}
	relations := schema.Properties["relations"].(map[string]any)
	items := relations["items"].(map[string]any)
	props := items["properties"].(map[string]any)
	if _, ok := props["kind"]; !ok {
		t.Fatalf("mainline relation schema missing kind: %#v", props)
	}
	spines := schema.Properties["spines"].(map[string]any)
	spineItems := spines["items"].(map[string]any)
	spineProps := spineItems["properties"].(map[string]any)
	if _, ok := spineProps["policy"]; !ok {
		t.Fatalf("mainline spine schema missing policy: %#v", spineProps)
	}
}
func TestMainlineCandidateEdgesRejectKeywordOnlySharedQuote(t *testing.T) {
	sharedQuote := "市场情绪转冷，投资者涌向现金，流动性资产被率先抛售；Barings仅满足44.3%的赎回请求，行业流动性承压"
	body := serializeMainlineCandidateEdges("", []graphNode{
		{ID: "n1", Text: "市场情绪转冷导致流动性资产被抛售", SourceQuote: sharedQuote},
		{ID: "n2", Text: "私募信贷行业流动性压力上升", SourceQuote: sharedQuote},
	})

	if contains(body, "n1 [市场情绪转冷导致流动性资产被抛售] -> n2 [私募信贷行业流动性压力上升]") {
		t.Fatalf("keyword-only shared quote produced jumpy private-credit candidate:\n%s", body)
	}
}
func TestMainlineCandidateEdgesIncludeFormationChainQuote(t *testing.T) {
	body := serializeMainlineCandidateEdges("", []graphNode{
		{ID: "n1", Text: "油价上涨", SourceQuote: "油价、通胀、利率、利息形成财政紧缩螺旋"},
		{ID: "n2", Text: "财政紧缩螺旋形成", SourceQuote: "油价、通胀、利率、利息形成财政紧缩螺旋"},
	})

	if !contains(body, "n1 [油价上涨] -> n2 [财政紧缩螺旋形成]") {
		t.Fatalf("formation-chain quote did not produce mainline candidate:\n%s", body)
	}
}
func TestMainlineCandidateEdgesIncludeArticleWindowAssetPressureBridge(t *testing.T) {
	article := "那他可能就去买大量的军火去了，一买军火的话那就没这么多钱去买美债美股了。如果要是石油美元离开了美国的美元的那些资产，美股 美债都会受到压力。"
	body := serializeMainlineCandidateEdges(article, []graphNode{
		{ID: "n4", Text: "中东国家购买美债美股的资金减少", SourceQuote: "一买军火的话那就没这么多钱去买美债美股了"},
		{ID: "n5", Text: "美股面临资金流出压力", SourceQuote: "如果要是石油美元离开了美国的美元的那些资产美股 美债都会受到压力"},
		{ID: "n6", Text: "美债面临资金流出压力", SourceQuote: "如果要是石油美元离开了美国的美元的那些资产美股 美债都会受到压力"},
	})

	for _, want := range []string{
		"n4 [中东国家购买美债美股的资金减少] -> n5 [美股面临资金流出压力]",
		"n4 [中东国家购买美债美股的资金减少] -> n6 [美债面临资金流出压力]",
		"article-window bridge from reduced US-asset buying to asset pressure",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline article-window candidates missing %q in:\n%s", want, body)
		}
	}
}
func TestMainlineCandidateEdgesIncludeArticleWindowCrowdedTradeBridge(t *testing.T) {
	article := "哪怕今天AI交易也是个拥挤交易。万一要是有钱往外走但没钱往里进，那随时可能出事了，这可能带来资产价格的变化。"
	body := serializeMainlineCandidateEdges(article, []graphNode{
		{ID: "n7", Text: "AI/M7交易仍处于拥挤状态", SourceQuote: "哪怕今天AI交易也是个拥挤交易"},
		{ID: "n8", Text: "AI拥挤交易在资金净流出时面临资产价格剧烈波动风险", SourceQuote: "万一要是有钱往外走但没钱往里进那随时可能出事了这可能带来资产价格的变化"},
	})

	for _, want := range []string{
		"n7 [AI/M7交易仍处于拥挤状态] -> n8 [AI拥挤交易在资金净流出时面临资产价格剧烈波动风险]",
		"article-window bridge from crowded positioning to outflow volatility risk",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline crowded-trade candidate missing %q in:\n%s", want, body)
		}
	}
}
func TestMainlineCandidateEdgesIncludeFinancialClaimsCycleBridge(t *testing.T) {
	article := "金融财富的发明和增长使货币不再受金银约束。货币和信贷约束减少使企业家能通过借贷和发行股票融资。金融财富增加后，金融财富和义务相对于有形财富上升至承诺无法兑现。"
	body := serializeMainlineCandidateEdges(article, []graphNode{
		{ID: "n20", Text: "财富变为交付货币的承诺（金融财富）", SourceQuote: "金融财富的发明和增长使货币不再受金银约束"},
		{ID: "n21", Text: "货币不再受金银约束", SourceQuote: "金融财富的发明和增长使货币不再受金银约束"},
		{ID: "n22", Text: "企业家能通过借贷和发行股票融资", SourceQuote: "货币和信贷约束减少使企业家能通过借贷和发行股票融资"},
		{ID: "agg4", Text: "金融财富增加", SourceQuote: "金融财富增加"},
		{ID: "n43", Text: "金融财富和义务相对于有形财富上升至承诺无法兑现", SourceQuote: "金融财富和义务相对于有形财富上升至承诺无法兑现"},
	})

	for _, want := range []string{
		"n20 [财富变为交付货币的承诺（金融财富）] -> n21 [货币不再受金银约束]",
		"n21 [货币不再受金银约束] -> n22 [企业家能通过借贷和发行股票融资]",
		"agg4 [金融财富增加] -> n43 [金融财富和义务相对于有形财富上升至承诺无法兑现]",
		"article-window bridge for financial-claims cycle spine",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline financial-claims candidate missing %q in:\n%s", want, body)
		}
	}
}
func TestMainlineCandidateEdgesIncludeArticleWindowRedemptionGateBridge(t *testing.T) {
	article := "此时此刻这个私募信贷开始被挤提了。条款里面说每一个季度开放赎回的时候最多只能赎回5%，到了这个额度就关门。"
	body := serializeMainlineCandidateEdges(article, []graphNode{
		{ID: "n11", Text: "私募信贷正遭遇集中赎回挤兑", SourceQuote: "此时此刻这个私募信贷开始被挤提了"},
		{ID: "n21_1", Text: "私募信贷实施赎回额度限制", SourceQuote: "每一个季度开放赎回的时候最多只能赎回5%也好7.5%也好等到了这个额度了对不起我就关门了"},
	})

	for _, want := range []string{
		"n11 [私募信贷正遭遇集中赎回挤兑] -> n21_1 [私募信贷实施赎回额度限制]",
		"article-window bridge from redemption run to gate/panic mechanics",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline redemption-gate candidate missing %q in:\n%s", want, body)
		}
	}
}
func TestMainlineCandidateEdgesIncludePrivateCreditConvergenceBridges(t *testing.T) {
	article := "大量的私募信贷就把钱借给了你你就建了这个数据中心。后来租金量没这么大了，甚至有一天破产了，私募基金肯定就完蛋了。那些有钱人高净值客户就开始追说我能不能赎回。中东的王爷们肯定不会再往里贴钱，甚至很多人开始把钱拿回来，那私募基金一定就会遭到挤提，只能够说每季度最多赎回5%，到了额度就关门。"
	body := serializeMainlineCandidateEdges(article, []graphNode{
		{ID: "n18", Text: "私募信贷资金大量流入数据中心建设项目", SourceQuote: "大量的私募信贷就把钱借给了你你就建了这个数据中心"},
		{ID: "n20", Text: "私募信贷数据中心项目面临违约风险", SourceQuote: "甚至有一天破产了，私募基金肯定就完蛋了"},
		{ID: "n21", Text: "高净值客户对私募信贷基金发起集中赎回请求", SourceQuote: "那些有钱人高净值客户就开始追说我能不能赎回"},
		{ID: "n23", Text: "中东资金从私募信贷基金撤出", SourceQuote: "很多人开始把钱拿回来"},
		{ID: "n24", Text: "私募信贷基金遭遇集中赎回", SourceQuote: "私募基金一定就会遭到挤提"},
		{ID: "n25", Text: "私募信贷基金在达到赎回上限后暂停当期赎回", SourceQuote: "每季度最多赎回5%，到了额度就关门"},
	})

	for _, want := range []string{
		"n18 [私募信贷资金大量流入数据中心建设项目] -> n20 [私募信贷数据中心项目面临违约风险]",
		"n20 [私募信贷数据中心项目面临违约风险] -> n21 [高净值客户对私募信贷基金发起集中赎回请求]",
		"n23 [中东资金从私募信贷基金撤出] -> n24 [私募信贷基金遭遇集中赎回]",
		"n23 [中东资金从私募信贷基金撤出] -> n25 [私募信贷基金在达到赎回上限后暂停当期赎回]",
		"article-window bridge from private-credit exposure to default risk",
		"article-window bridge from private-credit risk to redemption pressure",
		"article-window bridge from withdrawn private-credit funding to redemption run",
		"article-window bridge from withdrawn private-credit funding to redemption gate",
	} {
		if !contains(body, want) {
			t.Fatalf("mainline private-credit convergence candidate missing %q in:\n%s", want, body)
		}
	}
}
func TestPruneTransitiveRelationsKeepsMechanismPath(t *testing.T) {
	edges := pruneTransitiveRelations([]graphEdge{
		{From: "n16", To: "n19", SourceQuote: "redemption pressure triggers caps"},
		{From: "n19", To: "n20", SourceQuote: "caps trigger suspension"},
		{From: "n20", To: "n22", SourceQuote: "suspension triggers panic"},
		{From: "n22", To: "n23", SourceQuote: "panic triggers run"},
		{From: "n16", To: "n23", SourceQuote: "shortcut to run"},
		{From: "n18", To: "n19", SourceQuote: "withdrawals trigger caps"},
	})

	if hasEdge(edges, "n16", "n23") {
		t.Fatalf("transitive shortcut n16->n23 should be pruned: %#v", edges)
	}
	for _, want := range [][2]string{
		{"n16", "n19"},
		{"n19", "n20"},
		{"n20", "n22"},
		{"n22", "n23"},
		{"n18", "n19"},
	} {
		if !hasEdge(edges, want[0], want[1]) {
			t.Fatalf("edge %s->%s was pruned unexpectedly: %#v", want[0], want[1], edges)
		}
	}
}
