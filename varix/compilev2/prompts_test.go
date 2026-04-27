package compilev2

import "testing"

func TestPromptTemplatesPresentCoreInstructions(t *testing.T) {
	loader := newPromptLoader("")
	for _, tc := range []struct {
		name string
		file string
		want []string
	}{
		{
			name: "stage1",
			file: "extract_system.tmpl",
			want: []string{"single-sided", "off_graph", "Keep node text in the article's original language"},
		},
		{
			name: "stage3",
			file: "classify_system.tmpl",
			want: []string{"market outcome", "price", "flow", "decision"},
		},
		{
			name: "translate",
			file: "translate_system.tmpl",
			want: []string{"financial-Chinese translator", "already-Chinese", "translations"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := loader.render(tc.file, nil)
			if err != nil {
				t.Fatalf("render(%q) error = %v", tc.file, err)
			}
			for _, want := range tc.want {
				if !contains(body, want) {
					t.Fatalf("prompt missing %q", want)
				}
			}
		})
	}
}

func TestPromptTemplatesIncludeBoundaryFewShot(t *testing.T) {
	loader := newPromptLoader("")
	for _, tc := range []struct {
		name string
		file string
		want []string
	}{
		{
			name: "classify",
			file: "classify_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "aggregate",
			file: "aggregate_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "support",
			file: "support_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "mainline",
			file: "mainline_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
		{
			name: "stage4",
			file: "validate_system.tmpl",
			want: []string{"Boundary few-shot examples", "Example 1", "Input:", "Output:"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := loader.render(tc.file, nil)
			if err != nil {
				t.Fatalf("render(%q) error = %v", tc.file, err)
			}
			for _, want := range tc.want {
				if !contains(body, want) {
					t.Fatalf("prompt missing %q", want)
				}
			}
		})
	}
}

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

func TestStage2SupportPromptFindsSingleDirectionAuxiliaryLinks(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("support_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(support_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Your job is only to find single-direction auxiliary links:",
		"`from` = the auxiliary/supporting node.",
		"`to` = the node being served",
		"Allowed kinds:",
		"`evidence`: A proves, documents, or gives factual support for B.",
		"`explanation`: A explains why/how B is true without being the next downstream outcome",
		"`supplementary`: A is a local side, symptom, numeric face, rule, threshold, case detail, or concrete manifestation of B.",
		"Do not output mainline drive edges here.",
		"Do not choose branch heads here.",
		"Do not merge nodes here.",
		"If A naturally reads as \"therefore / then / which leads to B\", do not output it as support.",
		"Return JSON only:",
		"support_edges",
	} {
		if !contains(body, want) {
			t.Fatalf("support prompt missing %q", want)
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
		"Your job now is only to draw the retained relations and spines",
		"Every relation must include:",
		"source_quote: the quote span that grounds the drives relation",
		"relations: article-grounded A drives B edges among retained nodes",
		"spines: grouped causal spines over relations",
		"level: primary, branch, or local",
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
		"Do not turn section order into causal order.",
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

func TestMainlineUpstreamUserPromptsIncludeArticleContext(t *testing.T) {
	loader := newPromptLoader("")
	for _, tc := range []struct {
		name string
		file string
		data map[string]any
	}{
		{
			name: "extract",
			file: "extract_user.tmpl",
			data: map[string]any{"Article": "article context sentinel"},
		},
		{
			name: "refine",
			file: "refine_user.tmpl",
			data: map[string]any{"Article": "article context sentinel", "Nodes": "n1 | node"},
		},
		{
			name: "aggregate",
			file: "aggregate_user.tmpl",
			data: map[string]any{"Article": "article context sentinel", "Nodes": "Group 1 quote: q\n- n1: node"},
		},
		{
			name: "support",
			file: "support_user.tmpl",
			data: map[string]any{"Article": "article context sentinel", "Nodes": "n1 | node | role= | ontology= | quote=q"},
		},
		{
			name: "mainline",
			file: "mainline_user.tmpl",
			data: map[string]any{
				"Article":        "article context sentinel",
				"Nodes":          "n1 | node | role= | ontology= | quote=q",
				"BranchHeads":    "n1 | node",
				"CandidateEdges": "- (none)",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := loader.render(tc.file, tc.data)
			if err != nil {
				t.Fatalf("render(%s) error = %v", tc.file, err)
			}
			if !contains(body, "article context sentinel") {
				t.Fatalf("%s prompt missing article context:\n%s", tc.name, body)
			}
		})
	}
}

func TestRefinePromptSplitsCausalAndParallelNodes(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("refine_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(refine_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Only patch nodes that are structurally inaccurate",
		"Split any node that contains more than one independently meaningful `subject + change/state` unit.",
		"Do not output any edges.",
		"导致, 引发, 触发",
		"和, 及, 以及, 与",
		"Return JSON only:",
	} {
		if !contains(body, want) {
			t.Fatalf("refine prompt missing %q", want)
		}
	}
}

func TestRefineUserPromptIncludesArticleContext(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("refine_user.tmpl", map[string]any{
		"Article": "full article text for refine",
		"Nodes":   "n1 | 高利率压低股票和债券价格 | quote=高利率压低股票和债券价格",
	})
	if err != nil {
		t.Fatalf("render(refine_user.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"Article:",
		"full article text for refine",
		"Candidate nodes:",
		"n1 | 高利率压低股票和债券价格",
	} {
		if !contains(body, want) {
			t.Fatalf("refine user prompt missing %q", want)
		}
	}
}

func TestAggregatePromptCreatesNonCausalSummaryHeads(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("aggregate_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(aggregate_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"parallel sibling outcomes",
		"Do not include the upstream driver/source node.",
		"Must not contain causal wording.",
		"Good: `下游成本上升`",
		"Bad: `油价上涨推高下游成本`",
		"member_ids",
	} {
		if !contains(body, want) {
			t.Fatalf("aggregate prompt missing %q", want)
		}
	}
}

func TestAggregatePatchAddsSummaryNodeAndSupplementaryEdges(t *testing.T) {
	state := graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "油价上涨"},
			{ID: "n2", Text: "运输成本上升"},
			{ID: "n3", Text: "制造成本上升"},
		},
	}

	out := applyAggregatePatches(state, []aggregatePatch{{
		Text:        "下游成本上升",
		MemberIDs:   []string{"n2", "n3"},
		SourceQuote: "油贵了，运输、制造成本上升",
		Reason:      "parallel cost outcomes",
	}})

	if len(out.Nodes) != 4 {
		t.Fatalf("len(Nodes) = %d, want 4", len(out.Nodes))
	}
	if out.Nodes[3].ID != "agg_1" || out.Nodes[3].Text != "下游成本上升" {
		t.Fatalf("aggregate node = %#v, want agg_1 下游成本上升", out.Nodes[3])
	}
	if len(out.AuxEdges) != 2 {
		t.Fatalf("len(AuxEdges) = %d, want 2", len(out.AuxEdges))
	}
	for _, edge := range out.AuxEdges {
		if edge.To != "agg_1" || edge.Kind != "supplementary" {
			t.Fatalf("edge = %#v, want member -> agg_1 supplementary", edge)
		}
		if edge.From == "n1" {
			t.Fatal("upstream driver n1 should not be linked as aggregate member")
		}
	}
}

func TestAggregateCandidateGroupsSuggestAssetPriceLabel(t *testing.T) {
	body := serializeAggregateCandidateGroups([]graphNode{
		{ID: "n1", Text: "利率维持高位", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
		{ID: "n2", Text: "股票价格被压低", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
		{ID: "n3", Text: "债券价格被压低", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
		{ID: "n4", Text: "房产价格被压低", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
		{ID: "n5", Text: "私募资产价格被压低", SourceQuote: "高利率压低所有资产价格（股票、债券、房产、私募）"},
	})

	for _, want := range []string{
		"Suggested aggregate label: 资产价格被压低",
		"- n2: 股票价格被压低",
		"- n5: 私募资产价格被压低",
	} {
		if !contains(body, want) {
			t.Fatalf("aggregate candidate groups missing %q in:\n%s", want, body)
		}
	}
}

func TestStage2SupportUserPromptIncludesArticleContext(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("support_user.tmpl", map[string]any{
		"Nodes":   "n1 | A | role= | ontology= | quote=q1",
		"Article": "full article text",
	})
	if err != nil {
		t.Fatalf("render(support_user.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"n1 | A | role= | ontology= | quote=q1",
		"Full article (for discourse context only; do not invent new nodes):",
		"full article text",
	} {
		if !contains(body, want) {
			t.Fatalf("support user prompt missing %q", want)
		}
	}
	if contains(body, "Extract candidate edges") {
		t.Fatal("support user prompt unexpectedly contains candidate edges section")
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

func TestStage1PromptAllowsNormalizationButRejectsSemanticUpgrade(t *testing.T) {
	loader := newPromptLoader("")
	body, err := loader.render("extract_system.tmpl", nil)
	if err != nil {
		t.Fatalf("render(extract_system.tmpl) error = %v", err)
	}
	for _, want := range []string{
		"may normalize wording into a clearer subject + change form",
		"Do not upgrade the meaning",
		"Do not add direction, intensity, certainty, or causality that is not explicit in the source quote",
		"A valid node should be interpretable as `subject + change`.",
		"The subject must be the stable object that undergoes the change, not the action word itself.",
		"Prefer subjects that are stable objects such as:",
		"If a quote fragment only states the change but not a full subject, recover the subject from the nearest local context in the same branch before writing the node text.",
		"Resolve local referential subjects such as `该基金`, `该市场`, `这笔钱`, `这些请求`, `投资者资金` back to the nearest stable container subject in the local context.",
		"Do not leave a node subject as only `该基金` / `资金` / `请求` / `赎回` when the local context already tells you whose fund / whose funds / whose requests they are.",
		"A percentage, amount, threshold, time expression, or pure action noun is not a sufficient subject by itself.",
		"If a node loses the direction of change, it is invalid and should be rewritten or split",
		"Every node must be directly grounded in a source quote from the article",
		"If you cannot point to the quote that supports the node, do not output the node",
		"For explicit `X causes Y` wording, extract X and Y as separate nodes",
		"U.S. trade policy is causing a realignment of global economic relations",
		"Barings基金赎回请求仅满足44.3%",
		"Barings基金每季度最多允许5%赎回",
		"Barings基金投资者资金被锁定无法取出",
	} {
		if !contains(body, want) {
			t.Fatalf("stage1 prompt missing %q", want)
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

func TestSupportFormationChainIsReservedForMainline(t *testing.T) {
	edge := auxEdge{
		From:        "n1",
		To:          "n2",
		Kind:        "supplementary",
		SourceQuote: "油价、通胀、利率、利息形成财政紧缩螺旋",
	}
	from := graphNode{ID: "n1", Text: "油价上涨", SourceQuote: edge.SourceQuote}
	to := graphNode{ID: "n2", Text: "财政紧缩螺旋形成", SourceQuote: edge.SourceQuote}

	if !isLikelyMainlineAuxEdge(edge, from, to) {
		t.Fatal("formation chain should be reserved for mainline instead of collapsed as auxiliary support")
	}
}

func TestSupportBetweenTwoOutcomeLikeNodesFallsBackToSupplementHeuristic(t *testing.T) {
	from := graphNode{ID: "n1", Text: "Foreign portfolio inflows into US assets remain huge", Role: roleTransmission, Ontology: "flow", IsTarget: true}
	to := graphNode{ID: "n2", Text: "\"Sell America\" trade never existed", Role: roleTransmission, Ontology: "flow", IsTarget: true}
	if !shouldDemoteSupportToSupplement(from, to) {
		t.Fatal("expected support link to be demoted to supplement for two outcome-like nodes")
	}
	primary, secondary := chooseSupplementPrimary(from, to)
	if primary != "n1" || secondary.ID != "n2" {
		t.Fatalf("got primary=%s secondary=%s, want primary=n1 secondary=n2", primary, secondary.ID)
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

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
