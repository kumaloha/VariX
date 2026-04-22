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
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
	"github.com/kumaloha/VariX/varix/graphmodel"
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

const compileCommandUsage = "usage: varix compile <run|show|summary|compare|card> ..."

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
		fmt.Fprintln(stderr, compileCommandUsage)
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
		fmt.Fprintln(stderr, compileCommandUsage)
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
	setRawURLFromArg(fs, rawURL)
	if strings.TrimSpace(*rawURL) == "" && !hasContentTarget(*platform, *externalID) {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	app, store, err := openAppStore(projectRoot)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	defer store.Close()
	if !*noVerify {
		c.EnableFactWebVerification()
	}
	client, err := selectCompileClient(projectRoot, *pipeline, *noVerify, *noValidate)
	if err != nil {
		writeErr(stderr, err)
		return 2
	}
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !*force {
		switch {
		case strings.TrimSpace(*rawURL) != "":
			if parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL); err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
				if record, err := store.GetCompiledOutput(ctx, string(parsed.Platform), parsed.PlatformID); err == nil {
					return writeJSON(stdout, stderr, record)
				}
			}
		case hasContentTarget(*platform, *externalID):
			if record, err := store.GetCompiledOutput(ctx, *platform, *externalID); err == nil {
				return writeJSON(stdout, stderr, record)
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
			writeErr(stderr, err)
			return 1
		}
	}

	bundle := c.BuildBundle(raw)
	compileStart := time.Now()
	record, err := client.Compile(ctx, bundle)
	if err != nil {
		writeErr(stderr, err)
		return 1
	}
	if record.Metrics.CompileElapsedMS <= 0 {
		record.Metrics.CompileElapsedMS = time.Since(compileStart).Milliseconds()
		if record.Metrics.CompileElapsedMS <= 0 {
			record.Metrics.CompileElapsedMS = 1
		}
	}
	if err := store.UpsertCompiledOutput(ctx, record); err != nil {
		writeErr(stderr, err)
		return 1
	}
	return writeJSON(stdout, stderr, record)
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
		fmt.Fprintln(stderr, "usage: varix compile show --url <url> | --platform <platform> --id <external_id>")
		return 2
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
		fmt.Fprintln(stderr, "usage: varix compile summary --url <url> | --platform <platform> --id <external_id>")
		return 2
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
		fmt.Fprintln(stderr, "usage: varix compile compare --url <url> | --platform <platform> --id <external_id>")
		return 2
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

func writeHumanReadableCompileMetrics(w io.Writer, record c.Record) {
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
	Evidence            []string
	Explanations        []string
	LogicChains         []string
	VerificationSummary []string
}

func buildCompileCardProjection(record c.Record, subgraph *graphmodel.ContentSubgraph) compileCardProjection {
	projection := compileCardProjection{
		Summary:      record.Output.Summary,
		Topics:       cloneStringSlice(record.Output.Topics),
		Confidence:   record.Output.Confidence,
		Drivers:      cloneStringSlice(record.Output.Drivers),
		Targets:      cloneStringSlice(record.Output.Targets),
		Evidence:     cloneStringSlice(record.Output.EvidenceNodes),
		Explanations: cloneStringSlice(record.Output.ExplanationNodes),
		LogicChains:  legacyLogicChains(record),
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
	if len(projection.LogicChains) > 0 {
		fmt.Fprintf(&b, "Main logic\n- %s\n\n", projection.LogicChains[0])
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", projection.Confidence)
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

func legacyLogicChains(record c.Record) []string {
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
