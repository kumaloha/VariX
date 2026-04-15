package compile

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/forge/llm"
)

type compileMockProvider struct {
	requests  []llm.ProviderRequest
	responses []llm.ProviderResponse
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
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "verifier-model"},
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
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.requests))
	}
	if provider.requests[0].Model != "compile-model" {
		t.Fatalf("request model = %q, want compile-model", provider.requests[0].Model)
	}
	if record.Output.Verification.Model == "" || len(record.Output.Verification.FactChecks) != 1 {
		t.Fatalf("verification = %#v", record.Output.Verification)
	}
}

func TestClientCompileCarriesAttachmentTranscriptIntoForgePrompt(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "verifier-model"},
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
	if len(provider.requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.requests))
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
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "verifier-model"},
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
	if len(provider.requests) != 3 {
		t.Fatalf("call count = %d, want 3", len(provider.requests))
	}
	retryPrompt := provider.requests[1].UserParts[len(provider.requests[1].UserParts)-1].Text
	if !containsAll(retryPrompt, "显式条件 + 预测", "不能整句都标成显式条件") {
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
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"},{"node_id":"n2","status":"unverifiable","reason":"unclear"}]}`, Model: "verifier-model"},
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
	if len(provider.requests) != 4 {
		t.Fatalf("call count = %d, want 4", len(provider.requests))
	}
	if len(record.Output.Graph.Nodes) != 4 || len(record.Output.Graph.Edges) != 3 {
		t.Fatalf("graph = %#v", record.Output.Graph)
	}
}

func TestClientCompileRetriesWhenDetailsEmpty(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"},{"id":"n2","kind":"结论","text":"结论B","valid_from":"2026-04-14T00:00:00Z","valid_to":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "verifier-model"},
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
	if len(provider.requests) != 3 {
		t.Fatalf("call count = %d, want 3", len(provider.requests))
	}
	if len(record.Output.Details.Caveats) != 1 {
		t.Fatalf("details = %#v", record.Output.Details)
	}
}

func TestClientCompileRunsFactAndPredictionVerifierPasses(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A","occurred_at":"2026-04-14T00:00:00Z"},{"id":"n2","kind":"预测","text":"预测B","prediction_start_at":"2026-04-14T00:00:00Z","prediction_due_at":"2026-07-14T00:00:00Z"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-verifier-model"},
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
	if len(provider.requests) != 3 {
		t.Fatalf("provider calls = %d, want 3", len(provider.requests))
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
	if !containsAll(factPrompt, `"kind": "事实"`, `"occurred_at": "2026-04-14T00:00:00Z"`, `"as_of": "`) {
		t.Fatalf("fact verifier prompt missing occurred_at evidence: %q", factPrompt)
	}
	if strings.Contains(factPrompt, `"valid_from"`) {
		t.Fatalf("fact verifier prompt should prefer occurred_at over legacy valid_from: %q", factPrompt)
	}
	predictionPrompt := provider.requests[2].UserParts[len(provider.requests[2].UserParts)-1].Text
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
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-verifier-model"},
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
	if len(provider.requests) != 5 {
		t.Fatalf("provider calls = %d, want 5", len(provider.requests))
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
	explicitPrompt := provider.requests[2].UserParts[len(provider.requests[2].UserParts)-1].Text
	if !strings.Contains(explicitPrompt, `"as_of": "`) {
		t.Fatalf("explicit condition verifier prompt missing as_of context: %q", explicitPrompt)
	}
	if len(record.Output.Verification.ImplicitConditionChecks) != 1 {
		t.Fatalf("len(ImplicitConditionChecks) = %d, want 1", len(record.Output.Verification.ImplicitConditionChecks))
	}
	implicitPrompt := provider.requests[3].UserParts[len(provider.requests[3].UserParts)-1].Text
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
		{Text: `{"fact_checks":[{"node_id":"n1","status":"clearly_true","reason":"supported"}]}`, Model: "fact-verifier-model"},
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
