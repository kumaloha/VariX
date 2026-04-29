package compilev2

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
)

func TestBuildMainlineMarkdownSkipsDirectPathSelfLoop(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "direct-path",
		Render: compile.Output{
			Drivers: []string{"Driver A"},
			Targets: []string{"Target B"},
			TransmissionPaths: []compile.TransmissionPath{{
				Driver: "Driver A",
				Target: "Target B",
				Steps:  []string{"Driver A"},
			}},
		},
	})

	if strings.Contains(body, "n1 --> n1") {
		t.Fatalf("direct path rendered a self-loop:\n%s", body)
	}
	if !strings.Contains(body, "Driver A") || !strings.Contains(body, "Target B") {
		t.Fatalf("mainline markdown missing driver/target labels:\n%s", body)
	}
}

func TestBuildMainlineMarkdownPrefersRelationsGraph(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "mainline",
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "Claims expand"},
				{ID: "n2", Text: "Promises cannot be met"},
				{ID: "n3", Text: "Money is printed"},
			},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2"},
				{From: "n2", To: "n3"},
			},
		},
		Render: compile.Output{
			Drivers: []string{"Unrelated display driver"},
			Targets: []string{"Unrelated display target"},
			TransmissionPaths: []compile.TransmissionPath{{
				Driver: "Unrelated display driver",
				Target: "Unrelated display target",
				Steps:  []string{"Unrelated display driver"},
			}},
		},
	})

	for _, want := range []string{"Claims expand", "Promises cannot be met", "Money is printed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mainline markdown missing relation node %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Unrelated display driver") {
		t.Fatalf("mainline markdown used render projection instead of relations graph:\n%s", body)
	}
}

func TestDerivePreviewSpinesKeepsParallelBranchesSeparate(t *testing.T) {
	spines := derivePreviewSpines(PreviewGraph{
		Nodes: []PreviewNode{
			{ID: "g1", Text: "Geopolitical tension"},
			{ID: "g2", Text: "Global order uncertainty", IsTarget: true},
			{ID: "r1", Text: "Post-2008 regulation"},
			{ID: "r2", Text: "Productive lending pressure", IsTarget: true},
			{ID: "p1", Text: "Private credit opacity"},
			{ID: "p2", Text: "Markdown pressure", IsTarget: true},
		},
		Edges: []PreviewEdge{
			{From: "g1", To: "g2"},
			{From: "r1", To: "r2"},
			{From: "p1", To: "p2"},
		},
	})

	if len(spines) != 3 {
		t.Fatalf("len(spines) = %d, want 3 parallel spines: %#v", len(spines), spines)
	}
	for _, spine := range spines {
		if spine.Level != "branch" {
			t.Fatalf("spine level = %q, want branch for parallel two-node spine: %#v", spine.Level, spine)
		}
		if len(spine.NodeIDs) != 2 || len(spine.Edges) != 1 {
			t.Fatalf("spine shape = %#v, want two nodes and one edge", spine)
		}
	}
}

func TestPreviewGraphRoundTripsGraphStateForValidate(t *testing.T) {
	state := graphState{
		ArticleForm: "evidence_backed_forecast",
		Nodes: []graphNode{{
			ID:            "n1",
			Text:          "driver",
			SourceQuote:   "quote",
			Role:          roleDriver,
			DiscourseRole: "thesis",
			Ontology:      "claim",
			IsTarget:      true,
		}},
		Edges:       []graphEdge{{From: "n1", To: "n2", Kind: "causal", SourceQuote: "edge quote", Reason: "because"}},
		AuxEdges:    []auxEdge{{From: "n2", To: "n3", Kind: "supports", SourceQuote: "aux quote"}},
		OffGraph:    []offGraphItem{{ID: "off1", Text: "evidence", Role: "evidence", AttachesTo: "n1", SourceQuote: "off quote"}},
		BranchHeads: []string{"n1"},
		Spines:      []PreviewSpine{{ID: "s1", Level: "primary", Thesis: "spine", NodeIDs: []string{"n1", "n2"}}},
		Rounds:      1,
	}
	preview := toPreviewGraph(state)
	roundTrip := fromPreviewGraph(preview, state.Spines, state.ArticleForm)
	if len(roundTrip.Nodes) != 1 || roundTrip.Nodes[0].Role != roleDriver || !roundTrip.Nodes[0].IsTarget {
		t.Fatalf("round trip nodes = %#v", roundTrip.Nodes)
	}
	if len(roundTrip.Edges) != 1 || roundTrip.Edges[0].Kind != "causal" || roundTrip.Edges[0].Reason != "because" {
		t.Fatalf("round trip edges = %#v", roundTrip.Edges)
	}
	if len(roundTrip.AuxEdges) != 1 || roundTrip.AuxEdges[0].Kind != "supports" {
		t.Fatalf("round trip aux edges = %#v", roundTrip.AuxEdges)
	}
	if len(roundTrip.OffGraph) != 1 || roundTrip.OffGraph[0].Role != "evidence" {
		t.Fatalf("round trip off graph = %#v", roundTrip.OffGraph)
	}
	if len(roundTrip.Spines) != 1 || roundTrip.Spines[0].ID != "s1" {
		t.Fatalf("round trip spines = %#v", roundTrip.Spines)
	}
	if roundTrip.ArticleForm != "evidence_backed_forecast" || roundTrip.Rounds != 1 {
		t.Fatalf("round trip metadata = form %q rounds %d", roundTrip.ArticleForm, roundTrip.Rounds)
	}
}

