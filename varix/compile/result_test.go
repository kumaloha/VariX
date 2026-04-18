package compile

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOutputValidateAcceptsSupportedGraphSchema(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Drivers: []string{"美国增长叙事仍然吸引全球资金"},
		Targets: []string{"海外资金继续流入美国资产"},
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

func TestOutputValidateRejectsEmptyDriverEntry(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Drivers: []string{"  "},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want empty driver rejection")
	}
}

func TestOutputValidateRejectsEmptyTargetEntry(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Targets: []string{"\t"},
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgePositive}},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want empty target rejection")
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

func TestOutputValidateAcceptsMechanismNode(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeMechanism, Text: "增长预期压过政治风险定价并维持美国资产配置偏好", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
				{ID: "n2", Kind: NodeFact, Text: "海外资金继续流入美国资产", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
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

func TestOutputValidateRejectsPresetEdgeStartingFromNonConditionNode(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Text: "事实A", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
				{ID: "n2", Kind: NodePrediction, Text: "预测B", ValidFrom: mustTime(t, "2026-04-14T00:00:00Z"), ValidTo: mustTime(t, "2026-07-14T00:00:00Z")},
			},
			Edges: []GraphEdge{
				{From: "n1", To: "n2", Kind: EdgePresets},
			},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want preset source-kind rejection")
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

func TestGraphAliasesDecodeFormFunctionSchema(t *testing.T) {
	var out Output
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","form":"observation","function":"support","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","form":"observation","function":"transmission","text":"机制B","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n3","form":"judgment","function":"claim","text":"结论C"},
	      {"id":"n4","form":"forecast","function":"claim","text":"预测D","prediction_start_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[
	      {"from":"n2","to":"n1","kind":"正向"},
	      {"from":"n1","to":"n3","kind":"推出"},
	      {"from":"n3","to":"n4","kind":"推出"}
	    ]
	  },
	  "details":{"caveats":["说明"]},
	  "confidence":"medium"
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Graph.Nodes[0].Kind != NodeFact || out.Graph.Nodes[0].Form != NodeFormObservation || out.Graph.Nodes[0].Function != NodeFunctionSupport {
		t.Fatalf("node[0] = %#v, want observation/support fact", out.Graph.Nodes[0])
	}
	if out.Graph.Nodes[1].Kind != NodeMechanism || out.Graph.Nodes[1].Form != NodeFormObservation || out.Graph.Nodes[1].Function != NodeFunctionTransmission {
		t.Fatalf("node[1] = %#v, want observation/transmission mechanism", out.Graph.Nodes[1])
	}
	if out.Graph.Nodes[2].Kind != NodeConclusion || out.Graph.Nodes[2].Form != NodeFormJudgment || out.Graph.Nodes[2].Function != NodeFunctionClaim {
		t.Fatalf("node[2] = %#v, want judgment/claim conclusion", out.Graph.Nodes[2])
	}
	if out.Graph.Nodes[3].Kind != NodePrediction || out.Graph.Nodes[3].Form != NodeFormForecast || out.Graph.Nodes[3].Function != NodeFunctionClaim {
		t.Fatalf("node[3] = %#v, want forecast/claim prediction", out.Graph.Nodes[3])
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
	for _, want := range []string{`"kind":"事实"`, `"form":"observation"`, `"function":"support"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("marshal output missing %s: %s", want, got)
		}
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
	for _, want := range []string{`"kind":"预测"`, `"form":"forecast"`, `"function":"claim"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("marshal output missing %s: %s", want, got)
		}
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

func TestParseOutputPreservesDriversAndTargets(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "drivers":["美国增长叙事仍然吸引全球资金"],
	  "targets":["海外资金继续流入美国资产"],
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},
	      {"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}
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
	if !reflect.DeepEqual(out.Drivers, []string{"美国增长叙事仍然吸引全球资金"}) {
		t.Fatalf("Drivers = %#v", out.Drivers)
	}
	if !reflect.DeepEqual(out.Targets, []string{"海外资金继续流入美国资产"}) {
		t.Fatalf("Targets = %#v", out.Targets)
	}
}

func TestParseOutputTrimsDriversAndTargets(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "drivers":["  美国增长叙事仍然吸引全球资金  "],
	  "targets":["  海外资金继续流入美国资产  "],
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},
	      {"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}
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
	if !reflect.DeepEqual(out.Drivers, []string{"美国增长叙事仍然吸引全球资金"}) {
		t.Fatalf("Drivers = %#v", out.Drivers)
	}
	if !reflect.DeepEqual(out.Targets, []string{"海外资金继续流入美国资产"}) {
		t.Fatalf("Targets = %#v", out.Targets)
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

func TestParseOutputPreservesFormFunctionSchemaForG04(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","form":"observation","function":"support","text":"海外资金继续流入美国资产","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","form":"observation","function":"transmission","text":"增长与回报预期仍压过政治风险并维持美国资产配置偏好","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n3","form":"condition","function":"claim","text":"若增长溢价逆转"},
	      {"id":"n4","form":"judgment","function":"claim","text":"当前并不存在 sell America trade"},
	      {"id":"n5","form":"forecast","function":"claim","text":"资本流入会放缓","prediction_start_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[
	      {"from":"n2","to":"n1","kind":"正向"},
	      {"from":"n1","to":"n4","kind":"推出"},
	      {"from":"n3","to":"n5","kind":"预设"},
	      {"from":"n4","to":"n5","kind":"推出"}
	    ]
	  },
	  "details":{"caveats":["G04 regression"]},
	  "confidence":"medium"
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	gotKinds := []NodeKind{
		out.Graph.Nodes[0].Kind,
		out.Graph.Nodes[1].Kind,
		out.Graph.Nodes[2].Kind,
		out.Graph.Nodes[3].Kind,
		out.Graph.Nodes[4].Kind,
	}
	wantKinds := []NodeKind{NodeFact, NodeMechanism, NodeExplicitCondition, NodeConclusion, NodePrediction}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("node kinds = %#v, want %#v", gotKinds, wantKinds)
	}
	gotForms := []NodeForm{
		out.Graph.Nodes[0].Form,
		out.Graph.Nodes[1].Form,
		out.Graph.Nodes[2].Form,
		out.Graph.Nodes[3].Form,
		out.Graph.Nodes[4].Form,
	}
	wantForms := []NodeForm{NodeFormObservation, NodeFormObservation, NodeFormCondition, NodeFormJudgment, NodeFormForecast}
	if !reflect.DeepEqual(gotForms, wantForms) {
		t.Fatalf("node forms = %#v, want %#v", gotForms, wantForms)
	}
	gotFunctions := []NodeFunction{
		out.Graph.Nodes[0].Function,
		out.Graph.Nodes[1].Function,
		out.Graph.Nodes[2].Function,
		out.Graph.Nodes[3].Function,
		out.Graph.Nodes[4].Function,
	}
	wantFunctions := []NodeFunction{NodeFunctionSupport, NodeFunctionTransmission, NodeFunctionClaim, NodeFunctionClaim, NodeFunctionClaim}
	if !reflect.DeepEqual(gotFunctions, wantFunctions) {
		t.Fatalf("node functions = %#v, want %#v", gotFunctions, wantFunctions)
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

func TestParseOutputInfersPredictionDueAtFromQuarterAndYearHorizons(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "current quarter", text: "本季度油价会维持高位", want: "2026-06-30T23:59:59Z"},
		{name: "next quarter", text: "下季度信用利差会继续走阔", want: "2026-09-30T23:59:59Z"},
		{name: "next year", text: "明年出口会承压", want: "2027-12-31T23:59:59Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"` + tt.text + `","prediction_start_at":"2026-04-14T00:00:00Z"}
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
			want := mustTime(t, tt.want)
			if !out.Graph.Nodes[1].PredictionDueAt.Equal(want) {
				t.Fatalf("PredictionDueAt = %v, want %v", out.Graph.Nodes[1].PredictionDueAt, want)
			}
		})
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

func TestOutputValidateRejectsMismatchedFormFunctionAndKind(t *testing.T) {
	out := Output{
		Summary: "一句话总结",
		Graph: ReasoningGraph{
			Nodes: []GraphNode{
				{ID: "n1", Kind: NodeFact, Form: NodeFormJudgment, Function: NodeFunctionClaim, Text: "事实A", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
				{ID: "n2", Kind: NodeConclusion, Text: "结论B"},
			},
			Edges: []GraphEdge{{From: "n1", To: "n2", Kind: EdgeDerives}},
		},
		Details: HiddenDetails{Caveats: []string{"说明"}},
	}
	if err := out.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want form/function mismatch rejection")
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

func TestParseOutputKeepsLegacyVerificationCompatibleWithVerifyV2Fields(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"结论","text":"结论B"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["说明"]},
	  "verification":{
	    "verified_at":"2026-04-15T00:00:00Z",
	    "model":"legacy-verifier",
	    "fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]
	  }
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if len(out.Verification.FactChecks) != 1 {
		t.Fatalf("len(FactChecks) = %d, want 1", len(out.Verification.FactChecks))
	}
	assertVerifyV2StringField(t, out.Verification, "Version", "")
	assertVerifyV2StringField(t, out.Verification, "RolloutStage", "")
	assertVerifyV2SliceLen(t, out.Verification, "Passes", 0)
	if _, ok := tryResolveVerifyV2Path(out.Verification, []string{"CoverageSummary"}); ok {
		t.Fatal("legacy verification should not require verify-v2 coverage summary")
	}
}

func TestParseOutputPreservesVerifyV2FactsMetadataAlongsideLegacyArrays(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"事实","text":"事实B","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n3","kind":"结论","text":"结论C"}
	    ],
	    "edges":[{"from":"n1","to":"n3","kind":"推出"},{"from":"n2","to":"n3","kind":"正向"}]
	  },
	  "details":{"caveats":["说明"]},
	  "verification":{
	    "version":"verify_v2",
	    "rollout_stage":"facts_only",
	    "verified_at":"2026-04-15T03:00:00Z",
	    "model":"judge-model",
	    "fact_checks":[
	      {"node_id":"n1","status":"clearly_true","reason":"retrieved support"},
	      {"node_id":"n2","status":"unverifiable","reason":"bundle-only evidence"}
	    ],
	    "passes":[{
	      "kind":"fact",
	      "node_ids":["n1","n2"],
	      "coverage":{
	        "expected_node_ids":["n1","n2"],
	        "returned_node_ids":["n1","n2"],
	        "missing_node_ids":[],
	        "duplicate_node_ids":[],
	        "valid":true
	      },
	      "retrieval_summary":{
	        "retrieved_node_ids":["n1"],
	        "no_result_node_ids":["n2"],
	        "excerpt_truncated":true
	      },
	      "claim":{
	        "model":"claim-model",
	        "completed_at":"2026-04-15T01:00:00Z",
	        "parse_ok":true,
	        "output_node_ids":["n1","n2"]
	      },
	      "challenge":{
	        "model":"challenge-model",
	        "completed_at":"2026-04-15T02:00:00Z",
	        "parse_ok":true,
	        "output_node_ids":["n1","n2"]
	      },
	      "adjudication":{
	        "model":"judge-model",
	        "completed_at":"2026-04-15T03:00:00Z",
	        "parse_ok":true,
	        "output_node_ids":["n1","n2"]
	      }
	    }],
	    "coverage_summary":{
	      "total_expected_nodes":2,
	      "total_finalized_nodes":2,
	      "missing_node_ids":[],
	      "duplicate_node_ids":[],
	      "valid":true
	    }
	  }
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if len(out.Verification.FactChecks) != 2 {
		t.Fatalf("len(FactChecks) = %d, want 2", len(out.Verification.FactChecks))
	}
	assertVerifyV2StringField(t, out.Verification, "Version", "verify_v2")
	assertVerifyV2StringField(t, out.Verification, "RolloutStage", "facts_only")
	assertVerifyV2SliceLen(t, out.Verification, "Passes", 1)
	assertVerifyV2StringField(t, out.Verification, []string{"Passes", "0", "Kind"}, "fact")
	assertVerifyV2StringSlice(t, out.Verification, []string{"Passes", "0", "NodeIDs"}, []string{"n1", "n2"})
	assertVerifyV2BoolField(t, out.Verification, []string{"Passes", "0", "Coverage", "Valid"}, true)
	assertVerifyV2StringSlice(t, out.Verification, []string{"Passes", "0", "RetrievalSummary", "RetrievedNodeIDs"}, []string{"n1"})
	assertVerifyV2StringSlice(t, out.Verification, []string{"Passes", "0", "RetrievalSummary", "NoResultNodeIDs"}, []string{"n2"})
	assertVerifyV2BoolField(t, out.Verification, []string{"Passes", "0", "RetrievalSummary", "ExcerptTruncated"}, true)
	assertVerifyV2StringField(t, out.Verification, []string{"Passes", "0", "Claim", "Model"}, "claim-model")
	assertVerifyV2StringField(t, out.Verification, []string{"Passes", "0", "Challenge", "Model"}, "challenge-model")
	assertVerifyV2StringField(t, out.Verification, []string{"Passes", "0", "Adjudication", "Model"}, "judge-model")
	assertVerifyV2TimeField(t, out.Verification, []string{"Passes", "0", "Adjudication", "CompletedAt"}, mustTime(t, "2026-04-15T03:00:00Z"))
	assertVerifyV2BoolField(t, out.Verification, []string{"CoverageSummary", "Valid"}, true)
	assertVerifyV2IntField(t, out.Verification, []string{"CoverageSummary", "TotalExpectedNodes"}, 2)
	assertVerifyV2IntField(t, out.Verification, []string{"CoverageSummary", "TotalFinalizedNodes"}, 2)
}

func TestOutputValidateAcceptsVerifyV2MixedAdjudicationMetadataWithEmptyTopLevelModel(t *testing.T) {
	raw := `{
	  "summary":"一句话",
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"预测","text":"预测B","prediction_start_at":"2026-04-14T00:00:00Z","prediction_due_at":"2026-07-14T00:00:00Z"}
	    ],
	    "edges":[{"from":"n1","to":"n2","kind":"推出"}]
	  },
	  "details":{"caveats":["说明"]},
	  "verification":{
	    "version":"verify_v2",
	    "verified_at":"2026-04-15T04:00:00Z",
	    "model":"",
	    "fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}],
	    "prediction_checks":[{"node_id":"n2","status":"unresolved","reason":"still in window","as_of":"2026-04-15T04:00:00Z"}],
	    "passes":[
	      {
	        "kind":"fact",
	        "node_ids":["n1"],
	        "coverage":{"expected_node_ids":["n1"],"returned_node_ids":["n1"],"missing_node_ids":[],"duplicate_node_ids":[],"valid":true},
	        "claim":{"model":"claim-model","completed_at":"2026-04-15T01:00:00Z","parse_ok":true,"output_node_ids":["n1"]},
	        "challenge":{"model":"challenge-model","completed_at":"2026-04-15T02:00:00Z","parse_ok":true,"output_node_ids":["n1"]},
	        "adjudication":{"model":"judge-a","completed_at":"2026-04-15T03:00:00Z","parse_ok":true,"output_node_ids":["n1"]}
	      },
	      {
	        "kind":"prediction",
	        "node_ids":["n2"],
	        "coverage":{"expected_node_ids":["n2"],"returned_node_ids":["n2"],"missing_node_ids":[],"duplicate_node_ids":[],"valid":true},
	        "claim":{"model":"claim-model","completed_at":"2026-04-15T01:30:00Z","parse_ok":true,"output_node_ids":["n2"]},
	        "challenge":{"model":"challenge-model","completed_at":"2026-04-15T02:30:00Z","parse_ok":true,"output_node_ids":["n2"]},
	        "adjudication":{"model":"judge-b","completed_at":"2026-04-15T04:00:00Z","parse_ok":true,"output_node_ids":["n2"]}
	      }
	    ],
	    "coverage_summary":{"total_expected_nodes":2,"total_finalized_nodes":2,"missing_node_ids":[],"duplicate_node_ids":[],"valid":true}
	  }
	}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Verification.Model != "" {
		t.Fatalf("Verification.Model = %q, want empty when adjudication models differ", out.Verification.Model)
	}
	assertVerifyV2StringField(t, out.Verification, []string{"Passes", "0", "Adjudication", "Model"}, "judge-a")
	assertVerifyV2StringField(t, out.Verification, []string{"Passes", "1", "Adjudication", "Model"}, "judge-b")
	assertVerifyV2TimeField(t, out.Verification, []string{"Passes", "1", "Adjudication", "CompletedAt"}, mustTime(t, "2026-04-15T04:00:00Z"))
}

func assertVerifyV2FieldPresent(t *testing.T, root any, path any) {
	t.Helper()
	if _, ok := tryResolveVerifyV2Path(root, normalizeVerifyV2Path(path)); !ok {
		t.Fatalf("missing verify-v2 field at path %v", normalizeVerifyV2Path(path))
	}
}

func assertVerifyV2OneOfStringSlices(t *testing.T, root any, candidatePaths [][]string, want []string) {
	t.Helper()
	for _, path := range candidatePaths {
		if got, ok := tryResolveVerifyV2Path(root, path); ok && got.IsValid() && got.Kind() == reflect.Slice {
			if got.Len() != len(want) {
				continue
			}
			matched := true
			for i := range want {
				if got.Index(i).Kind() != reflect.String || got.Index(i).String() != want[i] {
					matched = false
					break
				}
			}
			if matched {
				return
			}
		}
	}
	t.Fatalf("none of the candidate verify-v2 paths matched %v: %v", want, candidatePaths)
}

func assertVerifyV2StringField(t *testing.T, root any, path any, want string) {
	t.Helper()
	got := mustResolveVerifyV2Path(t, root, path)
	if got.Kind() != reflect.String {
		t.Fatalf("path %v kind = %s, want string", normalizeVerifyV2Path(path), got.Kind())
	}
	if got.String() != want {
		t.Fatalf("path %v = %q, want %q", normalizeVerifyV2Path(path), got.String(), want)
	}
}

func assertVerifyV2BoolField(t *testing.T, root any, path any, want bool) {
	t.Helper()
	got := mustResolveVerifyV2Path(t, root, path)
	if got.Kind() != reflect.Bool {
		t.Fatalf("path %v kind = %s, want bool", normalizeVerifyV2Path(path), got.Kind())
	}
	if got.Bool() != want {
		t.Fatalf("path %v = %v, want %v", normalizeVerifyV2Path(path), got.Bool(), want)
	}
}

func assertVerifyV2TimeField(t *testing.T, root any, path any, want time.Time) {
	t.Helper()
	got := mustResolveVerifyV2Path(t, root, path)
	if got.Type() != reflect.TypeOf(time.Time{}) {
		t.Fatalf("path %v type = %s, want time.Time", normalizeVerifyV2Path(path), got.Type())
	}
	if !got.Interface().(time.Time).Equal(want) {
		t.Fatalf("path %v = %v, want %v", normalizeVerifyV2Path(path), got.Interface(), want)
	}
}

func assertVerifyV2IntField(t *testing.T, root any, path any, want int) {
	t.Helper()
	got := mustResolveVerifyV2Path(t, root, path)
	if got.Kind() != reflect.Int {
		t.Fatalf("path %v kind = %s, want int", normalizeVerifyV2Path(path), got.Kind())
	}
	if int(got.Int()) != want {
		t.Fatalf("path %v = %d, want %d", normalizeVerifyV2Path(path), got.Int(), want)
	}
}

func assertVerifyV2SliceLen(t *testing.T, root any, field string, want int) {
	t.Helper()
	got := mustResolveVerifyV2Path(t, root, []string{field})
	if got.Kind() != reflect.Slice {
		t.Fatalf("field %s kind = %s, want slice", field, got.Kind())
	}
	if got.Len() != want {
		t.Fatalf("field %s len = %d, want %d", field, got.Len(), want)
	}
}

func assertVerifyV2StringSlice(t *testing.T, root any, path []string, want []string) {
	t.Helper()
	got := mustResolveVerifyV2Path(t, root, path)
	if got.Kind() != reflect.Slice {
		t.Fatalf("path %v kind = %s, want slice", path, got.Kind())
	}
	if got.Len() != len(want) {
		t.Fatalf("path %v len = %d, want %d", path, got.Len(), len(want))
	}
	for i := range want {
		if got.Index(i).Kind() != reflect.String {
			t.Fatalf("path %v[%d] kind = %s, want string", path, i, got.Index(i).Kind())
		}
		if got.Index(i).String() != want[i] {
			t.Fatalf("path %v[%d] = %q, want %q", path, i, got.Index(i).String(), want[i])
		}
	}
}

func tryResolveVerifyV2Path(root any, parts []string) (reflect.Value, bool) {
	value := reflect.ValueOf(root)
	for _, part := range parts {
		for value.IsValid() && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if !value.IsValid() {
			return reflect.Value{}, false
		}
		if index, err := strconv.Atoi(part); err == nil {
			if value.Kind() != reflect.Slice || index < 0 || index >= value.Len() {
				return reflect.Value{}, false
			}
			value = value.Index(index)
			continue
		}
		if value.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		field := value.FieldByName(part)
		if !field.IsValid() {
			return reflect.Value{}, false
		}
		value = field
	}
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	return value, value.IsValid()
}

func mustResolveVerifyV2Path(t *testing.T, root any, path any) reflect.Value {
	t.Helper()
	parts := normalizeVerifyV2Path(path)
	value := reflect.ValueOf(root)
	for _, part := range parts {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				t.Fatalf("path %v hit nil pointer before %q", parts, part)
			}
			value = value.Elem()
		}
		if index, err := strconv.Atoi(part); err == nil {
			if value.Kind() != reflect.Slice {
				t.Fatalf("path %v reached non-slice %s before index %d", parts, value.Kind(), index)
			}
			if index < 0 || index >= value.Len() {
				t.Fatalf("path %v index %d out of range", parts, index)
			}
			value = value.Index(index)
			continue
		}
		if value.Kind() != reflect.Struct {
			t.Fatalf("path %v reached non-struct %s before field %q", parts, value.Kind(), part)
		}
		field := value.FieldByName(part)
		if !field.IsValid() {
			t.Fatalf("missing verify-v2 field %q at path %v", part, parts)
		}
		value = field
	}
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			t.Fatalf("path %v resolved to nil pointer", parts)
		}
		value = value.Elem()
	}
	return value
}

func normalizeVerifyV2Path(path any) []string {
	switch v := path.(type) {
	case string:
		return []string{v}
	case []string:
		return append([]string(nil), v...)
	default:
		panic("unsupported verify-v2 path type")
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
