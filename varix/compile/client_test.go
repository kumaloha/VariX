package compile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/forge/llm"
)

type compileMockProvider struct {
	requests  []llm.ProviderRequest
	responses []llm.ProviderResponse
}

type stubVerificationService struct {
	calls        int
	gotBundle    Bundle
	gotOutput    Output
	verification Verification
	err          error
}

func (s *stubVerificationService) Verify(_ context.Context, bundle Bundle, output Output) (Verification, error) {
	s.calls++
	s.gotBundle = bundle
	s.gotOutput = output
	if s.err != nil {
		return Verification{}, s.err
	}
	return s.verification, nil
}

func (p *compileMockProvider) Name() string { return "compile-mock" }

func (p *compileMockProvider) Call(_ context.Context, req llm.ProviderRequest) (llm.ProviderResponse, error) {
	p.requests = append(p.requests, req)
	if len(p.responses) == 0 {
		return llm.ProviderResponse{}, nil
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func newTestRuntime(provider llm.Provider, model string) *llm.Runtime {
	return llm.NewRuntime(llm.RuntimeConfig{
		Provider: provider,
		LLMConfig: llm.LLMConfig{
			Default: llm.DefaultConfig{
				Model:       model,
				Search:      false,
				Temperature: 0,
				Thinking:    false,
			},
		},
		MaxAttempts: 1,
	})
}

func compileStageResponses(t *testing.T, fullOutputJSON string, model string) []llm.ProviderResponse {
	t.Helper()
	out, err := ParseOutput(fullOutputJSON)
	if err != nil {
		t.Fatalf("ParseOutput(fullOutputJSON) error = %v", err)
	}
	driverTarget := deriveDriverTargetOutputForTest(out)
	driverTargetGeneratorRaw, err := json.Marshal(DriverTargetOutput{
		Drivers:    takeFirstN(driverTarget.Drivers, 1),
		Targets:    takeFirstN(driverTarget.Targets, 1),
		Details:    HiddenDetails{Caveats: []string{"generator"}},
		Topics:     out.Topics,
		Confidence: out.Confidence,
	})
	if err != nil {
		t.Fatalf("marshal driver-target generator stage: %v", err)
	}
	driverTargetChallengeRaw, err := json.Marshal(DriverTargetOutput{
		Drivers:    tailFrom(driverTarget.Drivers, 1),
		Targets:    tailFrom(driverTarget.Targets, 1),
		Details:    HiddenDetails{Caveats: []string{"challenge"}},
		Topics:     out.Topics,
		Confidence: out.Confidence,
	})
	if err != nil {
		t.Fatalf("marshal driver-target challenge stage: %v", err)
	}
	driverTargetJudgeRaw, err := json.Marshal(driverTarget)
	if err != nil {
		t.Fatalf("marshal driver-target judge stage: %v", err)
	}

	transmissionPaths := deriveTransmissionPathOutputForTest(out, driverTarget)
	transmissionPathGeneratorRaw, err := json.Marshal(TransmissionPathOutput{
		TransmissionPaths: takeFirstPathN(transmissionPaths.TransmissionPaths, 1),
		Details:           HiddenDetails{Caveats: []string{"generator"}},
		Topics:            out.Topics,
		Confidence:        out.Confidence,
	})
	if err != nil {
		t.Fatalf("marshal transmission-path generator stage: %v", err)
	}
	transmissionPathChallengeRaw, err := json.Marshal(TransmissionPathOutput{
		TransmissionPaths: tailPathFrom(transmissionPaths.TransmissionPaths, 1),
		Details:           HiddenDetails{Caveats: []string{"challenge"}},
		Topics:            out.Topics,
		Confidence:        out.Confidence,
	})
	if err != nil {
		t.Fatalf("marshal transmission-path challenge stage: %v", err)
	}
	transmissionPathJudgeRaw, err := json.Marshal(transmissionPaths)
	if err != nil {
		t.Fatalf("marshal transmission-path judge stage: %v", err)
	}

	aux := deriveEvidenceExplanationOutputForTest(out, driverTarget)
	evidenceExplanationGeneratorRaw, err := json.Marshal(EvidenceExplanationOutput{
		EvidenceNodes:    takeFirstN(aux.EvidenceNodes, 1),
		ExplanationNodes: takeFirstN(aux.ExplanationNodes, 1),
		Details:          HiddenDetails{Caveats: []string{"generator"}},
		Topics:           out.Topics,
		Confidence:       out.Confidence,
	})
	if err != nil {
		t.Fatalf("marshal evidence/explanation generator stage: %v", err)
	}
	evidenceExplanationChallengeRaw, err := json.Marshal(EvidenceExplanationOutput{
		EvidenceNodes:    tailFrom(aux.EvidenceNodes, 1),
		ExplanationNodes: tailFrom(aux.ExplanationNodes, 1),
		Details:          HiddenDetails{Caveats: []string{"challenge"}},
		Topics:           out.Topics,
		Confidence:       out.Confidence,
	})
	if err != nil {
		t.Fatalf("marshal evidence/explanation challenge stage: %v", err)
	}
	evidenceExplanationJudgeRaw, err := json.Marshal(aux)
	if err != nil {
		t.Fatalf("marshal evidence/explanation judge stage: %v", err)
	}

	return []llm.ProviderResponse{
		{Text: string(driverTargetGeneratorRaw), Model: model},
		{Text: string(driverTargetChallengeRaw), Model: model},
		{Text: string(driverTargetJudgeRaw), Model: model},
		{Text: string(transmissionPathGeneratorRaw), Model: model},
		{Text: string(transmissionPathChallengeRaw), Model: model},
		{Text: string(transmissionPathJudgeRaw), Model: model},
		{Text: string(evidenceExplanationGeneratorRaw), Model: model},
		{Text: string(evidenceExplanationChallengeRaw), Model: model},
		{Text: string(evidenceExplanationJudgeRaw), Model: model},
	}
}

func deriveDriverTargetOutputForTest(out Output) DriverTargetOutput {
	drivers := append([]string(nil), out.Drivers...)
	targets := append([]string(nil), out.Targets...)
	if len(drivers) == 0 || len(targets) == 0 {
		for _, node := range out.Graph.Nodes {
			switch node.Kind {
			case NodeMechanism:
				drivers = appendIfMissing(drivers, node.Text)
			case NodeConclusion, NodePrediction:
				targets = appendIfMissing(targets, node.Text)
			}
		}
	}
	if len(drivers) == 0 {
		for _, node := range out.Graph.Nodes {
			if node.Kind == NodeFact {
				drivers = appendIfMissing(drivers, node.Text)
				break
			}
		}
	}
	if len(targets) == 0 {
		for i := len(out.Graph.Nodes) - 1; i >= 0; i-- {
			targets = appendIfMissing(targets, out.Graph.Nodes[i].Text)
			if len(targets) > 0 {
				break
			}
		}
	}
	return DriverTargetOutput{
		Drivers:    drivers,
		Targets:    targets,
		Details:    out.Details,
		Topics:     out.Topics,
		Confidence: out.Confidence,
	}
}

func deriveTransmissionPathOutputForTest(out Output, driverTarget DriverTargetOutput) TransmissionPathOutput {
	paths := append([]TransmissionPath(nil), out.TransmissionPaths...)
	if len(paths) == 0 {
		byID := make(map[string]GraphNode, len(out.Graph.Nodes))
		for _, node := range out.Graph.Nodes {
			byID[node.ID] = node
		}
		for _, edge := range out.Graph.Edges {
			if edge.Kind != EdgePositive {
				continue
			}
			fromNode, fromOK := byID[edge.From]
			toNode, toOK := byID[edge.To]
			if !fromOK || !toOK {
				continue
			}
			driver := firstOrDefault(driverTarget.Drivers, fromNode.Text)
			target := toNode.Text
			paths = append(paths, TransmissionPath{
				Driver: driver,
				Target: target,
				Steps:  []string{fromNode.Text},
			})
		}
	}
	if len(paths) == 0 && len(driverTarget.Drivers) > 0 && len(driverTarget.Targets) > 0 {
		paths = append(paths, TransmissionPath{
			Driver: driverTarget.Drivers[0],
			Target: driverTarget.Targets[0],
			Steps:  []string{driverTarget.Drivers[0]},
		})
	}
	return TransmissionPathOutput{
		TransmissionPaths: paths,
		Details:           out.Details,
		Topics:            out.Topics,
		Confidence:        out.Confidence,
	}
}

func deriveEvidenceExplanationOutputForTest(out Output, driverTarget DriverTargetOutput) EvidenceExplanationOutput {
	evidence := append([]string(nil), out.EvidenceNodes...)
	explanations := append([]string(nil), out.ExplanationNodes...)
	targetSet := make(map[string]struct{}, len(driverTarget.Targets))
	for _, target := range driverTarget.Targets {
		targetSet[strings.TrimSpace(target)] = struct{}{}
	}
	for _, node := range out.Graph.Nodes {
		switch node.Kind {
		case NodeFact:
			evidence = appendIfMissing(evidence, node.Text)
		case NodeConclusion:
			if _, ok := targetSet[strings.TrimSpace(node.Text)]; !ok {
				explanations = appendIfMissing(explanations, node.Text)
			}
		}
	}
	if len(evidence) == 0 && len(out.Graph.Nodes) > 0 {
		evidence = appendIfMissing(evidence, out.Graph.Nodes[0].Text)
	}
	return EvidenceExplanationOutput{
		EvidenceNodes:    evidence,
		ExplanationNodes: explanations,
		Details:          out.Details,
		Topics:           out.Topics,
		Confidence:       out.Confidence,
	}
}

func appendIfMissing(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func takeFirstN(values []string, n int) []string {
	if n <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) < n {
		n = len(values)
	}
	return append([]string(nil), values[:n]...)
}

func tailFrom(values []string, start int) []string {
	if start < 0 || start >= len(values) {
		return nil
	}
	return append([]string(nil), values[start:]...)
}

func takeFirstPathN(values []TransmissionPath, n int) []TransmissionPath {
	if n <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) < n {
		n = len(values)
	}
	return append([]TransmissionPath(nil), values[:n]...)
}

func tailPathFrom(values []TransmissionPath, start int) []TransmissionPath {
	if start < 0 || start >= len(values) {
		return nil
	}
	return append([]TransmissionPath(nil), values[start:]...)
}

func firstOrDefault(values []string, fallback string) string {
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return strings.TrimSpace(fallback)
	}
	return strings.TrimSpace(values[0])
}

