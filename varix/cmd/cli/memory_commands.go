package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/memory"
)

func runMemoryCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: varix memory <accept|accept-batch|list|show-source|jobs|organize-run|organized|global-organize-run|global-organized|global-v2-organize-run|global-v2-organized|global-card|global-v2-card|global-compare> ...")
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
	case "jobs":
		return runMemoryJobs(args[1:], projectRoot, stdout, stderr)
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
	default:
		fmt.Fprintln(stderr, "usage: varix memory <accept|accept-batch|list|show-source|jobs|organize-run|organized|global-organize-run|global-organized|global-v2-organize-run|global-v2-organized|global-card|global-v2-card|global-compare> ...")
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
	itemType := fs.String("item-type", "", "optional filter: conclusion or conflict")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-v2-card --user <user_id>")
		return 2
	}
	if trimmed := strings.TrimSpace(*itemType); trimmed != "" && trimmed != "conclusion" && trimmed != "conflict" {
		fmt.Fprintln(stderr, "item-type must be one of: conclusion, conflict")
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
	if strings.TrimSpace(*itemType) != "" && len(filtered.TopMemoryItems) == 0 {
		fmt.Fprintf(stdout, "No %s items for user %s\n", strings.TrimSpace(*itemType), strings.TrimSpace(*userID))
		return 0
	}
	fmt.Fprint(stdout, formatGlobalV2Cards(filtered))
	return 0
}

func runMemoryGlobalCompare(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-compare --user <user_id>")
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

	v1, err := store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	v2, err := store.GetLatestGlobalMemoryOrganizationV2Output(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprint(stdout, formatGlobalCompare(v1, v2))
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

func formatGlobalV2Cards(out memory.GlobalMemoryV2Output) string {
	var b strings.Builder
	cardByID := map[string]memory.CognitiveCard{}
	for _, card := range out.CognitiveCards {
		cardByID[card.CardID] = card
	}
	for i, item := range out.TopMemoryItems {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "%s\n%s\n\n", strings.Title(item.ItemType), item.Headline)
		if strings.TrimSpace(item.SignalStrength) != "" {
			fmt.Fprintf(&b, "Signal\n%s\n\n", item.SignalStrength)
		}
		if strings.TrimSpace(item.Subheadline) != "" {
			fmt.Fprintf(&b, "Summary\n%s\n\n", item.Subheadline)
		}
		if item.ItemType == "conflict" {
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
		if item.ItemType == itemType {
			filtered.TopMemoryItems = append(filtered.TopMemoryItems, item)
		}
	}
	return filtered
}

func formatGlobalCompare(v1 memory.GlobalOrganizationOutput, v2 memory.GlobalMemoryV2Output) string {
	var b strings.Builder
	b.WriteString("V1 cluster-first\n")
	for _, cluster := range v1.Clusters {
		fmt.Fprintf(&b, "- %s\n", cluster.CanonicalProposition)
		if strings.TrimSpace(cluster.Summary) != "" {
			fmt.Fprintf(&b, "  summary: %s\n", cluster.Summary)
		}
	}
	b.WriteString("\nV2 thesis-first\n")
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