func TestBuildMainlineMarkdownUsesPreviewSpines(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "spines",
		Spines: []PreviewSpine{
			{
				ID:       "s1",
				Level:    "primary",
				Priority: 1,
				Thesis:   "Claims cycle",
				NodeIDs:  []string{"n1", "n2", "n3"},
				Edges: []PreviewEdge{
					{From: "n1", To: "n2"},
					{From: "n2", To: "n3"},
				},
			},
			{
				ID:       "s2",
				Level:    "branch",
				Priority: 2,
				Thesis:   "Regulation branch",
				NodeIDs:  []string{"n4", "n5"},
				Edges:    []PreviewEdge{{From: "n4", To: "n5"}},
			},
		},
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "Claims expand"},
				{ID: "n2", Text: "Promises cannot be met"},
				{ID: "n3", Text: "Money is printed"},
				{ID: "n4", Text: "Regulation rises"},
				{ID: "n5", Text: "Lending slows"},
			},
		},
	})

	for _, want := range []string{"Spine s1", "primary", "Claims cycle", "Spine s2", "Regulation branch"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mainline markdown missing spine marker %q:\n%s", want, body)
		}
	}
}

func TestBuildMainlineMarkdownCompressesSatiricalAnalogySpines(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "bilibili",
		ExternalID: "satire",
		Spines: []PreviewSpine{{
			ID:       "s1",
			Level:    "primary",
			Priority: 1,
			Policy:   "satirical_analogy",
			Thesis:   "Lucky game satire",
			NodeIDs:  []string{"n1", "n2", "n3", "n4", "n5", "n6", "n7"},
			Edges: []PreviewEdge{
				{From: "n1", To: "n2", Kind: "illustration"},
				{From: "n2", To: "n3", Kind: "illustration"},
				{From: "n3", To: "n4", Kind: "illustration"},
				{From: "n4", To: "n5", Kind: "illustration"},
				{From: "n5", To: "n6", Kind: "illustration"},
				{From: "n6", To: "n7", Kind: "illustration"},
			},
		}},
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "幸运游戏每月抽一人得40万，抽完为止"},
				{ID: "n2", Text: "新富通过委托贷款把1亿本金贷回给村长"},
				{ID: "n3", Text: "游戏结束后1亿本金自动归新富设立的基金"},
				{ID: "n4", Text: "新富指定内部人作为前25%赢家来吸引后75%参与者"},
				{ID: "n5", Text: "哄抢氛围使普通人放弃冷静思考急于参与"},
				{ID: "n6", Text: "前25%指定赢家、新富管理费、银行手续费和村长融资受益，后75%承担机会成本、管理费与最终本金转移"},
				{ID: "n7", Text: "现实中的骗局不主要考验智商而是风险识别"},
			},
		},
	})

	for _, want := range []string{"讽刺寓言：幸运游戏", "映射机制：前25%指定赢家、新富管理费、银行手续费和村长融资受益，后75%承担机会成本", "批判结论：现实中的骗局不主要考验智商而是风险识别"} {
		if !strings.Contains(body, want) {
			t.Fatalf("mainline markdown missing compressed satire label %q:\n%s", want, body)
		}
	}
	for _, noisy := range []string{"新富通过委托贷款把1亿本金贷回给村长", "游戏结束后1亿本金自动归新富设立的基金"} {
		if strings.Contains(body, noisy) {
			t.Fatalf("mainline markdown leaked in-story accounting detail %q:\n%s", noisy, body)
		}
	}
}

func TestBuildMainlineMarkdownRendersSingleNodeSpines(t *testing.T) {
	body := BuildMainlineMarkdown(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "single-node-spine",
		Spines: []PreviewSpine{
			{
				ID:       "s_geopolitical",
				Level:    "branch",
				Priority: 1,
				Thesis:   "Geopolitical tensions remain a major risk family",
				NodeIDs:  []string{"n1"},
			},
		},
		Relations: PreviewGraph{
			Nodes: []PreviewNode{
				{ID: "n1", Text: "Geopolitical tensions remain JPMorgan's primary risk"},
			},
		},
	})

	if !strings.Contains(body, "Spine s_geopolitical") {
		t.Fatalf("mainline markdown missing single-node spine listing:\n%s", body)
	}
	if !strings.Contains(body, "Geopolitical tensions remain JPMorgan's primary risk") {
		t.Fatalf("mainline markdown missing single-node spine node:\n%s", body)
	}
	if strings.Contains(body, "No mainline path") {
		t.Fatalf("mainline markdown rendered empty-path fallback despite single-node spine:\n%s", body)
	}
}

func TestFlowPreviewResultJSONUsesSpinesContract(t *testing.T) {
	payload, err := json.Marshal(FlowPreviewResult{
		Platform:   "web",
		ExternalID: "spines-contract",
		Spines: []PreviewSpine{{
			ID:          "s1",
			Level:       "primary",
			Priority:    1,
			Thesis:      "Article thesis",
			NodeIDs:     []string{"n1"},
			FamilyKey:   "macro_debt_cycle",
			FamilyLabel: "宏观债务周期",
			FamilyScope: "macro",
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"spines"`) {
		t.Fatalf("preview payload missing spines key: %s", body)
	}
	if strings.Contains(body, `"mainline_spines"`) {
		t.Fatalf("preview payload still exposes legacy mainline_spines key: %s", body)
	}
	for _, want := range []string{`"family_key":"macro_debt_cycle"`, `"family_label":"宏观债务周期"`, `"family_scope":"macro"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview payload missing %s: %s", want, body)
		}
	}
}
