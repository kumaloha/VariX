package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runCompileValidateRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile validate-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sourceRunIDsRaw := fs.String("source-run-ids", "", "comma-separated compile preview run ids to validate")
	workers := fs.Int("workers", 5, "parallel workers")
	itemTimeout := fs.Duration("item-timeout", 30*time.Minute, "per-sample validate timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	sourceRunIDs, err := parseCompileRunIDs(*sourceRunIDsRaw)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(sourceRunIDs) == 0 {
		fmt.Fprintln(stderr, "usage: varix compile validate-run --source-run-ids <id,id,...>")
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
	client := c.NewClientFromConfig(projectRoot, nil)
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}
	ctx := context.Background()
	sourceItems, err := compilePreviewItemsForRunIDs(ctx, store, sourceRunIDs)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if len(sourceItems) == 0 {
		fmt.Fprintln(stderr, "no finished preview items matched")
		return 1
	}

	scope := "author-validate:" + joinInt64s(sourceRunIDs)
	runID, err := store.CreateCompilePreviewRun(ctx, contentstore.CompilePreviewRun{
		Pipeline:               "compile-author-validate",
		SampleScope:            scope,
		SampleCount:            len(sourceItems),
		WorkerCount:            *workers,
		SkipValidate:           false,
		ValidateParagraphLimit: 0,
		Status:                 "running",
		StartedAt:              currentUTC().Format(time.RFC3339),
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
	for _, sourceItem := range sourceItems {
		sourceItem := sourceItem
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			itemStarted := currentUTC()
			item := contentstore.CompilePreviewRunItem{
				RunID:      runID,
				Platform:   sourceItem.Platform,
				ExternalID: sourceItem.ExternalID,
				URL:        sourceItem.URL,
				Status:     "running",
				StartedAt:  itemStarted.Format(time.RFC3339),
			}
			if err := store.UpsertCompilePreviewRunItem(context.Background(), item); err != nil {
				mu.Lock()
				failedSamples = append(failedSamples, sourceItem.Platform+":"+sourceItem.ExternalID+": store running status: "+err.Error())
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			raw, err := store.GetRawCapture(context.Background(), sourceItem.Platform, sourceItem.ExternalID)
			if err != nil {
				item.Status = "failed"
				item.ErrorDetail = err.Error()
				item.FinishedAt = currentUTC().Format(time.RFC3339)
				_ = store.UpsertCompilePreviewRunItem(context.Background(), item)
				mu.Lock()
				failedSamples = append(failedSamples, sourceItem.Platform+":"+sourceItem.ExternalID)
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			var sourceResult c.FlowPreviewResult
			if err := json.Unmarshal([]byte(sourceItem.PayloadJSON), &sourceResult); err != nil {
				item.Status = "failed"
				item.ErrorDetail = "parse source payload: " + err.Error()
				item.FinishedAt = currentUTC().Format(time.RFC3339)
				_ = store.UpsertCompilePreviewRunItem(context.Background(), item)
				mu.Lock()
				failedSamples = append(failedSamples, sourceItem.Platform+":"+sourceItem.ExternalID)
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			itemCtx, cancel := context.WithTimeout(context.Background(), *itemTimeout)
			defer cancel()
			result, err := client.AuthorValidatePreviewResult(itemCtx, c.BuildBundle(raw), sourceResult)
			item.FinishedAt = currentUTC().Format(time.RFC3339)
			if err != nil {
				item.Status = "failed"
				item.ErrorDetail = err.Error()
				_ = store.UpsertCompilePreviewRunItem(context.Background(), item)
				mu.Lock()
				failedSamples = append(failedSamples, sourceItem.Platform+":"+sourceItem.ExternalID)
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
				failedSamples = append(failedSamples, sourceItem.Platform+":"+sourceItem.ExternalID)
				mu.Unlock()
				atomic.AddInt64(&failedCount, 1)
				return
			}
			item.Status = "finished"
			item.PayloadJSON = string(payload)
			item.MainlineMarkdown = c.BuildMainlineMarkdown(result)
			item.ExtractNodes = len(result.Extract.Nodes)
			item.RelationsNodes = len(result.Relations.Nodes)
			item.RelationsEdges = len(result.Relations.Edges)
			item.ClassifyTargets = len(previewTargetNodesForCLI(result.Classify.Nodes))
			item.ValidateTargets = len(result.Render.AuthorValidation.ClaimChecks)
			item.RenderDrivers = len(result.Render.Drivers)
			item.RenderTargets = len(result.Render.Targets)
			item.RenderPaths = len(result.Render.TransmissionPaths)
			if err := store.UpsertCompilePreviewRunItem(context.Background(), item); err != nil {
				mu.Lock()
				failedSamples = append(failedSamples, sourceItem.Platform+":"+sourceItem.ExternalID+": store final status: "+err.Error())
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
		Pipeline:      "compile-author-validate",
		SampleScope:   scope,
		SampleCount:   len(sourceItems),
		WorkerCount:   *workers,
		FinishedCount: finishedCount,
		FailedCount:   failedCount,
		FailedSamples: failedSamples,
		Status:        status,
	})
}

func parseCompileRunIDs(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	out := make([]int64, 0, len(parts))
	seen := map[int64]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid run id %q", part)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func joinInt64s(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",")
}

func compilePreviewItemsForRunIDs(ctx context.Context, store *contentstore.SQLiteStore, runIDs []int64) ([]contentstore.CompilePreviewRunItem, error) {
	out := make([]contentstore.CompilePreviewRunItem, 0, len(runIDs))
	for _, runID := range runIDs {
		items, err := store.ListCompilePreviewRunItems(ctx, runID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.TrimSpace(item.Status) != "finished" || strings.TrimSpace(item.PayloadJSON) == "" {
				continue
			}
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RunID == out[j].RunID {
			return out[i].ItemID < out[j].ItemID
		}
		return out[i].RunID < out[j].RunID
	})
	return out, nil
}

func previewNodesByRoleForCLI(nodes []c.PreviewNode, role string) []c.PreviewNode {
	out := make([]c.PreviewNode, 0)
	for _, node := range nodes {
		if node.Role == role {
			out = append(out, node)
		}
	}
	return out
}

func previewTargetNodesForCLI(nodes []c.PreviewNode) []c.PreviewNode {
	out := make([]c.PreviewNode, 0)
	for _, node := range nodes {
		if node.IsTarget {
			out = append(out, node)
		}
	}
	return out
}
