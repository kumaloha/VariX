package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

const globalSynthesisItemTypeUsage = "item-type must be one of: card, conclusion, conflict"

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

func runMemoryGlobalSynthesisOrganizeRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-synthesis-run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-synthesis-run --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.RunGlobalMemorySynthesis(context.Background(), strings.TrimSpace(*userID), currentUTC())
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

func runMemoryGlobalSynthesisOrganized(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-synthesis", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-synthesis --user <user_id>")
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	out, err := store.GetLatestGlobalMemorySynthesisOutput(context.Background(), strings.TrimSpace(*userID))
	if err != nil {
		if err == sql.ErrNoRows {
			writeMissingMemoryAction(stderr, "no global synthesis memory output yet", "varix memory global-synthesis-run", strings.TrimSpace(*userID))
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

func runMemoryGlobalSynthesisCard(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-synthesis-card", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute synthesis output before rendering")
	itemType := fs.String("item-type", "", "optional filter: card, conclusion, or conflict")
	limit := fs.Int("limit", 0, "optional max number of top items to render")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*userID) == "" {
		fmt.Fprintln(stderr, "usage: varix memory global-synthesis-card --user <user_id>")
		return 2
	}
	trimmedUserID := strings.TrimSpace(*userID)
	trimmedItemType := strings.TrimSpace(*itemType)
	if !isGlobalSynthesisItemType(trimmedItemType) {
		fmt.Fprintln(stderr, globalSynthesisItemTypeUsage)
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	var out memory.GlobalMemorySynthesisOutput
	if *runNow {
		out, err = store.RunGlobalMemorySynthesis(context.Background(), trimmedUserID, currentUTC())
	} else {
		out, err = store.GetLatestGlobalMemorySynthesisOutput(context.Background(), trimmedUserID)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			writeMissingMemoryAction(stderr, "no synthesis card output yet", "varix memory global-synthesis-card --run", trimmedUserID)
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	filtered := filterGlobalSynthesisItems(out, trimmedItemType)
	filtered = limitGlobalSynthesisItems(filtered, *limit)
	if trimmedItemType != "" && len(filtered.TopMemoryItems) == 0 {
		fmt.Fprintf(stdout, "Items (0, filter=%s)\n\nNo %s items for user %s\n", trimmedItemType, trimmedItemType, trimmedUserID)
		return 0
	}
	fmt.Fprint(stdout, formatGlobalSynthesisCards(filtered, trimmedItemType))
	return 0
}

func runMemoryGlobalCompare(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory global-compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	userID := fs.String("user", "", "user id")
	runNow := fs.Bool("run", false, "recompute both cluster and synthesis outputs before comparing")
	limit := fs.Int("limit", 0, "optional max number of cluster and synthesis items to show")
	itemType := fs.String("item-type", "", "optional filter for synthesis side: card, conclusion, or conflict")
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
	if !isGlobalSynthesisItemType(trimmedItemType) {
		fmt.Fprintln(stderr, globalSynthesisItemTypeUsage)
		return 2
	}
	store, err := openStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()

	var cluster memory.GlobalOrganizationOutput
	var synthesis memory.GlobalMemorySynthesisOutput
	if *runNow {
		cluster, err = store.RunGlobalMemoryOrganization(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		synthesis, err = store.RunGlobalMemorySynthesis(context.Background(), trimmedUserID, now)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		cluster, err = store.GetLatestGlobalMemoryOrganizationOutput(context.Background(), trimmedUserID)
		if err != nil {
			if err == sql.ErrNoRows {
				writeMissingMemoryAction(stderr, "no global memory outputs yet", "varix memory global-compare --run", trimmedUserID)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		synthesis, err = store.GetLatestGlobalMemorySynthesisOutput(context.Background(), trimmedUserID)
		if err != nil {
			if err == sql.ErrNoRows {
				writeMissingMemoryAction(stderr, "no global memory outputs yet", "varix memory global-compare --run", trimmedUserID)
				return 1
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprint(stdout, formatGlobalCompare(limitGlobalOrganizationOutput(cluster, *limit), limitGlobalSynthesisItems(filterGlobalSynthesisItems(synthesis, trimmedItemType), *limit), trimmedItemType))
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

func formatGlobalSynthesisCards(out memory.GlobalMemorySynthesisOutput, itemType string) string {
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

func filterGlobalSynthesisItems(out memory.GlobalMemorySynthesisOutput, itemType string) memory.GlobalMemorySynthesisOutput {
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

func limitGlobalSynthesisItems(out memory.GlobalMemorySynthesisOutput, limit int) memory.GlobalMemorySynthesisOutput {
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

func isGlobalSynthesisItemType(itemType string) bool {
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

func formatGlobalCompare(cluster memory.GlobalOrganizationOutput, synthesis memory.GlobalMemorySynthesisOutput, itemType string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cluster-first (%d)\n", len(cluster.Clusters))
	for _, item := range cluster.Clusters {
		fmt.Fprintf(&b, "- %s\n", item.CanonicalProposition)
		if strings.TrimSpace(item.Summary) != "" {
			fmt.Fprintf(&b, "  summary: %s\n", item.Summary)
		}
	}
	if strings.TrimSpace(itemType) != "" {
		fmt.Fprintf(&b, "\nSynthesis (%d, filter=%s)\n", len(synthesis.TopMemoryItems), strings.TrimSpace(itemType))
	} else {
		fmt.Fprintf(&b, "\nSynthesis (%d)\n", len(synthesis.TopMemoryItems))
	}
	if strings.TrimSpace(itemType) != "" && len(synthesis.TopMemoryItems) == 0 {
		fmt.Fprintf(&b, "No %s items\n", strings.TrimSpace(itemType))
		return b.String()
	}
	for _, item := range synthesis.TopMemoryItems {
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
