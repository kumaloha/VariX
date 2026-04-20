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

func runMemoryCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: varix memory <accept|accept-batch|list|show-source|content-graphs|jobs|posterior-run|organize-run|organized|global-organize-run|global-organized|global-v2-organize-run|global-v2-organized|global-card|global-v2-card|global-compare|event-graphs|event-evidence|paradigms|paradigm-evidence|project-all> ...")
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
	default:
		fmt.Fprintln(stderr, "usage: varix memory <accept|accept-batch|list|show-source|content-graphs|jobs|posterior-run|organize-run|organized|global-organize-run|global-organized|global-v2-organize-run|global-v2-organized|global-card|global-v2-card|global-compare|event-graphs|event-evidence|paradigms|paradigm-evidence|project-all> ...")
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
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" || strings.TrimSpace(*nodeID) == "" {
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
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" || len(nodeIDs) == 0 {
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
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory show-source --user <user_id> --platform <platform> --id <external_id>")
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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory jobs --user <user_id>")
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
	jobs, err := store.ListMemoryJobs(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
	if strings.TrimSpace(*userID) == "" || (strings.TrimSpace(*externalID) != "" && strings.TrimSpace(*platform) == "") {
		fmt.Fprintln(stderr, "usage: varix memory posterior-run --user <user_id> [--platform <platform> --id <external_id>]")
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
	out, err := store.RunPosteriorVerification(context.Background(), memory.PosteriorRunRequest{
		UserID:           strings.TrimSpace(*userID),
		SourcePlatform:   strings.TrimSpace(*platform),
		SourceExternalID: strings.TrimSpace(*externalID),
	}, time.Now().UTC())
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
	out, err := store.RunNextMemoryOrganizationJob(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
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
	if strings.TrimSpace(*userID) == "" || strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory organized --user <user_id> --platform <platform> --id <external_id>")
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
	out, err := store.RunGlobalMemoryOrganization(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
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
	out, err := store.RunGlobalMemoryOrganizationV2(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
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
	out, err := store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Fprintf(stderr, "no v2 global memory output yet; run: varix memory global-v2-organize-run --user %s\n", strings.TrimSpace(*userID))
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
	if trimmed := strings.TrimSpace(*itemType); trimmed != "" && trimmed != "card" && trimmed != "conclusion" && trimmed != "conflict" {
		fmt.Fprintln(stderr, "item-type must be one of: card, conclusion, conflict")
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
	var out memory.GlobalMemoryV2Output
	if *runNow {
		out, err = store.RunGlobalMemoryOrganizationV2(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
	} else {
		out, err = store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), strings.TrimSpace(*userID))
	}
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Fprintf(stderr, "no v2 card output yet; run: varix memory global-v2-card --run --user %s\n", strings.TrimSpace(*userID))
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	filtered := filterGlobalV2Items(out, strings.TrimSpace(*itemType))
	filtered = limitGlobalV2Items(filtered, *limit)
	if strings.TrimSpace(*itemType) != "" && len(filtered.TopMemoryItems) == 0 {
		fmt.Fprintf(stdout, "Items (0, filter=%s)\n\nNo %s items for user %s\n", strings.TrimSpace(*itemType), strings.TrimSpace(*itemType), strings.TrimSpace(*userID))
		return 0
	}
	fmt.Fprint(stdout, formatGlobalV2Cards(filtered, strings.TrimSpace(*itemType)))
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
	if trimmed := strings.TrimSpace(*itemType); trimmed != "" && trimmed != "card" && trimmed != "conclusion" && trimmed != "conflict" {
		fmt.Fprintln(stderr, "item-type must be one of: card, conclusion, conflict")
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

	var v1 memory.GlobalOrganizationOutput
	var v2 memory.GlobalMemoryV2Output
	if *runNow {
		v1, err = store.RunGlobalMemoryOrganization(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		v2, err = store.RunGlobalMemoryOrganizationV2(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		v1, err = store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), strings.TrimSpace(*userID))
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Fprintf(stderr, "no global memory outputs yet; run: varix memory global-compare --run --user %s\n", strings.TrimSpace(*userID))
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		v2, err = store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), strings.TrimSpace(*userID))
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Fprintf(stderr, "no global memory outputs yet; run: varix memory global-compare --run --user %s\n", strings.TrimSpace(*userID))
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprint(stdout, formatGlobalCompare(limitGlobalOrganizationOutput(v1, *limit), limitGlobalV2Items(filterGlobalV2Items(v2, strings.TrimSpace(*itemType)), *limit), strings.TrimSpace(*itemType)))
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
	if *runNow {
		if _, err := store.RunEventGraphProjection(context.Background(), strings.TrimSpace(*userID), time.Now().UTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListEventGraphs(context.Background(), strings.TrimSpace(*userID))
	if err == nil && strings.TrimSpace(*scope) != "" {
		filtered := make([]contentstore.EventGraphRecord, 0, len(items))
		for _, item := range items {
			if item.Scope == strings.TrimSpace(*scope) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err == nil && strings.TrimSpace(*subject) != "" {
		filtered := make([]contentstore.EventGraphRecord, 0, len(items))
		for _, item := range items {
			if item.AnchorSubject == strings.TrimSpace(*subject) {
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
	if *runNow {
		if _, err := store.RunParadigmProjection(context.Background(), strings.TrimSpace(*userID), time.Now().UTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListParadigms(context.Background(), strings.TrimSpace(*userID))
	if err == nil && strings.TrimSpace(*subject) != "" {
		items, err = store.ListParadigmsBySubject(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*subject))
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
	if *runNow {
		if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
			fmt.Fprintln(stderr, "usage: varix memory content-graphs --run --user <user_id> --platform <platform> --id <external_id>")
			return 2
		}
		if err := store.PersistMemoryContentGraphFromCompiledOutput(context.Background(), strings.TrimSpace(*userID), strings.TrimSpace(*platform), strings.TrimSpace(*externalID), time.Now().UTC()); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	items, err := store.ListMemoryContentGraphs(context.Background(), strings.TrimSpace(*userID))
	if err == nil && strings.TrimSpace(*platform) != "" && strings.TrimSpace(*externalID) != "" {
		filtered := make([]graphmodel.ContentSubgraph, 0, len(items))
		for _, item := range items {
			if item.SourcePlatform == strings.TrimSpace(*platform) && item.SourceExternalID == strings.TrimSpace(*externalID) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if err == nil && strings.TrimSpace(*subject) != "" {
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
		if len(item.RepresentativeChanges) > 0 {
			fmt.Fprintf(&b, "- Representative changes: %s\n", strings.Join(item.RepresentativeChanges, ", "))
		}
		fmt.Fprintf(&b, "- Verification: %v\n\n", item.VerificationSummary)
	}
	return b.String()
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
	events, err := store.RunEventGraphProjection(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	paradigms, err := store.RunParadigmProjection(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	global, err := store.RunGlobalMemoryOrganizationV2(context.Background(), strings.TrimSpace(*userID), time.Now().UTC())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	contentGraphs, err := store.ListMemoryContentGraphs(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(map[string]any{"ok": true, "content_graphs": len(contentGraphs), "event_graphs": len(events), "paradigms": len(paradigms), "global_v2": global.OutputID}, "", "  ")
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
