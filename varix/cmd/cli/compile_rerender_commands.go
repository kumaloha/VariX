package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runCompileRerender(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile rerender", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceRunID := fs.Int64("source-run-id", 0, "compile preview run id to render from")
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	write := fs.Bool("write", false, "write rerendered output to compiled_outputs")
	timeout := fs.Duration("timeout", 5*time.Minute, "rerender timeout")
	llmCache := fs.String("llm-cache", string(contentstore.LLMCacheReadThrough), "LLM cache mode: read-through, refresh, off")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)
	if *sourceRunID <= 0 || (strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID)) {
		fmt.Fprintln(stderr, "usage: varix compile rerender --source-run-id <run_id> (--url <url> | --platform <platform> --id <external_id>) [--write]")
		return 2
	}
	cacheMode, err := parseLLMCacheMode(*llmCache)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	app, store, err := openRuntimeStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	*platform, *externalID, err = resolveContentTarget(ctx, app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	raw, err := store.GetRawCapture(ctx, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	item, err := compilePreviewItemForTarget(ctx, store, *sourceRunID, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	var preview c.FlowPreviewResult
	if err := json.Unmarshal([]byte(item.PayloadJSON), &preview); err != nil {
		writeErr(stderr, fmt.Errorf("preview payload parse: %w", err))
		return 1
	}

	renderer := buildCompilePreviewRenderer(projectRoot)
	if renderer == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}
	if currentClient, ok := renderer.(interface {
		EnableLLMCache(varixllm.CacheStore, contentstore.LLMCacheMode)
	}); ok {
		currentClient.EnableLLMCache(store, cacheMode)
	}
	bundle := model.BuildBundle(raw)
	start := time.Now()
	preview, err = renderer.RenderPreview(ctx, bundle, preview)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	record := recordFromRerenderedPreview(bundle, preview, time.Since(start))
	if *write {
		if err := store.UpsertCompiledOutput(ctx, record); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	return writeJSON(stdout, stderr, record)
}

func compilePreviewItemForTarget(ctx context.Context, store *contentstore.SQLiteStore, runID int64, platform, externalID string) (contentstore.CompilePreviewRunItem, error) {
	items, err := store.ListCompilePreviewRunItems(ctx, runID)
	if err != nil {
		return contentstore.CompilePreviewRunItem{}, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.Platform) == strings.TrimSpace(platform) && strings.TrimSpace(item.ExternalID) == strings.TrimSpace(externalID) {
			if strings.TrimSpace(item.PayloadJSON) == "" {
				return contentstore.CompilePreviewRunItem{}, fmt.Errorf("compile preview item has empty payload: %s:%s", platform, externalID)
			}
			return item, nil
		}
	}
	return contentstore.CompilePreviewRunItem{}, fmt.Errorf("compile preview item not found in run %d: %s:%s", runID, platform, externalID)
}

func recordFromRerenderedPreview(bundle model.Bundle, preview c.FlowPreviewResult, elapsed time.Duration) model.Record {
	elapsedMS := elapsed.Milliseconds()
	if elapsedMS <= 0 {
		elapsedMS = 1
	}
	return model.Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          "rerender",
		Output:         preview.Render,
		Metrics:        model.RecordMetrics{CompileElapsedMS: elapsedMS, CompileStageElapsedMS: compileStageMetricsFromPreview(preview.Metrics)},
		CompiledAt:     currentUTC(),
	}
}

func compileStageMetricsFromPreview(metrics map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(metrics))
	for key, value := range metrics {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		key = strings.TrimSuffix(key, "_ms")
		out[key] = value
	}
	return out
}
