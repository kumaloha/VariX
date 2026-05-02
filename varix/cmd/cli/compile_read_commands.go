package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/model"
)

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
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile show --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

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
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile summary --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

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
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile compare --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

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

func writeHumanReadableCompileMetrics(w io.Writer, record model.Record) {
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
