package verify

import (
	"context"
	"testing"

	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/forge/llm"
)

func TestClientEnableLLMCacheWrapsDetailedVerifierRuntime(t *testing.T) {
	store := newMemoryLLMCacheStore()
	rt := &fakeVerifyRuntime{responses: []llm.Response{
		{Text: `{"node_verifications":[{"node_id":"n1","status":"proved","reason":"supported"}]}`, Model: "verify-model"},
		{Text: `{}`, Model: "verify-model"},
		{Text: `{"node_verifications":[{"node_id":"n1","status":"proved","reason":"supported"}]}`, Model: "verify-model"},
	}}
	client := NewClientWithRuntime(rt, "verify-model")
	client.EnableLLMCache(store, varixllm.CacheReadThrough)
	bundle := Bundle{UnitID: "web:verify-cache", Source: "web", ExternalID: "verify-cache", Content: "Driver A"}
	output := Output{Graph: model.ReasoningGraph{Nodes: []GraphNode{{
		ID:   "n1",
		Text: "Driver A",
		Kind: NodeFact,
	}}}}

	if _, err := client.VerifyDetailed(context.Background(), bundle, output); err != nil {
		t.Fatalf("first VerifyDetailed() error = %v", err)
	}
	if _, err := client.VerifyDetailed(context.Background(), bundle, output); err != nil {
		t.Fatalf("second VerifyDetailed() error = %v", err)
	}
	if rt.calls != 3 {
		t.Fatalf("runtime calls = %d, want second detailed verify served from cache", rt.calls)
	}
	if len(store.entries) != 3 {
		t.Fatalf("cache entries = %d, want bull/bear/judge node stages", len(store.entries))
	}
}

type fakeVerifyRuntime struct {
	responses []llm.Response
	calls     int
}

func (f *fakeVerifyRuntime) Call(_ context.Context, _ llm.ProviderRequest) (llm.Response, error) {
	if f.calls >= len(f.responses) {
		return llm.Response{Text: `{}`, Model: "verify-model"}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
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
