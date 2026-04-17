package compile

import (
	"context"
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
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}}
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
	if record.Output.Summary != "一句话" {
		t.Fatalf("Summary = %q", record.Output.Summary)
	}
	if len(provider.requests) != 4 {
		t.Fatalf("provider calls = %d, want 4", len(provider.requests))
	}
	if provider.requests[0].Model != "compile-model" {
		t.Fatalf("request model = %q, want compile-model", provider.requests[0].Model)
	}
	if record.Output.Verification.Model == "" || len(record.Output.Verification.FactChecks) != 1 {
		t.Fatalf("verification = %#v", record.Output.Verification)
	}
}

func TestClientCompileProjectsInjectedVerificationServiceIntoCompatibilityOutput(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{{
		Text:  `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`,
		Model: "compile-model",
	}}}
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
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d, want compile-only call when verifier is injected", len(provider.requests))
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

func TestClientCompileUsesConfiguredPromptsDir(t *testing.T) {
	root := t.TempDir()
	settings := config.DefaultSettings(root)
	for rel, body := range map[string]string{
		"compile/system.tmpl":                      "compile system min={{.MinNodes}} edges={{.MinEdges}}",
		"compile/user.tmpl":                        "compile user {{.PayloadJSON}}",
		"compile/retry_suffix.tmpl":                "retry requires min={{.MinNodes}} edges={{.MinEdges}}",
		"compile/verifier/fact_claim.tmpl":         "fact claim prompt",
		"compile/verifier/fact_challenge.tmpl":     "fact challenge prompt",
		"compile/verifier/fact_adjudicate.tmpl":    "fact adjudication prompt",
		"compile/verifier/prediction.tmpl":         "prediction verifier prompt",
		"compile/verifier/explicit_condition.tmpl": "explicit verifier prompt",
		"compile/verifier/implicit_condition.tmpl": "implicit verifier prompt",
	} {
		path := filepath.Join(settings.PromptsDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}

	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","occurred_at":"2026-04-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}}
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
	if got := provider.requests[0].System; got != "compile system min=2 edges=1" {
		t.Fatalf("compile system prompt = %q", got)
	}
	if got := provider.requests[0].UserParts[len(provider.requests[0].UserParts)-1].Text; !strings.Contains(got, "compile user") {
		t.Fatalf("compile user prompt = %q", got)
	}
	if got := provider.requests[1].System; got != "fact claim prompt" {
		t.Fatalf("fact verifier system prompt = %q", got)
	}
}

func TestClientCompileCarriesAttachmentTranscriptIntoForgePrompt(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}}
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
	if len(provider.requests) != 4 {
		t.Fatalf("provider calls = %d, want 4", len(provider.requests))
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
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{},"details":{},"topics":[],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}}
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
	if len(provider.requests) != 5 {
		t.Fatalf("call count = %d, want 5", len(provider.requests))
	}
	retryPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	if !containsAll(
		retryPrompt,
		"显式条件 + 预测",
		"不能整句都标成显式条件",
		"两步或以上因果链",
		"不要把整条宏观链压成一个“胖事实”节点",
	) {
		t.Fatalf("retry prompt missing mixed-clause split guidance: %q", retryPrompt)
	}
	if len(record.Output.Graph.Nodes) != 2 {
		t.Fatalf("nodes = %#v", record.Output.Graph.Nodes)
	}
}

func TestClientCompileRetriesWhenLongformGraphTooSparse(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"事实","text":"事实B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n3","kind":"隐含条件","text":"条件C","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n4","kind":"结论","text":"结论D","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n3","kind":"正向"},{"from":"n2","to":"n3","kind":"正向"},{"from":"n3","to":"n4","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n2","status":"unverifiable","reason":"unclear"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"},{"node_id":"n2","assessment":"insufficient_evidence","reason":"evidence is weak"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n2","status":"unverifiable","reason":"unclear"}]}`, Model: "fact-judge-model"},
		{Text: `{"implicit_condition_checks":[{"node_id":"n3","status":"unverifiable","reason":"implicit premise not evidenced"}]}`, Model: "implicit-verifier-model"},
	}}
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
	if len(provider.requests) != 6 {
		t.Fatalf("call count = %d, want 6", len(provider.requests))
	}
	if len(record.Output.Graph.Nodes) != 4 || len(record.Output.Graph.Edges) != 3 {
		t.Fatalf("graph = %#v", record.Output.Graph)
	}
}

func TestClientCompileRetriesWhenDetailsEmpty(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}}
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
	if len(provider.requests) != 5 {
		t.Fatalf("call count = %d, want 5", len(provider.requests))
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

	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"预测","text":"预测B","prediction_start_at":"2026-04-14T00:00:00Z","prediction_due_at":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
		{Text: `{"prediction_checks":[{"node_id":"n2","status":"unresolved","reason":"still in window","as_of":"2026-04-15T00:00:00Z"}]}`, Model: "prediction-verifier-model"},
	}}
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
	if len(provider.requests) != 5 {
		t.Fatalf("provider calls = %d, want 5", len(provider.requests))
	}
	if len(record.Output.Verification.FactChecks) != 1 || len(record.Output.Verification.PredictionChecks) != 1 {
		t.Fatalf("verification = %#v", record.Output.Verification)
	}
	if record.Output.Verification.FactChecks[0].Status != FactStatusClearlyTrue {
		t.Fatalf("fact check = %#v", record.Output.Verification.FactChecks[0])
	}
	if record.Output.Verification.PredictionChecks[0].Status != PredictionStatusUnresolved {
		t.Fatalf("prediction check = %#v", record.Output.Verification.PredictionChecks[0])
	}
	factPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	if !containsAll(factPrompt, `"kind": "事实"`, `"occurred_at": "2026-04-14T00:00:00Z"`, `"as_of": "`, `"retrieval_context"`, `"https://example.com/fact"`) {
		t.Fatalf("fact verifier prompt missing occurred_at evidence: %q", factPrompt)
	}
	if strings.Contains(factPrompt, `"valid_from"`) {
		t.Fatalf("fact verifier prompt should prefer occurred_at over legacy valid_from: %q", factPrompt)
	}
	predictionPrompt := provider.requests[4].UserParts[len(provider.requests[4].UserParts)-1].Text
	if !containsAll(predictionPrompt, `"kind": "预测"`, `"prediction_start_at": "2026-04-14T00:00:00Z"`, `"prediction_due_at": "2026-07-14T00:00:00Z"`, `"as_of": "`) {
		t.Fatalf("prediction verifier prompt missing prediction window evidence: %q", predictionPrompt)
	}
	if strings.Contains(predictionPrompt, `"valid_from"`) {
		t.Fatalf("prediction verifier prompt should prefer prediction_start_at over legacy valid_from: %q", predictionPrompt)
	}
}

