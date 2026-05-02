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

func runMemoryBackfill(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	layer := fs.String("layer", "", "content | event | paradigm | global-synthesis | all")
	platform := fs.String("platform", "", "source platform (required for content layer)")
	externalID := fs.String("id", "", "source external id (required for content layer)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	selectedLayer := strings.TrimSpace(*layer)
	switch selectedLayer {
	case "content":
		if invalidRequiredMemorySource(*userID, *platform, *externalID) {
			fmt.Fprintln(stderr, "usage: varix memory backfill --layer content --user <user_id> --platform <platform> --id <external_id>")
			return 2
		}
	case "event", "paradigm", "global-synthesis", "all":
		if strings.TrimSpace(*userID) == "" {
			fmt.Fprintln(stderr, "usage: varix memory backfill --layer <event|paradigm|global-synthesis|all> --user <user_id>")
			return 2
		}
	default:
		fmt.Fprintln(stderr, "usage: varix memory backfill --layer <content|event|paradigm|global-synthesis|all> ...")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedPlatform := strings.TrimSpace(*platform)
	trimmedExternalID := strings.TrimSpace(*externalID)
	now := currentUTC()
	switch selectedLayer {
	case "content":
		if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), trimmedUserID, trimmedPlatform, trimmedExternalID, now); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		contentGraphs, err := store.ListMemoryContentGraphs(context.Background(), trimmedUserID)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		count := 0
		for _, graph := range contentGraphs {
			if graph.SourcePlatform == trimmedPlatform && graph.SourceExternalID == trimmedExternalID {
				count++
			}
		}
		payload, err := json.MarshalIndent(map[string]any{
			"ok":             true,
			"layer":          "content",
			"user":           trimmedUserID,
			"platform":       trimmedPlatform,
			"id":             trimmedExternalID,
			"content_graphs": count,
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	case "event":
		start := time.Now()
		events, err := store.RunEventGraphProjection(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		payload, err := json.MarshalIndent(map[string]any{"ok": true, "layer": "event", "user": trimmedUserID, "event_graphs": len(events), "metrics": map[string]any{"event_graph_rebuild_ms": time.Since(start).Milliseconds()}}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	case "paradigm":
		start := time.Now()
		paradigms, err := store.RunParadigmProjection(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		payload, err := json.MarshalIndent(map[string]any{"ok": true, "layer": "paradigm", "user": trimmedUserID, "paradigms": len(paradigms), "metrics": map[string]any{"paradigm_recompute_ms": time.Since(start).Milliseconds()}}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	case "global-synthesis":
		start := time.Now()
		global, err := store.RunGlobalMemorySynthesis(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		payload, err := json.MarshalIndent(map[string]any{"ok": true, "layer": "global-synthesis", "user": trimmedUserID, "global_synthesis": global.OutputID, "metrics": map[string]any{"global_synthesis_rebuild_ms": time.Since(start).Milliseconds()}}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	case "all":
		contentGraphs, err := store.ListMemoryContentGraphs(context.Background(), trimmedUserID)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
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
		global, err := store.RunGlobalMemorySynthesis(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		globalDurationMS := time.Since(globalStart).Milliseconds()
		payload, err := json.MarshalIndent(map[string]any{
			"ok":               true,
			"layer":            "all",
			"user":             trimmedUserID,
			"content_graphs":   len(contentGraphs),
			"event_graphs":     len(events),
			"paradigms":        len(paradigms),
			"global_synthesis": global.OutputID,
			"metrics": map[string]any{
				"event_graph_rebuild_ms":      eventDurationMS,
				"paradigm_recompute_ms":       paradigmDurationMS,
				"global_synthesis_rebuild_ms": globalDurationMS,
			},
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	default:
		fmt.Fprintln(stderr, "usage: varix memory backfill --layer <content|event|paradigm|global-synthesis|all> ...")
		return 2
	}
}
