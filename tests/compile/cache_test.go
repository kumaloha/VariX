package compile

import (
	"context"
	"testing"

	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/forge/llm"
)

func TestCachedRuntimeCachesStageJSONResponses(t *testing.T) {
	store := newMemoryLLMCacheStore()
	rt := &fakeRuntime{responses: []llm.Response{{
		Text:   `{"summary":"cached result"}`,
		Model:  "compile-model",
		Tokens: llm.TokenUsage{TotalTokens: 17},
	}}}
	cached := newCachedRuntime(rt, store, varixllm.CacheReadThrough)
	bundle := Bundle{UnitID: "web:cache", Source: "web", ExternalID: "cache", Content: "same content"}

	var first struct {
		Summary string `json:"summary"`
	}
	if err := stageJSONCall(context.Background(), cached, "compile-model", bundle, "system", "user", "summary", &first); err != nil {
		t.Fatalf("first stageJSONCall() error = %v", err)
	}
	var second struct {
		Summary string `json:"summary"`
	}
	if err := stageJSONCall(context.Background(), cached, "compile-model", bundle, "system", "user", "summary", &second); err != nil {
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
	store := newMemoryLLMCacheStore()
	rt := &fakeRuntime{responses: []llm.Response{{
		Text: `{"article_form":"single_thesis","nodes":[{"id":"n1","text":"Driver A","source_quote":"Driver A","role":"thesis"}],"off_graph":[]}`,
	}}}
	client := &Client{runtime: rt, model: "compile-model"}
	client.EnableLLMCache(store, varixllm.CacheReadThrough)

	if _, err := stage1Extract(context.Background(), client.runtime, "compile-model", Bundle{UnitID: "web:cache-client", Source: "web", ExternalID: "cache-client", Content: "Driver A"}); err != nil {
		t.Fatalf("first stage1Extract() error = %v", err)
	}
	if _, err := stage1Extract(context.Background(), client.runtime, "compile-model", Bundle{UnitID: "web:cache-client", Source: "web", ExternalID: "cache-client", Content: "Driver A"}); err != nil {
		t.Fatalf("second stage1Extract() error = %v", err)
	}
	if rt.calls != 1 {
		t.Fatalf("runtime calls = %d, want second extract served from client cache", rt.calls)
	}
}

type memoryLLMCacheStore struct {
	entries map[string]varixllm.CacheEntry
}

func newMemoryLLMCacheStore() *memoryLLMCacheStore {
	return &memoryLLMCacheStore{entries: map[string]varixllm.CacheEntry{}}
}

func (s *memoryLLMCacheStore) GetLLMCacheEntry(_ context.Context, cacheKey string, mode varixllm.CacheMode) (varixllm.CacheEntry, bool, error) {
	if mode == varixllm.CacheOff || mode == varixllm.CacheRefresh {
		return varixllm.CacheEntry{}, false, nil
	}
	entry, ok := s.entries[cacheKey]
	return entry, ok, nil
}

func (s *memoryLLMCacheStore) UpsertLLMCacheEntry(_ context.Context, entry varixllm.CacheEntry, mode varixllm.CacheMode) error {
	if mode == varixllm.CacheOff {
		return nil
	}
	s.entries[entry.CacheKey] = entry
	return nil
}
