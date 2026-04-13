package compile

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

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

func TestClientCompileParsesChatCompletionResponse(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", req.URL.Path)
		}
		body := `{"choices":[{"message":{"content":"{\"summary\":\"一句话\",\"graph\":{\"nodes\":[{\"id\":\"n1\",\"kind\":\"事实\",\"text\":\"事实A\"},{\"id\":\"n2\",\"kind\":\"结论\",\"text\":\"结论B\"}],\"edges\":[{\"from\":\"n1\",\"to\":\"n2\",\"kind\":\"推出\"}]},\"details\":{\"caveats\":[\"待确认\"]},\"topics\":[\"topic\"],\"confidence\":\"medium\"}"}}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}, "https://dashscope.example/v1", "test-key", Qwen36PlusModel)

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
}

func TestClientCompileRetriesWhenFirstResponseHasEmptyGraph(t *testing.T) {
	call := 0
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		call++
		var body string
		if call == 1 {
			body = `{"choices":[{"message":{"content":"{\"summary\":\"一句话\",\"graph\":{},\"details\":{},\"topics\":[],\"confidence\":\"medium\"}"}}]}`
		} else {
			body = `{"choices":[{"message":{"content":"{\"summary\":\"一句话\",\"graph\":{\"nodes\":[{\"id\":\"n1\",\"kind\":\"事实\",\"text\":\"事实A\"},{\"id\":\"n2\",\"kind\":\"结论\",\"text\":\"结论B\"}],\"edges\":[{\"from\":\"n1\",\"to\":\"n2\",\"kind\":\"推出\"}]},\"details\":{\"caveats\":[\"待确认\"]},\"topics\":[\"topic\"],\"confidence\":\"medium\"}"}}]}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}, "https://dashscope.example/v1", "test-key", Qwen36PlusModel)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    "root body",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if call != 2 {
		t.Fatalf("call count = %d, want 2", call)
	}
	if len(record.Output.Graph.Nodes) != 2 {
		t.Fatalf("nodes = %#v", record.Output.Graph.Nodes)
	}
}

func TestClientCompileRetriesWhenLongformGraphTooSparse(t *testing.T) {
	call := 0
	client := NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		call++
		var body string
		if call == 1 {
			body = `{"choices":[{"message":{"content":"{\"summary\":\"一句话\",\"graph\":{\"nodes\":[{\"id\":\"n1\",\"kind\":\"事实\",\"text\":\"事实A\"},{\"id\":\"n2\",\"kind\":\"结论\",\"text\":\"结论B\"}],\"edges\":[{\"from\":\"n1\",\"to\":\"n2\",\"kind\":\"推出\"}]},\"details\":{\"caveats\":[\"待确认\"]},\"topics\":[\"topic\"],\"confidence\":\"medium\"}"}}]}`
		} else {
			body = `{"choices":[{"message":{"content":"{\"summary\":\"一句话\",\"graph\":{\"nodes\":[{\"id\":\"n1\",\"kind\":\"事实\",\"text\":\"事实A\"},{\"id\":\"n2\",\"kind\":\"事实\",\"text\":\"事实B\"},{\"id\":\"n3\",\"kind\":\"隐含条件\",\"text\":\"条件C\"},{\"id\":\"n4\",\"kind\":\"结论\",\"text\":\"结论D\"}],\"edges\":[{\"from\":\"n1\",\"to\":\"n3\",\"kind\":\"正向\"},{\"from\":\"n2\",\"to\":\"n3\",\"kind\":\"正向\"},{\"from\":\"n3\",\"to\":\"n4\",\"kind\":\"推出\"}]},\"details\":{\"caveats\":[\"待确认\"]},\"topics\":[\"topic\"],\"confidence\":\"medium\"}"}}]}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}, "https://dashscope.example/v1", "test-key", Qwen36PlusModel)

	record, err := client.Compile(context.Background(), Bundle{
		UnitID:     "twitter:123",
		Source:     "twitter",
		ExternalID: "123",
		Content:    strings.Repeat("长文", 2000),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if call != 2 {
		t.Fatalf("call count = %d, want 2", call)
	}
	if len(record.Output.Graph.Nodes) != 4 || len(record.Output.Graph.Edges) != 3 {
		t.Fatalf("graph = %#v", record.Output.Graph)
	}
}