func TestClientCompileRoutesConditionAndConclusionNodesThroughVerifier(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"显式条件","text":"条件B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n3","kind":"隐含条件","text":"条件C","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n4","kind":"结论","text":"结论D","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n5","kind":"预测","text":"预测E","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"正向"},{"from":"n2","to":"n3","kind":"预设"},{"from":"n3","to":"n4","kind":"推出"},{"from":"n4","to":"n5","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
		{Text: `{"explicit_condition_checks":[{"node_id":"n2","status":"unknown","reason":"future condition uncertain"}]}`, Model: "explicit-verifier-model"},
		{Text: `{"implicit_condition_checks":[{"node_id":"n3","status":"unverifiable","reason":"implicit premise not evidenced"}]}`, Model: "implicit-verifier-model"},
		{Text: `{"prediction_checks":[{"node_id":"n5","status":"unresolved","reason":"still in window","as_of":"2026-04-15T00:00:00Z"}]}`, Model: "prediction-verifier-model"},
	}}
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
	if len(provider.requests) != 7 {
		t.Fatalf("provider calls = %d, want 7", len(provider.requests))
	}
	factPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	for _, want := range []string{`"kind": "事实"`} {
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
	if len(record.Output.Verification.ExplicitConditionChecks) != 1 {
		t.Fatalf("len(ExplicitConditionChecks) = %d, want 1", len(record.Output.Verification.ExplicitConditionChecks))
	}
	explicitPrompt := provider.requests[4].UserParts[len(provider.requests[4].UserParts)-1].Text
	if !strings.Contains(explicitPrompt, `"as_of": "`) {
		t.Fatalf("explicit condition verifier prompt missing as_of context: %q", explicitPrompt)
	}
	if len(record.Output.Verification.ImplicitConditionChecks) != 1 {
		t.Fatalf("len(ImplicitConditionChecks) = %d, want 1", len(record.Output.Verification.ImplicitConditionChecks))
	}
	implicitPrompt := provider.requests[5].UserParts[len(provider.requests[5].UserParts)-1].Text
	if !strings.Contains(implicitPrompt, `"as_of": "`) {
		t.Fatalf("implicit condition verifier prompt missing as_of context: %q", implicitPrompt)
	}
	if len(record.Output.Verification.PredictionChecks) != 1 {
		t.Fatalf("len(PredictionChecks) = %d, want 1", len(record.Output.Verification.PredictionChecks))
	}
}

func TestClientCompileCarriesStructuredWeiboEvidenceIntoVerifierPrompt(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"正向"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"supported","reason":"claim seems grounded"}]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-judge-model"},
	}}
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

	factPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
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

	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"claim_checks":[{"node_id":"n1","status":"clearly_true","reason":"retrieval-backed claim"}],"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"retrieval-backed claim"}],"output_node_ids":["n1"]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"contested","reason":"challenge raised"}],"output_node_ids":["n1"]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"retrieval-backed adjudication"}],"output_node_ids":["n1"]}`, Model: "fact-judge-model"},
	}}
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
	factPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	if !containsAll(factPrompt, `"retrieval_context"`, `"https://example.com/fact"`) {
		t.Fatalf("fact verifier prompt missing retrieval context: %q", factPrompt)
	}
}

func TestClientCompileFailsDeterministicallyOnVerifyV2CoverageMismatch(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"事实","text":"事实B","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n3","kind":"结论","text":"结论C"}],"edges":[{"from":"n1","to":"n3","kind":"推出"},{"from":"n2","to":"n3","kind":"正向"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"claim_checks":[{"node_id":"n1","status":"clearly_true","reason":"claim"},{"node_id":"n2","status":"unverifiable","reason":"claim"}],"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"claim"},{"node_id":"n2","status":"unverifiable","reason":"claim"}],"output_node_ids":["n1","n2"]}`, Model: "fact-claim-model"},
		{Text: `{"challenges":[{"node_id":"n1","assessment":"contested","reason":"challenge"},{"node_id":"n2","assessment":"insufficient_evidence","reason":"challenge"}],"output_node_ids":["n1","n2"]}`, Model: "fact-challenge-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"adjudicated only one node"}],"output_node_ids":["n1"]}`, Model: "fact-judge-model"},
	}}
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
	for _, want := range []string{"coverage", "n2"} {
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

func mustClientTime(t *testing.T, raw string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", raw, err)
	}
	return parsed
}
