package compilev2

import "testing"

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
