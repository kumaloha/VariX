package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	forgellm "github.com/kumaloha/forge/llm"
)

type Runtime interface {
	Call(ctx context.Context, req forgellm.ProviderRequest) (forgellm.Response, error)
}

type StageRuntime interface {
	CallStage(ctx context.Context, stageName string, req forgellm.ProviderRequest) (forgellm.Response, error)
}

type CacheStore interface {
	GetLLMCacheEntry(ctx context.Context, cacheKey string, mode CacheMode) (CacheEntry, bool, error)
	UpsertLLMCacheEntry(ctx context.Context, entry CacheEntry, mode CacheMode) error
}

type CachedRuntime struct {
	next      Runtime
	store     CacheStore
	mode      CacheMode
	namespace string
}

func NewCachedRuntime(next Runtime, store CacheStore, mode CacheMode, namespace string) Runtime {
	if next == nil || store == nil || mode == CacheOff {
		return next
	}
	return &CachedRuntime{
		next:      next,
		store:     store,
		mode:      mode,
		namespace: strings.TrimSpace(namespace),
	}
}

func (r *CachedRuntime) Call(ctx context.Context, req forgellm.ProviderRequest) (forgellm.Response, error) {
	return r.CallStage(ctx, "unknown", req)
}

func (r *CachedRuntime) CallStage(ctx context.Context, stageName string, req forgellm.ProviderRequest) (forgellm.Response, error) {
	cacheKey, promptHash, inputHash := r.buildStageCacheKey(stageName, req)
	if entry, ok, err := r.store.GetLLMCacheEntry(ctx, cacheKey, r.mode); err != nil {
		return forgellm.Response{}, err
	} else if ok {
		return forgellm.Response{
			Text:   entry.ResponseJSON,
			Model:  strings.TrimSpace(entry.Model),
			Tokens: forgellm.TokenUsage{TotalTokens: entry.TokenCount},
		}, nil
	}
	start := time.Now()
	resp, err := r.next.Call(ctx, req)
	if err != nil {
		return resp, err
	}
	if err := r.store.UpsertLLMCacheEntry(ctx, CacheEntry{
		CacheKey:      cacheKey,
		StageName:     strings.TrimSpace(stageName),
		PromptHash:    promptHash,
		Model:         strings.TrimSpace(req.Model),
		InputHash:     inputHash,
		SchemaVersion: r.stageSchemaVersion(stageName),
		RequestJSON:   marshalProviderRequestForCache(req),
		ResponseJSON:  strings.TrimSpace(resp.Text),
		TokenCount:    resp.Tokens.TotalTokens,
		LatencyMS:     time.Since(start).Milliseconds(),
	}, r.mode); err != nil {
		return forgellm.Response{}, err
	}
	return resp, nil
}

func CallStage(ctx context.Context, rt Runtime, stageName string, req forgellm.ProviderRequest) (forgellm.Response, error) {
	if stageRT, ok := rt.(StageRuntime); ok {
		return stageRT.CallStage(ctx, stageName, req)
	}
	return rt.Call(ctx, req)
}

func (r *CachedRuntime) buildStageCacheKey(stageName string, req forgellm.ProviderRequest) (cacheKey, promptHash, inputHash string) {
	promptHash = hashText(req.System)
	inputHash = hashText(requestUserText(req))
	cacheKey = BuildCacheKey(stageName, promptHash, req.Model, inputHash, r.stageSchemaVersion(stageName), map[string]string{
		"search":      boolString(req.Search),
		"thinking":    boolString(req.Thinking),
		"temperature": floatString(req.Temperature),
	})
	return cacheKey, promptHash, inputHash
}

func requestUserText(req forgellm.ProviderRequest) string {
	if strings.TrimSpace(req.User) != "" {
		return strings.TrimSpace(req.User)
	}
	parts := make([]string, 0, len(req.UserParts))
	for _, part := range req.UserParts {
		parts = append(parts, part.Type, part.Text, part.ImageURL)
	}
	return strings.Join(parts, "\n")
}

func marshalProviderRequestForCache(req forgellm.ProviderRequest) string {
	payload, err := json.Marshal(struct {
		Model       string                 `json:"model"`
		System      string                 `json:"system"`
		User        string                 `json:"user,omitempty"`
		UserParts   []forgellm.ContentPart `json:"user_parts,omitempty"`
		Temperature float64                `json:"temperature"`
		Search      bool                   `json:"search"`
		Thinking    bool                   `json:"thinking"`
		Schema      *forgellm.Schema       `json:"schema,omitempty"`
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

func (r *CachedRuntime) stageSchemaVersion(stageName string) string {
	namespace := strings.TrimSpace(r.namespace)
	if namespace == "" {
		namespace = "llm"
	}
	return namespace + ":" + strings.TrimSpace(stageName) + ":json-schema:current"
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
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
