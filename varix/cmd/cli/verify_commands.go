package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/model"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
	verification "github.com/kumaloha/VariX/varix/verify"
)

const verifyCommandUsage = "usage: varix verify <run|show|queue|sweep> ..."

func runVerifyCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, verifyCommandUsage)
		return 2
	}

	switch args[0] {
	case "run":
		return runVerifyRun(args[1:], projectRoot, stdout, stderr)
	case "show":
		return runVerifyShow(args[1:], projectRoot, stdout, stderr)
	case "queue":
		return runVerifyQueue(args[1:], projectRoot, stdout, stderr)
	case "sweep":
		return runVerifySweep(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, verifyCommandUsage)
		return 2
	}
}

func runVerifyRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	force := fs.Bool("force", false, "force re-verification even if verification result already exists")
	timeout := fs.Duration("timeout", 10*time.Minute, "verify timeout")
	llmCache := fs.String("llm-cache", string(contentstore.LLMCacheReadThrough), "LLM cache mode: read-through, refresh, off")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix verify run --url <url> | --platform <platform> --id <external_id>")
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
	verification.EnableFactWebVerification()
	client := buildVerifyClient(projectRoot)
	if client == nil {
		fmt.Fprintln(stderr, "verify client config missing")
		return 1
	}
	if currentClient, ok := client.(interface {
		EnableLLMCache(contentstore.LLMCacheStore, contentstore.LLMCacheMode)
	}); ok {
		currentClient.EnableLLMCache(store, cacheMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	*platform, *externalID, err = resolveContentTarget(ctx, app, *rawURL, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}

	if !*force {
		if existing, err := store.GetVerificationResult(ctx, *platform, *externalID); err == nil {
			return writeJSON(stdout, stderr, existing)
		}
	}

	record, err := store.GetCompiledOutput(ctx, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	raw, err := store.GetRawCapture(ctx, *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	bundle := model.BuildBundle(types.RawContent(raw))
	verification, err := client.VerifyDetailed(ctx, bundle, record.Output)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	verifyRecord := model.VerificationRecord{
		UnitID:         record.UnitID,
		Source:         record.Source,
		ExternalID:     record.ExternalID,
		RootExternalID: record.RootExternalID,
		Model:          record.Model,
		Verification:   verification,
		VerifiedAt:     firstVerificationTime(verification),
	}
	if err := store.UpsertVerificationResult(ctx, verifyRecord); err != nil {
		writeErr(stderr, err)
		return 1
	}
	if err := store.ApplyVerificationRecordToContentSubgraph(ctx, verifyRecord); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, verifyRecord)
}

func runVerifyShow(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	setRawURLFromArg(fs, rawURL)

	app, store, err := openRuntimeStore(projectRoot)
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
		fmt.Fprintln(stderr, "usage: varix verify show --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	record, err := store.GetVerificationResult(context.Background(), *platform, *externalID)
	if err != nil {
		record, err = store.BuildVerificationRecordFromContentSubgraph(context.Background(), *platform, *externalID)
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
	}
	return writeJSON(stdout, stderr, record)
}

func firstVerificationTime(v model.Verification) time.Time {
	if !v.VerifiedAt.IsZero() {
		return v.VerifiedAt
	}
	return currentUTC()
}

func runVerifyQueue(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify queue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "max items")
	status := fs.String("status", "", "optional filter: queued, running, retry, done")
	summary := fs.Bool("summary", false, "print queue status counts instead of items")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *summary {
		detail, err := store.GetVerifyQueueSummaryDetailed(context.Background(), currentUTC())
		if err != nil {
			writeErr(stderr, err)
			return 1
		}
		return writeJSON(stdout, stderr, detail)
	}
	items, err := store.ListVerifyQueueItemsByStatus(context.Background(), strings.TrimSpace(*status), *limit)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, items)
}

func runVerifySweep(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify sweep", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "max items")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	result, err := store.RunVerifyQueueSweepFromContentGraphState(context.Background(), currentUTC(), *limit)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, result)
}
