package compilev2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/storage/contentstore"
	"github.com/kumaloha/forge/llm"
)

type LLMCacheStore interface {
	GetLLMCacheEntry(ctx context.Context, cacheKey string, mode contentstore.LLMCacheMode) (contentstore.LLMCacheEntry, bool, error)
	UpsertLLMCacheEntry(ctx context.Context, entry contentstore.LLMCacheEntry, mode contentstore.LLMCacheMode) error
}

type stageRuntime interface {
	CallStage(ctx context.Context, stageName string, req llm.ProviderRequest) (llm.Response, error)
}

type cachedRuntime struct {
	next  runtimeChat
	store LLMCacheStore
	mode  contentstore.LLMCacheMode
}

func newCachedRuntime(next runtimeChat, store LLMCacheStore, mode contentstore.LLMCacheMode) runtimeChat {
	if next == nil || store == nil || mode == contentstore.LLMCacheOff {
		return next
	}
	return &cachedRuntime{next: next, store: store, mode: mode}
}

func (r *cachedRuntime) Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error) {
	return r.CallStage(ctx, "unknown", req)
}

func (r *cachedRuntime) CallStage(ctx context.Context, stageName string, req llm.ProviderRequest) (llm.Response, error) {
	cacheKey, promptHash, inputHash := buildStageCacheKey(stageName, req)
	if entry, ok, err := r.store.GetLLMCacheEntry(ctx, cacheKey, r.mode); err != nil {
		return llm.Response{}, err
	} else if ok {
		return llm.Response{Text: entry.ResponseJSON, Model: strings.TrimSpace(entry.Model), Tokens: llm.TokenUsage{TotalTokens: entry.TokenCount}}, nil
	}
	start := time.Now()
	resp, err := r.next.Call(ctx, req)
	if err != nil {
		return resp, err
	}
	requestJSON := marshalProviderRequestForCache(req)
	if err := r.store.UpsertLLMCacheEntry(ctx, contentstore.LLMCacheEntry{
		CacheKey:      cacheKey,
		StageName:     strings.TrimSpace(stageName),
		PromptHash:    promptHash,
		Model:         strings.TrimSpace(req.Model),
		InputHash:     inputHash,
		SchemaVersion: stageJSONSchemaVersion(stageName),
		RequestJSON:   requestJSON,
		ResponseJSON:  strings.TrimSpace(resp.Text),
		TokenCount:    resp.Tokens.TotalTokens,
		LatencyMS:     time.Since(start).Milliseconds(),
	}, r.mode); err != nil {
		return llm.Response{}, err
	}
	return resp, nil
}

func callStageRuntime(ctx context.Context, rt runtimeChat, stageName string, req llm.ProviderRequest) (llm.Response, error) {
	if stageRT, ok := rt.(stageRuntime); ok {
		return stageRT.CallStage(ctx, stageName, req)
	}
	return rt.Call(ctx, req)
}

func buildStageCacheKey(stageName string, req llm.ProviderRequest) (cacheKey, promptHash, inputHash string) {
	promptHash = hashText(req.System)
	inputHash = hashText(requestUserText(req))
	cacheKey = contentstore.BuildLLMCacheKey(stageName, promptHash, req.Model, inputHash, stageJSONSchemaVersion(stageName), map[string]string{
		"search":      boolString(req.Search),
		"thinking":    boolString(req.Thinking),
		"temperature": floatString(req.Temperature),
	})
	return cacheKey, promptHash, inputHash
}

func requestUserText(req llm.ProviderRequest) string {
	if strings.TrimSpace(req.User) != "" {
		return strings.TrimSpace(req.User)
	}
	parts := make([]string, 0, len(req.UserParts))
	for _, part := range req.UserParts {
		parts = append(parts, part.Type, part.Text, part.ImageURL)
	}
	return strings.Join(parts, "\n")
}

func marshalProviderRequestForCache(req llm.ProviderRequest) string {
	payload, err := json.Marshal(struct {
		Model       string            `json:"model"`
		System      string            `json:"system"`
		User        string            `json:"user,omitempty"`
		UserParts   []llm.ContentPart `json:"user_parts,omitempty"`
		Temperature float64           `json:"temperature"`
		Search      bool              `json:"search"`
		Thinking    bool              `json:"thinking"`
		Schema      *llm.Schema       `json:"schema,omitempty"`
	}{
		Model:       req.Model,
		System:      req.System,
		User:        req.User,
		UserParts:   req.UserParts,
		Temperature: req.Temperature,
		Search:      req.Search,
		Thinking:    req.Thinking,
		Schema:      req.JSONSchema,
	})
	if err != nil {
		return ""
	}
	return string(payload)
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func stageJSONSchemaVersion(stageName string) string {
	return "compilev2:" + strings.TrimSpace(stageName) + ":json-schema:v1"
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func floatString(value float64) string {
	out := strings.TrimRight(strings.TrimRight(jsonNumber(value), "0"), ".")
	if out == "" || out == "-" {
		return "0"
	}
	return out
}

func jsonNumber(value float64) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return "0"
	}
	return string(payload)
}
