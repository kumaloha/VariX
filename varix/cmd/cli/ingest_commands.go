package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/bootstrap"
	"github.com/kumaloha/VariX/varix/ingest/polling"
	"github.com/kumaloha/VariX/varix/ingest/provenance"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

var buildApp = bootstrap.BuildApp

func runIngestCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "unknown command")
		return 2
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	switch args[0] {
	case "fetch":
		fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
		fs.SetOutput(stderr)
		rawURL := fs.String("url", "", "content url")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		setRawURLFromArg(fs, rawURL)
		if strings.TrimSpace(*rawURL) == "" {
			fmt.Fprintln(stderr, "usage: varix ingest fetch <url>")
			return 2
		}
		items, err := fetchURLItems(context.Background(), app, *rawURL)
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

	case "follow":
		return runFollow(app, args[1:], stdout, stderr)

	case "list-follows":
		return runFollowList(app, stdout, stderr)

	case "poll":
		fs := flag.NewFlagSet("poll", flag.ContinueOnError)
		fs.SetOutput(stderr)
		loop := fs.Bool("loop", false, "run poll forever")
		interval := fs.Duration("interval", app.Settings.PollInterval, "loop interval")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}

		runOnce := func() error {
			report, items, storeWarnings, pollWarnings, err := app.Polling.Poll(context.Background())
			if len(storeWarnings) > 0 {
				printWarnings(stderr, storeWarnings)
			}
			if len(pollWarnings) > 0 {
				printPollWarnings(stderr, pollWarnings)
			}
			for _, item := range items {
				fmt.Fprintf(stdout, "source=%s external_id=%s url=%s\n", item.Source, item.ExternalID, item.URL)
			}
			printPollReport(stderr, report)
			if err != nil {
				return err
			}
			return nil
		}

		if !*loop {
			if err := runOnce(); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}

		if err := runOnce(); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := runOnce(); err != nil {
				fmt.Fprintln(stderr, err)
			}
		}
		return 0

	case "provenance-run":
		fs := flag.NewFlagSet("provenance-run", flag.ContinueOnError)
		fs.SetOutput(stderr)
		limit := fs.Int("limit", 100, "maximum pending lookup jobs to process")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		report, err := app.Provenance.RunOnce(context.Background(), *limit)
		printProvenanceReport(stderr, report)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0

	default:
		fmt.Fprintln(stderr, "unknown command")
		return 2
	}
}

func fetchURLItems(ctx context.Context, app *bootstrap.App, rawURL string) ([]types.RawContent, error) {
	if app == nil {
		return nil, fmt.Errorf("app is nil")
	}
	if app.Polling != nil {
		return app.Polling.FetchURL(ctx, rawURL)
	}
	if app.Dispatcher == nil {
		return nil, fmt.Errorf("dispatcher is nil")
	}
	parsed, err := app.Dispatcher.ParseURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return app.Dispatcher.FetchByParsedURL(ctx, parsed)
}

func runFollow(app *bootstrap.App, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: cli follow <add|list|remove> [flags]")
		return 2
	}
	if strings.HasPrefix(args[0], "-") {
		return runFollowAdd(app, args, stdout, stderr)
	}

	switch args[0] {
	case "add":
		return runFollowAdd(app, args[1:], stdout, stderr)
	case "list":
		return runFollowList(app, stdout, stderr)
	case "remove":
		return runFollowRemove(app, args[1:], stderr)
	default:
		fmt.Fprintln(stderr, "unknown follow subcommand")
		return 2
	}
}