func mustFindGraphNodeByText(t *testing.T, nodes []GraphNode, want string) GraphNode {
	t.Helper()
	for _, node := range nodes {
		if strings.TrimSpace(node.Text) == strings.TrimSpace(want) {
			return node
		}
	}
	t.Fatalf("graph node %q not found in %#v", want, nodes)
	return GraphNode{}
}

func directThreeStepResponses() []llm.ProviderResponse {
	return []llm.ProviderResponse{
		{
			Text:  `{"drivers":["美国增长与回报预期继续压过政治风险定价"],"targets":["海外资金继续流入美国资产"],"details":{"caveats":["generator"]},"topics":["macro"],"confidence":"medium"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"drivers":[],"targets":["没有形成 sell America 交易"],"details":{"caveats":["challenge"]},"topics":["macro"],"confidence":"medium"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"drivers":["美国增长与回报预期继续压过政治风险定价"],"targets":["海外资金继续流入美国资产","没有形成 sell America 交易"],"details":{"caveats":["judge"]},"topics":["macro"],"confidence":"high"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"transmission_paths":[{"driver":"美国增长与回报预期继续压过政治风险定价","target":"海外资金继续流入美国资产","steps":["增长与回报预期继续压过政治风险定价","资本继续配置美国资产"]}],"details":{"caveats":["generator"]},"topics":["macro"],"confidence":"medium"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"transmission_paths":[{"driver":"美国增长与回报预期继续压过政治风险定价","target":"没有形成 sell America 交易","steps":["资本没有撤出美国资产"]}],"details":{"caveats":["challenge"]},"topics":["macro"],"confidence":"medium"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"transmission_paths":[{"driver":"美国增长与回报预期继续压过政治风险定价","target":"海外资金继续流入美国资产","steps":["增长与回报预期继续压过政治风险定价","资本继续配置美国资产"]},{"driver":"美国增长与回报预期继续压过政治风险定价","target":"没有形成 sell America 交易","steps":["资本没有撤出美国资产"]}],"details":{"caveats":["judge"]},"topics":["macro"],"confidence":"high"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"evidence_nodes":["海外资金继续流入美国资产"],"explanation_nodes":["市场仍按美国例外论框架理解风险"],"details":{"caveats":["generator"]},"topics":["macro"],"confidence":"medium"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"evidence_nodes":["美元指数并未出现持续性崩跌"],"explanation_nodes":[],"details":{"caveats":["challenge"]},"topics":["macro"],"confidence":"medium"}`,
			Model: "compile-model",
		},
		{
			Text:  `{"evidence_nodes":["海外资金继续流入美国资产","美元指数并未出现持续性崩跌"],"explanation_nodes":["市场仍按美国例外论框架理解风险"],"details":{"caveats":["judge"]},"topics":["macro"],"confidence":"high"}`,
			Model: "compile-model",
		},
	}
}

