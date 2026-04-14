package compile

import (
	"encoding/json"
	"strings"
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

func TestGraphNodeMarshalJSONPrefersSemanticTimeFields(t *testing.T) {
	node := GraphNode{
		ID:                "n1",
		Kind:              NodeFact,
		Text:              "事实A",
		ValidFrom:         mustTime(t, "2026-04-14T00:00:00Z"),
		ValidTo:           mustTime(t, "2026-07-14T00:00:00Z"),
		OccurredAt:        mustTime(t, "1974-01-01T00:00:00Z"),
		PredictionStartAt: time.Time{},
		PredictionDueAt:   time.Time{},
	}
	raw, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"occurred_at":"1974-01-01T00:00:00Z"`) {
		t.Fatalf("marshal output missing occurred_at: %s", got)
	}
	for _, unwanted := range []string{`"valid_from"`, `"valid_to"`, `"prediction_start_at"`, `"prediction_due_at"`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("marshal output should omit %s when semantic field is present: %s", unwanted, got)
		}
	}
}

func TestGraphNodeMarshalJSONPrefersPredictionWindowFields(t *testing.T) {
	node := GraphNode{
		ID:                "n2",
		Kind:              NodePrediction,
		Text:              "预测B",
		ValidFrom:         mustTime(t, "2026-04-14T00:00:00Z"),
		ValidTo:           mustTime(t, "2026-07-14T00:00:00Z"),
		PredictionStartAt: mustTime(t, "2026-05-01T00:00:00Z"),
		PredictionDueAt:   mustTime(t, "2026-09-01T00:00:00Z"),
	}
	raw, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"prediction_start_at":"2026-05-01T00:00:00Z"`) || !strings.Contains(got, `"prediction_due_at":"2026-09-01T00:00:00Z"`) {
		t.Fatalf("marshal output missing prediction window: %s", got)
	}
	for _, unwanted := range []string{`"valid_from"`, `"valid_to"`, `"occurred_at"`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("marshal output should omit %s when semantic field is present: %s", unwanted, got)
		}
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

func TestParseOutputPreservesPredictionKindForConditionalFutureClause(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2025-01-01T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"若中东地缘冲突升级叠加流动性收紧，私募信贷极大概率爆发挤兑，并可能引发华尔街系统性金融危机","prediction_start_at":"2025-06-01T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["说明"]},
	  "confidence":"medium"
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Graph.Nodes[1].Kind != NodePrediction {
		t.Fatalf("node kind = %q, want prediction preserved", out.Graph.Nodes[1].Kind)
	}
}

func TestParseOutputInfersPredictionDueAtFromRelativeWindow(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"未来三个月市场会承压","prediction_start_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["说明"]},
	  "confidence":"medium"
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	want := mustTime(t, "2026-07-14T00:00:00Z")
	if !out.Graph.Nodes[1].PredictionDueAt.Equal(want) {
		t.Fatalf("PredictionDueAt = %v, want %v", out.Graph.Nodes[1].PredictionDueAt, want)
	}
}

func TestParseOutputInfersPredictionDueAtFromBoundedCalendarWindow(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "current quarter", text: "本季度市场会承压", want: "2026-06-30T23:59:59Z"},
		{name: "next quarter", text: "下季度市场会承压", want: "2026-09-30T23:59:59Z"},
		{name: "next year", text: "明年市场会承压", want: "2027-12-31T23:59:59Z"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"` + tc.text + `","prediction_start_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["说明"]},
	  "confidence":"medium"
	}`
			out, err := ParseOutput(raw)
			if err != nil {
				t.Fatalf("ParseOutput() error = %v", err)
			}
			want := mustTime(t, tc.want)
			if !out.Graph.Nodes[1].PredictionDueAt.Equal(want) {
				t.Fatalf("PredictionDueAt = %v, want %v", out.Graph.Nodes[1].PredictionDueAt, want)
			}
		})
	}
}

func TestParseOutputDoesNotInventPredictionDueAtForVagueYears(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"未来几年市场会承压","prediction_start_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["说明"]},
	  "confidence":"medium"
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if !out.Graph.Nodes[1].PredictionDueAt.IsZero() {
		t.Fatalf("PredictionDueAt = %v, want zero for vague horizon", out.Graph.Nodes[1].PredictionDueAt)
	}
}

func TestOutputValidateRejectsMissingFactTime(t *testing.T) {
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
		t.Fatal("Validate() error = nil, want fact timing rejection")
	}
}

func TestOutputValidateAcceptsOccurredAtAndPredictionWindow(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", OccurredAt: mustTime(t, "1974-01-01T00:00:00Z")},
				{ID: "n2", Kind: NodeExplicitCondition, Text: "如果政策变化"},
				{ID: "n3", Kind: NodeConclusion, Text: "结论C"},
				{ID: "n4", Kind: NodePrediction, Text: "预测D", PredictionStartAt: mustTime(t, "2026-04-14T00:00:00Z"), PredictionDueAt: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n3", Kind: EdgePositive},
				{From: "n2", To: "n4", Kind: EdgePresets},
				{From: "n3", To: "n4", Kind: EdgeDerives},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestOutputValidateAcceptsOpenEndedPrediction(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", OccurredAt: mustTime(t, "1974-01-01T00:00:00Z")},
				{ID: "n2", Kind: NodePrediction, Text: "预测B", PredictionStartAt: mustTime(t, "2026-04-14T00:00:00Z")},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgeDerives}},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	start, end := out.Graph.Nodes[1].LegacyValidityWindow()
	if start.IsZero() || end.IsZero() || !end.After(start) {
		t.Fatalf("LegacyValidityWindow() = %v -> %v, want derived open-ended window", start, end)
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
