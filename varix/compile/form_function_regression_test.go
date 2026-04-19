package compile

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/forge/llm"
)

func TestParseOutputFormFunctionGraphSupportsSemanticRoles(t *testing.T) {
	raw := `{
	  "summary":"美国增长叙事暂时压过政治风险，海外资金继续流入美国资产。",
	  "drivers":["  美国增长叙事暂时压过政治风险定价  "],
	  "targets":["  海外资金继续流入美国资产  "],
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"事实","text":"海外资金继续流入美国资产","occurred_at":"2026-07-14T00:00:00Z"},
	      {"id":"n2","kind":"事实","text":"如果政治风险最终压过增长预期","valid_from":"2026-07-14T00:00:00Z","valid_to":"2026-10-14T00:00:00Z"},
	      {"id":"n3","kind":"机制","text":"增长预期压过政治风险定价并维持美国资产配置偏好","occurred_at":"2026-07-14T00:00:00Z"},
	      {"id":"n4","kind":"结论","text":"没有形成 sell America 交易"},
	      {"id":"n5","kind":"预测","text":"未来3个月海外资金流入美国资产放缓","prediction_start_at":"2026-07-14T00:00:00Z"}
	    ],
	    "edges":[
	      {"from":"n3","to":"n1","kind":"正向"},
	      {"from":"n1","to":"n4","kind":"推出"},
	      {"from":"n2","to":"n5","kind":"预设"}
	    ]
	  },
	  "details":{"caveats":["G04-style flow thesis with conditional downside branch"]},
	  "topics":["macro"],
	  "confidence":"medium"
	}`

	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Graph.Nodes[1].Kind != NodeExplicitCondition {
		t.Fatalf("node n2 kind = %q, want %q", out.Graph.Nodes[1].Kind, NodeExplicitCondition)
	}
	if out.Graph.Nodes[2].Kind != NodeMechanism {
		t.Fatalf("node n3 kind = %q, want %q", out.Graph.Nodes[2].Kind, NodeMechanism)
	}
	if out.Graph.Nodes[4].PredictionDueAt.IsZero() {
		t.Fatalf("node n5 prediction due = zero, want inferred due date")
	}
	wantDue := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	if !out.Graph.Nodes[4].PredictionDueAt.Equal(wantDue) {
		t.Fatalf("node n5 prediction due = %v, want %v", out.Graph.Nodes[4].PredictionDueAt, wantDue)
	}
	if got, want := out.Drivers, []string{"美国增长叙事暂时压过政治风险定价"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("Drivers = %#v, want %#v", got, want)
	}
	if got, want := out.Targets, []string{"海外资金继续流入美国资产"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("Targets = %#v, want %#v", got, want)
	}
}

func TestClientCompileNodeChallengeCarriesG04BridgeMechanismAudit(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"海外资金继续流入美国资产","occurred_at":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"没有形成 sell America 交易"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["macro"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	if _, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:g04",
		Source:     "web",
		ExternalID: "g04",
		Content:    "root body",
	}); err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) < 2 {
		t.Fatalf("provider calls = %d, want unified three-call compile requests", len(provider.requests))
	}
	nodeChallengeSystem := provider.requests[1].System
	for _, want := range []string{
		"unified compile challenger",
		"detect missing bridge steps",
		"support/explanation rather than transmission",
	} {
		if !strings.Contains(nodeChallengeSystem, want) {
			t.Fatalf("node challenge system prompt missing %q in %q", want, nodeChallengeSystem)
		}
	}
	nodeChallengePrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	for _, want := range []string{"海外资金继续流入美国资产", "没有形成 sell America 交易", "Generated draft:"} {
		if !strings.Contains(nodeChallengePrompt, want) {
			t.Fatalf("node challenge prompt missing %q in %q", want, nodeChallengePrompt)
		}
	}
}

func TestGoldDatasetG04PreservesNegatedTradeFlowRegression(t *testing.T) {
	dataset, err := LoadGoldDataset(batch1GoldDatasetPath(t))
	if err != nil {
		t.Fatalf("LoadGoldDataset() error = %v", err)
	}
	var sample GoldSample
	found := false
	for _, candidate := range dataset.Samples {
		if candidate.ID == "G04" {
			sample = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("G04 sample not found")
	}
	for _, want := range []string{"卖出美国", "资金", "美国例外论"} {
		if !strings.Contains(sample.Summary, want) {
			t.Fatalf("G04 summary missing %q in %q", want, sample.Summary)
		}
	}
	for _, want := range []string{
		"美国增长叙事仍然吸引全球资金",
		"政治风险没有压倒市场对美国资产的增长偏好",
		"海外资金继续流入美国资产",
		"没有形成 sell America 交易",
		"没有形成 hedge America 交易",
	} {
		if !containsExactString(sample.Drivers, want) && !containsExactString(sample.Targets, want) {
			t.Fatalf("G04 regression fixture missing %q in drivers=%#v targets=%#v", want, sample.Drivers, sample.Targets)
		}
	}
}

func containsExactString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
