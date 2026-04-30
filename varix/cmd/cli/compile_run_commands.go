package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runCompileRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	force := fs.Bool("force", false, "force recompilation even if compiled output already exists")
	noVerify := fs.Bool("no-verify", false, "skip compile-time verification and retrieval")
	pipeline := fs.String("pipeline", "legacy", "compile pipeline: legacy | v2")
	timeout := fs.Duration("timeout", 20*time.Minute, "compile timeout")
	llmCache := fs.String("llm-cache", string(contentstore.LLMCacheReadThrough), "LLM cache mode for v2: read-through, refresh, off")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	cacheMode, err := parseCompileLLMCacheMode(*llmCache)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *force && !flagWasSet(fs, "llm-cache") {
		cacheMode = contentstore.LLMCacheRefresh
	}

	app, store, err := openAppStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if !*noVerify {
		c.EnableFactWebVerification()
	}
	client, err := selectCompileClient(projectRoot, *pipeline, *noVerify)
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}
	if v2Client, ok := client.(*cv2.Client); ok {
		v2Client.EnableLLMCache(store, cacheMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !*force {
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

	bundle := c.BuildBundle(raw)
	compileStart := time.Now()
	record, err := client.Compile(ctx, bundle)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if record.Metrics.CompileElapsedMS <= 0 {
		record.Metrics.CompileElapsedMS = time.Since(compileStart).Milliseconds()
		if record.Metrics.CompileElapsedMS <= 0 {
			record.Metrics.CompileElapsedMS = 1
		}
	}
	if err := store.UpsertCompiledOutput(ctx, record); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, record)
}
