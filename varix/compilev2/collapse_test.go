package compilev2

import "testing"

func TestCollapseDoesNotChooseAuxSourceAsHead(t *testing.T) {
	state := graphState{
		Nodes: []graphNode{
			{ID: "n1", Text: "Barings基金每季度最多允许5%赎回"},
			{ID: "n2", Text: "Barings基金赎回请求仅满足44.3%"},
			{ID: "n3", Text: "整个私募信贷行业面临赎回拥堵问题"},
		},
		AuxEdges: []auxEdge{
			{From: "n1", To: "n2", Kind: "supplementary"},
			{From: "n2", To: "n3", Kind: "evidence"},
		},
	}

	collapsed := collapseClusters(state)

	if len(collapsed.BranchHeads) != 1 || collapsed.BranchHeads[0] != "n3" {
		t.Fatalf("BranchHeads = %#v, want only n3", collapsed.BranchHeads)
	}
	if len(collapsed.Nodes) != 1 || collapsed.Nodes[0].ID != "n3" {
		t.Fatalf("Nodes = %#v, want only n3", collapsed.Nodes)
	}
	for _, item := range collapsed.OffGraph {
		if item.AttachesTo != "n3" {
			t.Fatalf("OffGraph item %#v attaches to %q, want n3", item.ID, item.AttachesTo)
		}
	}
}
