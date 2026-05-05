package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runCompileRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	force := fs.Bool("force", false, "force recompilation even if compiled output already exists")
	timeout := fs.Duration("timeout", 20*time.Minute, "compile timeout")
	llmCache := fs.String("llm-cache", string(contentstore.LLMCacheReadThrough), "LLM cache mode: read-through, refresh, off")
	var includes compileIncludeFlag
	fs.Var(&includes, "include", "additional raw capture as platform:id; repeatable")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id> [--include <platform:id>]...")
		return 2
	}
	cacheMode, err := parseLLMCacheMode(*llmCache)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *force && !flagWasSet(fs, "llm-cache") {
		cacheMode = contentstore.LLMCacheRefresh
	}

	app, store, err := openRuntimeStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	client := buildCompileClientCurrent(projectRoot)
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}
	if currentClient, ok := client.(interface {
		EnableLLMCache(varixllm.CacheStore, contentstore.LLMCacheMode)
	}); ok {
		currentClient.EnableLLMCache(store, cacheMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !*force && len(includes) == 0 {
		switch {
		case strings.TrimSpace(*rawURL) != "":
			if parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL); err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
				if record, err := store.GetCompiledOutput(ctx, string(parsed.Platform), parsed.PlatformID); err == nil {
					return writeJSON(stdout, stderr, record)
				}
			}
		case hasContentTarget(*platform, *externalID):
			if record, err := store.GetCompiledOutput(ctx, *platform, *externalID); err == nil {
				return writeJSON(stdout, stderr, record)
			}
		}
	}

	var raw types.RawContent
	switch {
	case strings.TrimSpace(*rawURL) != "":
		parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL)
		if err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
			existing, getErr := store.GetRawCapture(ctx, string(parsed.Platform), parsed.PlatformID)
			if getErr == nil {
				raw = existing
				break
			}
		}
		items, err := fetchURLItems(ctx, app, *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(items) == 0 {
			fmt.Fprintln(stderr, "no items fetched")
			return 1
		}
		raw = items[0]
	default:
		raw, err = store.GetRawCapture(ctx, *platform, *externalID)
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
	}

	var record model.Record
	if len(includes) > 0 {
		included, includeErr := loadCompileIncludes(ctx, store, raw, includes)
		if includeErr != nil {
			writeErr(stderr, includeErr)
			return 1
		}
		record, err = compileBundle(ctx, client, model.BuildMergedBundle(raw, included))
	} else {
		record, err = compileRawCapture(ctx, client, raw)
	}
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if err := store.UpsertCompiledOutput(ctx, record); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, record)
}

type compileIncludeFlag []string

func (f *compileIncludeFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *compileIncludeFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("--include requires platform:id")
	}
	*f = append(*f, value)
	return nil
}

type compileIncludeTarget struct {
	platform   string
	externalID string
}

func loadCompileIncludes(ctx context.Context, store *contentstore.SQLiteStore, primary types.RawContent, values []string) ([]types.RawContent, error) {
	seen := map[string]struct{}{
		compileIncludeKey(primary.Source, primary.ExternalID): {},
	}
	included := make([]types.RawContent, 0, len(values))
	for _, value := range values {
		target, err := parseCompileIncludeTarget(value)
		if err != nil {
			return nil, err
		}
		key := compileIncludeKey(target.platform, target.externalID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		raw, err := store.GetRawCapture(ctx, target.platform, target.externalID)
		if err != nil {
			return nil, fmt.Errorf("load --include %s:%s: %w", target.platform, target.externalID, err)
		}
		included = append(included, raw)
	}
	return included, nil
}

func parseCompileIncludeTarget(value string) (compileIncludeTarget, error) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 {
		return compileIncludeTarget{}, fmt.Errorf("--include must use platform:id, got %q", value)
	}
	target := compileIncludeTarget{
		platform:   strings.TrimSpace(parts[0]),
		externalID: strings.TrimSpace(parts[1]),
	}
	if target.platform == "" || target.externalID == "" {
		return compileIncludeTarget{}, fmt.Errorf("--include must use platform:id, got %q", value)
	}
	return target, nil
}

func compileIncludeKey(platform, externalID string) string {
	return strings.TrimSpace(platform) + "\x00" + strings.TrimSpace(externalID)
}
