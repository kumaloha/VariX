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
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
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

var buildCompileClientNoVerifyNoValidate = func(projectRoot string) compileClient {
	return c.NewClientFromConfigNoVerifyNoValidate(projectRoot, nil)
}

var buildCompileClientV2 = func(projectRoot string) compileClient {
	return cv2.NewClientFromConfig(projectRoot, nil)
}

var openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
	return contentstore.NewSQLiteStore(path)
}

func selectCompileClient(projectRoot, pipeline string, noVerify, noValidate bool) (compileClient, error) {
	switch strings.TrimSpace(pipeline) {
	case "", "legacy":
		switch {
		case noVerify && noValidate:
			return buildCompileClientNoVerifyNoValidate(projectRoot), nil
		case noVerify:
			return buildCompileClientNoVerify(projectRoot), nil
		default:
			return buildCompileClient(projectRoot), nil
		}
	case "v2":
		if noVerify || noValidate {
			return nil, fmt.Errorf("--no-verify/--no-validate are not supported with --pipeline v2")
		}
		return buildCompileClientV2(projectRoot), nil
	default:
		return nil, fmt.Errorf("unsupported compile pipeline")
	}
}

func runCompileCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: varix compile <run|show> ...")
		return 2
	}

	switch args[0] {
	case "run":
		return runCompileRun(args[1:], projectRoot, stdout, stderr)
	case "show":
		return runCompileShow(args[1:], projectRoot, stdout, stderr)
	case "summary":
		return runCompileSummary(args[1:], projectRoot, stdout, stderr)
	case "compare":
		return runCompileCompare(args[1:], projectRoot, stdout, stderr)
	case "card":
		return runCompileCard(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: varix compile <run|show|summary|compare|card> ...")
		return 2
	}
}

func runCompileRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	force := fs.Bool("force", false, "force recompilation even if compiled output already exists")
	noVerify := fs.Bool("no-verify", false, "skip compile-time verification and retrieval")
	noValidate := fs.Bool("no-validate", false, "skip compile output validation (evaluation/debug only)")
	pipeline := fs.String("pipeline", "legacy", "compile pipeline: legacy | v2")
	timeout := fs.Duration("timeout", 20*time.Minute, "compile timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}
	if strings.TrimSpace(*rawURL) == "" && (strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "") {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !*noVerify {
		c.EnableFactWebVerification()
	}
	client, err := selectCompileClient(projectRoot, *pipeline, *noVerify, *noValidate)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
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

	if !*force {
		switch {
		case strings.TrimSpace(*rawURL) != "":
			if parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL); err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
				if record, err := store.GetCompiledOutput(ctx, string(parsed.Platform), parsed.PlatformID); err == nil {
					payload, marshalErr := json.MarshalIndent(record, "", "  ")
					if marshalErr != nil {
						fmt.Fprintln(stderr, marshalErr)
						return 1
					}
					fmt.Fprintln(stdout, string(payload))
					return 0
				}
			}
		case strings.TrimSpace(*platform) != "" && strings.TrimSpace(*externalID) != "":
			if record, err := store.GetCompiledOutput(ctx, *platform, *externalID); err == nil {
				payload, marshalErr := json.MarshalIndent(record, "", "  ")
				if marshalErr != nil {
					fmt.Fprintln(stderr, marshalErr)
					return 1
				}
				fmt.Fprintln(stdout, string(payload))
				return 0
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
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	bundle := c.BuildBundle(raw)
	record, err := client.Compile(ctx, bundle)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := store.UpsertCompiledOutput(ctx, record); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
		fmt.Fprintln(stderr, "usage: varix compile show --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
		fmt.Fprintln(stderr, "usage: varix compile summary --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
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
		fmt.Fprintln(stderr, "usage: varix compile compare --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	raw, err := store.GetRawCapture(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
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
	return 0
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
		fmt.Fprintln(stderr, "usage: varix compile card --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *compact {
		fmt.Fprint(stdout, formatCompactCompileCard(record))
		return 0
	}
	fmt.Fprint(stdout, formatCompileCard(record))
	return 0
}

func formatCompileCard(record c.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", record.Output.Summary)
	if len(record.Output.Topics) > 0 {
		fmt.Fprintf(&b, "Topics\n- %s\n\n", strings.Join(record.Output.Topics, "\n- "))
	}
	if chains := logicChains(record); len(chains) > 0 {
		fmt.Fprintf(&b, "Logic chain\n")
		for _, chain := range chains {
			fmt.Fprintf(&b, "- %s\n", chain)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", record.Output.Confidence)
	return b.String()
}

func formatCompactCompileCard(record c.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", record.Output.Summary)
	writeCompactNodeSection(&b, "Drivers", truncateList(record.Output.Drivers, 3))
	writeCompactNodeSection(&b, "Targets", truncateList(record.Output.Targets, 3))
	writeCompactNodeSection(&b, "Evidence", truncateList(record.Output.EvidenceNodes, 3))
	writeCompactNodeSection(&b, "Explanations", truncateList(record.Output.ExplanationNodes, 2))
	if chains := logicChains(record); len(chains) > 0 {
		fmt.Fprintf(&b, "Main logic\n- %s\n\n", chains[0])
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", record.Output.Confidence)
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

func logicChains(record c.Record) []string {
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
