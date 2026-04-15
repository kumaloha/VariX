package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type compileClient interface {
	Compile(ctx context.Context, bundle c.Bundle) (c.Record, error)
}

var buildCompileClient = func(projectRoot string) compileClient {
	return c.NewClientFromConfig(projectRoot, nil)
}

var openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
	return contentstore.NewSQLiteStore(path)
}

func runCompileCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: varix compile <run|show> ...")
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
		fmt.Fprintln(stderr, "usage: varix compile <run|show|summary|compare|card> ...")
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
	timeout := fs.Duration("timeout", 10*time.Minute, "compile timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}
	if strings.TrimSpace(*rawURL) == "" && (strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "") {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	c.EnableFactWebVerification()
	client := buildCompileClient(projectRoot)
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()

	if !*force {
		switch {
		case strings.TrimSpace(*rawURL) != "":
			if parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL); err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
				if record, err := store.GetCompiledOutput(ctx, string(parsed.Platform), parsed.PlatformID); err == nil {
					payload, marshalErr := json.MarshalIndent(record, "", "  ")
					if marshalErr != nil {
						fmt.Fprintln(stderr, marshalErr)
						return 1
					}
					fmt.Fprintln(stdout, string(payload))
					return 0
				}
			}
		case strings.TrimSpace(*platform) != "" && strings.TrimSpace(*externalID) != "":
			if record, err := store.GetCompiledOutput(ctx, *platform, *externalID); err == nil {
				payload, marshalErr := json.MarshalIndent(record, "", "  ")
				if marshalErr != nil {
					fmt.Fprintln(stderr, marshalErr)
					return 1
				}
				fmt.Fprintln(stdout, string(payload))
				return 0
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
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	bundle := c.BuildBundle(raw)
	record, err := client.Compile(ctx, bundle)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := store.UpsertCompiledOutput(ctx, record); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*rawURL) != "" {
		parsed, err := app.Dispatcher.ParseURL(context.Background(), *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*platform = string(parsed.Platform)
		*externalID = parsed.PlatformID
	}
	if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix compile show --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, string(payload))
	return 0
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
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*rawURL) != "" {
		parsed, err := app.Dispatcher.ParseURL(context.Background(), *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*platform = string(parsed.Platform)
		*externalID = parsed.PlatformID
	}
	if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix compile summary --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintf(stdout, "Summary: %s\n", record.Output.Summary)
	fmt.Fprintf(stdout, "Nodes: %d\n", len(record.Output.Graph.Nodes))
	fmt.Fprintf(stdout, "Edges: %d\n", len(record.Output.Graph.Edges))
	if len(record.Output.Topics) > 0 {
		fmt.Fprintf(stdout, "Topics: %s\n", strings.Join(record.Output.Topics, ", "))
	}
	fmt.Fprintf(stdout, "Confidence: %s\n", record.Output.Confidence)
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
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*rawURL) != "" {
		parsed, err := app.Dispatcher.ParseURL(context.Background(), *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*platform = string(parsed.Platform)
		*externalID = parsed.PlatformID
	}
	if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix compile compare --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	raw, err := store.GetRawCapture(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	rawPreview := raw.Content
	if len(rawPreview) > 500 {
		rawPreview = rawPreview[:500]
	}
	fmt.Fprintf(stdout, "Raw preview: %s\n", rawPreview)
	fmt.Fprintf(stdout, "Summary: %s\n", record.Output.Summary)
	fmt.Fprintf(stdout, "Nodes: %d\n", len(record.Output.Graph.Nodes))
	fmt.Fprintf(stdout, "Edges: %d\n", len(record.Output.Graph.Edges))
	fmt.Fprintf(stdout, "Confidence: %s\n", record.Output.Confidence)
	return 0
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
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if strings.TrimSpace(*rawURL) != "" {
		parsed, err := app.Dispatcher.ParseURL(context.Background(), *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		*platform = string(parsed.Platform)
		*externalID = parsed.PlatformID
	}
	if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "" {
		fmt.Fprintln(stderr, "usage: varix compile card --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()
	record, err := store.GetCompiledOutput(context.Background(), *platform, *externalID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if *compact {
		fmt.Fprint(stdout, formatCompactCompileCard(record))
		return 0
	}
	fmt.Fprint(stdout, formatCompileCard(record))
	return 0
}

func formatCompileCard(record c.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", record.Output.Summary)
	if len(record.Output.Topics) > 0 {
		fmt.Fprintf(&b, "Topics\n- %s\n\n", strings.Join(record.Output.Topics, "\n- "))
	}
	if chains := logicChains(record); len(chains) > 0 {
		fmt.Fprintf(&b, "Logic chain\n")
		for _, chain := range chains {
			fmt.Fprintf(&b, "- %s\n", chain)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", record.Output.Confidence)
	return b.String()
}

func formatCompactCompileCard(record c.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary\n%s\n\n", record.Output.Summary)
	writeCompactNodeSection(&b, "Facts", pickNodesByKinds(record, 2, c.NodeFact))
	writeCompactNodeSection(&b, "Conditions", pickConditionPoints(record, 3))
	writeCompactNodeSection(&b, "Conclusions", pickNodesByKinds(record, 2, c.NodeConclusion))
	writeCompactNodeSection(&b, "Predictions", pickPredictionPoints(record, 2))
	if chains := logicChains(record); len(chains) > 0 {
		fmt.Fprintf(&b, "Main logic\n- %s\n\n", chains[0])
	}
	fmt.Fprintf(&b, "Confidence\n%s\n", record.Output.Confidence)
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

func pickNodesByKinds(record c.Record, max int, kinds ...c.NodeKind) []string {
	allowed := map[c.NodeKind]struct{}{}
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}
	out := make([]string, 0, max)
	for _, node := range record.Output.Graph.Nodes {
		if strings.TrimSpace(node.Text) == "" {
			continue
		}
		if _, ok := allowed[node.Kind]; !ok {
			continue
		}
		out = append(out, truncate(node.Text, 100))
		if len(out) == max {
			break
		}
	}
	return out
}

func pickConditionPoints(record c.Record, max int) []string {
	explicitStatus := map[string]string{}
	for _, check := range record.Output.Verification.ExplicitConditionChecks {
		explicitStatus[check.NodeID] = string(check.Status)
	}
	implicitStatus := map[string]string{}
	for _, check := range record.Output.Verification.ImplicitConditionChecks {
		implicitStatus[check.NodeID] = string(check.Status)
	}
	out := make([]string, 0, max)
	for _, node := range record.Output.Graph.Nodes {
		if strings.TrimSpace(node.Text) == "" {
			continue
		}
		switch node.Kind {
		case c.NodeExplicitCondition:
			label := "[显]"
			if status := strings.TrimSpace(explicitStatus[node.ID]); status != "" {
				label = "[显|" + status + "]"
			}
			out = append(out, label+" "+truncate(node.Text, 100))
		case c.NodeImplicitCondition:
			label := "[隐]"
			if status := strings.TrimSpace(implicitStatus[node.ID]); status != "" {
				label = "[隐|" + status + "]"
			}
			out = append(out, label+" "+truncate(node.Text, 100))
		default:
			continue
		}
		if len(out) == max {
			break
		}
	}
	return out
}

func pickPredictionPoints(record c.Record, max int) []string {
	statusByNode := map[string]string{}
	for _, check := range record.Output.Verification.PredictionChecks {
		statusByNode[check.NodeID] = string(check.Status)
	}
	out := make([]string, 0, max)
	for _, node := range record.Output.Graph.Nodes {
		if node.Kind != c.NodePrediction || strings.TrimSpace(node.Text) == "" {
			continue
		}
		label := "[预]"
		if status := strings.TrimSpace(statusByNode[node.ID]); status != "" {
			label = "[预|" + status + "]"
		}
		out = append(out, label+" "+truncate(node.Text, 100))
		if len(out) == max {
			break
		}
	}
	return out
}

func logicChains(record c.Record) []string {
	if len(record.Output.Graph.Edges) == 0 || len(record.Output.Graph.Nodes) == 0 {
		return nil
	}
	nodeText := map[string]string{}
	for _, node := range record.Output.Graph.Nodes {
		nodeText[node.ID] = node.Text
	}
	outgoing := map[string][]c.GraphEdge{}
	inDegree := map[string]int{}
	for _, edge := range record.Output.Graph.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge)
		inDegree[edge.To]++
		if _, ok := inDegree[edge.From]; !ok {
			inDegree[edge.From] = inDegree[edge.From]
		}
	}
	visited := map[string]bool{}
	var chains []string
	for _, edge := range record.Output.Graph.Edges {
		if visited[edge.From+"->"+edge.To+"#"+string(edge.Kind)] {
			continue
		}
		if inDegree[edge.From] == 1 {
			continue
		}
		chain := formatChain(edge, outgoing, inDegree, visited, nodeText)
		if chain != "" {
			chains = append(chains, chain)
		}
	}
	for _, edge := range record.Output.Graph.Edges {
		key := edge.From + "->" + edge.To + "#" + string(edge.Kind)
		if visited[key] {
			continue
		}
		chain := formatChain(edge, outgoing, inDegree, visited, nodeText)
		if chain != "" {
			chains = append(chains, chain)
		}
	}
	return chains
}

func formatChain(start c.GraphEdge, outgoing map[string][]c.GraphEdge, inDegree map[string]int, visited map[string]bool, nodeText map[string]string) string {
	parts := []string{truncate(nodeText[start.From], 50)}
	current := start
	for {
		key := current.From + "->" + current.To + "#" + string(current.Kind)
		visited[key] = true
		parts = append(parts, fmt.Sprintf("--%s-->", current.Kind), truncate(nodeText[current.To], 50))
		nextEdges := outgoing[current.To]
		if len(nextEdges) != 1 || inDegree[current.To] != 1 {
			break
		}
		next := nextEdges[0]
		nextKey := next.From + "->" + next.To + "#" + string(next.Kind)
		if visited[nextKey] {
			break
		}
		current = next
	}
	return strings.Join(parts, " ")
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}
