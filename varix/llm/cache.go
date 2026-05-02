package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

type CacheMode string

const (
	CacheReadThrough CacheMode = "read-through"
	CacheRefresh     CacheMode = "refresh"
	CacheOff         CacheMode = "off"
)

type CacheEntry struct {
	CacheKey      string `json:"cache_key"`
	StageName     string `json:"stage_name"`
	PromptHash    string `json:"prompt_hash"`
	Model         string `json:"model"`
	InputHash     string `json:"input_hash"`
	SchemaVersion string `json:"schema_version,omitempty"`
	RequestJSON   string `json:"request_json,omitempty"`
	ResponseJSON  string `json:"response_json"`
	TokenCount    int    `json:"token_count,omitempty"`
	CostMicros    int64  `json:"cost_micros,omitempty"`
	LatencyMS     int64  `json:"latency_ms,omitempty"`
	HitCount      int    `json:"hit_count,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

func BuildCacheKey(stageName, promptHash, model, inputHash, schemaVersion string, params map[string]string) string {
	parts := []string{
		"stage=" + normalizeCacheKeyPart(stageName),
		"prompt=" + normalizeCacheKeyPart(promptHash),
		"model=" + normalizeCacheKeyPart(model),
		"input=" + normalizeCacheKeyPart(inputHash),
		"schema=" + normalizeCacheKeyPart(schemaVersion),
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, "param."+normalizeCacheKeyPart(key)+"="+normalizeCacheKeyPart(params[key]))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func normalizeCacheKeyPart(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
