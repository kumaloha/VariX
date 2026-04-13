package compile

import (
	"encoding/json"
	"testing"
)

func TestOutputValidateAcceptsSupportedGraphSchema(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A"},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B"},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgePositive},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestOutputValidateRejectsUnsupportedEdgeType(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A"},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B"},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgeKind("支撑")},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported edge kind rejection")
	}
}

func TestOutputValidateRejectsUnknownNodeReference(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A"},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgePositive},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unknown node rejection")
	}
}

func TestOutputValidateRejectsEmptyDetails(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A"},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B"},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgePositive},
			},
		},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want empty details rejection")
	}
}

func TestGraphAliasesDecode(t *testing.T) {
	var out Output
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[{"id":"n1","kind":"事实","content":"事实A"},{"id":"n2","kind":"结论","content":"结论B"}],
	    "edges":[{"source":"n1","target":"n2","kind":"推出"}]
	  }
	}`
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if out.Graph.Nodes[0].Text != "事实A" {
		t.Fatalf("node text = %q", out.Graph.Nodes[0].Text)
	}
	if out.Graph.Edges[0].From != "n1" || out.Graph.Edges[0].To != "n2" {
		t.Fatalf("edge = %#v", out.Graph.Edges[0])
	}
}