func TestClientCompileUsesDirectThreeStepPipeline(t *testing.T) {
	provider := &compileMockProvider{responses: directThreeStepResponses()}
	client := NewClientWithRuntimePromptsAndVerifier(
		newTestRuntime(provider, "compile-model"),
		"compile-model",
		newPromptRegistry(""),
		noopVerificationService{},
	)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:three-step",
		Source:     "web",
		ExternalID: "three-step",
		Content:    "root body",
		PostedAt:   mustClientTime(t, "2026-04-14T09:30:00Z"),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("provider calls = %d, want 9 direct-stage calls", len(provider.requests))
	}
	if got, want := record.Output.Drivers, []string{"美国增长与回报预期继续压过政治风险定价"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Drivers = %#v, want %#v", got, want)
	}
	if got, want := record.Output.Targets, []string{"海外资金继续流入美国资产", "没有形成 sell America 交易"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets = %#v, want %#v", got, want)
	}
	if len(record.Output.TransmissionPaths) != 2 {
		t.Fatalf("TransmissionPaths = %#v, want 2 paths", record.Output.TransmissionPaths)
	}
	if got, want := record.Output.EvidenceNodes, []string{"海外资金继续流入美国资产", "美元指数并未出现持续性崩跌"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EvidenceNodes = %#v, want %#v", got, want)
	}
	if got, want := record.Output.ExplanationNodes, []string{"市场仍按美国例外论框架理解风险"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExplanationNodes = %#v, want %#v", got, want)
	}
	if len(record.Output.Graph.Nodes) < 6 {
		t.Fatalf("derived graph nodes = %#v, want compatibility graph from direct pipeline", record.Output.Graph.Nodes)
	}
	if len(record.Output.Graph.Edges) < 5 {
		t.Fatalf("derived graph edges = %#v, want compatibility graph edges", record.Output.Graph.Edges)
	}
	if record.Output.Summary == "" {
		t.Fatalf("Summary = empty, want derived summary")
	}
}

