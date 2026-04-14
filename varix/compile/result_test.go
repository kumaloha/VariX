package compile

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOutputValidateAcceptsSupportedGraphSchema(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
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

func TestOutputValidateAcceptsExplicitAndImplicitConditions(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeKind("显式条件"), Text: "如果政策落地", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n3", Kind: NodeKind("隐含条件"), Text: "流动性继续改善", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n4", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgePositive},
				{From: "n2", To: "n3", Kind: EdgePresets},
				{From: "n3", To: "n4", Kind: EdgeDerives},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.ValidateWithThresholds(4, 3); err != nil {
		t.Fatalf("ValidateWithThresholds() error = %v", err)
	}
}

func TestOutputValidateRejectsUnsupportedEdgeType(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
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
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
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
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
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
	    "nodes":[
	      {"id":"n1","kind":"事实","content":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},
	      {"id":"n2","kind":"结论","content":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}
	    ],
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
	if out.Graph.Nodes[0].ValidFrom.IsZero() || out.Graph.Nodes[0].ValidTo.IsZero() {
		t.Fatalf("validity window missing: %#v", out.Graph.Nodes[0])
	}
}

func TestParseOutputNormalizesExplicitConditionText(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"如果美国安全保障减弱，中东资金将减少购买美债美股。","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"市场会承压","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"预设"}]
	  },
	  "details":{"caveats":["说明"]},
	  "confidence":"medium"
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Graph.Nodes[0].Kind != NodeExplicitCondition {
		t.Fatalf("node kind = %q, want explicit condition", out.Graph.Nodes[0].Kind)
	}
}

func TestOutputValidateRejectsMissingValidityWindow(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A"},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validity window rejection")
	}
}

func TestOutputValidateAcceptsVerificationSection(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeImplicitCondition, Text: "隐含条件B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n3", Kind: NodeExplicitCondition, Text: "显式条件C", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n4", Kind: NodePrediction, Text: "预测D", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgePositive},
				{From: "n2", To: "n4", Kind: EdgeDerives},
				{From: "n3", To: "n4", Kind: EdgePresets},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
		Verification: Verification{
			VerifiedAt: mustTime(t, "2026-04-15T00:00:00Z"),
			Model:      "verifier-model",
			FactChecks: []FactCheck{
				{NodeID: "n1", Status: FactStatusClearlyTrue, Reason: "supported"},
				{NodeID: "n2", Status: FactStatusUnverifiable, Reason: "assumed premise"},
			},
			ExplicitConditionChecks: []ExplicitConditionCheck{{
				NodeID: "n3", Status: ExplicitConditionStatusUnknown, Reason: "insufficient foresight",
			}},
			PredictionChecks: []PredictionCheck{{
				NodeID: "n4", Status: PredictionStatusUnresolved, Reason: "still in window", AsOf: mustTime(t, "2026-04-15T00:00:00Z"),
			}},
		},
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestOutputValidateRejectsUnsupportedExplicitConditionStatus(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeExplicitCondition, Text: "显式条件A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodePrediction, Text: "预测B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePresets}},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
		Verification: Verification{
			ExplicitConditionChecks: []ExplicitConditionCheck{{
				NodeID: "n1", Status: ExplicitConditionStatus("impossible"),
			}},
		},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported explicit condition status rejection")
	}
}

func mustTime(t *testing.T, raw string) time.Time {
	t.Helper()
	got, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", raw, err)
	}
	return got.UTC()
}
