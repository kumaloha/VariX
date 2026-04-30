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
	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

const memoryCommandUsage = "usage: varix memory <accept|accept-batch|list|show-source|content-graphs|subject-timeline|subject-horizon|subject-experience|jobs|posterior-run|organize-run|organized|global-organize-run|global-organized|global-v2-organize-run|global-v2-organized|global-card|global-v2-card|global-compare|event-graphs|event-evidence|paradigms|paradigm-evidence|project-all|projection-sweep|backfill|cleanup-stale|canonical-entities|canonical-entity-upsert> ..."
const globalV2ItemTypeUsage = "item-type must be one of: card, conclusion, conflict"

func runMemoryCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, memoryCommandUsage)
		return 2
	}
	switch args[0] {
	case "accept":
		return runMemoryAccept(args[1:], projectRoot, stdout, stderr)
	case "accept-batch":
		return runMemoryAcceptBatch(args[1:], projectRoot, stdout, stderr)
	case "list":
		return runMemoryList(args[1:], projectRoot, stdout, stderr)
	case "show-source":
		return runMemoryShowSource(args[1:], projectRoot, stdout, stderr)
	case "content-graphs":
		return runMemoryContentGraphs(args[1:], projectRoot, stdout, stderr)
	case "subject-timeline":
		return runMemorySubjectTimeline(args[1:], projectRoot, stdout, stderr)
	case "subject-horizon":
		return runMemorySubjectHorizon(args[1:], projectRoot, stdout, stderr)
	case "subject-experience":
		return runMemorySubjectExperience(args[1:], projectRoot, stdout, stderr)
	case "jobs":
		return runMemoryJobs(args[1:], projectRoot, stdout, stderr)
	case "posterior-run":
		return runMemoryPosteriorRun(args[1:], projectRoot, stdout, stderr)
	case "organize-run":
		return runMemoryOrganizeRun(args[1:], projectRoot, stdout, stderr)
	case "organized":
		return runMemoryOrganized(args[1:], projectRoot, stdout, stderr)
	case "global-organize-run":
		return runMemoryGlobalOrganizeRun(args[1:], projectRoot, stdout, stderr)
	case "global-organized":
		return runMemoryGlobalOrganized(args[1:], projectRoot, stdout, stderr)
	case "global-v2-organize-run":
		return runMemoryGlobalV2OrganizeRun(args[1:], projectRoot, stdout, stderr)
	case "global-v2-organized":
		return runMemoryGlobalV2Organized(args[1:], projectRoot, stdout, stderr)
	case "global-card":
		return runMemoryGlobalCard(args[1:], projectRoot, stdout, stderr)
	case "global-v2-card":
		return runMemoryGlobalV2Card(args[1:], projectRoot, stdout, stderr)
	case "global-compare":
		return runMemoryGlobalCompare(args[1:], projectRoot, stdout, stderr)
	case "event-graphs":
		return runMemoryEventGraphs(args[1:], projectRoot, stdout, stderr)
	case "event-evidence":
		return runMemoryEventEvidence(args[1:], projectRoot, stdout, stderr)
	case "paradigms":
		return runMemoryParadigms(args[1:], projectRoot, stdout, stderr)
	case "paradigm-evidence":
		return runMemoryParadigmEvidence(args[1:], projectRoot, stdout, stderr)
	case "project-all":
		return runMemoryProjectAll(args[1:], projectRoot, stdout, stderr)
	case "projection-sweep":
		return runMemoryProjectionSweep(args[1:], projectRoot, stdout, stderr)
	case "backfill":
		return runMemoryBackfill(args[1:], projectRoot, stdout, stderr)
	case "cleanup-stale":
		return runMemoryCleanupStale(args[1:], projectRoot, stdout, stderr)
	case "canonical-entities":
		return runMemoryCanonicalEntities(args[1:], projectRoot, stdout, stderr)
	case "canonical-entity-upsert":
		return runMemoryCanonicalEntityUpsert(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, memoryCommandUsage)
		return 2
	}
}

func runMemoryAccept(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory accept", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	platform := fs.String("platform", "", "source platform")
	externalID := fs.String("id", "", "source external id")
	nodeID := fs.String("node", "", "compile node id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidRequiredMemorySource(*userID, *platform, *externalID) || strings.TrimSpace(*nodeID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory accept --user <user_id> --platform <platform> --id <external_id> --node <node_id>")
		return 2
	}
	return runMemoryAcceptRequest(projectRoot, stdout, stderr, memory.AcceptRequest{
		UserID:           strings.TrimSpace(*userID),
		SourcePlatform:   strings.TrimSpace(*platform),
		SourceExternalID: strings.TrimSpace(*externalID),
		NodeIDs:          []string{strings.TrimSpace(*nodeID)},
	})
}

func runMemoryAcceptBatch(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory accept-batch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	platform := fs.String("platform", "", "source platform")
	externalID := fs.String("id", "", "source external id")
	nodes := fs.String("nodes", "", "comma-separated compile node ids")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rawNodes := strings.Split(strings.TrimSpace(*nodes), ",")
	nodeIDs := make([]string, 0, len(rawNodes))
	for _, nodeID := range rawNodes {
		if trimmed := strings.TrimSpace(nodeID); trimmed != "" {
			nodeIDs = append(nodeIDs, trimmed)
		}
	}
	if invalidRequiredMemorySource(*userID, *platform, *externalID) || len(nodeIDs) == 0 {
		fmt.Fprintln(stderr, "usage: varix memory accept-batch --user <user_id> --platform <platform> --id <external_id> --nodes <id1,id2,...>")
		return 2
	}
	return runMemoryAcceptRequest(projectRoot, stdout, stderr, memory.AcceptRequest{
		UserID:           strings.TrimSpace(*userID),
		SourcePlatform:   strings.TrimSpace(*platform),
		SourceExternalID: strings.TrimSpace(*externalID),
		NodeIDs:          nodeIDs,
	})
}

