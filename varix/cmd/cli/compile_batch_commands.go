package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type compileBatchRunSummary struct {
	RunID         int64    `json:"run_id"`
	Pipeline      string   `json:"pipeline"`
	SampleScope   string   `json:"sample_scope"`
	SampleCount   int      `json:"sample_count"`
	WorkerCount   int      `json:"worker_count"`
	FinishedCount int64    `json:"finished_count"`
	FailedCount   int64    `json:"failed_count"`
	FailedSamples []string `json:"failed_samples,omitempty"`
	Status        string   `json:"status"`
}

func runCompileBatchRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile batch-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pipeline := fs.String("pipeline", "v2", "compile pipeline: v2")
	limit := fs.Int("limit", 0, "max raw captures to process; 0 means all")
	workers := fs.Int("workers", 5, "parallel workers")
	platform := fs.String("platform", "", "optional source platform filter")
	externalID := fs.String("id", "", "optional single external id (requires --platform)")
	stopAfter := fs.String("stop-after", "", "stop preview after a compile stage (extract|refine|aggregate|support|collapse|relations|spines|classify)")
	itemTimeout := fs.Duration("item-timeout", 30*time.Minute, "per-sample preview timeout")
	llmCache := fs.String("llm-cache", string(contentstore.LLMCacheReadThrough), "LLM cache mode: read-through, refresh, off")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cacheMode, err := parseCompileLLMCacheMode(*llmCache)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if strings.TrimSpace(*pipeline) != "v2" {
		fmt.Fprintln(stderr, "compile batch-run currently supports only --pipeline v2")
		return 2
	}
	if strings.TrimSpace(*externalID) != "" && strings.TrimSpace(*platform) == "" {
		fmt.Fprintln(stderr, "compile batch-run --id requires --platform")
		return 2
	}
	if *workers <= 0 {
		*workers = 5
	}
	if *itemTimeout <= 0 {
		*itemTimeout = 30 * time.Minute
	}

	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()

	client := cv2.NewClientFromConfig(projectRoot, nil)
	if client == nil {
		fmt.Fprintln(stderr, "compile v2 client config missing")
		return 1
	}
	client.EnableLLMCache(store, cacheMode)

	ctx := context.Background()
	var refs []contentstore.RawCaptureRef
	if strings.TrimSpace(*externalID) != "" {
		raw, err := store.GetRawCapture(ctx, strings.TrimSpace(*platform), strings.TrimSpace(*externalID))
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		refs = []contentstore.RawCaptureRef{{
			Platform:   raw.Source,
			ExternalID: raw.ExternalID,
			URL:        raw.URL,
		}}
	} else {
		refs, err = store.ListRawCaptureRefs(ctx, *limit, strings.TrimSpace(*platform))
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	if len(refs) == 0 {
		fmt.Fprintln(stderr, "no raw captures matched")
		return 1
	}

	scope := "all"
	switch {
	case strings.TrimSpace(*externalID) != "":
		scope = strings.TrimSpace(*platform) + ":" + strings.TrimSpace(*externalID)
	case strings.TrimSpace(*platform) != "":
		scope = "platform:" + strings.TrimSpace(*platform)
	}
	startedAt := currentUTC()
	runID, err := store.CreateCompilePreviewRun(ctx, contentstore.CompilePreviewRun{
		Pipeline:    "v2",
		SampleScope: scope,
		SampleCount: len(refs),
		WorkerCount: *workers,
		Status:      "running",
		StartedAt:   startedAt.Format(time.RFC3339),
	})
	if err != nil {
		writeErr(stderr, err)
		return 1
	}

	var finishedCount int64
	var failedCount int64
	failedSamples := make([]string, 0)
	var mu sync.Mutex
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	for _, ref := range refs {
		ref := ref
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			itemStarted := currentUTC()
			item := contentstore.CompilePreviewRunItem{
				RunID:      runID,
				Platform:   ref.Platform,
				ExternalID: ref.ExternalID,
				URL:        ref.URL,
				Status:     "running",
				StartedAt:  itemStarted.Format(time.RFC3339),
			}
			if err := store.UpsertCompilePreviewRunItem(context.Background(), item); err != nil {
				mu.Lock()
				failedSamples = append(failedSamples, ref.Platform+":"+ref.ExternalID+": store running status: "+err.Error())
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}

			raw, err := store.GetRawCapture(context.Background(), ref.Platform, ref.ExternalID)
			if err != nil {
				item.Status = "failed"
				item.ErrorDetail = err.Error()
				item.FinishedAt = currentUTC().Format(time.RFC3339)
				_ = store.UpsertCompilePreviewRunItem(context.Background(), item)
				mu.Lock()
				failedSamples = append(failedSamples, ref.Platform+":"+ref.ExternalID)
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}

			itemCtx, cancel := context.WithTimeout(context.Background(), *itemTimeout)
			defer cancel()
			result, err := client.PreviewFlow(itemCtx, c.BuildBundle(raw), cv2.FlowPreviewOptions{
				StopAfter: strings.TrimSpace(*stopAfter),
			})
			item.FinishedAt = currentUTC().Format(time.RFC3339)
			if err != nil {
				item.Status = "failed"
				item.ErrorDetail = err.Error()
				_ = store.UpsertCompilePreviewRunItem(context.Background(), item)
				mu.Lock()
				failedSamples = append(failedSamples, ref.Platform+":"+ref.ExternalID)
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			payload, err := json.Marshal(result)
			if err != nil {
				item.Status = "failed"
				item.ErrorDetail = err.Error()
				_ = store.UpsertCompilePreviewRunItem(context.Background(), item)
				mu.Lock()
				failedSamples = append(failedSamples, ref.Platform+":"+ref.ExternalID)
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			item.Status = "finished"
			item.PayloadJSON = string(payload)
			item.MainlineMarkdown = cv2.BuildMainlineMarkdown(result)
			item.ExtractNodes = len(result.Extract.Nodes)
			item.RelationsNodes = len(result.Relations.Nodes)
			item.RelationsEdges = len(result.Relations.Edges)
			item.ClassifyTargets = len(previewTargetNodesForCLI(result.Classify.Nodes))
			item.RenderDrivers = len(result.Render.Drivers)
			item.RenderTargets = len(result.Render.Targets)
			item.RenderPaths = len(result.Render.TransmissionPaths)
			if err := store.UpsertCompilePreviewRunItem(context.Background(), item); err != nil {
				mu.Lock()
				failedSamples = append(failedSamples, ref.Platform+":"+ref.ExternalID+": store final status: "+err.Error())
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			atomic.AddInt64(&finishedCount, 1)
		}()
	}
	wg.Wait()

	status := "finished"
	errorDetail := ""
	if failedCount > 0 {
		status = "failed"
		errorDetail = fmt.Sprintf("%d sample(s) failed", failedCount)
	}
	if err := store.UpdateCompilePreviewRunStatus(ctx, runID, status, errorDetail, currentUTC()); err != nil {
		writeErr(stderr, err)
		return 1
	}

	sort.Strings(failedSamples)
	return writeJSON(stdout, stderr, compileBatchRunSummary{
		RunID:         runID,
		Pipeline:      "v2",
		SampleScope:   scope,
		SampleCount:   len(refs),
		WorkerCount:   *workers,
		FinishedCount: finishedCount,
		FailedCount:   failedCount,
		FailedSamples: failedSamples,
		Status:        status,
	})
}
