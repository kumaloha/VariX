package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

func runMemoryProjectAll(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory project-all", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory project-all --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	now := currentUTC()
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	eventStart := time.Now()
	events, err := store.RunEventGraphProjection(context.Background(), trimmedUserID, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	eventDurationMS := time.Since(eventStart).Milliseconds()
	paradigmStart := time.Now()
	paradigms, err := store.RunParadigmProjection(context.Background(), trimmedUserID, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	paradigmDurationMS := time.Since(paradigmStart).Milliseconds()
	globalStart := time.Now()
	global, err := store.RunGlobalMemoryOrganizationV2(context.Background(), trimmedUserID, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	globalDurationMS := time.Since(globalStart).Milliseconds()
	contentGraphs, err := store.ListMemoryContentGraphs(context.Background(), trimmedUserID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(map[string]any{
		"ok":             true,
		"content_graphs": len(contentGraphs),
		"event_graphs":   len(events),
		"paradigms":      len(paradigms),
		"global_v2":      global.OutputID,
		"metrics": map[string]any{
			"event_graph_rebuild_ms": eventDurationMS,
			"paradigm_recompute_ms":  paradigmDurationMS,
			"global_v2_rebuild_ms":   globalDurationMS,
		},
	}, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}
func runMemoryProjectionSweep(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory projection-sweep", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "optional user id; empty sweeps all pending users")
	limit := fs.Int("limit", 100, "max dirty projection marks to process")
	workers := fs.Int("workers", 1, "max users to sweep concurrently")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *limit <= 0 || *workers <= 0 {
		fmt.Fprintln(stderr, "usage: varix memory projection-sweep [--user <user_id>] [--limit <n>] [--workers <n>]")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	result, err := store.RunProjectionDirtySweepWithWorkers(context.Background(), strings.TrimSpace(*userID), *limit, *workers, currentUTC())
	if err != nil {
		payload, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr == nil {
			fmt.Fprintln(stdout, string(payload))
		}
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
