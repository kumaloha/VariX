package compilev2

import "testing"

func TestApplyRefineReplacementsSplitsNodeAndRedirectsOffGraph(t *testing.T) {
	state := graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "通胀高企导致利率无法下降", SourceQuote: "通胀压不下去 利率就降不了"},
			{ID: "n2", Text: "长期债券价格承压", SourceQuote: "长期债券继续承压"},
		},
		OffGraph: []offGraphItem{
			{ID: "o1", Text: "通胀臭鼬比喻", Role: "explanation", AttachesTo: "n1"},
		},
	}

	got := applyRefineReplacements(state, []refineReplacement{
		{
			ReplaceID:    "n1",
			RelationType: "causal",
			Nodes: []refineReplacementNode{
				{Text: "通胀高企", SourceQuote: "通胀压不下去"},
				{Text: "利率无法下降", SourceQuote: "利率就降不了"},
			},
			Reason: "mixed causal node",
		},
	})

	if len(got.Nodes) != 3 {
		t.Fatalf("nodes = %#v, want 3 nodes after split", got.Nodes)
	}
	if got.Nodes[0].ID != "n1_1" || got.Nodes[0].Text != "通胀高企" {
		t.Fatalf("first replacement = %#v", got.Nodes[0])
	}
	if got.Nodes[1].ID != "n1_2" || got.Nodes[1].Text != "利率无法下降" {
		t.Fatalf("second replacement = %#v", got.Nodes[1])
	}
	if got.Nodes[2].ID != "n2" {
		t.Fatalf("unchanged node = %#v, want original n2 after replacements", got.Nodes[2])
	}
	if got.OffGraph[0].AttachesTo != "n1_1" {
		t.Fatalf("off_graph attaches_to = %q, want redirected first replacement", got.OffGraph[0].AttachesTo)
	}
}