func runFollowAdd(app *bootstrap.App, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("follow add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kind := fs.String("kind", "", "follow kind (search)")
	rawURL := fs.String("url", "", "profile/feed url")
	platform := fs.String("platform", "", "search platform")
	query := fs.String("query", "", "search query")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var (
		target types.FollowTarget
		err    error
	)
	switch {
	case strings.TrimSpace(*rawURL) != "" && strings.TrimSpace(*kind) == "":
		target, err = app.Polling.FollowURL(context.Background(), *rawURL)
	case strings.EqualFold(strings.TrimSpace(*kind), "search"):
		target, err = app.Polling.FollowSearch(context.Background(), types.Platform(strings.TrimSpace(*platform)), *query)
	default:
		fmt.Fprintln(stderr, "follow add requires either -url or -kind search -platform <platform> -query <query>")
		return 2
	}
	if err != nil {
		printFollowOperatorError(context.Background(), app, *rawURL, err, stderr)
		return 1
	}
	printFollowTarget(stdout, target)
	return 0
}

func runFollowList(app *bootstrap.App, stdout, stderr io.Writer) int {
	items, warnings, err := app.Polling.ListFollows(context.Background())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	printWarnings(stderr, warnings)
	for _, item := range items {
		printFollowTarget(stdout, item)
	}
	return 0
}

func runFollowRemove(app *bootstrap.App, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("follow remove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kind := fs.String("kind", "", "follow kind (search)")
	rawURL := fs.String("url", "", "profile/feed url")
	platform := fs.String("platform", "", "search platform")
	query := fs.String("query", "", "search query")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var err error
	switch {
	case strings.TrimSpace(*rawURL) != "" && strings.TrimSpace(*kind) == "":
		err = app.Polling.RemoveFollowURL(context.Background(), *rawURL)
	case strings.EqualFold(strings.TrimSpace(*kind), "search"):
		err = app.Polling.RemoveFollowSearch(context.Background(), types.Platform(strings.TrimSpace(*platform)), *query)
	default:
		fmt.Fprintln(stderr, "follow remove requires either -url or -kind search -platform <platform> -query <query>")
		return 2
	}
	if err != nil {
		printFollowOperatorError(context.Background(), app, *rawURL, err, stderr)
		return 1
	}
	return 0
}

func printFollowOperatorError(ctx context.Context, app *bootstrap.App, rawURL string, err error, stderr io.Writer) {
	if strings.TrimSpace(rawURL) != "" && strings.Contains(err.Error(), "follow strategy not supported: native/twitter") {
		parsed, parseErr := app.Dispatcher.ParseURL(ctx, rawURL)
		if parseErr == nil && parsed.ContentType == types.ContentTypeProfile && parsed.Platform == types.PlatformTwitter {
			fmt.Fprintln(stderr, "twitter profile follow is not supported in Phase 1; use search follow for twitter targets")
			return
		}
	}
	fmt.Fprintln(stderr, err)
}

func printFollowTarget(w io.Writer, target types.FollowTarget) {
	fmt.Fprintf(
		w,
		"kind=%s platform=%s locator=%s url=%s query=%s\n",
		target.Kind,
		target.Platform,
		target.Locator,
		target.URL,
		target.Query,
	)
}

func printWarnings(w io.Writer, warnings []contentstore.ScanWarning) {
	for _, warning := range warnings {
		fmt.Fprintf(w, "warning: %s %s %s\n", warning.Kind, warning.Path, warning.Detail)
	}
}

func printPollWarnings(w io.Writer, warnings []polling.PollWarning) {
	for _, warning := range warnings {
		fmt.Fprintf(w, "warning: %s %s %s %s\n", warning.Kind, warning.Target, warning.ItemURL, warning.Detail)
	}
}

func printPollReport(w io.Writer, report types.PollReport) {
	fmt.Fprintf(
		w,
		"poll-summary: targets=%d discovered=%d fetched=%d skipped=%d store_warnings=%d poll_warnings=%d\n",
		report.TargetCount,
		report.DiscoveredCount,
		report.FetchedCount,
		report.SkippedCount,
		report.StoreWarningCount,
		report.PollWarningCount,
	)
}

func printProvenanceReport(w io.Writer, report provenance.Report) {
	fmt.Fprintf(
		w,
		"provenance-summary: processed=%d found=%d not_found=%d failed=%d\n",
		report.ProcessedCount,
		report.FoundCount,
		report.NotFoundCount,
		report.FailedCount,
	)
}
