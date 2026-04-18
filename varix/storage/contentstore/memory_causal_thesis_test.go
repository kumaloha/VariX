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

func TestBuildCausalThesis_PreservesConditionAndMechanismInCorePath(t *testing.T) {
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-1",
		NodeIDs:  []string{"weibo:S1:n1", "weibo:S1:n2", "weibo:S1:n3", "weibo:S1:n4", "weibo:S1:n5"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {NodeID: "weibo:S1:n1", NodeKind: string(compile.NodeFact), NodeText: "流动性收紧"},
		"weibo:S1:n2": {NodeID: "weibo:S1:n2", NodeKind: string(compile.NodeExplicitCondition), NodeText: "若融资环境继续恶化"},
		"weibo:S1:n3": {NodeID: "weibo:S1:n3", NodeKind: string(compile.NodeImplicitCondition), NodeText: "高杠杆会放大资产价格脆弱性"},
		"weibo:S1:n4": {NodeID: "weibo:S1:n4", NodeKind: string(compile.NodeConclusion), NodeText: "风险资产承压"},
		"weibo:S1:n5": {NodeID: "weibo:S1:n5", NodeKind: string(compile.NodePrediction), NodeText: "未来数月波动加大"},
	}

	got := buildCausalThesis(thesis, nodesByID)
	want := []string{"weibo:S1:n1", "weibo:S1:n2", "weibo:S1:n3", "weibo:S1:n4", "weibo:S1:n5"}
	if len(got.CorePathNodeIDs) != len(want) {
		t.Fatalf("CorePathNodeIDs = %#v, want %#v", got.CorePathNodeIDs, want)
	}
	for i := range want {
		if got.CorePathNodeIDs[i] != want[i] {
			t.Fatalf("CorePathNodeIDs[%d] = %q, want %q", i, got.CorePathNodeIDs[i], want[i])
		}
	}
}

func TestBuildCausalThesis_InfersConditionToMechanismAndConclusionEdges(t *testing.T) {
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-1",
		NodeIDs:  []string{"weibo:S1:n1", "weibo:S1:n2", "weibo:S1:n3", "weibo:S1:n4"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:S1:n1": {NodeID: "weibo:S1:n1", NodeKind: string(compile.NodeFact), NodeText: "流动性收紧"},
		"weibo:S1:n2": {NodeID: "weibo:S1:n2", NodeKind: string(compile.NodeExplicitCondition), NodeText: "若融资环境继续恶化"},
		"weibo:S1:n3": {NodeID: "weibo:S1:n3", NodeKind: string(compile.NodeImplicitCondition), NodeText: "高杠杆会放大资产价格脆弱性"},
		"weibo:S1:n4": {NodeID: "weibo:S1:n4", NodeKind: string(compile.NodeConclusion), NodeText: "风险资产承压"},
	}

	got := buildCausalThesis(thesis, nodesByID)
	edges := map[string]string{}
	for _, edge := range got.Edges {
		edges[edge.From+"->"+edge.To] = edge.Kind
	}

	if kind := edges["weibo:S1:n2->weibo:S1:n3"]; kind != "supports" {
		t.Fatalf("condition->mechanism edge kind = %q, want supports", kind)
	}
	if kind := edges["weibo:S1:n2->weibo:S1:n4"]; kind != "causes" {
		t.Fatalf("condition->conclusion edge kind = %q, want causes", kind)
	}
	if kind := edges["weibo:S1:n3->weibo:S1:n4"]; kind != "causes" {
		t.Fatalf("mechanism->conclusion edge kind = %q, want causes", kind)
	}
}

func TestBuildCausalThesis_PreservesTransmissionBridgeNodeInCorePath(t *testing.T) {
	thesis := memory.CandidateThesis{
		ThesisID: "thesis-g04",
		NodeIDs:  []string{"weibo:G04:n1", "weibo:G04:n2", "weibo:G04:n3"},
	}
	nodesByID := map[string]memory.AcceptedNode{
		"weibo:G04:n1": {NodeID: "weibo:G04:n1", NodeKind: string(compile.NodeFact), NodeText: "海外资金继续流入美国资产"},
		"weibo:G04:n2": {NodeID: "weibo:G04:n2", NodeKind: string(compile.NodeMechanism), NodeText: "增长与回报预期仍压过政治风险并维持美国资产配置偏好"},
		"weibo:G04:n3": {NodeID: "weibo:G04:n3", NodeKind: string(compile.NodeConclusion), NodeText: "当前并不存在 sell America trade"},
	}

	got := buildCausalThesis(thesis, nodesByID)
	if got.NodeRoles["weibo:G04:n2"] != "mechanism" {
		t.Fatalf("role(n2) = %q, want mechanism", got.NodeRoles["weibo:G04:n2"])
	}
	want := []string{"weibo:G04:n1", "weibo:G04:n2", "weibo:G04:n3"}
	if len(got.CorePathNodeIDs) != len(want) {
		t.Fatalf("CorePathNodeIDs = %#v, want %#v", got.CorePathNodeIDs, want)
	}
	for i := range want {
		if got.CorePathNodeIDs[i] != want[i] {
			t.Fatalf("CorePathNodeIDs[%d] = %q, want %q", i, got.CorePathNodeIDs[i], want[i])
		}
	}
	edges := map[string]string{}
	for _, edge := range got.Edges {
		edges[edge.From+"->"+edge.To] = edge.Kind
	}
	if kind := edges["weibo:G04:n1->weibo:G04:n2"]; kind != "supports" {
		t.Fatalf("fact->mechanism edge kind = %q, want supports", kind)
	}
	if kind := edges["weibo:G04:n2->weibo:G04:n3"]; kind != "causes" {
		t.Fatalf("mechanism->conclusion edge kind = %q, want causes", kind)
	}
}