func TestParseOutputAcceptsJSONString(t *testing.T) {
	raw := `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Summary != "一句话" {
		t.Fatalf("Summary = %q", out.Summary)
	}
}

func TestClientCompileUsesForgeRuntime(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if strings.TrimSpace(record.Output.Summary) == "" {
		t.Fatalf("Summary = %q, want non-empty", record.Output.Summary)
	}
	if len(provider.requests) != 12 {
		t.Fatalf("provider calls = %d, want 12", len(provider.requests))
	}
	if provider.requests[0].Model != "compile-model" {
		t.Fatalf("request model = %q, want compile-model", provider.requests[0].Model)
	}
	if record.Output.Verification.Model == "" || len(record.Output.Verification.FactChecks) != 1 {
		t.Fatalf("verification = %#v", record.Output.Verification)
	}
}

func TestClientCompileProjectsInjectedVerificationServiceIntoCompatibilityOutput(t *testing.T) {
	provider := &compileMockProvider{responses: compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model")}
	verifier := &stubVerificationService{
		verification: Verification{
			Model:        "downstream-verify",
			Version:      "verify_v2",
			RolloutStage: "facts_only",
			FactChecks: []FactCheck{{
				NodeID: "n1",
				Status: FactStatusClearlyTrue,
				Reason: "projected by downstream verifier",
			}},
		},
	}
	client := NewClientWithRuntimePromptsAndVerifier(
		newTestRuntime(provider, "compile-model"),
		"compile-model",
		newPromptRegistry(""),
		verifier,
	)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:projection",
		Source:     "weibo",
		ExternalID: "projection",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("provider calls = %d, want 9 compile-stage calls when verifier is injected", len(provider.requests))
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", verifier.calls)
	}
	if verifier.gotBundle.ExternalID != "projection" {
		t.Fatalf("verifier bundle = %#v", verifier.gotBundle)
	}
	if len(verifier.gotOutput.Graph.Nodes) != 2 {
		t.Fatalf("verifier output graph = %#v", verifier.gotOutput.Graph)
	}
	if record.Output.Verification.Model != "downstream-verify" {
		t.Fatalf("record verification model = %q, want downstream-verify", record.Output.Verification.Model)
	}
	if len(record.Output.Verification.FactChecks) != 1 || record.Output.Verification.FactChecks[0].NodeID != "n1" {
		t.Fatalf("record verification = %#v", record.Output.Verification)
	}
}

func TestNoopVerificationServiceSkipsVerificationProjection(t *testing.T) {
	provider := &compileMockProvider{responses: compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model")}
	client := NewClientWithRuntimePromptsAndVerifier(
		newTestRuntime(provider, "compile-model"),
		"compile-model",
		newPromptRegistry(""),
		noopVerificationService{},
	)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:no-verify",
		Source:     "weibo",
		ExternalID: "no-verify",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("provider calls = %d, want 9", len(provider.requests))
	}
	if !record.Output.Verification.VerifiedAt.IsZero() || record.Output.Verification.Model != "" || len(record.Output.Verification.FactChecks) != 0 {
		t.Fatalf("verification = %#v, want zero-value verification", record.Output.Verification)
	}
}

func TestApplyBundleTimingFallbacksUsesPostedAtForFactMechanismAndPrediction(t *testing.T) {
	postedAt := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	graph := ReasoningGraph{
		Nodes: []GraphNode{
			{ID: "n1", Kind: NodeFact, Text: "事实A"},
			{ID: "n2", Kind: NodeMechanism, Text: "机制B"},
			{ID: "n3", Kind: NodePrediction, Text: "预测C"},
			{ID: "n4", Kind: NodeConclusion, Text: "结论D"},
		},
	}
	applyBundleTimingFallbacks(Bundle{PostedAt: postedAt}, &graph)
	if !graph.Nodes[0].OccurredAt.Equal(postedAt) {
		t.Fatalf("fact OccurredAt = %v, want %v", graph.Nodes[0].OccurredAt, postedAt)
	}
	if !graph.Nodes[1].OccurredAt.Equal(postedAt) {
		t.Fatalf("mechanism OccurredAt = %v, want %v", graph.Nodes[1].OccurredAt, postedAt)
	}
	if !graph.Nodes[2].PredictionStartAt.Equal(postedAt) {
		t.Fatalf("prediction start = %v, want %v", graph.Nodes[2].PredictionStartAt, postedAt)
	}
	if !graph.Nodes[3].ValidFrom.IsZero() || !graph.Nodes[3].ValidTo.IsZero() || !graph.Nodes[3].OccurredAt.IsZero() {
		t.Fatalf("conclusion timing should remain untouched: %#v", graph.Nodes[3])
	}
}

func TestClientCompileUsesPostedAtFallbackForTransmissionBridgeNodeTiming(t *testing.T) {
	provider := &compileMockProvider{responses: directThreeStepResponses()}
	client := NewClientWithRuntimePromptsAndVerifier(
		newTestRuntime(provider, "compile-model"),
		"compile-model",
		newPromptRegistry(""),
		noopVerificationService{},
	)

	postedAt := mustClientTime(t, "2026-04-14T09:30:00Z")
	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:g04",
		Source:     "web",
		ExternalID: "g04",
		Content:    "root body",
		PostedAt:   postedAt,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	bridge := mustFindGraphNodeByText(t, record.Output.Graph.Nodes, "资本继续配置美国资产")
	if !bridge.OccurredAt.Equal(postedAt) {
		t.Fatalf("bridge OccurredAt = %v, want %v", bridge.OccurredAt, postedAt)
	}
}

func TestBuildCausalProjectionKeepsOnlyDrivesEdgesAndConnectedNodes(t *testing.T) {
	nodes := []GraphNode{
		{ID: "n1", Form: NodeFormObservation, Function: NodeFunctionSupport, Text: "海外资金继续流入美国资产", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
		{ID: "n2", Form: NodeFormObservation, Function: NodeFunctionTransmission, Text: "增长预期压过政治风险定价并维持美国资产配置偏好", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
		{ID: "n3", Form: NodeFormCondition, Function: NodeFunctionClaim, Text: "若增长溢价逆转"},
		{ID: "n4", Form: NodeFormJudgment, Function: NodeFunctionClaim, Text: "当前并不存在 sell America trade"},
		{ID: "n5", Form: NodeFormObservation, Function: NodeFunctionSupport, Text: "市场仍按美国例外论框架理解风险", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
		{ID: "n6", Form: NodeFormForecast, Function: NodeFunctionClaim, Text: "资本流入会放缓", PredictionStartAt: mustTime(t, "2026-04-14T00:00:00Z")},
		{ID: "n7", Form: NodeFormObservation, Function: NodeFunctionSupport, Text: "不在主因果链上的旁支观察", OccurredAt: mustTime(t, "2026-04-14T00:00:00Z")},
	}
	edges := []GraphEdge{
		{From: "n1", To: "n4", Kind: EdgeDerives},
		{From: "n2", To: "n4", Kind: EdgePositive},
		{From: "n3", To: "n6", Kind: EdgePresets},
		{From: "n5", To: "n4", Kind: EdgeExplains},
	}

	projection := buildCausalProjection(nodes, edges)

	if !reflect.DeepEqual(projection.Edges, []GraphEdge{{From: "n2", To: "n4", Kind: EdgePositive}}) {
		t.Fatalf("projection edges = %#v", projection.Edges)
	}
	gotNodeIDs := make([]string, 0, len(projection.Nodes))
	for _, node := range projection.Nodes {
		gotNodeIDs = append(gotNodeIDs, node.ID)
	}
	if !reflect.DeepEqual(gotNodeIDs, []string{"n2", "n4"}) {
		t.Fatalf("projection node ids = %#v, want n2/n4 only", gotNodeIDs)
	}
}

func TestClientCompileCarriesPrimaryTransmissionBridgeIntoThesisProjection(t *testing.T) {
	provider := &compileMockProvider{responses: directThreeStepResponses()}
	client := NewClientWithRuntimePromptsAndVerifier(
		newTestRuntime(provider, "compile-model"),
		"compile-model",
		newPromptRegistry(""),
		noopVerificationService{},
	)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:g04-bridge",
		Source:     "web",
		ExternalID: "g04-bridge",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("provider calls = %d, want 9 compile-stage calls", len(provider.requests))
	}
	if !containsGraphNodeText(record.Output.Graph.Nodes, "资本继续配置美国资产") {
		t.Fatalf("merged graph missing transmission bridge: %#v", record.Output.Graph.Nodes)
	}
	judgePrompt := provider.requests[5].UserParts[len(provider.requests[5].UserParts)-1].Text
	for _, want := range []string{
		"美国增长与回报预期继续压过政治风险定价",
		"没有形成 sell America 交易",
		"Challenge draft:",
	} {
		if !strings.Contains(judgePrompt, want) {
			t.Fatalf("transmission-path judge prompt missing %q in %q", want, judgePrompt)
		}
	}
}

func TestClientCompileUsesConfiguredPromptsDir(t *testing.T) {
	root := t.TempDir()
	settings := config.DefaultSettings(root)
	for rel, body := range map[string]string{
		"compile/driver_target_generator_system.tmpl":        "driver-target generator system",
		"compile/driver_target_generator_user.tmpl":          "driver-target generator user {{.PayloadJSON}}",
		"compile/driver_target_challenge_system.tmpl":        "driver-target challenge system",
		"compile/driver_target_challenge_user.tmpl":          "driver-target challenge user {{.GeneratedJSON}} {{.PayloadJSON}}",
		"compile/driver_target_judge_system.tmpl":            "driver-target judge system",
		"compile/driver_target_judge_user.tmpl":              "driver-target judge user {{.GeneratedJSON}} {{.ChallengedJSON}} {{.PayloadJSON}}",
		"compile/transmission_path_generator_system.tmpl":    "transmission-path generator system",
		"compile/transmission_path_generator_user.tmpl":      "transmission-path generator user {{.DriverTargetJSON}} {{.PayloadJSON}}",
		"compile/transmission_path_challenge_system.tmpl":    "transmission-path challenge system",
		"compile/transmission_path_challenge_user.tmpl":      "transmission-path challenge user {{.GeneratedJSON}} {{.DriverTargetJSON}} {{.PayloadJSON}}",
		"compile/transmission_path_judge_system.tmpl":        "transmission-path judge system",
		"compile/transmission_path_judge_user.tmpl":          "transmission-path judge user {{.GeneratedJSON}} {{.ChallengedJSON}} {{.DriverTargetJSON}} {{.PayloadJSON}}",
		"compile/evidence_explanation_generator_system.tmpl": "evidence-explanation generator system",
		"compile/evidence_explanation_generator_user.tmpl":   "evidence-explanation generator user {{.PathsJSON}} {{.DriverTargetJSON}} {{.PayloadJSON}}",
		"compile/evidence_explanation_challenge_system.tmpl": "evidence-explanation challenge system",
		"compile/evidence_explanation_challenge_user.tmpl":   "evidence-explanation challenge user {{.GeneratedJSON}} {{.PathsJSON}} {{.DriverTargetJSON}} {{.PayloadJSON}}",
		"compile/evidence_explanation_judge_system.tmpl":     "evidence-explanation judge system",
		"compile/evidence_explanation_judge_user.tmpl":       "evidence-explanation judge user {{.GeneratedJSON}} {{.ChallengedJSON}} {{.PathsJSON}} {{.DriverTargetJSON}} {{.PayloadJSON}}",
		"compile/verifier/fact_claim.tmpl":                   "fact claim prompt",
		"compile/verifier/fact_challenge.tmpl":               "fact challenge prompt",
		"compile/verifier/fact_adjudicate.tmpl":              "fact adjudication prompt",
		"compile/verifier/prediction.tmpl":                   "prediction verifier prompt",
		"compile/verifier/explicit_condition.tmpl":           "explicit verifier prompt",
		"compile/verifier/implicit_condition.tmpl":           "implicit verifier prompt",
	} {
		path := filepath.Join(settings.PromptsDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}

	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","occurred_at":"2026-04-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntimeAndPrompts(newTestRuntime(provider, "compile-model"), "compile-model", newPromptRegistry(settings.PromptsDir))

	_, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got := provider.requests[0].System; got != "driver-target generator system" {
		t.Fatalf("driver-target generator system prompt = %q", got)
	}
	if got := provider.requests[1].System; got != "driver-target challenge system" {
		t.Fatalf("driver-target challenge system prompt = %q", got)
	}
	if got := provider.requests[2].System; got != "driver-target judge system" {
		t.Fatalf("driver-target judge system prompt = %q", got)
	}
	if got := provider.requests[3].System; got != "transmission-path generator system" {
		t.Fatalf("transmission-path generator system prompt = %q", got)
	}
	if got := provider.requests[4].System; got != "transmission-path challenge system" {
		t.Fatalf("transmission-path challenge system prompt = %q", got)
	}
	if got := provider.requests[5].System; got != "transmission-path judge system" {
		t.Fatalf("transmission-path judge system prompt = %q", got)
	}
	if got := provider.requests[6].System; got != "evidence-explanation generator system" {
		t.Fatalf("evidence-explanation generator system prompt = %q", got)
	}
	if got := provider.requests[7].System; got != "evidence-explanation challenge system" {
		t.Fatalf("evidence-explanation challenge system prompt = %q", got)
	}
	if got := provider.requests[8].System; got != "evidence-explanation judge system" {
		t.Fatalf("evidence-explanation judge system prompt = %q", got)
	}
	if got := provider.requests[9].System; got != "fact claim prompt" {
		t.Fatalf("fact verifier system prompt = %q", got)
	}
}

func TestClientCompileConfiguredPromptsDirFallsBackToDefaultForMissingThreeStageTemplates(t *testing.T) {
	root := t.TempDir()
	settings := config.DefaultSettings(root)
	for rel, body := range map[string]string{
		"compile/node_system.tmpl": "legacy node system min={{.MinNodes}}",
		"compile/node_user.tmpl":   "legacy node user {{.PayloadJSON}}",
	} {
		path := filepath.Join(settings.PromptsDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}

	provider := &compileMockProvider{responses: directThreeStepResponses()}
	client := NewClientWithRuntimePromptsAndVerifier(
		newTestRuntime(provider, "compile-model"),
		"compile-model",
		newPromptRegistry(settings.PromptsDir),
		noopVerificationService{},
	)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:fallback-prompts",
		Source:     "web",
		ExternalID: "fallback-prompts",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("provider calls = %d, want 9 direct-stage calls", len(provider.requests))
	}
	if strings.TrimSpace(record.Output.Summary) == "" {
		t.Fatalf("Summary = %q, want non-empty", record.Output.Summary)
	}
}

func TestBuildCompatibilityGraphPadsSparseDirectOutput(t *testing.T) {
	graph := buildCompatibilityGraph(
		Bundle{Content: "root body"},
		DriverTargetOutput{Drivers: []string{"增长溢价上升"}, Targets: []string{"美元走强"}},
		TransmissionPathOutput{},
		EvidenceExplanationOutput{},
	)
	if len(graph.Nodes) < 2 {
		t.Fatalf("len(nodes) = %d, want at least 2", len(graph.Nodes))
	}
	if len(graph.Edges) < 1 {
		t.Fatalf("len(edges) = %d, want at least 1", len(graph.Edges))
	}
}

func TestClientCompileCarriesAttachmentTranscriptIntoForgePrompt(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	_, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:1",
		Source:     "weibo",
		ExternalID: "1",
		Content:    "root body",
		Attachments: []types.Attachment{{
			Type:       "video",
			Transcript: "私募信贷会先爆雷",
		}},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 12 {
		t.Fatalf("provider calls = %d, want 12", len(provider.requests))
	}
	if len(provider.requests[0].UserParts) == 0 {
		t.Fatalf("provider request missing user parts: %#v", provider.requests[0])
	}
	got := provider.requests[0].UserParts[len(provider.requests[0].UserParts)-1].Text
	if got == "" || !containsAll(got, "[ATTACHMENT TRANSCRIPT 1]", "私募信贷会先爆雷") {
		t.Fatalf("provider user prompt missing attachment transcript: %q", got)
	}
}

func TestClientCompileRetriesWhenFirstResponseHasEmptyGraph(t *testing.T) {
	t.Skip("legacy node/edge retry path removed by direct three-stage compile redesign")
	validStages := compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model")
	provider := &compileMockProvider{responses: append([]llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{},"details":{},"topics":[],"confidence":"medium"}`, Model: "compile-model"},
	}, append(validStages, []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("call count = %d, want 9", len(provider.requests))
	}
	retryPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	if !containsAll(
		retryPrompt,
		"output nodes only; do not output any edges",
		"every node must include `form` and `function`",
		"use only `form` values `observation`, `condition`, `judgment`, `forecast`",
		"split mixed fact / judgment / prediction statements into separate nodes when possible",
	) {
		t.Fatalf("retry prompt missing mixed-clause split guidance: %q", retryPrompt)
	}
	if len(record.Output.Graph.Nodes) != 2 {
		t.Fatalf("nodes = %#v", record.Output.Graph.Nodes)
	}
}

