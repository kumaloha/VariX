package contentstore

import (
	"context"
	"testing"
)

func TestSQLiteStore_LLMCacheReadThroughHitMissAndRefresh(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	key := BuildLLMCacheKey("extract", "prompt-a", "model-a", "input-a", "v1", map[string]string{"temperature": "0"})

	if _, ok, err := store.GetLLMCacheEntry(ctx, key, LLMCacheReadThrough); err != nil || ok {
		t.Fatalf("initial GetLLMCacheEntry() ok=%v err=%v, want miss", ok, err)
	}
	if err := store.UpsertLLMCacheEntry(ctx, LLMCacheEntry{
		CacheKey:      key,
		StageName:     "extract",
		PromptHash:    "prompt-a",
		Model:         "model-a",
		InputHash:     "input-a",
		SchemaVersion: "v1",
		RequestJSON:   `{"input":"a"}`,
		ResponseJSON:  `{"output":"first"}`,
		TokenCount:    10,
		LatencyMS:     123,
	}, LLMCacheReadThrough); err != nil {
		t.Fatalf("UpsertLLMCacheEntry() error = %v", err)
	}
	got, ok, err := store.GetLLMCacheEntry(ctx, key, LLMCacheReadThrough)
	if err != nil || !ok {
		t.Fatalf("cached GetLLMCacheEntry() ok=%v err=%v, want hit", ok, err)
	}
	if got.ResponseJSON != `{"output":"first"}` || got.HitCount != 0 {
		t.Fatalf("cache hit = %#v, want first response without synchronous hit_count write", got)
	}
	if _, ok, err := store.GetLLMCacheEntry(ctx, key, LLMCacheRefresh); err != nil || ok {
		t.Fatalf("refresh GetLLMCacheEntry() ok=%v err=%v, want forced miss", ok, err)
	}
	if err := store.UpsertLLMCacheEntry(ctx, LLMCacheEntry{
		CacheKey:      key,
		StageName:     "extract",
		PromptHash:    "prompt-a",
		Model:         "model-a",
		InputHash:     "input-a",
		SchemaVersion: "v1",
		ResponseJSON:  `{"output":"second"}`,
	}, LLMCacheRefresh); err != nil {
		t.Fatalf("refresh UpsertLLMCacheEntry() error = %v", err)
	}
	got, ok, err = store.GetLLMCacheEntry(ctx, key, LLMCacheReadThrough)
	if err != nil || !ok {
		t.Fatalf("post-refresh GetLLMCacheEntry() ok=%v err=%v, want hit", ok, err)
	}
	if got.ResponseJSON != `{"output":"second"}` {
		t.Fatalf("ResponseJSON = %s, want refreshed response", got.ResponseJSON)
	}
}

func TestSQLiteStore_LLMCacheOffDoesNotReadOrWrite(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	ctx := context.Background()
	key := BuildLLMCacheKey("render", "prompt", "model", "input", "v1", nil)

	if err := store.UpsertLLMCacheEntry(ctx, LLMCacheEntry{
		CacheKey:     key,
		StageName:    "render",
		PromptHash:   "prompt",
		Model:        "model",
		InputHash:    "input",
		ResponseJSON: `{"output":"ignored"}`,
	}, LLMCacheOff); err != nil {
		t.Fatalf("UpsertLLMCacheEntry(off) error = %v", err)
	}
	if _, ok, err := store.GetLLMCacheEntry(ctx, key, LLMCacheReadThrough); err != nil || ok {
		t.Fatalf("GetLLMCacheEntry() after off write ok=%v err=%v, want miss", ok, err)
	}
}

func TestSQLiteStore_LLMCacheRejectsInvalidEntry(t *testing.T) {
	store := newSubjectTimelineTestStore(t)
	if err := store.UpsertLLMCacheEntry(context.Background(), LLMCacheEntry{CacheKey: "missing-fields"}, LLMCacheReadThrough); err == nil {
		t.Fatal("UpsertLLMCacheEntry(invalid) error = nil, want error")
	}
}
