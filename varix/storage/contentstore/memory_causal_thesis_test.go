package contentstore

import (
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func TestBuildCausalThesis_AssignsRoles(t *testing.T) {
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-1",
		NodeIDs:  []string{"weibo:S1:n1", "weibo:S1:n2", "weibo:S1:n3"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {NodeID: "weibo:S1:n1", NodeKind: string(compile.NodeFact), NodeText: "流动性收紧"},
		"weibo:S1:n2": {NodeID: "weibo:S1:n2", NodeKind: string(compile.NodeConclusion), NodeText: "风险资产承压"},
		"weibo:S1:n3": {NodeID: "weibo:S1:n3", NodeKind: string(compile.NodePrediction), NodeText: "未来数月波动加大"},
	}

	got := buildCausalThesis(thesis, nodesByID)
	if got.NodeRoles["weibo:S1:n1"] != "fact" {
		t.Fatalf("role(n1) = %q, want fact", got.NodeRoles["weibo:S1:n1"])
	}
	if got.NodeRoles["weibo:S1:n2"] != "conclusion" {
		t.Fatalf("role(n2) = %q, want conclusion", got.NodeRoles["weibo:S1:n2"])
	}
	if got.NodeRoles["weibo:S1:n3"] != "prediction" {
		t.Fatalf("role(n3) = %q, want prediction", got.NodeRoles["weibo:S1:n3"])
	}
}

func TestBuildCausalThesis_ExtractsCorePath(t *testing.T) {
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-1",
		NodeIDs:  []string{"weibo:S1:n1", "weibo:S1:n2", "weibo:S1:n3"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {NodeID: "weibo:S1:n1", NodeKind: string(compile.NodeFact), NodeText: "流动性收紧"},
		"weibo:S1:n2": {NodeID: "weibo:S1:n2", NodeKind: string(compile.NodeConclusion), NodeText: "风险资产承压"},
		"weibo:S1:n3": {NodeID: "weibo:S1:n3", NodeKind: string(compile.NodePrediction), NodeText: "未来数月波动加大"},
	}

	got := buildCausalThesis(thesis, nodesByID)
	want := []string{"weibo:S1:n1", "weibo:S1:n2", "weibo:S1:n3"}
	if len(got.CorePathNodeIDs) != len(want) {
		t.Fatalf("CorePathNodeIDs = %#v, want %#v", got.CorePathNodeIDs, want)
	}
	for i := range want {
		if got.CorePathNodeIDs[i] != want[i] {
			t.Fatalf("CorePathNodeIDs[%d] = %q, want %q", i, got.CorePathNodeIDs[i], want[i])
		}
	}
	if len(got.Edges) < 2 {
		t.Fatalf("Edges = %#v, want inferred core-path edges", got.Edges)
	}
}
