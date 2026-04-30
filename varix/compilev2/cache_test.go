package compilev2

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"github.com/kumaloha/forge/llm"
)

func TestCachedRuntimeCachesStageJSONResponses(t *testing.T) {
	store, err := contentstore.NewSQLiteStore(filepath.Join(t.TempDir(), "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	rt := &fakeRuntime{responses: []llm.Response{{
		Text:   `{"summary":"cached result"}`,
		Model:  "compilev2-model",
		Tokens: llm.TokenUsage{TotalTokens: 17},
	}}}
	cached := newCachedRuntime(rt, store, contentstore.LLMCacheReadThrough)
	bundle := compile.Bundle{UnitID: "web:cache", Source: "web", ExternalID: "cache", Content: "same content"}

	var first struct {
		Summary string `json:"summary"`
	}
	if err := stageJSONCall(context.Background(), cached, "compilev2-model", bundle, "system", "user", "summary", &first); err != nil {
		t.Fatalf("first stageJSONCall() error = %v", err)
	}
	var second struct {
		Summary string `json:"summary"`
	}
	if err := stageJSONCall(context.Background(), cached, "compilev2-model", bundle, "system", "user", "summary", &second); err != nil {
		t.Fatalf("second stageJSONCall() error = %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("runtime calls = %d, want cache hit on second call", rt.calls)
	}
	if first.Summary != "cached result" || second.Summary != "cached result" {
		t.Fatalf("summaries = %q/%q, want cached result", first.Summary, second.Summary)
	}
}

func TestClientEnableLLMCacheWrapsCompileRuntime(t *testing.T) {
	store, err := contentstore.NewSQLiteStore(filepath.Join(t.TempDir(), "data", "content.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"article_form":"single_thesis","nodes":[{"id":"n1","text":"Driver A","source_quote":"Driver A","role":"thesis"}],"off_graph":[]}`,
	}}}
	client := &Client{runtime: rt, model: "compilev2-model"}
	client.EnableLLMCache(store, contentstore.LLMCacheReadThrough)

	if _, err := stage1Extract(context.Background(), client.runtime, "compilev2-model", compile.Bundle{UnitID: "web:cache-client", Source: "web", ExternalID: "cache-client", Content: "Driver A"}); err != nil {
		t.Fatalf("first stage1Extract() error = %v", err)
	}
	if _, err := stage1Extract(context.Background(), client.runtime, "compilev2-model", compile.Bundle{UnitID: "web:cache-client", Source: "web", ExternalID: "cache-client", Content: "Driver A"}); err != nil {
		t.Fatalf("second stage1Extract() error = %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("runtime calls = %d, want second extract served from client cache", rt.calls)
	}
}
