package contentstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type LLMCacheMode string

const (
	LLMCacheReadThrough LLMCacheMode = "read-through"
	LLMCacheRefresh     LLMCacheMode = "refresh"
	LLMCacheOff         LLMCacheMode = "off"
)

type LLMCacheEntry struct {
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

func BuildLLMCacheKey(stageName, promptHash, model, inputHash, schemaVersion string, params map[string]string) string {
	parts := []string{
		"stage=" + normalizeCachePart(stageName),
		"prompt=" + normalizeCachePart(promptHash),
		"model=" + normalizeCachePart(model),
		"input=" + normalizeCachePart(inputHash),
		"schema=" + normalizeCachePart(schemaVersion),
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, "param."+normalizeCachePart(key)+"="+normalizeCachePart(params[key]))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func (s *SQLiteStore) GetLLMCacheEntry(ctx context.Context, cacheKey string, mode LLMCacheMode) (LLMCacheEntry, bool, error) {
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return LLMCacheEntry{}, false, fmt.Errorf("cache key is required")
	}
	switch normalizeLLMCacheMode(mode) {
	case LLMCacheOff, LLMCacheRefresh:
		return LLMCacheEntry{}, false, nil
	}
	var out LLMCacheEntry
	err := s.db.QueryRowContext(ctx, `SELECT cache_key, stage_name, prompt_hash, model, input_hash, schema_version, request_json, response_json, token_count, cost_micros, latency_ms, hit_count, created_at, updated_at
		FROM llm_cache_entries WHERE cache_key = ?`, cacheKey).
		Scan(&out.CacheKey, &out.StageName, &out.PromptHash, &out.Model, &out.InputHash, &out.SchemaVersion, &out.RequestJSON, &out.ResponseJSON, &out.TokenCount, &out.CostMicros, &out.LatencyMS, &out.HitCount, &out.CreatedAt, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return LLMCacheEntry{}, false, nil
	}
	if err != nil {
		return LLMCacheEntry{}, false, err
	}
	return out, true, nil
}

func (s *SQLiteStore) UpsertLLMCacheEntry(ctx context.Context, entry LLMCacheEntry, mode LLMCacheMode) error {
	if normalizeLLMCacheMode(mode) == LLMCacheOff {
		return nil
	}
	entry.CacheKey = strings.TrimSpace(entry.CacheKey)
	entry.StageName = strings.TrimSpace(entry.StageName)
	entry.PromptHash = strings.TrimSpace(entry.PromptHash)
	entry.Model = strings.TrimSpace(entry.Model)
	entry.InputHash = strings.TrimSpace(entry.InputHash)
	entry.SchemaVersion = strings.TrimSpace(entry.SchemaVersion)
	if entry.CacheKey == "" || entry.StageName == "" || entry.PromptHash == "" || entry.Model == "" || entry.InputHash == "" || strings.TrimSpace(entry.ResponseJSON) == "" {
		return fmt.Errorf("invalid llm cache entry")
	}
	now := currentSQLiteTimestamp()
	createdAt := strings.TrimSpace(entry.CreatedAt)
	if createdAt == "" {
		createdAt = now
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return fmt.Errorf("created_at must be RFC3339: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO llm_cache_entries(cache_key, stage_name, prompt_hash, model, input_hash, schema_version, request_json, response_json, token_count, cost_micros, latency_ms, hit_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			stage_name = excluded.stage_name,
			prompt_hash = excluded.prompt_hash,
			model = excluded.model,
			input_hash = excluded.input_hash,
			schema_version = excluded.schema_version,
			request_json = excluded.request_json,
			response_json = excluded.response_json,
			token_count = excluded.token_count,
			cost_micros = excluded.cost_micros,
			latency_ms = excluded.latency_ms,
			updated_at = excluded.updated_at`,
		entry.CacheKey,
		entry.StageName,
		entry.PromptHash,
		entry.Model,
		entry.InputHash,
		entry.SchemaVersion,
		strings.TrimSpace(entry.RequestJSON),
		strings.TrimSpace(entry.ResponseJSON),
		entry.TokenCount,
		entry.CostMicros,
		entry.LatencyMS,
		entry.HitCount,
		createdAt,
		now,
	)
	return err
}

func normalizeLLMCacheMode(mode LLMCacheMode) LLMCacheMode {
	switch mode {
	case LLMCacheRefresh, LLMCacheOff:
		return mode
	default:
		return LLMCacheReadThrough
	}
}

func normalizeCachePart(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
