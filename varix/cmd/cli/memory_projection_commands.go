package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

func runMemoryEventGraphs(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory event-graphs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute event graph projection before reading")
	scope := fs.String("scope", "", "optional filter: driver or target")
	subject := fs.String("subject", "", "optional filter on anchor subject")
	card := fs.Bool("card", false, "render a readable event graph card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory event-graphs --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedScope := strings.TrimSpace(*scope)
	trimmedSubject := strings.TrimSpace(*subject)
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *runNow {
		if _, err := store.RunEventGraphProjection(context.Background(), trimmedUserID, currentUTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	var items []contentstore.EventGraphRecord
	if trimmedSubject != "" {
		items, err = store.ListEventGraphsBySubject(context.Background(), trimmedUserID, trimmedSubject)
	} else {
		items, err = store.ListEventGraphs(context.Background(), trimmedUserID)
	}
	if err == nil && trimmedScope != "" {
		filtered := make([]contentstore.EventGraphRecord, 0, len(items))
		for _, item := range items {
			if item.Scope == trimmedScope {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No event graphs matched")
			return 0
		}
		fmt.Fprint(stdout, formatEventGraphCards(items))
		return 0
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryParadigms(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory paradigms", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute paradigm projection before reading")
	subject := fs.String("subject", "", "optional filter on driver subject")
	card := fs.Bool("card", false, "render a readable paradigm card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory paradigms --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedSubject := strings.TrimSpace(*subject)
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *runNow {
		if _, err := store.RunParadigmProjection(context.Background(), trimmedUserID, currentUTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListParadigms(context.Background(), trimmedUserID)
	if err == nil && trimmedSubject != "" {
		items, err = store.ListParadigmsBySubject(context.Background(), trimmedUserID, trimmedSubject)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No paradigms matched")
			return 0
		}
		fmt.Fprint(stdout, formatParadigmCards(items))
		return 0
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryContentGraphs(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory content-graphs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "rebuild one snapshot from current compiled output before reading")
	card := fs.Bool("card", false, "render a readable content graph card view")
	subject := fs.String("subject", "", "optional filter on subject")
	platform := fs.String("platform", "", "source platform (required with --run)")
	externalID := fs.String("id", "", "source external id (required with --run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory content-graphs --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if *runNow {
		if !hasContentTarget(*platform, *externalID) {
			fmt.Fprintln(stderr, "usage: varix memory content-graphs --run --user <user_id> --platform <platform> --id <external_id>")
			return 2
		}
		if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), currentUTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListMemoryContentGraphs(context.Background(), strings.TrimSpace(*userID))
	if err == nil && hasContentTarget(*platform, *externalID) {
		filtered := make([]graphmodel.ContentSubgraph, 0, len(items))
		for _, item := range items {
			if item.SourcePlatform == strings.TrimSpace(*platform) && item.SourceExternalID == strings.TrimSpace(*externalID) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err == nil && strings.TrimSpace(*subject) != "" {
		resolvedSubject, resolveErr := store.FindCanonicalEntityByAlias(context.Background(), strings.TrimSpace(*subject))
		if resolveErr == nil && strings.TrimSpace(resolvedSubject.CanonicalName) != "" {
			*subject = strings.TrimSpace(resolvedSubject.CanonicalName)
		} else if resolveErr != nil && !errors.Is(resolveErr, sql.ErrNoRows) {
			fmt.Fprintln(stderr, resolveErr)
			return 1
		}
		filtered := make([]graphmodel.ContentSubgraph, 0, len(items))
		for _, item := range items {
			matched := false
			for _, node := range item.Nodes {
				if node.SubjectText == strings.TrimSpace(*subject) || node.SubjectCanonical == strings.TrimSpace(*subject) {
					matched = true
					break
				}
			}
			if matched {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No content graphs matched")
			return 0
		}
		fmt.Fprint(stdout, formatContentGraphCards(items))
		return 0
	}
	payload, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func formatContentGraphCards(items []graphmodel.ContentSubgraph) string {
	var b strings.Builder
	for _, item := range items {
		primaryCount := 0
		primarySubjects := make([]string, 0)
		for _, node := range item.Nodes {
			if node.IsPrimary {
				primaryCount++
				if strings.TrimSpace(node.SubjectText) != "" {
					primarySubjects = append(primarySubjects, node.SubjectText)
				}
			}
		}
		fmt.Fprintf(&b, "Content Graph\n- Platform: %s\n- External ID: %s\n- Article ID: %s\n- Primary nodes: %d/%d\n", item.SourcePlatform, item.SourceExternalID, item.ArticleID, primaryCount, len(item.Nodes))
		if len(primarySubjects) > 0 {
			seen := map[string]struct{}{}
			uniq := make([]string, 0, len(primarySubjects))
			for _, subject := range primarySubjects {
				if _, ok := seen[subject]; ok {
					continue
				}
				seen[subject] = struct{}{}
				uniq = append(uniq, subject)
			}
			fmt.Fprintf(&b, "- Subjects: %s\n", strings.Join(uniq, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatEventGraphCards(items []contentstore.EventGraphRecord) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Event Graph\n- Scope: %s\n- Anchor: %s\n- Time bucket: %s\n", item.Scope, item.AnchorSubject, item.TimeBucket)
		if timeRange := formatEventGraphTimeRange(item); timeRange != "" {
			fmt.Fprintf(&b, "- Time: %s\n", timeRange)
		}
		if len(item.RepresentativeChanges) > 0 {
			fmt.Fprintf(&b, "- Representative changes: %s\n", strings.Join(item.RepresentativeChanges, ", "))
		}
		fmt.Fprintf(&b, "- Verification: %v\n\n", item.VerificationSummary)
	}
	return b.String()
}

func formatEventGraphTimeRange(item contentstore.EventGraphRecord) string {
	start := strings.TrimSpace(item.TimeStart)
	end := strings.TrimSpace(item.TimeEnd)
	switch {
	case start == "" && end == "":
		return ""
	case start == "" || start == end:
		return firstNonEmpty(end, start)
	case end == "":
		return start
	default:
		return start + " -> " + end
	}
}

func formatParadigmCards(items []contentstore.ParadigmRecord) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Paradigm\n- Driver: %s\n- Target: %s\n- Time bucket: %s\n- Credibility: %s (%.1f)\n", item.DriverSubject, item.TargetSubject, item.TimeBucket, item.CredibilityState, item.CredibilityScore)
		if len(item.RepresentativeChanges) > 0 {
			fmt.Fprintf(&b, "- Representative changes: %s\n", strings.Join(item.RepresentativeChanges, ", "))
		}
		fmt.Fprintf(&b, "- Success/Failure: %d/%d\n\n", item.SuccessCount, item.FailureCount)
	}
	return b.String()
}

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

func runMemoryBackfill(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	layer := fs.String("layer", "", "content | event | paradigm | global-v2 | all")
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
	case "event", "paradigm", "global-v2", "all":
		if strings.TrimSpace(*userID) == "" {
			fmt.Fprintln(stderr, "usage: varix memory backfill --layer <event|paradigm|global-v2|all> --user <user_id>")
			return 2
		}
	default:
		fmt.Fprintln(stderr, "usage: varix memory backfill --layer <content|event|paradigm|global-v2|all> ...")
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
	case "global-v2":
		start := time.Now()
		global, err := store.RunGlobalMemoryOrganizationV2(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		payload, err := json.MarshalIndent(map[string]any{"ok": true, "layer": "global-v2", "user": trimmedUserID, "global_v2": global.OutputID, "metrics": map[string]any{"global_v2_rebuild_ms": time.Since(start).Milliseconds()}}, "", "  ")
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
		global, err := store.RunGlobalMemoryOrganizationV2(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		globalDurationMS := time.Since(globalStart).Milliseconds()
		payload, err := json.MarshalIndent(map[string]any{
			"ok":             true,
			"layer":          "all",
			"user":           trimmedUserID,
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
	default:
		fmt.Fprintln(stderr, "usage: varix memory backfill --layer <content|event|paradigm|global-v2|all> ...")
		return 2
	}
}

func runMemoryEventEvidence(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory event-evidence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	eventGraphID := fs.String("event-graph-id", "", "optional filter on one event graph id")
	card := fs.Bool("card", false, "render a readable event-evidence view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory event-evidence --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	links, err := store.ListEventGraphEvidenceLinksByUser(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*eventGraphID) != "" {
		filtered := make([]contentstore.EventGraphEvidenceLink, 0, len(links))
		for _, item := range links {
			if item.EventGraphID == strings.TrimSpace(*eventGraphID) {
				filtered = append(filtered, item)
			}
		}
		links = filtered
	}
	if len(links) == 0 {
		fmt.Fprintln(stdout, "No event evidence matched")
		return 0
	}
	if *card {
		fmt.Fprint(stdout, formatEventEvidenceCards(links))
		return 0
	}
	payload, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryParadigmEvidence(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory paradigm-evidence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	paradigmID := fs.String("paradigm-id", "", "optional filter on one paradigm id")
	card := fs.Bool("card", false, "render a readable paradigm-evidence view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory paradigm-evidence --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	links, err := store.ListParadigmEvidenceLinksByUser(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*paradigmID) != "" {
		filtered := make([]contentstore.ParadigmEvidenceLink, 0, len(links))
		for _, item := range links {
			if item.ParadigmID == strings.TrimSpace(*paradigmID) {
				filtered = append(filtered, item)
			}
		}
		links = filtered
	}
	if len(links) == 0 {
		fmt.Fprintln(stdout, "No paradigm evidence matched")
		return 0
	}
	if *card {
		fmt.Fprint(stdout, formatParadigmEvidenceCards(links))
		return 0
	}
	payload, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func formatEventEvidenceCards(items []contentstore.EventGraphEvidenceLink) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Event Evidence\n- event_graph_id: %s\n- subgraph_id: %s\n- node_id: %s\n\n", item.EventGraphID, item.SubgraphID, item.NodeID)
	}
	return b.String()
}

func formatParadigmEvidenceCards(items []contentstore.ParadigmEvidenceLink) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "Paradigm Evidence\n- paradigm_id: %s\n- event_graph_id: %s\n- subgraph_id: %s\n\n", item.ParadigmID, item.EventGraphID, item.SubgraphID)
	}
	return b.String()
}
