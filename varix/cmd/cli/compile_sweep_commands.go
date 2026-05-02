package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	varixllm "github.com/kumaloha/VariX/varix/llm"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type compileSweepSummary struct {
	Scanned                 int      `json:"scanned"`
	Compiled                int64    `json:"compiled"`
	Skipped                 int64    `json:"skipped"`
	Failed                  int64    `json:"failed"`
	ContentGraphsBackfilled int64    `json:"content_graphs_backfilled,omitempty"`
	Platform                string   `json:"platform,omitempty"`
	User                    string   `json:"user,omitempty"`
	Force                   bool     `json:"force"`
	WorkerCount             int      `json:"worker_count"`
	FailedSamples           []string `json:"failed_samples,omitempty"`
	Status                  string   `json:"status"`
}

func runCompileSweep(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile sweep", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "max raw captures to compile; 0 means all")
	workers := fs.Int("workers", 1, "parallel compile workers")
	platform := fs.String("platform", "", "optional source platform filter")
	userID := fs.String("user", "", "optional user id for memory content graph backfill")
	force := fs.Bool("force", false, "recompile matching raw captures even when compiled output exists")
	itemTimeout := fs.Duration("item-timeout", 20*time.Minute, "per-item compile timeout")
	llmCache := fs.String("llm-cache", string(contentstore.LLMCacheReadThrough), "LLM cache mode: read-through, refresh, off")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *workers <= 0 || *limit < 0 || *itemTimeout <= 0 {
		fmt.Fprintln(stderr, "usage: varix compile sweep [--limit <n>] [--workers <n>] [--platform <platform>] [--user <user_id>] [--force]")
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

	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()

	ctx := context.Background()
	var refs []contentstore.RawCaptureRef
	if *force {
		refs, err = store.ListRawCaptureRefs(ctx, *limit, strings.TrimSpace(*platform))
	} else {
		refs, err = store.ListUncompiledRawCaptureRefs(ctx, *limit, strings.TrimSpace(*platform))
	}
	if err != nil {
		writeErr(stderr, err)
		return 1
	}

	var client compileClient
	if len(refs) > 0 {
		client = buildCompileClientCurrent(projectRoot)
		if client == nil {
			fmt.Fprintln(stderr, "compile client config missing")
			return 1
		}
		if currentClient, ok := client.(interface {
			EnableLLMCache(varixllm.CacheStore, contentstore.LLMCacheMode)
		}); ok {
			currentClient.EnableLLMCache(store, cacheMode)
		}
	}

	summary := sweepCompileRefs(ctx, store, client, refs, compileSweepOptions{
		workers:     *workers,
		itemTimeout: *itemTimeout,
		userID:      strings.TrimSpace(*userID),
		force:       *force,
		platform:    strings.TrimSpace(*platform),
	})
	if writeJSON(stdout, stderr, summary) != 0 {
		return 1
	}
	if summary.Failed > 0 {
		return 1
	}
	return 0
}

type compileSweepOptions struct {
	workers     int
	itemTimeout time.Duration
	userID      string
	force       bool
	platform    string
}

func sweepCompileRefs(ctx context.Context, store *contentstore.SQLiteStore, client compileClient, refs []contentstore.RawCaptureRef, opts compileSweepOptions) compileSweepSummary {
	if opts.workers <= 0 {
		opts.workers = 1
	}
	var compiled int64
	var skipped int64
	var failed int64
	var contentGraphsBackfilled int64
	failedSamples := make([]string, 0)
	var mu sync.Mutex
	sem := make(chan struct{}, opts.workers)
	var wg sync.WaitGroup
	for _, ref := range refs {
		ref := ref
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if !opts.force {
				if _, err := store.GetCompiledOutput(ctx, ref.Platform, ref.ExternalID); err == nil {
					atomic.AddInt64(&skipped, 1)
					return
				}
			}
			raw, err := store.GetRawCapture(ctx, ref.Platform, ref.ExternalID)
			if err != nil {
				recordCompileSweepFailure(&mu, &failedSamples, &failed, ref, err)
				return
			}
			itemCtx, cancel := context.WithTimeout(ctx, opts.itemTimeout)
			record, err := compileRawCapture(itemCtx, client, raw)
			cancel()
			if err != nil {
				recordCompileSweepFailure(&mu, &failedSamples, &failed, ref, err)
				return
			}
			if err := store.UpsertCompiledOutput(ctx, record); err != nil {
				recordCompileSweepFailure(&mu, &failedSamples, &failed, ref, err)
				return
			}
			atomic.AddInt64(&compiled, 1)
			if opts.userID == "" {
				return
			}
			if err := store.PersistMemoryContentGraphFromCompiledOutput(ctx, opts.userID, record.Source, record.ExternalID, currentUTC()); err != nil {
				recordCompileSweepFailure(&mu, &failedSamples, &failed, ref, err)
				return
			}
			atomic.AddInt64(&contentGraphsBackfilled, 1)
		}()
	}
	wg.Wait()
	sort.Strings(failedSamples)
	status := "ok"
	if failed > 0 {
		status = "failed"
	}
	return compileSweepSummary{
		Scanned:                 len(refs),
		Compiled:                compiled,
		Skipped:                 skipped,
		Failed:                  failed,
		ContentGraphsBackfilled: contentGraphsBackfilled,
		Platform:                opts.platform,
		User:                    opts.userID,
		Force:                   opts.force,
		WorkerCount:             opts.workers,
		FailedSamples:           failedSamples,
		Status:                  status,
	}
}

func compileRawCapture(ctx context.Context, client compileClient, raw types.RawContent) (model.Record, error) {
	if client == nil {
		return model.Record{}, fmt.Errorf("compile client is nil")
	}
	bundle := model.BuildBundle(raw)
	compileStart := time.Now()
	record, err := client.Compile(ctx, bundle)
	if err != nil {
		return model.Record{}, err
	}
	if record.Metrics.CompileElapsedMS <= 0 {
		record.Metrics.CompileElapsedMS = time.Since(compileStart).Milliseconds()
		if record.Metrics.CompileElapsedMS <= 0 {
			record.Metrics.CompileElapsedMS = 1
		}
	}
	return record, nil
}

func recordCompileSweepFailure(mu *sync.Mutex, failedSamples *[]string, failed *int64, ref contentstore.RawCaptureRef, err error) {
	atomic.AddInt64(failed, 1)
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		detail = "unknown error"
	}
	mu.Lock()
	*failedSamples = append(*failedSamples, ref.Platform+":"+ref.ExternalID+": "+detail)
	mu.Unlock()
}
