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
	"github.com/kumaloha/VariX/varix/ingest/types"
)

func runVerifyCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: varix verify <run|show|queue|sweep> ...")
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
		fmt.Fprintln(stderr, "usage: varix verify <run|show|queue|sweep> ...")
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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}
	if strings.TrimSpace(*rawURL) == "" && (strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "") {
		fmt.Fprintln(stderr, "usage: varix verify run --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	c.EnableFactWebVerification()
	client := buildCompileClient(projectRoot)
	if client == nil {
		fmt.Fprintln(stderr, "verify client config missing")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()

	if strings.TrimSpace(*rawURL) != "" {
		parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*platform = string(parsed.Platform)
		*externalID = parsed.PlatformID
	}

	if !*force {
		if existing, err := store.GetVerificationResult(ctx, *platform, *externalID); err == nil {
			payload, marshalErr := json.MarshalIndent(existing, "", "  ")
			if marshalErr != nil {
				fmt.Fprintln(stderr, marshalErr)
				return 1
			}
			fmt.Fprintln(stdout, string(payload))
			return 0
		}
	}

	record, err := store.GetCompiledOutput(ctx, *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	raw, err := store.GetRawCapture(ctx, *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	bundle := c.BuildBundle(types.RawContent(raw))
	verification, err := client.VerifyDetailed(ctx, bundle, record.Output)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	verifyRecord := c.VerificationRecord{
		UnitID:         record.UnitID,
		Source:         record.Source,
		ExternalID:     record.ExternalID,
		RootExternalID: record.RootExternalID,
		Model:          record.Model,
		Verification:   verification,
		VerifiedAt:     firstVerificationTime(verification),
	}
	if err := store.UpsertVerificationResult(ctx, verifyRecord); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := store.ApplyVerificationRecordToContentSubgraph(ctx, verifyRecord); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	payload, err := json.MarshalIndent(verifyRecord, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*rawURL) != "" {
		parsed, err := app.Dispatcher.ParseURL(context.Background(), *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*platform = string(parsed.Platform)
		*externalID = parsed.PlatformID
	}
	if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix verify show --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetVerificationResult(context.Background(), *platform, *externalID)
	if err != nil {
		record, err = store.BuildVerificationRecordFromContentSubgraph(context.Background(), *platform, *externalID)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func firstVerificationTime(v c.Verification) time.Time {
	if !v.VerifiedAt.IsZero() {
		return v.VerifiedAt
	}
	return time.Now().UTC()
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
	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	if *summary {
		detail, err := store.GetVerifyQueueSummaryDetailed(context.Background(), time.Now().UTC())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		payload, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	items, err := store.ListVerifyQueueItemsByStatus(context.Background(), strings.TrimSpace(*status), *limit)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runVerifySweep(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify sweep", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "max items")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	result, err := store.RunVerifyQueueSweepFromContentGraphState(context.Background(), time.Now().UTC(), *limit)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
