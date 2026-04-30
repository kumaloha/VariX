package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
)

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
	setRawURLFromArg(fs, rawURL)

	app, store, err := openAppStore(projectRoot)
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
	if !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile card --url <url> | --platform <platform> --id <external_id>")
		return 2
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	var subgraph *graphmodel.ContentSubgraph
	if graph, graphErr := store.GetContentSubgraph(context.Background(), *platform, *externalID); graphErr == nil {
		subgraph = &graph
	} else if !errors.Is(graphErr, sql.ErrNoRows) {
		fmt.Fprintln(stderr, graphErr)
		return 1
	}
	projection := buildCompileCardProjection(record, subgraph)

	if *compact {
		fmt.Fprint(stdout, formatCompactCompileCard(projection))
		return 0
	}
	fmt.Fprint(stdout, formatCompileCard(projection))
	return 0
}

type compileCardProjection struct {
	Summary             string
	Topics              []string
	Confidence          string
	Drivers             []string
	Targets             []string
	Branches            []c.Branch
	Evidence            []string
	Explanations        []string
	LogicChains         []string
	VerificationSummary []string
	AuthorValidation    []string
}

func buildCompileCardProjection(record c.Record, subgraph *graphmodel.ContentSubgraph) compileCardProjection {
	projection := compileCardProjection{
		Summary:          record.Output.Summary,
		Topics:           cloneStringSlice(record.Output.Topics),
		Confidence:       record.Output.Confidence,
		Drivers:          cloneStringSlice(record.Output.Drivers),
		Targets:          cloneStringSlice(record.Output.Targets),
		Branches:         cloneBranches(record.Output.Branches),
		Evidence:         cloneStringSlice(record.Output.EvidenceNodes),
		Explanations:     cloneStringSlice(record.Output.ExplanationNodes),
		LogicChains:      legacyLogicChains(record),
		AuthorValidation: authorValidationSummaryLines(record.Output.AuthorValidation),
	}
	if subgraph == nil {
		return projection
	}
	if drivers := graphFirstNodeSection(*subgraph, func(node graphmodel.GraphNode) bool {
		return node.IsPrimary && node.GraphRole == graphmodel.GraphRoleDriver
	}); len(drivers) > 0 {
		projection.Drivers = preferGraphFirstSection(projection.Drivers, drivers)
	}
	if targets := graphFirstNodeSection(*subgraph, func(node graphmodel.GraphNode) bool {
		return node.IsPrimary && node.GraphRole == graphmodel.GraphRoleTarget
	}); len(targets) > 0 {
		projection.Targets = preferGraphFirstSection(projection.Targets, targets)
	}
	if evidence := graphFirstEvidenceSection(*subgraph); len(evidence) > 0 {
		projection.Evidence = preferGraphFirstSection(projection.Evidence, evidence)
	}
	if explanations := graphFirstExplanationSection(*subgraph); len(explanations) > 0 {
		projection.Explanations = preferGraphFirstSection(projection.Explanations, explanations)
	}
	if chains := graphFirstLogicChains(*subgraph); len(chains) > 0 {
		projection.LogicChains = preferGraphFirstLogicChains(projection.LogicChains, chains)
	}
	if verification := graphFirstVerificationSummary(*subgraph); len(verification) > 0 {
		projection.VerificationSummary = verification
	}
	return projection
}

func formatCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	if len(projection.Topics) > 0 {
		fmt.Fprintf(&b, "Topics\n- %s\n\n", strings.Join(projection.Topics, "\n- "))
	}
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 5))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 5))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 5))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 5))
	writeBranchSection(&b, projection.Branches, 5)
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Logic chain\n")
		for _, chain := range projection.LogicChains {
			fmt.Fprintf(&b, "- %s\n", chain)
		}
		b.WriteString("\n")
	}
	if len(projection.VerificationSummary) > 0 {
		fmt.Fprintf(&b, "Verification\n")
		for _, line := range projection.VerificationSummary {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	if len(projection.AuthorValidation) > 0 {
		fmt.Fprintf(&b, "Author validation\n")
		for _, line := range projection.AuthorValidation {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
	return b.String()
}

func formatCompactCompileCard(projection compileCardProjection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", projection.Summary)
	writeCompactNodeSection(&b, "Drivers", truncateList(projection.Drivers, 3))
	writeCompactNodeSection(&b, "Targets", truncateList(projection.Targets, 3))
	writeCompactNodeSection(&b, "Evidence", truncateList(projection.Evidence, 3))
	writeCompactNodeSection(&b, "Explanations", truncateList(projection.Explanations, 2))
	writeBranchSection(&b, projection.Branches, 2)
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Main logic\n- %s\n\n", projection.LogicChains[0])
	}
	if len(projection.AuthorValidation) > 0 {
		fmt.Fprintf(&b, "Author validation\n")
		for _, line := range truncateList(projection.AuthorValidation, 3) {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
	return b.String()
}

func authorValidationSummaryLines(validation c.AuthorValidation) []string {
	if validation.IsZero() {
		return nil
	}
	summary := validation.Summary
	lines := []string{
		"Verdict: " + firstNonEmpty(summary.Verdict, "insufficient_evidence"),
		fmt.Sprintf("Claims: supported %d, contradicted %d, unverified %d, interpretive %d", summary.SupportedClaims, summary.ContradictedClaims, summary.UnverifiedClaims, summary.InterpretiveClaims),
		fmt.Sprintf("Inferences: sound %d, weak %d, unsupported %d", summary.SoundInferences, summary.WeakInferences, summary.UnsupportedInferences),
	}
	if summary.NotAuthorClaims > 0 || summary.NotAuthorInferences > 0 {
		lines = append(lines, fmt.Sprintf("Not author claims/inferences: %d/%d", summary.NotAuthorClaims, summary.NotAuthorInferences))
	}
	for _, check := range validation.ClaimChecks {
		if check.Status != c.AuthorClaimContradicted && check.Status != c.AuthorClaimUnverified && check.Status != c.AuthorClaimNotAuthorClaim {
			continue
		}
		lines = append(lines, fmt.Sprintf("Claim %s: %s%s", truncate(check.Text, 42), check.Status, authorValidationNoteSuffix(firstNonEmpty(check.DecisionNote, check.Reason))))
		if len(lines) >= 6 {
			break
		}
	}
	for _, check := range validation.InferenceChecks {
		if check.Status != c.AuthorInferenceWeak && check.Status != c.AuthorInferenceUnsupportedJump && check.Status != c.AuthorInferenceNotAuthorInference {
			continue
		}
		lines = append(lines, fmt.Sprintf("Path %s -> %s: %s%s", truncate(check.From, 24), truncate(check.To, 24), check.Status, authorValidationNoteSuffix(firstNonEmpty(check.DecisionNote, check.Reason))))
		if len(lines) >= 6 {
			break
		}
	}
	return lines
}

func authorValidationNoteSuffix(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return " — 说明: " + truncate(note, 160)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func writeBranchSection(b *strings.Builder, branches []c.Branch, limit int) {
	if len(branches) == 0 || limit <= 0 {
		return
	}
	if len(branches) < limit {
		limit = len(branches)
	}
	fmt.Fprintf(b, "Branches\n")
	for _, branch := range branches[:limit] {
		label := strings.TrimSpace(branch.Thesis)
		if label == "" {
			label = strings.TrimSpace(branch.ID)
		}
		if label == "" {
			label = "branch"
		}
		fmt.Fprintf(b, "- %s\n", label)
		if len(branch.Anchors) > 0 {
			fmt.Fprintf(b, "  - Anchor: %s\n", strings.Join(truncateList(branch.Anchors, 3), " / "))
		}
		if len(branch.BranchDrivers) > 0 {
			fmt.Fprintf(b, "  - Branch driver: %s\n", strings.Join(truncateList(branch.BranchDrivers, 3), " / "))
		}
		for _, chain := range branchLogicChains(branch) {
			fmt.Fprintf(b, "  - %s\n", chain)
		}
	}
	b.WriteString("\n")
}

func branchLogicChains(branch c.Branch) []string {
	if len(branch.TransmissionPaths) == 0 {
		return nil
	}
	chains := make([]string, 0, len(branch.TransmissionPaths))
	for _, path := range branch.TransmissionPaths {
		parts := []string{}
		parts = appendChainPart(parts, path.Driver)
		for _, step := range path.Steps {
			parts = appendChainPart(parts, step)
		}
		parts = appendChainPart(parts, path.Target)
		if len(parts) > 0 {
			chains = append(chains, strings.Join(parts, " -> "))
		}
	}
	return chains
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

func cloneBranches(values []c.Branch) []c.Branch {
	out := make([]c.Branch, 0, len(values))
	for _, branch := range values {
		branch.Anchors = cloneStringSlice(branch.Anchors)
		branch.BranchDrivers = cloneStringSlice(branch.BranchDrivers)
		branch.Drivers = cloneStringSlice(branch.Drivers)
		branch.Targets = cloneStringSlice(branch.Targets)
		branch.TransmissionPaths = cloneTransmissionPaths(branch.TransmissionPaths)
		out = append(out, branch)
	}
	return out
}

func cloneTransmissionPaths(values []c.TransmissionPath) []c.TransmissionPath {
	out := make([]c.TransmissionPath, 0, len(values))
	for _, path := range values {
		path.Steps = cloneStringSlice(path.Steps)
		out = append(out, path)
	}
	return out
}

func legacyLogicChains(record c.Record) []string {
	if len(record.Output.TransmissionPaths) == 0 {
		return nil
	}
	chains := make([]string, 0, len(record.Output.TransmissionPaths))
	for _, path := range record.Output.TransmissionPaths {
		parts := []string{}
		parts = appendChainPart(parts, path.Driver)
		for _, step := range path.Steps {
			parts = appendChainPart(parts, step)
		}
		parts = appendChainPart(parts, path.Target)
		if len(parts) > 0 {
			chains = append(chains, strings.Join(parts, " -> "))
		}
	}
	return chains
}

func appendChainPart(parts []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return parts
	}
	value = truncate(value, 50)
	if len(parts) > 0 && parts[len(parts)-1] == value {
		return parts
	}
	return append(parts, value)
}

func graphFirstNodeSection(subgraph graphmodel.ContentSubgraph, keep func(graphmodel.GraphNode) bool) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, node := range subgraph.Nodes {
		if !keep(node) {
			continue
		}
		label := strings.TrimSpace(graphFirstNodeLabel(node))
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}

func graphFirstEvidenceSection(subgraph graphmodel.ContentSubgraph) []string {
	out := graphFirstNodeSection(subgraph, func(node graphmodel.GraphNode) bool {
		return node.GraphRole == graphmodel.GraphRoleEvidence
	})
	if len(out) > 0 {
		return out
	}
	byID := graphNodeIndex(subgraph)
	for _, edge := range subgraph.Edges {
		if edge.Type != graphmodel.EdgeTypeSupports {
			continue
		}
		if node, ok := byID[edge.From]; ok {
			out = appendUniqueString(out, graphFirstNodeLabel(node))
		}
	}
	return out
}

func graphFirstExplanationSection(subgraph graphmodel.ContentSubgraph) []string {
	out := graphFirstNodeSection(subgraph, func(node graphmodel.GraphNode) bool {
		return node.GraphRole == graphmodel.GraphRoleContext
	})
	byID := graphNodeIndex(subgraph)
	for _, edge := range subgraph.Edges {
		if edge.Type != graphmodel.EdgeTypeExplains {
			continue
		}
		if node, ok := byID[edge.From]; ok {
			out = appendUniqueString(out, graphFirstNodeLabel(node))
		}
	}
	return out
}

func graphFirstLogicChains(subgraph graphmodel.ContentSubgraph) []string {
	nodeByID := graphNodeIndex(subgraph)
	primaryDriveAdj := map[string][]string{}
	primaryDriveNodes := map[string]struct{}{}
	for _, edge := range subgraph.Edges {
		if edge.Type != graphmodel.EdgeTypeDrives {
			continue
		}
		if !edge.IsPrimary {
			continue
		}
		primaryDriveAdj[edge.From] = append(primaryDriveAdj[edge.From], edge.To)
		primaryDriveNodes[edge.From] = struct{}{}
		primaryDriveNodes[edge.To] = struct{}{}
	}
	if len(primaryDriveAdj) == 0 {
		return nil
	}
	for from := range primaryDriveAdj {
		sort.Strings(primaryDriveAdj[from])
	}
	starts := make([]string, 0)
	targets := map[string]struct{}{}
	for _, node := range subgraph.Nodes {
		if !node.IsPrimary {
			continue
		}
		if node.GraphRole == graphmodel.GraphRoleDriver {
			starts = append(starts, node.ID)
		}
		if node.GraphRole == graphmodel.GraphRoleTarget {
			targets[node.ID] = struct{}{}
		}
	}
	sort.Strings(starts)
	chains := make([]string, 0)
	seen := map[string]struct{}{}
	for _, start := range starts {
		graphFirstCollectPaths(start, primaryDriveAdj, targets, nodeByID, nil, map[string]bool{}, &chains, seen)
	}
	return chains
}

func graphFirstCollectPaths(current string, adj map[string][]string, targets map[string]struct{}, nodeByID map[string]graphmodel.GraphNode, path []string, visiting map[string]bool, out *[]string, seen map[string]struct{}) {
	if visiting[current] {
		return
	}
	node, ok := nodeByID[current]
	if !ok {
		return
	}
	label := graphFirstNodeLabel(node)
	if strings.TrimSpace(label) == "" {
		return
	}
	path = append(path, truncate(label, 50))
	if _, isTarget := targets[current]; isTarget {
		chain := strings.Join(path, " -> ")
		if _, ok := seen[chain]; !ok {
			seen[chain] = struct{}{}
			*out = append(*out, chain)
		}
	}
	nexts := adj[current]
	if len(nexts) == 0 {
		return
	}
	visiting[current] = true
	for _, next := range nexts {
		graphFirstCollectPaths(next, adj, targets, nodeByID, path, visiting, out, seen)
	}
	delete(visiting, current)
}

func graphNodeIndex(subgraph graphmodel.ContentSubgraph) map[string]graphmodel.GraphNode {
	out := make(map[string]graphmodel.GraphNode, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		out[node.ID] = node
	}
	return out
}

func graphFirstNodeLabel(node graphmodel.GraphNode) string {
	rawText := strings.TrimSpace(node.RawText)
	sourceQuote := strings.TrimSpace(node.SourceQuote)
	subjectText := strings.TrimSpace(node.SubjectText)
	changeText := strings.TrimSpace(node.ChangeText)
	switch {
	case rawText != "":
		return rawText
	case sourceQuote != "":
		return sourceQuote
	case c.HasDistinctNonEmptyPair(subjectText, changeText):
		return subjectText + " " + changeText
	default:
		return strings.TrimSpace(c.FirstNonEmpty(subjectText, changeText, node.ID))
	}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func graphFirstChainRichness(chains []string) int {
	best := 0
	for _, chain := range chains {
		parts := strings.Split(chain, "->")
		if len(parts) > best {
			best = len(parts)
		}
	}
	return best
}

func graphFirstSectionRichness(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func preferGraphFirstSection(legacy, graph []string) []string {
	if graphFirstSectionRichness(graph) >= graphFirstSectionRichness(legacy) {
		return graph
	}
	return legacy
}

func preferGraphFirstLogicChains(legacy, graph []string) []string {
	if graphFirstChainRichness(graph) >= graphFirstChainRichness(legacy) {
		return graph
	}
	return legacy
}

func graphFirstVerificationSummary(subgraph graphmodel.ContentSubgraph) []string {
	nodeCounts := map[graphmodel.VerificationStatus]int{}
	edgeCounts := map[graphmodel.VerificationStatus]int{}
	for _, node := range subgraph.Nodes {
		nodeCounts[node.VerificationStatus]++
	}
	for _, edge := range subgraph.Edges {
		status := edge.VerificationStatus
		if status == "" {
			status = graphmodel.VerificationPending
		}
		edgeCounts[status]++
	}
	out := make([]string, 0, 2)
	if len(nodeCounts) > 0 {
		out = append(out, "Nodes: "+formatVerificationCounts(nodeCounts))
	}
	if len(edgeCounts) > 0 {
		out = append(out, "Edges: "+formatVerificationCounts(edgeCounts))
	}
	return out
}

func formatVerificationCounts(counts map[graphmodel.VerificationStatus]int) string {
	parts := make([]string, 0, 4)
	for _, status := range []graphmodel.VerificationStatus{
		graphmodel.VerificationPending,
		graphmodel.VerificationProved,
		graphmodel.VerificationDisproved,
		graphmodel.VerificationUnverifiable,
	} {
		if counts[status] == 0 {
			continue
		}
		parts = append(parts, string(status)+"="+fmt.Sprintf("%d", counts[status]))
	}
	return strings.Join(parts, ", ")
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