func TestClientCompileRetriesWhenLongformGraphTooSparse(t *testing.T) {
	t.Skip("legacy node/edge retry path removed by direct three-stage compile redesign")
	sparseNode := `{"graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`
	validStages := compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"事实","text":"事实B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n3","kind":"隐含条件","text":"条件C","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n4","kind":"结论","text":"结论D","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n3","kind":"正向"},{"from":"n2","to":"n3","kind":"正向"},{"from":"n3","to":"n4","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model")
	provider := &compileMockProvider{responses: append([]llm.ProviderResponse{
		{Text: sparseNode, Model: "compile-model"},
	}, append(validStages, []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n2","status":"unverifiable","reason":"unclear"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"},{"node_id":"n2","assessment":"insufficient_evidence","reason":"evidence is weak"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n2","status":"unverifiable","reason":"unclear"}]}`, Model: "fact-judge-model"},
		{Text: `{"implicit_condition_checks":[{"node_id":"n3","status":"unverifiable","reason":"implicit premise not evidenced"}]}`, Model: "implicit-verifier-model"},
	}...)...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    strings.Repeat("长文", 2000),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 10 {
		t.Fatalf("call count = %d, want 10", len(provider.requests))
	}
	retryPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	if !containsAll(
		retryPrompt,
		"for flow/positioning articles, split support observations, transmission mechanisms, and judgment/forecast claims into separate nodes",
		"use only `function` values `support`, `transmission`, `claim`",
	) {
		t.Fatalf("retry prompt missing form/function guidance: %q", retryPrompt)
	}
	if len(record.Output.Graph.Nodes) != 4 || len(record.Output.Graph.Edges) != 3 {
		t.Fatalf("graph = %#v", record.Output.Graph)
	}
}