func runMemoryAcceptRequest(projectRoot string, stdout, stderr io.Writer, req memory.AcceptRequest) int {
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	result, err := store.AcceptMemoryNodes(context.Background(), req)
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

func runMemoryList(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory list --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	items, err := store.ListUserMemory(context.Background(), strings.TrimSpace(*userID))
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

func runMemoryShowSource(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory show-source", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	platform := fs.String("platform", "", "source platform")
	externalID := fs.String("id", "", "source external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidRequiredMemorySource(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory show-source --user <user_id> --platform <platform> --id <external_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	items, err := store.ListUserMemoryBySource(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID))
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

func runMemoryJobs(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory jobs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	status := fs.String("status", "", "optional filter: queued, running, done")
	summary := fs.Bool("summary", false, "print status counts instead of full jobs")
	platform := fs.String("platform", "", "optional source platform filter")
	externalID := fs.String("id", "", "optional source external id filter (requires --platform)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidScopedMemorySourceRequest(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory jobs --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	jobs, err := store.ListMemoryJobs(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*platform) != "" {
		filtered := make([]memory.OrganizationJob, 0, len(jobs))
		for _, job := range jobs {
			if job.SourcePlatform != strings.TrimSpace(*platform) {
				continue
			}
			if strings.TrimSpace(*externalID) != "" && job.SourceExternalID != strings.TrimSpace(*externalID) {
				continue
			}
			filtered = append(filtered, job)
		}
		jobs = filtered
	}
	if strings.TrimSpace(*status) != "" {
		filtered := make([]memory.OrganizationJob, 0, len(jobs))
		for _, job := range jobs {
			if job.Status == strings.TrimSpace(*status) {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}
	if *summary {
		counts := map[string]int{}
		now := currentUTC()
		var oldestQueuedAt time.Time
		var oldestRunningAt time.Time
		for _, job := range jobs {
			counts[job.Status]++
			switch job.Status {
			case "queued":
				if oldestQueuedAt.IsZero() || (!job.CreatedAt.IsZero() && job.CreatedAt.Before(oldestQueuedAt)) {
					oldestQueuedAt = job.CreatedAt
				}
			case "running":
				staleAt := job.CreatedAt
				if !job.StartedAt.IsZero() {
					staleAt = job.StartedAt
				}
				if oldestRunningAt.IsZero() || (!staleAt.IsZero() && staleAt.Before(oldestRunningAt)) {
					oldestRunningAt = staleAt
				}
			}
			switch job.Status {
			case "queued":
				if !job.CreatedAt.IsZero() && now.Sub(job.CreatedAt) > 24*time.Hour {
					counts["stale_candidates"]++
					counts["stale_queued"]++
				}
			case "running":
				staleAt := job.CreatedAt
				if !job.StartedAt.IsZero() {
					staleAt = job.StartedAt
				}
				if !staleAt.IsZero() && now.Sub(staleAt) > 24*time.Hour {
					counts["stale_candidates"]++
					counts["stale_running"]++
				}
			}
		}
		payload, err := json.MarshalIndent(map[string]any{
			"user":              strings.TrimSpace(*userID),
			"counts":            counts,
			"oldest_queued_at":  formatMaybeRFC3339(oldestQueuedAt),
			"oldest_running_at": formatMaybeRFC3339(oldestRunningAt),
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	payload, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func formatMaybeRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func runMemoryPosteriorRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory posterior-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	platform := fs.String("platform", "", "source platform")
	externalID := fs.String("id", "", "source external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidScopedMemorySourceRequest(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory posterior-run --user <user_id> [--platform <platform> --id <external_id>]")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.RunPosteriorVerification(context.Background(), memory.PosteriorRunRequest{
		UserID:           strings.TrimSpace(*userID),
		SourcePlatform:   strings.TrimSpace(*platform),
		SourceExternalID: strings.TrimSpace(*externalID),
	}, currentUTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryOrganizeRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory organize-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory organize-run --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), strings.TrimSpace(*userID), currentUTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryOrganized(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory organized", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	platform := fs.String("platform", "", "source platform")
	externalID := fs.String("id", "", "source external id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidRequiredMemorySource(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory organized --user <user_id> --platform <platform> --id <external_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetLatestMemoryOrganizationOutput(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID))
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Fprintf(stderr, "no memory output yet; run: varix memory organize-run --user %s\n", strings.TrimSpace(*userID))
			return 1
		}
		if errors.Is(err, contentstore.ErrMemoryOrganizationOutputStale) {
			fmt.Fprintf(stderr, "%v; run: varix memory organize-run --user %s\n", err, strings.TrimSpace(*userID))
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryGlobalOrganizeRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-organize-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-organize-run --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.RunGlobalMemoryOrganization(context.Background(), strings.TrimSpace(*userID), currentUTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryGlobalOrganized(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-organized", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-organized --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryGlobalV2OrganizeRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-v2-organize-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-v2-organize-run --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), strings.TrimSpace(*userID), currentUTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryGlobalV2Organized(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-v2-organized", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-v2-organized --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		if err == sql.ErrNoRows {
			writeMissingMemoryAction(stderr, "no v2 global memory output yet", "varix memory global-v2-organize-run", strings.TrimSpace(*userID))
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryGlobalCard(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-card", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-card --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprint(stdout, formatGlobalClusterCards(out))
	return 0
}

func runMemoryGlobalV2Card(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-v2-card", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute v2 output before rendering")
	itemType := fs.String("item-type", "", "optional filter: card, conclusion, or conflict")
	limit := fs.Int("limit", 0, "optional max number of top items to render")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-v2-card --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedItemType := strings.TrimSpace(*itemType)
	if !isGlobalV2ItemType(trimmedItemType) {
		fmt.Fprintln(stderr, globalV2ItemTypeUsage)
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	var out memory.GlobalMemoryV2Output
	if *runNow {
		out, err = store.RunGlobalMemoryOrganizationV2(context.Background(), trimmedUserID, currentUTC())
	} else {
		out, err = store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), trimmedUserID)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			writeMissingMemoryAction(stderr, "no v2 card output yet", "varix memory global-v2-card --run", trimmedUserID)
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	filtered := filterGlobalV2Items(out, trimmedItemType)
	filtered = limitGlobalV2Items(filtered, *limit)
	if trimmedItemType != "" && len(filtered.TopMemoryItems) == 0 {
		fmt.Fprintf(stdout, "Items (0, filter=%s)\n\nNo %s items for user %s\n", trimmedItemType, trimmedItemType, trimmedUserID)
		return 0
	}
	fmt.Fprint(stdout, formatGlobalV2Cards(filtered, trimmedItemType))
	return 0
}

func runMemoryGlobalCompare(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute both v1 and v2 outputs before comparing")
	limit := fs.Int("limit", 0, "optional max number of v1 and v2 items to show")
	itemType := fs.String("item-type", "", "optional filter for v2 side: card, conclusion, or conflict")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-compare --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedItemType := strings.TrimSpace(*itemType)
	now := currentUTC()
	if !isGlobalV2ItemType(trimmedItemType) {
		fmt.Fprintln(stderr, globalV2ItemTypeUsage)
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()

	var v1 memory.GlobalOrganizationOutput
	var v2 memory.GlobalMemoryV2Output
	if *runNow {
		v1, err = store.RunGlobalMemoryOrganization(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		v2, err = store.RunGlobalMemoryOrganizationV2(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		v1, err = store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), trimmedUserID)
		if err != nil {
			if err == sql.ErrNoRows {
				writeMissingMemoryAction(stderr, "no global memory outputs yet", "varix memory global-compare --run", trimmedUserID)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		v2, err = store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), trimmedUserID)
		if err != nil {
			if err == sql.ErrNoRows {
				writeMissingMemoryAction(stderr, "no global memory outputs yet", "varix memory global-compare --run", trimmedUserID)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprint(stdout, formatGlobalCompare(limitGlobalOrganizationOutput(v1, *limit), limitGlobalV2Items(filterGlobalV2Items(v2, trimmedItemType), *limit), trimmedItemType))
	return 0
}

func formatGlobalClusterCards(out memory.GlobalOrganizationOutput) string {
	var b strings.Builder
	nodeText := map[string]string{}
	for _, node := range out.ActiveNodes {
		nodeText[node.NodeID] = node.NodeText
	}
	for _, node := range out.InactiveNodes {
		nodeText[node.NodeID] = node.NodeText
	}
	for i, cluster := range out.Clusters {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "Cluster\n%s\n\n", cluster.CanonicalProposition)
		if strings.TrimSpace(cluster.Summary) != "" {
			fmt.Fprintf(&b, "Summary\n%s\n\n", cluster.Summary)
		}
		writeNodeSection(&b, "Why", cluster.CoreSupportingNodeIDs, nodeText)
		writeNodeSection(&b, "Conditions", cluster.CoreConditionalNodeIDs, nodeText)
		writeNodeSection(&b, "Current judgment", cluster.CoreConclusionNodeIDs, nodeText)
		writeNodeSection(&b, "What next", cluster.CorePredictiveNodeIDs, nodeText)
		if len(cluster.ConflictingNodeIDs) > 0 {
			writeNodeSection(&b, "Conflicts", cluster.ConflictingNodeIDs, nodeText)
		}
		if len(cluster.SynthesizedEdges) > 0 {
			fmt.Fprintf(&b, "Logic\n")
			for _, edge := range cluster.SynthesizedEdges {
				fmt.Fprintf(&b, "- %s --%s--> %s\n", resolveNodeLabel(edge.From, nodeText), edge.Kind, resolveNodeLabel(edge.To, nodeText))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func formatGlobalV2Cards(out memory.GlobalMemoryV2Output, itemType string) string {
	var b strings.Builder
	if strings.TrimSpace(itemType) != "" {
		fmt.Fprintf(&b, "Items (%d, filter=%s)\n\n", len(out.TopMemoryItems), strings.TrimSpace(itemType))
	} else {
		fmt.Fprintf(&b, "Items\n%d\n\n", len(out.TopMemoryItems))
	}
	cardByID := map[string]memory.CognitiveCard{}
	for _, card := range out.CognitiveCards {
		cardByID[card.CardID] = card
	}
	for i, item := range out.TopMemoryItems {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "%s\n%s\n\n", strings.Title(string(item.ItemType)), item.Headline)
		if strings.TrimSpace(string(item.SignalStrength)) != "" {
			fmt.Fprintf(&b, "Signal\n%s\n\n", item.SignalStrength)
		}
		if strings.TrimSpace(item.Subheadline) != "" {
			fmt.Fprintf(&b, "Summary\n%s\n\n", item.Subheadline)
		}
		if item.ItemType == memory.TopMemoryItemConflict {
			for _, conflict := range out.ConflictSets {
				if conflict.ConflictID != item.BackingObjectID {
					continue
				}
				writeStringSection(&b, "Side A", []string{conflict.SideASummary})
				writeStringSection(&b, "Side B", []string{conflict.SideBSummary})
				writeStringSection(&b, "Why A", conflict.SideAWhy)
				writeStringSection(&b, "Why B", conflict.SideBWhy)
				writeStringSection(&b, "Sources A", conflict.SideASourceRefs)
				writeStringSection(&b, "Sources B", conflict.SideBSourceRefs)
			}
			continue
		}
		if item.ItemType == memory.TopMemoryItemCard {
			card, ok := cardByID[item.BackingObjectID]
			if !ok {
				continue
			}
			writeLogicSection(&b, card.CausalChain)
			writeStringSection(&b, "Mechanism", cardMechanismTexts(card))
			writeStringSection(&b, "Why", card.KeyEvidence)
			writeStringSection(&b, "Conditions", card.Conditions)
			writeStringSection(&b, "What next", card.Predictions)
			writeStringSection(&b, "Sources", card.SourceRefs)
			continue
		}
		for _, conclusion := range out.CognitiveConclusions {
			if conclusion.ConclusionID != item.BackingObjectID {
				continue
			}
			for _, cardID := range conclusion.BackingCardIDs {
				card, ok := cardByID[cardID]
				if !ok {
					continue
				}
				writeLogicSection(&b, card.CausalChain)
				writeStringSection(&b, "Mechanism", cardMechanismTexts(card))
				writeStringSection(&b, "Why", card.KeyEvidence)
				writeStringSection(&b, "Conditions", card.Conditions)
				writeStringSection(&b, "What next", card.Predictions)
				writeStringSection(&b, "Sources", card.SourceRefs)
			}
		}
	}
	return b.String()
}

func filterGlobalV2Items(out memory.GlobalMemoryV2Output, itemType string) memory.GlobalMemoryV2Output {
	itemType = strings.TrimSpace(itemType)
	if itemType == "" {
		return out
	}
	filtered := out
	filtered.TopMemoryItems = nil
	for _, item := range out.TopMemoryItems {
		if item.ItemType == memory.TopMemoryItemType(itemType) {
			filtered.TopMemoryItems = append(filtered.TopMemoryItems, item)
		}
	}
	return filtered
}

func limitGlobalV2Items(out memory.GlobalMemoryV2Output, limit int) memory.GlobalMemoryV2Output {
	if limit <= 0 || len(out.TopMemoryItems) <= limit {
		return out
	}
	limited := out
	limited.TopMemoryItems = append([]memory.TopMemoryItem(nil), out.TopMemoryItems[:limit]...)
	return limited
}

func limitGlobalOrganizationOutput(out memory.GlobalOrganizationOutput, limit int) memory.GlobalOrganizationOutput {
	if limit <= 0 || len(out.Clusters) <= limit {
		return out
	}
	limited := out
	limited.Clusters = append([]memory.GlobalCluster(nil), out.Clusters[:limit]...)
	return limited
}

func isGlobalV2ItemType(itemType string) bool {
	switch itemType {
	case "", "card", "conclusion", "conflict":
		return true
	default:
		return false
	}
}

func writeMissingMemoryAction(w io.Writer, message, command, userID string) {
	fmt.Fprintf(w, "%s; run: %s --user %s\n", message, command, userID)
}

func invalidScopedMemorySourceRequest(userID, platform, externalID string) bool {
	return strings.TrimSpace(userID) == "" || (strings.TrimSpace(externalID) != "" && strings.TrimSpace(platform) == "")
}

func invalidRequiredMemorySource(userID, platform, externalID string) bool {
	return strings.TrimSpace(userID) == "" || !hasContentTarget(platform, externalID)
}

func formatGlobalCompare(v1 memory.GlobalOrganizationOutput, v2 memory.GlobalMemoryV2Output, itemType string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "V1 cluster-first (%d)\n", len(v1.Clusters))
	for _, cluster := range v1.Clusters {
		fmt.Fprintf(&b, "- %s\n", cluster.CanonicalProposition)
		if strings.TrimSpace(cluster.Summary) != "" {
			fmt.Fprintf(&b, "  summary: %s\n", cluster.Summary)
		}
	}
	if strings.TrimSpace(itemType) != "" {
		fmt.Fprintf(&b, "\nV2 thesis-first (%d, filter=%s)\n", len(v2.TopMemoryItems), strings.TrimSpace(itemType))
	} else {
		fmt.Fprintf(&b, "\nV2 thesis-first (%d)\n", len(v2.TopMemoryItems))
	}
	if strings.TrimSpace(itemType) != "" && len(v2.TopMemoryItems) == 0 {
		fmt.Fprintf(&b, "No %s items\n", strings.TrimSpace(itemType))
		return b.String()
	}
	for _, item := range v2.TopMemoryItems {
		fmt.Fprintf(&b, "- %s: %s\n", item.ItemType, item.Headline)
		if strings.TrimSpace(item.Subheadline) != "" {
			fmt.Fprintf(&b, "  summary: %s\n", item.Subheadline)
		}
	}
	return b.String()
}

func writeStringSection(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s\n", item)
	}
	b.WriteString("\n")
}

func writeLogicSection(b *strings.Builder, steps []memory.CardChainStep) {
	if len(steps) == 0 {
		return
	}
	fmt.Fprintf(b, "Logic\n")
	for _, step := range steps {
		if strings.TrimSpace(step.Label) == "" {
			continue
		}
		fmt.Fprintf(b, "- %s (%s)\n", step.Label, step.Role)
	}
	b.WriteString("\n")
}

func cardMechanismTexts(card memory.CognitiveCard) []string {
	items := make([]string, 0)
	for _, step := range card.CausalChain {
		if step.Role == "mechanism" && strings.TrimSpace(step.Label) != "" {
			items = append(items, step.Label)
		}
	}
	return uniqueStringSlice(items)
}

func uniqueStringSlice(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func writeNodeSection(b *strings.Builder, title string, ids []string, nodeText map[string]string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, id := range ids {
		fmt.Fprintf(b, "- %s\n", resolveNodeLabel(id, nodeText))
	}
	b.WriteString("\n")
}

func resolveNodeLabel(id string, nodeText map[string]string) string {
	if text := strings.TrimSpace(nodeText[id]); text != "" {
		return text
	}
	return id
}

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

func runMemorySubjectTimeline(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory subject-timeline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	subject := fs.String("subject", "", "subject or alias")
	card := fs.Bool("card", false, "render a readable subject timeline card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*subject) == "" {
		fmt.Fprintln(stderr, "usage: varix memory subject-timeline --user <user_id> --subject <subject>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	timeline, err := store.BuildSubjectTimeline(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject), currentUTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		fmt.Fprint(stdout, formatSubjectTimelineCard(timeline))
		return 0
	}
	payload, err := json.MarshalIndent(timeline, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemorySubjectHorizon(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory subject-horizon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	subject := fs.String("subject", "", "subject or alias")
	horizon := fs.String("horizon", "1w", "horizon: 1w, 1m, 1q, 1y, 2y, 5y")
	refresh := fs.Bool("refresh", false, "force recomputing this subject horizon memory")
	card := fs.Bool("card", false, "render a readable subject horizon card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*subject) == "" {
		fmt.Fprintln(stderr, "usage: varix memory subject-horizon --user <user_id> --subject <subject> --horizon <1w|1m|1q|1y|2y|5y>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetSubjectHorizonMemory(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject), strings.TrimSpace(*horizon), currentUTC(), *refresh)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		fmt.Fprint(stdout, formatSubjectHorizonCard(out))
		return 0
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemorySubjectExperience(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory subject-experience", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	subject := fs.String("subject", "", "subject or alias")
	horizons := fs.String("horizons", "1w,1m,1q,1y,2y,5y", "comma-separated horizons: 1w,1m,1q,1y,2y,5y")
	refresh := fs.Bool("refresh", false, "force recomputing subject horizon inputs and experience memory")
	card := fs.Bool("card", false, "render a readable subject experience card view")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*subject) == "" {
		fmt.Fprintln(stderr, "usage: varix memory subject-experience --user <user_id> --subject <subject> --horizons <1w,1m,1q,1y,2y,5y>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetSubjectExperienceMemory(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject), splitMemoryHorizons(*horizons), currentUTC(), *refresh)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if *card {
		fmt.Fprint(stdout, formatSubjectExperienceCard(out))
		return 0
	}
	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func splitMemoryHorizons(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func formatSubjectTimelineCard(timeline memory.SubjectTimeline) string {
	var b strings.Builder
	subject := strings.TrimSpace(timeline.CanonicalSubject)
	if subject == "" {
		subject = strings.TrimSpace(timeline.Subject)
	}
	fmt.Fprintf(&b, "Subject Timeline\n- Subject: %s\n- Changes: %d\n", subject, len(timeline.Entries))
	if strings.TrimSpace(timeline.Summary) != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", timeline.Summary)
	}
	for _, entry := range timeline.Entries {
		when := subjectTimelineCardWhen(entry)
		if when == "" {
			when = "timeless"
		}
		fmt.Fprintf(&b, "\nChange\n- When: %s\n- Change: %s\n- Role: %s primary=%t\n- Relation: %s\n- Structure: %s\n- Source: %s:%s#%s\n", when, entry.ChangeText, entry.GraphRole, entry.IsPrimary, entry.RelationToPrior, entry.Structure, entry.SourcePlatform, entry.SourceExternalID, entry.NodeID)
		if strings.TrimSpace(entry.VerificationStatus) != "" {
			fmt.Fprintf(&b, "- Verification: %s", entry.VerificationStatus)
			if strings.TrimSpace(entry.VerificationReason) != "" {
				fmt.Fprintf(&b, " (%s)", entry.VerificationReason)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func subjectTimelineCardWhen(entry memory.SubjectChangeEntry) string {
	return firstNonEmpty(entry.TimeStart, entry.TimeEnd, entry.VerificationAsOf, entry.SourceCompiledAt, entry.SourceUpdatedAt, entry.TimeText, entry.TimeBucket)
}

func formatSubjectHorizonCard(out memory.SubjectHorizonMemory) string {
	var b strings.Builder
	subject := firstNonEmpty(strings.TrimSpace(out.CanonicalSubject), strings.TrimSpace(out.Subject))
	fmt.Fprintf(&b, "Subject Horizon\n- Subject: %s\n- Horizon: %s\n- Window: %s -> %s\n- Policy: %s next=%s\n- Cache: %s\n- Changes: %d sources=%d\n", subject, out.Horizon, out.WindowStart, out.WindowEnd, out.RefreshPolicy, out.NextRefreshAt, out.CacheStatus, out.SampleCount, out.SourceCount)
	if strings.TrimSpace(out.DominantPattern) != "" {
		fmt.Fprintf(&b, "- Pattern: %s\n", out.DominantPattern)
	}
	if strings.TrimSpace(out.Abstraction) != "" {
		fmt.Fprintf(&b, "- Abstraction: %s\n", out.Abstraction)
	}
	if len(out.DriverClusters) > 0 {
		driverParts := make([]string, 0, len(out.DriverClusters))
		for _, driver := range out.DriverClusters {
			driverParts = append(driverParts, fmt.Sprintf("%s(%d)", driver.Subject, driver.Count))
		}
		fmt.Fprintf(&b, "- Key factors: %s\n", strings.Join(driverParts, ", "))
	}
	for _, change := range out.KeyChanges {
		fmt.Fprintf(&b, "\nChange\n- When: %s\n- Change: %s\n- Relation: %s\n- Source: %s:%s#%s\n", firstNonEmpty(change.When, "timeless"), change.ChangeText, change.RelationToPrior, change.SourcePlatform, change.SourceExternalID, change.NodeID)
	}
	b.WriteString("\n")
	return b.String()
}

func formatSubjectExperienceCard(out memory.SubjectExperienceMemory) string {
	var b strings.Builder
	subject := firstNonEmpty(strings.TrimSpace(out.CanonicalSubject), strings.TrimSpace(out.Subject))
	fmt.Fprintf(&b, "主体归因总结\n- 主体: %s\n- 观察窗口: %s\n", subject, formatRecentHorizons(out.Horizons))
	if out.AttributionSummary.ChangeCount > 0 || out.AttributionSummary.FactorCount > 0 {
		fmt.Fprintf(&b, "- 变化数: %d\n- 因素数: %d\n", out.AttributionSummary.ChangeCount, out.AttributionSummary.FactorCount)
		if scope := subjectAttributionEvidenceScope(out); scope != "" {
			fmt.Fprintf(&b, "- 证据范围: %s\n", scope)
		}
		if note := subjectAttributionHorizonNote(out); note != "" {
			fmt.Fprintf(&b, "- 窗口提示: %s\n", note)
		}
	}
	if len(out.HorizonSummaries) > 0 {
		for _, summary := range out.HorizonSummaries {
			fmt.Fprintf(&b, "- %s: 样本=%d 趋势=%s 波动=%s", summary.Horizon, summary.SampleCount, summary.TrendDirection, summary.VolatilityState)
			if len(summary.TopDrivers) > 0 {
				fmt.Fprintf(&b, " 关键因素=%s", strings.Join(summary.TopDrivers, ", "))
			}
			b.WriteString("\n")
		}
	}
	if out.AttributionSummary.ChangeCount > 0 || strings.TrimSpace(out.AttributionSummary.PrimaryFactor.Subject) != "" {
		fmt.Fprintf(&b, "\n归因总结\n")
		if strings.TrimSpace(out.AttributionSummary.PrimaryFactor.Subject) != "" {
			primary := out.AttributionSummary.PrimaryFactor
			fmt.Fprintf(&b, "- 当前样本主要因素: %s（%d 个来源，support=%d", primary.Subject, primary.SourceCount, primary.Support)
			if len(primary.Horizons) > 0 {
				fmt.Fprintf(&b, "，%s", strings.Join(primary.Horizons, "/"))
			}
			b.WriteString("）\n")
			if strings.TrimSpace(primary.Reason) != "" {
				fmt.Fprintf(&b, "- 判断: %s\n", primary.Reason)
			}
		}
		if len(out.AttributionSummary.ChangeAttributions) > 0 {
			b.WriteString("- 变化归因:\n")
			for _, item := range out.AttributionSummary.ChangeAttributions {
				when := item.When
				if len(when) >= len("2006-01-02") {
					when = when[:len("2006-01-02")]
				}
				if strings.TrimSpace(when) == "" {
					when = "时间未知"
				}
				factors := "暂无"
				if len(item.Factors) > 0 {
					factors = strings.Join(item.Factors, ", ")
				}
				fmt.Fprintf(&b, "  - %s %s <= %s\n", when, item.ChangeText, factors)
			}
		}
		if len(out.AttributionSummary.FactorRelations) > 0 {
			b.WriteString("- 因素关系:\n")
			for _, relation := range out.AttributionSummary.FactorRelations {
				factors := relation.Factors
				if len(factors) == 0 {
					factors = []string{relation.Left, relation.Right}
				}
				fmt.Fprintf(&b, "  - %s: %s（%d 个来源）\n", strings.Join(factors, " + "), relation.Relation, relation.SourceCount)
			}
		} else {
			b.WriteString("- 因素关系: 暂无多因素共同变化\n")
		}
		return b.String()
	}
	for _, lesson := range out.Lessons {
		fmt.Fprintf(&b, "\n经验\n- 类型: %s\n- 结论: %s\n- 触发条件: %s\n", subjectExperienceKindLabel(lesson.Kind), lesson.Statement, lesson.Trigger)
		if strings.TrimSpace(lesson.Mechanism) != "" {
			fmt.Fprintf(&b, "- 作用机制: %s\n", lesson.Mechanism)
		}
		if strings.TrimSpace(lesson.Boundary) != "" {
			fmt.Fprintf(&b, "- 适用边界: %s\n", lesson.Boundary)
		}
		if strings.TrimSpace(lesson.TransferRule) != "" {
			fmt.Fprintf(&b, "- 迁移判断: %s\n", lesson.TransferRule)
		}
		fmt.Fprintf(&b, "- 置信度: %.2f support=%d\n", lesson.Confidence, lesson.SupportCount)
		if len(lesson.Horizons) > 0 {
			fmt.Fprintf(&b, "- 时间尺度: %s\n", strings.Join(lesson.Horizons, ", "))
		}
		if len(lesson.DriverSubjects) > 0 {
			fmt.Fprintf(&b, "- 关键因素: %s\n", strings.Join(lesson.DriverSubjects, ", "))
		}
		if len(lesson.EvidenceSourceRefs) > 0 {
			fmt.Fprintf(&b, "- 证据来源: %s\n", strings.Join(formatExperienceEvidenceRefs(lesson.EvidenceSourceRefs), ", "))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func subjectAttributionEvidenceScope(out memory.SubjectExperienceMemory) string {
	items := out.AttributionSummary.ChangeAttributions
	if len(items) == 0 {
		return ""
	}
	start := strings.TrimSpace(items[0].When)
	end := start
	for _, item := range items[1:] {
		when := strings.TrimSpace(item.When)
		if when == "" {
			continue
		}
		if start == "" || when < start {
			start = when
		}
		if end == "" || when > end {
			end = when
		}
	}
	start = datePrefix(start)
	end = datePrefix(end)
	if start == "" && end == "" {
		return fmt.Sprintf("仅基于当前 %d 条变化样本", len(items))
	}
	if start == end {
		return fmt.Sprintf("%s，基于当前 %d 条变化样本", start, len(items))
	}
	return fmt.Sprintf("%s 至 %s，基于当前 %d 条变化样本", start, end, len(items))
}

func subjectAttributionHorizonNote(out memory.SubjectExperienceMemory) string {
	if len(out.Horizons) <= 1 || len(out.HorizonSummaries) <= 1 {
		return ""
	}
	if attributionHorizonsUseSameSamples(out) {
		return fmt.Sprintf("%s 目前使用同一批样本，只能代表这批证据，不能推出更长窗口的主导因素。", formatRecentHorizons(out.Horizons))
	}
	return "不同观察窗口的主导因素需要分别读取，不能把较短窗口的结论直接套到较长窗口。"
}

func formatRecentHorizons(horizons []string) string {
	if len(horizons) == 0 {
		return ""
	}
	out := make([]string, 0, len(horizons))
	for _, horizon := range horizons {
		horizon = strings.TrimSpace(horizon)
		if horizon == "" {
			continue
		}
		out = append(out, "最近 "+horizon)
	}
	return strings.Join(out, ", ")
}

func attributionHorizonsUseSameSamples(out memory.SubjectExperienceMemory) bool {
	if len(out.HorizonSummaries) == 0 {
		return false
	}
	want := out.HorizonSummaries[0].SampleCount
	if want == 0 {
		return false
	}
	for _, summary := range out.HorizonSummaries[1:] {
		if summary.SampleCount != want {
			return false
		}
	}
	return want == out.AttributionSummary.ChangeCount
}

func datePrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return value
}

func subjectExperienceKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "driver-pattern":
		return "可复用解释因素"
	case "horizon-shift":
		return "时间尺度变化"
	default:
		return strings.TrimSpace(kind)
	}
}

func formatExperienceEvidenceRefs(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		source := strings.TrimSpace(ref)
		if before, _, ok := strings.Cut(source, "#"); ok {
			source = before
		}
		if source != "" {
			out = append(out, source)
		}
	}
	return uniqueCLIStrings(out)
}

func uniqueCLIStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
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

func runMemoryCleanupStale(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory cleanup-stale", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	olderThan := fs.Duration("older-than", 24*time.Hour, "delete queued/running jobs older than this duration")
	platform := fs.String("platform", "", "optional source platform filter")
	externalID := fs.String("id", "", "optional source external id filter (requires --platform)")
	dryRun := fs.Bool("dry-run", false, "report stale jobs without deleting them")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if invalidScopedMemorySourceRequest(*userID, *platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix memory cleanup-stale --user <user_id> [--older-than 24h] [--platform <platform> --id <external_id>]")
		return 2
	}
	if *olderThan <= 0 {
		fmt.Fprintln(stderr, "--older-than must be positive")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	cutoff := currentUTC().Add(-*olderThan)
	var deleted int64
	if *dryRun {
		deleted, err = store.CountStaleMemoryJobs(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), cutoff)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		deleted, err = store.CleanupStaleMemoryJobs(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), cutoff)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	payload, err := json.MarshalIndent(map[string]any{
		"ok":            true,
		"user":          strings.TrimSpace(*userID),
		"deleted_jobs":  deleted,
		"dry_run":       *dryRun,
		"cutoff_before": cutoff.Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
}

func runMemoryCanonicalEntities(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory canonical-entities", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entityID := fs.String("id", "", "optional canonical entity id filter")
	alias := fs.String("alias", "", "optional alias lookup filter")
	entityType := fs.String("type", "", "optional filter: driver | target | both")
	status := fs.String("status", "", "optional filter: active | merged | split | retired")
	card := fs.Bool("card", false, "render a readable canonical entity view")
	summary := fs.Bool("summary", false, "print aggregate counts instead of full entities")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	var items []memory.CanonicalEntity
	if strings.TrimSpace(*entityID) != "" {
		entity, err := store.GetCanonicalEntity(context.Background(), strings.TrimSpace(*entityID))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		items = []memory.CanonicalEntity{entity}
	} else if strings.TrimSpace(*alias) != "" {
		entity, err := store.FindCanonicalEntityByAlias(context.Background(), strings.TrimSpace(*alias))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		items = []memory.CanonicalEntity{entity}
	} else {
		items, err = store.ListCanonicalEntities(context.Background())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if strings.TrimSpace(*entityType) != "" {
		filtered := make([]memory.CanonicalEntity, 0, len(items))
		for _, item := range items {
			if string(item.EntityType) == strings.TrimSpace(*entityType) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if strings.TrimSpace(*status) != "" {
		filtered := make([]memory.CanonicalEntity, 0, len(items))
		for _, item := range items {
			if string(item.Status) == strings.TrimSpace(*status) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if *summary {
		byType := map[string]int{}
		byStatus := map[string]int{}
		totalAliases := 0
		for _, item := range items {
			byType[string(item.EntityType)]++
			byStatus[string(item.Status)]++
			totalAliases += len(item.Aliases)
		}
		payload, err := json.MarshalIndent(map[string]any{
			"total_entities": len(items),
			"total_aliases":  totalAliases,
			"by_type":        byType,
			"by_status":      byStatus,
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, string(payload))
		return 0
	}
	if *card {
		if len(items) == 0 {
			fmt.Fprintln(stdout, "No canonical entities matched")
			return 0
		}
		var b strings.Builder
		for _, item := range items {
			fmt.Fprintf(&b, "Canonical Entity\n- entity_id: %s\n- canonical_name: %s\n- type: %s\n- status: %s\n", item.EntityID, item.CanonicalName, item.EntityType, item.Status)
			if len(item.Aliases) > 0 {
				fmt.Fprintf(&b, "- aliases: %s\n", strings.Join(item.Aliases, ", "))
			}
			b.WriteString("\n")
		}
		fmt.Fprint(stdout, b.String())
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

func runMemoryCanonicalEntityUpsert(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory canonical-entity-upsert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entityID := fs.String("id", "", "canonical entity id")
	entityType := fs.String("type", "", "driver | target | both")
	name := fs.String("name", "", "canonical display name")
	aliasesRaw := fs.String("aliases", "", "optional comma-separated aliases")
	status := fs.String("status", string(memory.CanonicalEntityActive), "active | merged | split | retired")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	entityIDValue := strings.TrimSpace(*entityID)
	entityTypeValue := strings.TrimSpace(*entityType)
	nameValue := strings.TrimSpace(*name)
	statusValue := strings.TrimSpace(*status)
	aliasesValue := strings.TrimSpace(*aliasesRaw)
	if entityIDValue == "" || entityTypeValue == "" || nameValue == "" {
		fmt.Fprintln(stderr, "usage: varix memory canonical-entity-upsert --id <entity_id> --type <driver|target|both> --name <canonical_name> [--aliases a,b]")
		return 2
	}
	var typ memory.CanonicalEntityType
	switch entityTypeValue {
	case string(memory.CanonicalEntityDriver):
		typ = memory.CanonicalEntityDriver
	case string(memory.CanonicalEntityTarget):
		typ = memory.CanonicalEntityTarget
	case string(memory.CanonicalEntityBoth):
		typ = memory.CanonicalEntityBoth
	default:
		fmt.Fprintln(stderr, "--type must be one of: driver, target, both")
		return 2
	}
	var entityStatus memory.CanonicalEntityStatus
	switch statusValue {
	case string(memory.CanonicalEntityActive):
		entityStatus = memory.CanonicalEntityActive
	case string(memory.CanonicalEntityMerged):
		entityStatus = memory.CanonicalEntityMerged
	case string(memory.CanonicalEntitySplit):
		entityStatus = memory.CanonicalEntitySplit
	case string(memory.CanonicalEntityRetired):
		entityStatus = memory.CanonicalEntityRetired
	default:
		fmt.Fprintln(stderr, "--status must be one of: active, merged, split, retired")
		return 2
	}
	aliases := make([]string, 0)
	for _, part := range strings.Split(aliasesValue, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			aliases = append(aliases, trimmed)
		}
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	entity := memory.CanonicalEntity{
		EntityID:      entityIDValue,
		EntityType:    typ,
		CanonicalName: nameValue,
		Aliases:       aliases,
		Status:        entityStatus,
	}
	if err := store.UpsertCanonicalEntity(context.Background(), entity); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(map[string]any{"ok": true, "entity_id": entity.EntityID}, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
