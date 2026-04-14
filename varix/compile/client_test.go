package compile

import (
	"context"
	"strings"
	"testing"

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
	raw := `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`
	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}
	if out.Summary != "一句话" {
		t.Fatalf("Summary = %q", out.Summary)
	}
}

func TestClientCompileUsesForgeRuntime(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{{
		Text:  `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`,
		Model: "compile-model",
	}}}
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
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.requests))
	}
	if provider.requests[0].Model != "compile-model" {
		t.Fatalf("request model = %q, want compile-model", provider.requests[0].Model)
	}
}

func TestClientCompileCarriesAttachmentTranscriptIntoForgePrompt(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{{
		Text:  `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`,
		Model: "compile-model",
	}}}
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
	if len(provider.requests) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.requests))
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
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
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
	if len(provider.requests) != 2 {
		t.Fatalf("call count = %d, want 2", len(provider.requests))
	}
	if len(record.Output.Graph.Nodes) != 2 {
		t.Fatalf("nodes = %#v", record.Output.Graph.Nodes)
	}
}

func TestClientCompileRetriesWhenLongformGraphTooSparse(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"事实","text":"事实B"},{"id":"n3","kind":"隐含条件","text":"条件C"},{"id":"n4","kind":"结论","text":"结论D"}],"edges":[{"from":"n1","to":"n3","kind":"正向"},{"from":"n2","to":"n3","kind":"正向"},{"from":"n3","to":"n4","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
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
	if len(provider.requests) != 2 {
		t.Fatalf("call count = %d, want 2", len(provider.requests))
	}
	if len(record.Output.Graph.Nodes) != 4 || len(record.Output.Graph.Edges) != 3 {
		t.Fatalf("graph = %#v", record.Output.Graph)
	}
}

func TestClientCompileRetriesWhenDetailsEmpty(t *testing.T) {
	provider := &compileMockProvider{responses: []llm.ProviderResponse{
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
		{Text: `{"summary":"一句话","graph":{"nodes":[{"id":"n1","kind":"事实","text":"事实A"},{"id":"n2","kind":"结论","text":"结论B"}],"edges":[{"from":"n1","to":"n2","kind":"推出"}]},"details":{"caveats":["待确认"]},"topics":["topic"],"confidence":"medium"}`, Model: "compile-model"},
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
	if len(provider.requests) != 2 {
		t.Fatalf("call count = %d, want 2", len(provider.requests))
	}
	if len(record.Output.Details.Caveats) != 1 {
		t.Fatalf("details = %#v", record.Output.Details)
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