func TestClientCompileRetriesWhenDetailsEmpty(t *testing.T) {
	t.Skip("legacy thesis retry path removed by direct three-stage compile redesign")
	nodeStages := compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model")
	emptyDetailsThesis := `{"summary":"一句话","drivers":["d"],"targets":["t"],"details":{},"topics":["topic"],"confidence":"medium"}`
	provider := &compileMockProvider{responses: append([]llm.ProviderResponse{
		nodeStages[0],
		nodeStages[1],
		nodeStages[2],
		nodeStages[3],
		{Text: emptyDetailsThesis, Model: "compile-model"},
		nodeStages[4],
	}, []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 9 {
		t.Fatalf("call count = %d, want 9", len(provider.requests))
	}
	if len(record.Output.Details.Caveats) != 1 {
		t.Fatalf("details = %#v", record.Output.Details)
	}
}

func TestClientCompileRunsFactAndPredictionVerifierPasses(t *testing.T) {
	prevBuildFactRetrievalContext := buildFactRetrievalContext
	t.Cleanup(func() { buildFactRetrievalContext = prevBuildFactRetrievalContext })
	buildFactRetrievalContext = func(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, error) {
		return []map[string]any{{
			"node_id": "n1",
			"results": []map[string]any{{
				"url":     "https://example.com/fact",
				"title":   "Example",
				"excerpt": "验证材料",
			}},
		}}, nil
	}

	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"预测","text":"预测B","prediction_start_at":"2026-04-14T00:00:00Z","prediction_due_at":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
		{Text: `{"prediction_checks":[{"node_id":"n2","status":"unresolved","reason":"still in window","as_of":"2026-04-15T00:00:00Z"}]}`, Model: "prediction-verifier-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:123",
		Source:     "weibo",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 12 {
		t.Fatalf("provider calls = %d, want 12", len(provider.requests))
	}
	if len(record.Output.Verification.FactChecks) != 1 {
		t.Fatalf("verification = %#v", record.Output.Verification)
	}
	if record.Output.Verification.FactChecks[0].Status != FactStatusClearlyTrue {
		t.Fatalf("fact check = %#v", record.Output.Verification.FactChecks[0])
	}
	factPrompt := provider.requests[9].UserParts[len(provider.requests[9].UserParts)-1].Text
	if !containsAll(factPrompt, `"kind": "机制"`, `"occurred_at": "`, `"as_of": "`, `"retrieval_context"`, `"https://example.com/fact"`) {
		t.Fatalf("fact verifier prompt missing occurred_at evidence: %q", factPrompt)
	}
	if strings.Contains(factPrompt, `"valid_from"`) {
		t.Fatalf("fact verifier prompt should prefer occurred_at over legacy valid_from: %q", factPrompt)
	}
}

func TestClientCompileRoutesConditionAndConclusionNodesThroughVerifier(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"显式条件","text":"条件B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n3","kind":"隐含条件","text":"条件C","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n4","kind":"结论","text":"结论D","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n5","kind":"预测","text":"预测E","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"正向"},{"from":"n2","to":"n3","kind":"预设"},{"from":"n3","to":"n4","kind":"推出"},{"from":"n4","to":"n5","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
		{Text: `{"explicit_condition_checks":[{"node_id":"n2","status":"unknown","reason":"future condition uncertain"}]}`, Model: "explicit-verifier-model"},
		{Text: `{"implicit_condition_checks":[{"node_id":"n3","status":"unverifiable","reason":"implicit premise not evidenced"}]}`, Model: "implicit-verifier-model"},
		{Text: `{"prediction_checks":[{"node_id":"n5","status":"unresolved","reason":"still in window","as_of":"2026-04-15T00:00:00Z"}]}`, Model: "prediction-verifier-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:123",
		Source:     "weibo",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 12 {
		t.Fatalf("provider calls = %d, want 12", len(provider.requests))
	}
	factPrompt := provider.requests[9].UserParts[len(provider.requests[9].UserParts)-1].Text
	for _, want := range []string{`"kind": "机制"`} {
		if !strings.Contains(factPrompt, want) {
			t.Fatalf("fact verifier prompt missing %q in %q", want, factPrompt)
		}
	}
	if !strings.Contains(factPrompt, `"as_of": "`) {
		t.Fatalf("fact verifier prompt missing as_of context: %q", factPrompt)
	}
	for _, unwanted := range []string{`"kind": "显式条件"`, `"kind": "隐含条件"`, `"kind": "结论"`, `"kind": "预测"`} {
		if strings.Contains(factPrompt, unwanted) {
			t.Fatalf("fact verifier prompt should exclude %q: %q", unwanted, factPrompt)
		}
	}
	if len(record.Output.Verification.FactChecks) != 1 {
		t.Fatalf("len(FactChecks) = %d, want 1", len(record.Output.Verification.FactChecks))
	}
	if len(record.Output.Verification.ExplicitConditionChecks) != 0 || len(record.Output.Verification.ImplicitConditionChecks) != 0 || len(record.Output.Verification.PredictionChecks) != 0 {
		t.Fatalf("compatibility output should not synthesize legacy condition/prediction verifier passes: %#v", record.Output.Verification)
	}
}

func TestClientCompileRoutesObservationTransmissionNodesThroughFactVerifier(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","form":"observation","function":"support","text":"海外资金继续流入美国资产","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","form":"observation","function":"transmission","text":"增长预期仍压过政治风险并维持美国资产配置偏好","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n3","form":"judgment","function":"claim","text":"当前不存在 sell America trade"}],"edges":[{"from":"n2","to":"n1","kind":"正向"},{"from":"n1","to":"n3","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n4","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"},{"node_id":"n4","assessment":"supported","reason":"bridge mechanism is grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n4","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "web:g04-routing",
		Source:     "web",
		ExternalID: "g04-routing",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(provider.requests) != 12 {
		t.Fatalf("provider calls = %d, want 12", len(provider.requests))
	}
	factPrompt := provider.requests[9].UserParts[len(provider.requests[9].UserParts)-1].Text
	for _, want := range []string{`"kind": "事实"`, `"kind": "机制"`} {
		if !strings.Contains(factPrompt, want) {
			t.Fatalf("fact verifier prompt missing %q in %q", want, factPrompt)
		}
	}
	if strings.Contains(factPrompt, `"kind": "结论"`) {
		t.Fatalf("fact verifier prompt should exclude judgment nodes: %q", factPrompt)
	}
	if len(record.Output.Verification.FactChecks) != 2 {
		t.Fatalf("len(FactChecks) = %d, want 2", len(record.Output.Verification.FactChecks))
	}
	if !reflect.DeepEqual(
		[]string{record.Output.Verification.FactChecks[0].NodeID, record.Output.Verification.FactChecks[1].NodeID},
		[]string{"n1", "n4"},
	) {
		t.Fatalf("FactChecks = %#v", record.Output.Verification.FactChecks)
	}
	if record.Output.Verification.CoverageSummary == nil || record.Output.Verification.CoverageSummary.TotalExpectedNodes != 2 {
		t.Fatalf("CoverageSummary = %#v, want 2 expected verified observation nodes", record.Output.Verification.CoverageSummary)
	}
}

func TestClientCompileCarriesStructuredWeiboEvidenceIntoVerifierPrompt(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"正向"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	_, err := client.Compile(context.Background(), Bundle{
		UnitID:         "weibo:123",
		Source:         "weibo",
		ExternalID:     "123",
		RootExternalID: "120",
		AuthorName:     "alice",
		AuthorID:       "u1",
		URL:            "https://weibo.com/u1/123",
		PostedAt:       mustClientTime(t, "2026-04-14T09:30:00Z"),
		Content:        "直播里说风险开始暴露",
		Quotes: []types.Quote{{
			Content: "嘉宾说风险已经扩散。",
		}},
		References: []types.Reference{{
			Content: "财报里确认应收回款放缓。",
			URL:     "https://example.com/report",
		}},
		ThreadSegments: []types.ThreadSegment{{
			Position: 2,
			Content:  "补充了一条现场观察。",
		}},
		Attachments: []types.Attachment{{
			Type:       "video",
			Transcript: "视频里明确说今天上午开始挤兑。",
		}},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	factPrompt := provider.requests[9].UserParts[len(provider.requests[9].UserParts)-1].Text
	for _, want := range []string{
		`"root_external_id": "120"`,
		`"author_name": "alice"`,
		`"author_id": "u1"`,
		`"url": "https://weibo.com/u1/123"`,
		`"posted_at": "2026-04-14T09:30:00Z"`,
		`财报里确认应收回款放缓。`,
		`补充了一条现场观察。`,
		`视频里明确说今天上午开始挤兑。`,
		`[ATTACHMENT TRANSCRIPT 1]`,
	} {
		if !strings.Contains(factPrompt, want) {
			t.Fatalf("fact verifier prompt missing %q in %q", want, factPrompt)
		}
	}
}

func TestClientCompileAppliesVerifyV2FactsMetadataWithoutBreakingLegacyArrays(t *testing.T) {
	prevBuildFactRetrievalContext := buildFactRetrievalContext
	t.Cleanup(func() { buildFactRetrievalContext = prevBuildFactRetrievalContext })
	buildFactRetrievalContext = func(ctx context.Context, bundle Bundle, nodes []GraphNode) ([]map[string]any, error) {
		return []map[string]any{{
			"node_id": "n1",
			"results": []map[string]any{{
				"url":     "https://example.com/fact",
				"title":   "Example",
				"excerpt": "验证材料",
			}},
		}}, nil
	}

	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"claim_checks":[{"node_id":"n1","status":"clearly_true","reason":"retrieval-backed claim"}],"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"retrieval-backed claim"}],"output_node_ids":["n1"]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"contested","reason":"challenge raised"}],"output_node_ids":["n1"]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"retrieval-backed adjudication"}],"output_node_ids":["n1"]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:verify-v2",
		Source:     "weibo",
		ExternalID: "verify-v2",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(record.Output.Verification.FactChecks) != 1 {
		t.Fatalf("len(FactChecks) = %d, want 1", len(record.Output.Verification.FactChecks))
	}
	if record.Output.Verification.FactChecks[0].NodeID != "n1" || record.Output.Verification.FactChecks[0].Status != FactStatusClearlyTrue {
		t.Fatalf("FactChecks = %#v", record.Output.Verification.FactChecks)
	}
	assertClientVerifyV2StringField(t, record.Output.Verification, "Version", "verify_v2")
	assertClientVerifyV2StringField(t, record.Output.Verification, "RolloutStage", "facts_only")
	assertClientVerifyV2SliceLen(t, record.Output.Verification, "Passes", 1)
	assertClientVerifyV2StringField(t, record.Output.Verification, []string{"Passes", "0", "Kind"}, "fact")
	assertClientVerifyV2BoolField(t, record.Output.Verification, []string{"Passes", "0", "Coverage", "Valid"}, true)
	assertClientVerifyV2StringSlice(t, record.Output.Verification, []string{"Passes", "0", "RetrievalSummary", "RetrievedNodeIDs"}, []string{"n1"})
	assertClientVerifyV2StringField(t, record.Output.Verification, []string{"Passes", "0", "Adjudication", "Model"}, "fact-judge-model")
	assertClientVerifyV2TimeFieldMatchesVerification(t, record.Output.Verification, []string{"Passes", "0", "Adjudication", "CompletedAt"})
	assertClientVerifyV2IntField(t, record.Output.Verification, []string{"CoverageSummary", "TotalExpectedNodes"}, 1)
	assertClientVerifyV2IntField(t, record.Output.Verification, []string{"CoverageSummary", "TotalFinalizedNodes"}, 1)
	assertClientVerifyV2BoolField(t, record.Output.Verification, []string{"CoverageSummary", "Valid"}, true)
	if record.Output.Verification.Model != "fact-judge-model" {
		t.Fatalf("Verification.Model = %q, want fact-judge-model", record.Output.Verification.Model)
	}
	factPrompt := provider.requests[9].UserParts[len(provider.requests[9].UserParts)-1].Text
	if !containsAll(factPrompt, `"retrieval_context"`, `"https://example.com/fact"`) {
		t.Fatalf("fact verifier prompt missing retrieval context: %q", factPrompt)
	}
}

func TestClientCompileFailsDeterministicallyOnVerifyV2CoverageMismatch(t *testing.T) {
	provider := &compileMockProvider{responses: append(compileStageResponses(t,
		`{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"事实","text":"事实B","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n3","kind":"结论","text":"结论C"}],"edges":[{"from":"n1","to":"n3","kind":"推出"},{"from":"n2","to":"n3","kind":"正向"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, "compile-model"), []llm.ProviderResponse{
		{Text: `{"claim_checks":[{"node_id":"n1","status":"clearly_true","reason":"claim"},{"node_id":"n2","status":"unverifiable","reason":"claim"}],"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"claim"},{"node_id":"n2","status":"unverifiable","reason":"claim"}],"output_node_ids":["n1","n2"]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"contested","reason":"challenge"},{"node_id":"n2","assessment":"insufficient_evidence","reason":"challenge"}],"output_node_ids":["n1","n2"]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"adjudicated only one node"}],"output_node_ids":["n1"]}`, Model: "fact-judge-model"},
	}...)}
	client := NewClientWithRuntime(newTestRuntime(provider, "compile-model"), "compile-model")

	_, err := client.Compile(context.Background(), Bundle{
		UnitID:     "weibo:coverage-mismatch",
		Source:     "weibo",
		ExternalID: "coverage-mismatch",
		Content:    "root body",
	})
	if err == nil {
		t.Fatal("Compile() error = nil, want coverage mismatch failure")
	}
	for _, want := range []string{"coverage", "n3"} {
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
			t.Fatalf("Compile() error = %q, want substring %q", err.Error(), want)
		}
	}
}

func assertClientVerifyV2StringField(t *testing.T, root any, path any, want string) {
	t.Helper()
	got := mustResolveClientVerifyV2Path(t, root, path)
	if got.Kind() != reflect.String {
		t.Fatalf("path %v kind = %s, want string", normalizeClientVerifyV2Path(path), got.Kind())
	}
	if got.String() != want {
		t.Fatalf("path %v = %q, want %q", normalizeClientVerifyV2Path(path), got.String(), want)
	}
}

func assertClientVerifyV2BoolField(t *testing.T, root any, path any, want bool) {
	t.Helper()
	got := mustResolveClientVerifyV2Path(t, root, path)
	if got.Kind() != reflect.Bool {
		t.Fatalf("path %v kind = %s, want bool", normalizeClientVerifyV2Path(path), got.Kind())
	}
	if got.Bool() != want {
		t.Fatalf("path %v = %v, want %v", normalizeClientVerifyV2Path(path), got.Bool(), want)
	}
}

func assertClientVerifyV2StringSlice(t *testing.T, root any, path []string, want []string) {
	t.Helper()
	got := mustResolveClientVerifyV2Path(t, root, path)
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

func assertClientVerifyV2SliceLen(t *testing.T, root any, field string, want int) {
	t.Helper()
	got := mustResolveClientVerifyV2Path(t, root, []string{field})
	if got.Kind() != reflect.Slice {
		t.Fatalf("field %s kind = %s, want slice", field, got.Kind())
	}
	if got.Len() != want {
		t.Fatalf("field %s len = %d, want %d", field, got.Len(), want)
	}
}

func assertClientVerifyV2IntField(t *testing.T, root any, path any, want int) {
	t.Helper()
	got := mustResolveClientVerifyV2Path(t, root, path)
	if got.Kind() != reflect.Int {
		t.Fatalf("path %v kind = %s, want int", normalizeClientVerifyV2Path(path), got.Kind())
	}
	if int(got.Int()) != want {
		t.Fatalf("path %v = %d, want %d", normalizeClientVerifyV2Path(path), got.Int(), want)
	}
}

func assertClientVerifyV2TimeFieldMatchesVerification(t *testing.T, verification Verification, path []string) {
	t.Helper()
	got := mustResolveClientVerifyV2Path(t, verification, path)
	if got.Type() != reflect.TypeOf(time.Time{}) {
		t.Fatalf("path %v type = %s, want time.Time", path, got.Type())
	}
	completedAt := got.Interface().(time.Time)
	if !verification.VerifiedAt.Equal(completedAt) {
		t.Fatalf("Verification.VerifiedAt = %v, want adjudication completed_at %v", verification.VerifiedAt, completedAt)
	}
}

func mustResolveClientVerifyV2Path(t *testing.T, root any, path any) reflect.Value {
	t.Helper()
	parts := normalizeClientVerifyV2Path(path)
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

func normalizeClientVerifyV2Path(path any) []string {
	switch v := path.(type) {
	case string:
		return []string{v}
	case []string:
		return append([]string(nil), v...)
	default:
		panic("unsupported verify-v2 path type")
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func containsGraphNodeText(nodes []GraphNode, want string) bool {
	for _, node := range nodes {
		if node.Text == want {
			return true
		}
	}
	return false
}

func mustClientTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", raw, err)
	}
	return parsed
}
