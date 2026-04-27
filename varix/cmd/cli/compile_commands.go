package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type compileClient interface {
	Compile(ctx context.Context, bundle c.Bundle) (c.Record, error)
	Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error)
	VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error)
}

var buildCompileClient = func(projectRoot string) compileClient {
	return c.NewClientFromConfig(projectRoot, nil)
}

var buildCompileClientNoVerify = func(projectRoot string) compileClient {
	return c.NewClientFromConfigNoVerify(projectRoot, nil)
}

var buildCompileClientV2 = func(projectRoot string) compileClient {
	return cv2.NewClientFromConfig(projectRoot, nil)
}

var openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
	return contentstore.NewSQLiteStore(path)
}

const compileCommandUsage = "usage: varix compile <run|batch-run|show|summary|compare|card|gold-score> ..."

func selectCompileClient(projectRoot, pipeline string, noVerify bool) (compileClient, error) {
	switch strings.TrimSpace(pipeline) {
	case "", "legacy":
		if noVerify {
			return buildCompileClientNoVerify(projectRoot), nil
		}
		return buildCompileClient(projectRoot), nil
	case "v2":
		if noVerify {
			return nil, fmt.Errorf("--no-verify is not supported with --pipeline v2")
		}
		return buildCompileClientV2(projectRoot), nil
	default:
		return nil, fmt.Errorf("unsupported compile pipeline")
	}
}

func runCompileCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, compileCommandUsage)
		return 2
	}

	switch args[0] {
	case "run":
		return runCompileRun(args[1:], projectRoot, stdout, stderr)
	case "batch-run":
		return runCompileBatchRun(args[1:], projectRoot, stdout, stderr)
	case "show":
		return runCompileShow(args[1:], projectRoot, stdout, stderr)
	case "summary":
		return runCompileSummary(args[1:], projectRoot, stdout, stderr)
	case "compare":
		return runCompileCompare(args[1:], projectRoot, stdout, stderr)
	case "card":
		return runCompileCard(args[1:], projectRoot, stdout, stderr)
	case "gold-score":
		return runCompileGoldScore(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, compileCommandUsage)
		return 2
	}
}

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
	if err := fs.Parse(args); err != nil {
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

func previewNodesByRoleForCLI(nodes []cv2.PreviewNode, role string) []cv2.PreviewNode {
	out := make([]cv2.PreviewNode, 0)
	for _, node := range nodes {
		if node.Role == role {
			out = append(out, node)
		}
	}
	return out
}

func previewTargetNodesForCLI(nodes []cv2.PreviewNode) []cv2.PreviewNode {
	out := make([]cv2.PreviewNode, 0)
	for _, node := range nodes {
		if node.IsTarget {
			out = append(out, node)
		}
	}
	return out
}

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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id>")
		return 2
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

func runCompileGoldScore(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile gold-score", flag.ContinueOnError)
	fs.SetOutput(stderr)
	goldPath := fs.String("gold", "", "gold dataset JSON path")
	candidatePath := fs.String("candidate", "", "candidate JSON path with [{sample_id, output}]")
	candidateDir := fs.String("candidate-dir", "", "directory of candidate JSON reports named by sample id")
	outPath := fs.String("out", "", "optional output JSON path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*goldPath) == "" || (strings.TrimSpace(*candidatePath) == "" && strings.TrimSpace(*candidateDir) == "") {
		fmt.Fprintln(stderr, "usage: varix compile gold-score --gold <gold.json> (--candidate <candidate.json> | --candidate-dir <dir>)")
		return 2
	}
	if strings.TrimSpace(*candidatePath) != "" && strings.TrimSpace(*candidateDir) != "" {
		fmt.Fprintln(stderr, "compile gold-score accepts only one of --candidate or --candidate-dir")
		return 2
	}
	dataset, err := c.LoadGoldDataset(*goldPath)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	var candidates []c.GoldCandidate
	if strings.TrimSpace(*candidateDir) != "" {
		candidates, err = loadGoldCandidatesFromDir(*candidateDir)
	} else {
		candidates, err = loadGoldCandidates(*candidatePath)
	}
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	scorecard := c.ScoreGoldDataset(dataset, candidates)
	if strings.TrimSpace(*outPath) != "" {
		if err := writeGoldScorecardFile(*outPath, scorecard); err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	return writeJSON(stdout, stderr, scorecard)
}

func loadGoldCandidates(path string) ([]c.GoldCandidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gold candidates: %w", err)
	}
	var candidates []c.GoldCandidate
	if err := json.Unmarshal(raw, &candidates); err != nil {
		return nil, fmt.Errorf("parse gold candidates: %w", err)
	}
	return candidates, nil
}

func loadGoldCandidatesFromDir(dir string) ([]c.GoldCandidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read gold candidate dir: %w", err)
	}
	candidates := make([]c.GoldCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		candidate, err := loadGoldCandidateFile(path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(candidate.SampleID) == "" {
			candidate.SampleID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func loadGoldCandidateFile(path string) (c.GoldCandidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return c.GoldCandidate{}, fmt.Errorf("read gold candidate %s: %w", path, err)
	}
	var wrapped struct {
		SampleID string   `json:"sample_id"`
		ID       string   `json:"id"`
		Output   c.Output `json:"output"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return c.GoldCandidate{}, fmt.Errorf("parse gold candidate %s: %w", path, err)
	}
	if outputHasGoldContent(wrapped.Output) {
		id := strings.TrimSpace(wrapped.SampleID)
		if id == "" {
			id = strings.TrimSpace(wrapped.ID)
		}
		return c.GoldCandidate{SampleID: id, Output: wrapped.Output}, nil
	}
	var output c.Output
	if err := json.Unmarshal(raw, &output); err != nil {
		return c.GoldCandidate{}, fmt.Errorf("parse gold candidate output %s: %w", path, err)
	}
	return c.GoldCandidate{Output: output}, nil
}

func outputHasGoldContent(output c.Output) bool {
	return strings.TrimSpace(output.Summary) != "" ||
		len(output.Drivers) > 0 ||
		len(output.Targets) > 0 ||
		len(output.TransmissionPaths) > 0
}

func writeGoldScorecardFile(path string, scorecard c.GoldScorecard) error {
	payload, err := json.MarshalIndent(scorecard, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gold scorecard: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write gold scorecard: %w", err)
	}
	return nil
}

func runCompileShow(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)

	app, store, err := openAppStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	*platform, *externalID, err = resolveContentTarget(context.Background(), app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile show --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, record)
}

func runCompileSummary(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile summary", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)

	app, store, err := openAppStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	*platform, *externalID, err = resolveContentTarget(context.Background(), app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile summary --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Summary: %s\n", record.Output.Summary)
	fmt.Fprintf(stdout, "Drivers: %d\n", len(record.Output.Drivers))
	fmt.Fprintf(stdout, "Targets: %d\n", len(record.Output.Targets))
	fmt.Fprintf(stdout, "Paths: %d\n", len(record.Output.TransmissionPaths))
	if len(record.Output.Topics) > 0 {
		fmt.Fprintf(stdout, "Topics: %s\n", strings.Join(record.Output.Topics, ", "))
	}
	fmt.Fprintf(stdout, "Confidence: %s\n", record.Output.Confidence)
	writeHumanReadableCompileMetrics(stdout, record)
	return 0
}

func runCompileCompare(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)

	app, store, err := openAppStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	*platform, *externalID, err = resolveContentTarget(context.Background(), app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile compare --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	raw, err := store.GetRawCapture(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}

	rawPreview := raw.Content
	if len(rawPreview) > 500 {
		rawPreview = rawPreview[:500]
	}
	fmt.Fprintf(stdout, "Raw preview: %s\n", rawPreview)
	fmt.Fprintf(stdout, "Summary: %s\n", record.Output.Summary)
	fmt.Fprintf(stdout, "Drivers: %d\n", len(record.Output.Drivers))
	fmt.Fprintf(stdout, "Targets: %d\n", len(record.Output.Targets))
	fmt.Fprintf(stdout, "Paths: %d\n", len(record.Output.TransmissionPaths))
	fmt.Fprintf(stdout, "Confidence: %s\n", record.Output.Confidence)
	writeHumanReadableCompileMetrics(stdout, record)
	return 0
}

func writeHumanReadableCompileMetrics(w io.Writer, record c.Record) {
	if record.Metrics.CompileElapsedMS > 0 {
		fmt.Fprintf(w, "Compile elapsed: %dms\n", record.Metrics.CompileElapsedMS)
	} else {
		fmt.Fprintln(w, "Compile elapsed: unavailable")
	}
	if len(record.Metrics.CompileStageElapsedMS) == 0 {
		fmt.Fprintln(w, "Stages: unavailable")
		return
	}
	stages := make([]string, 0, len(record.Metrics.CompileStageElapsedMS))
	for stage, ms := range record.Metrics.CompileStageElapsedMS {
		stages = append(stages, fmt.Sprintf("%s=%dms", stage, ms))
	}
	sort.Strings(stages)
	fmt.Fprintf(w, "Stages: %s\n", strings.Join(stages, ", "))
}

func runCompileCard(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile card", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	compact := fs.Bool("compact", false, "render a compact product-facing card")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)

	app, store, err := openAppStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	*platform, *externalID, err = resolveContentTarget(context.Background(), app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile card --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	var subgraph *graphmodel.ContentSubgraph
	if graph, graphErr := store.GetContentSubgraph(context.Background(), *platform, *externalID); graphErr == nil {
		subgraph = &graph
	} else if !errors.Is(graphErr, sql.ErrNoRows) {
		fmt.Fprintln(stderr, graphErr)
		return 1
	}
	projection := buildCompileCardProjection(record, subgraph)

	if *compact {
		fmt.Fprint(stdout, formatCompactCompileCard(projection))
		return 0
	}
	fmt.Fprint(stdout, formatCompileCard(projection))
	return 0
}

type compileCardProjection struct {
	Summary             string
	Topics              []string
	Confidence          string
	Drivers             []string
	Targets             []string
	Evidence            []string
	Explanations        []string
	LogicChains         []string
	VerificationSummary []string
}

func buildCompileCardProjection(record c.Record, subgraph *graphmodel.ContentSubgraph) compileCardProjection {
	projection := compileCardProjection{
		Summary:      record.Output.Summary,
		Topics:       cloneStringSlice(record.Output.Topics),
		Confidence:   record.Output.Confidence,
		Drivers:      cloneStringSlice(record.Output.Drivers),
		Targets:      cloneStringSlice(record.Output.Targets),
		Evidence:     cloneStringSlice(record.Output.EvidenceNodes),
		Explanations: cloneStringSlice(record.Output.ExplanationNodes),
		LogicChains:  legacyLogicChains(record),
	}
	if subgraph == nil {
		return projection
	}
	if drivers := graphFirstNodeSection(*subgraph, func(node graphmodel.GraphNode) bool {
		return node.IsPrimary && node.GraphRole == graphmodel.GraphRoleDriver
	}); len(drivers) > 0 {
		projection.Drivers = preferGraphFirstSection(projection.Drivers, drivers)
	}
	if targets := graphFirstNodeSection(*subgraph, func(node graphmodel.GraphNode) bool {
		return node.IsPrimary && node.GraphRole == graphmodel.GraphRoleTarget
	}); len(targets) > 0 {
		projection.Targets = preferGraphFirstSection(projection.Targets, targets)
	}
	if evidence := graphFirstEvidenceSection(*subgraph); len(evidence) > 0 {
		projection.Evidence = preferGraphFirstSection(projection.Evidence, evidence)
	}
	if explanations := graphFirstExplanationSection(*subgraph); len(explanations) > 0 {
		projection.Explanations = preferGraphFirstSection(projection.Explanations, explanations)
	}
	if chains := graphFirstLogicChains(*subgraph); len(chains) > 0 {
		projection.LogicChains = preferGraphFirstLogicChains(projection.LogicChains, chains)
	}
	if verification := graphFirstVerificationSummary(*subgraph); len(verification) > 0 {
		projection.VerificationSummary = verification
	}
	return projection
}

func formatCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	if len(projection.Topics) > 0 {
		fmt.Fprintf(&b, "Topics\n- %s\n\n", strings.Join(projection.Topics, "\n- "))
	}
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 5))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 5))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 5))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 5))
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Logic chain\n")
		for _, chain := range projection.LogicChains {
			fmt.Fprintf(&b, "- %s\n", chain)
		}
		b.WriteString("\n")
	}
	if len(projection.VerificationSummary) > 0 {
		fmt.Fprintf(&b, "Verification\n")
		for _, line := range projection.VerificationSummary {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
	return b.String()
}

func formatCompactCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 3))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 3))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 3))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 2))
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Main logic\n- %s\n\n", projection.LogicChains[0])
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
	return b.String()
}

func writeCompactNodeSection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
	b.WriteString("\n")
}

func legacyLogicChains(record c.Record) []string {
	if len(record.Output.TransmissionPaths) == 0 {
		return nil
	}
	chains := make([]string, 0, len(record.Output.TransmissionPaths))
	for _, path := range record.Output.TransmissionPaths {
		parts := []string{}
		if strings.TrimSpace(path.Driver) != "" {
			parts = append(parts, truncate(path.Driver, 50))
		}
		for _, step := range path.Steps {
			parts = append(parts, truncate(step, 50))
		}
		if strings.TrimSpace(path.Target) != "" {
			parts = append(parts, truncate(path.Target, 50))
		}
		if len(parts) > 0 {
			chains = append(chains, strings.Join(parts, " -> "))
		}
	}
	return chains
}

func graphFirstNodeSection(subgraph graphmodel.ContentSubgraph, keep func(graphmodel.GraphNode) bool) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, node := range subgraph.Nodes {
		if !keep(node) {
			continue
		}
		label := strings.TrimSpace(graphFirstNodeLabel(node))
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func graphFirstEvidenceSection(subgraph graphmodel.ContentSubgraph) []string {
	out := graphFirstNodeSection(subgraph, func(node graphmodel.GraphNode) bool {
		return node.GraphRole == graphmodel.GraphRoleEvidence
	})
	if len(out) > 0 {
		return out
	}
	byID := graphNodeIndex(subgraph)
	for _, edge := range subgraph.Edges {
		if edge.Type != graphmodel.EdgeTypeSupports {
			continue
		}
		if node, ok := byID[edge.From]; ok {
			out = appendUniqueString(out, graphFirstNodeLabel(node))
		}
	}
	return out
}

func graphFirstExplanationSection(subgraph graphmodel.ContentSubgraph) []string {
	out := graphFirstNodeSection(subgraph, func(node graphmodel.GraphNode) bool {
		return node.GraphRole == graphmodel.GraphRoleContext
	})
	byID := graphNodeIndex(subgraph)
	for _, edge := range subgraph.Edges {
		if edge.Type != graphmodel.EdgeTypeExplains {
			continue
		}
		if node, ok := byID[edge.From]; ok {
			out = appendUniqueString(out, graphFirstNodeLabel(node))
		}
	}
	return out
}

func graphFirstLogicChains(subgraph graphmodel.ContentSubgraph) []string {
	nodeByID := graphNodeIndex(subgraph)
	primaryDriveAdj := map[string][]string{}
	primaryDriveNodes := map[string]struct{}{}
	for _, edge := range subgraph.Edges {
		if edge.Type != graphmodel.EdgeTypeDrives {
			continue
		}
		if !edge.IsPrimary {
			continue
		}
		primaryDriveAdj[edge.From] = append(primaryDriveAdj[edge.From], edge.To)
		primaryDriveNodes[edge.From] = struct{}{}
		primaryDriveNodes[edge.To] = struct{}{}
	}
	if len(primaryDriveAdj) == 0 {
		return nil
	}
	for from := range primaryDriveAdj {
		sort.Strings(primaryDriveAdj[from])
	}
	starts := make([]string, 0)
	targets := map[string]struct{}{}
	for _, node := range subgraph.Nodes {
		if !node.IsPrimary {
			continue
		}
		if node.GraphRole == graphmodel.GraphRoleDriver {
			starts = append(starts, node.ID)
		}
		if node.GraphRole == graphmodel.GraphRoleTarget {
			targets[node.ID] = struct{}{}
		}
	}
	sort.Strings(starts)
	chains := make([]string, 0)
	seen := map[string]struct{}{}
	for _, start := range starts {
		graphFirstCollectPaths(start, primaryDriveAdj, targets, nodeByID, nil, map[string]bool{}, &chains, seen)
	}
	return chains
}

func graphFirstCollectPaths(current string, adj map[string][]string, targets map[string]struct{}, nodeByID map[string]graphmodel.GraphNode, path []string, visiting map[string]bool, out *[]string, seen map[string]struct{}) {
	if visiting[current] {
		return
	}
	node, ok := nodeByID[current]
	if !ok {
		return
	}
	label := graphFirstNodeLabel(node)
	if strings.TrimSpace(label) == "" {
		return
	}
	path = append(path, truncate(label, 50))
	if _, isTarget := targets[current]; isTarget {
		chain := strings.Join(path, " -> ")
		if _, ok := seen[chain]; !ok {
			seen[chain] = struct{}{}
			*out = append(*out, chain)
		}
	}
	nexts := adj[current]
	if len(nexts) == 0 {
		return
	}
	visiting[current] = true
	for _, next := range nexts {
		graphFirstCollectPaths(next, adj, targets, nodeByID, path, visiting, out, seen)
	}
	delete(visiting, current)
}

func graphNodeIndex(subgraph graphmodel.ContentSubgraph) map[string]graphmodel.GraphNode {
	out := make(map[string]graphmodel.GraphNode, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		out[node.ID] = node
	}
	return out
}

func graphFirstNodeLabel(node graphmodel.GraphNode) string {
	rawText := strings.TrimSpace(node.RawText)
	sourceQuote := strings.TrimSpace(node.SourceQuote)
	subjectText := strings.TrimSpace(node.SubjectText)
	changeText := strings.TrimSpace(node.ChangeText)
	switch {
	case rawText != "":
		return rawText
	case sourceQuote != "":
		return sourceQuote
	case c.HasDistinctNonEmptyPair(subjectText, changeText):
		return subjectText + " " + changeText
	default:
		return strings.TrimSpace(c.FirstNonEmpty(subjectText, changeText, node.ID))
	}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func graphFirstChainRichness(chains []string) int {
	best := 0
	for _, chain := range chains {
		parts := strings.Split(chain, "->")
		if len(parts) > best {
			best = len(parts)
		}
	}
	return best
}

func graphFirstSectionRichness(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func preferGraphFirstSection(legacy, graph []string) []string {
	if graphFirstSectionRichness(graph) >= graphFirstSectionRichness(legacy) {
		return graph
	}
	return legacy
}

func preferGraphFirstLogicChains(legacy, graph []string) []string {
	if graphFirstChainRichness(graph) >= graphFirstChainRichness(legacy) {
		return graph
	}
	return legacy
}

func graphFirstVerificationSummary(subgraph graphmodel.ContentSubgraph) []string {
	nodeCounts := map[graphmodel.VerificationStatus]int{}
	edgeCounts := map[graphmodel.VerificationStatus]int{}
	for _, node := range subgraph.Nodes {
		nodeCounts[node.VerificationStatus]++
	}
	for _, edge := range subgraph.Edges {
		status := edge.VerificationStatus
		if status == "" {
			status = graphmodel.VerificationPending
		}
		edgeCounts[status]++
	}
	out := make([]string, 0, 2)
	if len(nodeCounts) > 0 {
		out = append(out, "Nodes: "+formatVerificationCounts(nodeCounts))
	}
	if len(edgeCounts) > 0 {
		out = append(out, "Edges: "+formatVerificationCounts(edgeCounts))
	}
	return out
}

func formatVerificationCounts(counts map[graphmodel.VerificationStatus]int) string {
	parts := make([]string, 0, 4)
	for _, status := range []graphmodel.VerificationStatus{
		graphmodel.VerificationPending,
		graphmodel.VerificationProved,
		graphmodel.VerificationDisproved,
		graphmodel.VerificationUnverifiable,
	} {
		if counts[status] == 0 {
			continue
		}
		parts = append(parts, string(status)+"="+fmt.Sprintf("%d", counts[status]))
	}
	return strings.Join(parts, ", ")
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

func truncateList(values []string, max int) []string {
	out := make([]string, 0, max)
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, truncate(value, 100))
		if len(out) == max {
			break
		}
	}
	return out
}
