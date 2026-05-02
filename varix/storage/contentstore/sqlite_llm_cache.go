package contentstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	varixllm "github.com/kumaloha/VariX/varix/llm"
)

type LLMCacheMode = varixllm.CacheMode

const (
	LLMCacheReadThrough LLMCacheMode = varixllm.CacheReadThrough
	LLMCacheRefresh     LLMCacheMode = varixllm.CacheRefresh
	LLMCacheOff         LLMCacheMode = varixllm.CacheOff
)

type LLMCacheEntry = varixllm.CacheEntry
type LLMCacheStore = varixllm.CacheStore

func BuildLLMCacheKey(stageName, promptHash, model, inputHash, schemaVersion string, params map[string]string) string {
	return varixllm.BuildCacheKey(stageName, promptHash, model, inputHash, schemaVersion, params)
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
