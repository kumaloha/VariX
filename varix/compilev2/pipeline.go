package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
)

type graphRole string

const (
	roleDriver             graphRole = "driver"
	roleTransmission       graphRole = "transmission"
	roleTargetCandidate    graphRole = "target_candidate"
	roleTarget             graphRole = "target"
	roleOrphan             graphRole = "orphan"
)

type graphNode struct {
	ID          string
	Text        string
	SourceQuote string
	Role        graphRole
	Ontology    string
}

type graphEdge struct {
	From string
	To   string
}

type offGraphItem struct {
	ID          string
	Text        string
	Role        string
	AttachesTo  string
	SourceQuote string
}

type graphState struct {
	Nodes    []graphNode
	Edges    []graphEdge
	OffGraph []offGraphItem
	Rounds   int
}

func stage1Extract(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle) (graphState, error) {
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage1SystemPrompt, fmt.Sprintf(stage1UserPrompt, bundle.TextContext()))
	if err != nil {
		return graphState{}, err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return graphState{}, err
	}
	var payload map[string]any
	if err := parseJSONObject(resp.Text, &payload); err != nil {
		return graphState{}, fmt.Errorf("stage1 extract parse: %w", err)
	}
	state := graphState{}
	state.Nodes = decodeStage1Nodes(payload["nodes"])
	state.Edges = decodeStage1Edges(payload["edges"])
	state.OffGraph = decodeStage1OffGraph(payload["off_graph"])
	state = fillMissingStage1IDs(state)
	return state, nil
}

func stage2Dedup(state graphState) graphState {
	seen := map[string]graphNode{}
	redirect := map[string]string{}
	for _, n := range state.Nodes {
		key := normalizeText(n.Text)
		if existing, ok := seen[key]; ok {
			redirect[n.ID] = existing.ID
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID: fmt.Sprintf("sup_%s", n.ID), Text: n.Text, Role: "supplementary", AttachesTo: existing.ID, SourceQuote: n.SourceQuote,
			})
			continue
		}
		seen[key] = n
		redirect[n.ID] = n.ID
	}
	nodes := make([]graphNode, 0, len(seen))
	for _, n := range seen {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	edgeSet := map[string]struct{}{}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		from := redirect[e.From]
		to := redirect[e.To]
		if from == "" || to == "" || from == to {
			continue
		}
		key := from + "->" + to
		if _, ok := edgeSet[key]; ok {
			continue
		}
		edgeSet[key] = struct{}{}
		edges = append(edges, graphEdge{From: from, To: to})
	}
	state.Nodes = nodes
	state.Edges = edges
	return state
}

func decodeStage1Nodes(raw any) []graphNode {
	items, _ := raw.([]any)
	out := make([]graphNode, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, graphNode{Text: strings.TrimSpace(v)})
		case map[string]any:
			out = append(out, graphNode{
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(firstNonEmpty(asString(v["text"]), asString(v["content"]))),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
}

func decodeStage1Edges(raw any) []graphEdge {
	items, _ := raw.([]any)
	out := make([]graphEdge, 0, len(items))
	for _, item := range items {
		v, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, graphEdge{
			From: strings.TrimSpace(firstNonEmpty(asString(v["from"]), asString(v["source"]))),
			To:   strings.TrimSpace(firstNonEmpty(asString(v["to"]), asString(v["target"]))),
		})
	}
	return out
}

func decodeStage1OffGraph(raw any) []offGraphItem {
	items, _ := raw.([]any)
	out := make([]offGraphItem, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, offGraphItem{Text: strings.TrimSpace(v), Role: "supplementary"})
		case map[string]any:
			out = append(out, offGraphItem{
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(asString(v["text"])),
				Role:        strings.TrimSpace(asString(v["role"])),
				AttachesTo:  strings.TrimSpace(asString(v["attaches_to"])),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
}

func fillMissingStage1IDs(state graphState) graphState {
	for i := range state.Nodes {
		if strings.TrimSpace(state.Nodes[i].ID) == "" {
			state.Nodes[i].ID = fmt.Sprintf("n%d", i+1)
		}
		if strings.TrimSpace(state.Nodes[i].SourceQuote) == "" {
			state.Nodes[i].SourceQuote = state.Nodes[i].Text
		}
	}
	for i := range state.OffGraph {
		if strings.TrimSpace(state.OffGraph[i].ID) == "" {
			state.OffGraph[i].ID = fmt.Sprintf("o%d", i+1)
		}
		if strings.TrimSpace(state.OffGraph[i].Role) == "" {
			state.OffGraph[i].Role = "supplementary"
		}
		if strings.TrimSpace(state.OffGraph[i].SourceQuote) == "" {
			state.OffGraph[i].SourceQuote = state.OffGraph[i].Text
		}
	}
	return state
}

func stage3Classify(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	for _, n := range state.Nodes {
		inDegree[n.ID] = 0
		outDegree[n.ID] = 0
	}
	for _, e := range state.Edges {
		outDegree[e.From]++
		inDegree[e.To]++
	}
	for i := range state.Nodes {
		n := &state.Nodes[i]
		switch {
		case inDegree[n.ID] == 0 && outDegree[n.ID] > 0:
			n.Role = roleDriver
		case outDegree[n.ID] == 0 && inDegree[n.ID] > 0:
			n.Role = roleTargetCandidate
		case inDegree[n.ID] > 0 && outDegree[n.ID] > 0:
			n.Role = roleTransmission
		default:
			n.Role = roleOrphan
		}
	}
	filtered := make([]graphNode, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		if n.Role != roleTargetCandidate {
			filtered = append(filtered, n)
			continue
		}
		req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage3SystemPrompt, fmt.Sprintf(stage3UserPrompt, n.Text, n.SourceQuote))
		if err != nil {
			return graphState{}, err
		}
		resp, err := rt.Call(ctx, req)
		if err != nil {
			return graphState{}, err
		}
		var result struct {
			IsMarketOutcome bool   `json:"is_market_outcome"`
			Category        string `json:"category"`
		}
		if err := parseJSONObject(resp.Text, &result); err != nil {
			return graphState{}, fmt.Errorf("stage3 classify parse: %w", err)
		}
		if result.IsMarketOutcome {
			n.Role = roleTarget
			n.Ontology = result.Category
			filtered = append(filtered, n)
			continue
		}
		attachTo := predecessorOf(state.Edges, n.ID)
		if attachTo != "" {
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID: fmt.Sprintf("sup_%s", n.ID), Text: n.Text, Role: "supplementary", AttachesTo: attachTo, SourceQuote: n.SourceQuote,
			})
		}
	}
	state.Nodes = filtered
	return state, nil
}

func stage5Render(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (compile.Output, error) {
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterNodesByRole(state.Nodes, roleTarget)
	if len(drivers) == 0 && len(state.Nodes) > 0 {
		drivers = append(drivers, state.Nodes[0])
	}
	if len(targets) == 0 && len(state.Nodes) > 1 {
		targets = append(targets, state.Nodes[len(state.Nodes)-1])
	}
	paths := extractPaths(state, drivers, targets)
	if len(paths) == 0 && len(drivers) > 0 && len(targets) > 0 {
		paths = append(paths, renderedPath{
			driver: drivers[0],
			target: targets[0],
			steps:  []graphNode{{ID: drivers[0].ID, Text: drivers[0].Text}},
		})
		if !hasEdge(state.Edges, drivers[0].ID, targets[0].ID) {
			state.Edges = append(state.Edges, graphEdge{From: drivers[0].ID, To: targets[0].ID})
		}
	}
	translated, err := translateAll(ctx, rt, model, uniqueTexts(drivers, targets, paths, state.OffGraph))
	if err != nil {
		return compile.Output{}, err
	}
	cn := func(id, fallback string) string {
		if value, ok := translated[id]; ok && strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	driversOut := make([]string, 0, len(drivers))
	for _, d := range drivers {
		driversOut = append(driversOut, cn(d.ID, d.Text))
	}
	targetsOut := make([]string, 0, len(targets))
	for _, t := range targets {
		targetsOut = append(targetsOut, cn(t.ID, t.Text))
	}
	transmission := make([]compile.TransmissionPath, 0, len(paths))
	for _, p := range paths {
		steps := make([]string, 0, len(p.steps))
		for _, s := range p.steps {
			steps = append(steps, cn(s.ID, s.Text))
		}
		transmission = append(transmission, compile.TransmissionPath{
			Driver: cn(p.driver.ID, p.driver.Text),
			Target: cn(p.target.ID, p.target.Text),
			Steps:  steps,
		})
	}
	evidence, explanation, supplementary := renderOffGraph(state.OffGraph, cn)
	summary, err := summarizeChinese(ctx, rt, model, driversOut, targetsOut, transmission, bundle)
	if err != nil {
		return compile.Output{}, err
	}
	graph := compile.ReasoningGraph{}
	for _, n := range state.Nodes {
		kind := compile.NodeMechanism
		form := compile.NodeFormObservation
		function := compile.NodeFunctionTransmission
		switch n.Role {
		case roleDriver:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		case roleTransmission:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		case roleTarget:
			kind = compile.NodeConclusion
			form = compile.NodeFormJudgment
			function = compile.NodeFunctionClaim
			if n.Ontology == "flow" {
				kind = compile.NodeConclusion
			}
		}
		graph.Nodes = append(graph.Nodes, compile.GraphNode{
			ID:         n.ID,
			Kind:       kind,
			Form:       form,
			Function:   function,
			Text:       cn(n.ID, n.Text),
			OccurredAt: bundle.PostedAt,
		})
	}
	for _, e := range state.Edges {
		graph.Edges = append(graph.Edges, compile.GraphEdge{From: e.From, To: e.To, Kind: compile.EdgePositive})
	}
	return compile.Output{
		Summary:            summary,
		Drivers:            driversOut,
		Targets:            targetsOut,
		TransmissionPaths:  transmission,
		EvidenceNodes:      evidence,
		ExplanationNodes:   explanation,
		SupplementaryNodes: supplementary,
		Graph:              graph,
		Details:            compile.HiddenDetails{Caveats: []string{"compile v2 mvp"}},
		Topics:             nil,
		Confidence:         "medium",
	}, nil
}

type renderedPath struct {
	driver graphNode
	target graphNode
	steps  []graphNode
}

func extractPaths(state graphState, drivers, targets []graphNode) []renderedPath {
	adj := map[string][]string{}
	for _, e := range state.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	var out []renderedPath
	for _, d := range drivers {
		for _, t := range targets {
			pathIDs := shortestPath(adj, d.ID, t.ID)
			if len(pathIDs) < 2 {
				continue
			}
			steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
			for _, id := range pathIDs[1 : len(pathIDs)-1] {
				if node, ok := nodeByID(state.Nodes, id); ok {
					steps = append(steps, node)
				}
			}
			out = append(out, renderedPath{driver: d, target: t, steps: steps})
		}
	}
	return out
}

func hasEdge(edges []graphEdge, from, to string) bool {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func shortestPath(adj map[string][]string, start, target string) []string {
	type item struct {
		id   string
		path []string
	}
	queue := []item{{id: start, path: []string{start}}}
	seen := map[string]struct{}{start: {}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == target {
			return cur.path
		}
		for _, next := range adj[cur.id] {
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, item{id: next, path: append(append([]string(nil), cur.path...), next)})
		}
	}
	return nil
}

func filterNodesByRole(nodes []graphNode, role graphRole) []graphNode {
	out := make([]graphNode, 0)
	for _, n := range nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

func predecessorOf(edges []graphEdge, id string) string {
	for _, e := range edges {
		if e.To == id {
			return e.From
		}
	}
	return ""
}

func nodeByID(nodes []graphNode, id string) (graphNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return graphNode{}, false
}

func normalizeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func uniqueTexts(nodes []graphNode, targets []graphNode, paths []renderedPath, off []offGraphItem) []map[string]string {
	seen := map[string]struct{}{}
	out := make([]map[string]string, 0)
	add := func(id, text string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, map[string]string{"id": id, "text": text})
	}
	for _, n := range nodes {
		add(n.ID, n.Text)
	}
	for _, n := range targets {
		add(n.ID, n.Text)
	}
	for _, p := range paths {
		add(p.driver.ID, p.driver.Text)
		add(p.target.ID, p.target.Text)
		for _, s := range p.steps {
			add(s.ID, s.Text)
		}
	}
	for _, o := range off {
		add(o.ID, o.Text)
	}
	return out
}

func translateAll(ctx context.Context, rt runtimeChat, model string, items []map[string]string) (map[string]string, error) {
	if len(items) == 0 {
		return map[string]string{}, nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	bundle := compile.Bundle{UnitID: "translate", Source: "compilev2", ExternalID: "translate", Content: string(payload)}
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage5TranslateSystemPrompt, fmt.Sprintf(stage5TranslateUserPrompt, string(payload)))
	if err != nil {
		return nil, err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	var result struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := parseJSONObject(resp.Text, &result); err != nil {
		return nil, fmt.Errorf("stage5 translate parse: %w", err)
	}
	out := map[string]string{}
	for _, item := range result.Translations {
		out[item.ID] = strings.TrimSpace(item.Text)
	}
	return out, nil
}

func summarizeChinese(ctx context.Context, rt runtimeChat, model string, drivers, targets []string, paths []compile.TransmissionPath, bundle compile.Bundle) (string, error) {
	payload, err := json.Marshal(map[string]any{"drivers": drivers, "targets": targets, "paths": paths})
	if err != nil {
		return "", err
	}
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage5SummarySystemPrompt, fmt.Sprintf(stage5SummaryUserPrompt, string(payload)))
	if err != nil {
		return "", err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return "", err
	}
	var result struct{ Summary string `json:"summary"` }
	if err := parseJSONObject(resp.Text, &result); err != nil {
		return "", fmt.Errorf("stage5 summary parse: %w", err)
	}
	return strings.TrimSpace(result.Summary), nil
}

func renderOffGraph(items []offGraphItem, cn func(id, fallback string) string) (evidence, explanation, supplementary []string) {
	for _, item := range items {
		switch item.Role {
		case "evidence":
			evidence = append(evidence, cn(item.ID, item.Text))
		case "explanation":
			explanation = append(explanation, cn(item.ID, item.Text))
		default:
			supplementary = append(supplementary, cn(item.ID, item.Text))
		}
	}
	return
}

func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

const stage1SystemPrompt = `You are a causal graph extractor for financial analysis articles. Extract atomic nodes, causal edges, and off-graph items. Return JSON only.`

const stage1UserPrompt = `Extract a causal graph from this article. Keep node text in the article's original language. Split mixed cause/effect clauses into atomic nodes. Return JSON with keys nodes, edges, off_graph.

Article:
%s`

const stage3SystemPrompt = `You are an ontology classifier for financial market outcomes. Return JSON only.`

const stage3UserPrompt = `Decide whether the node below is a market outcome.
Node: %s
Source quote: %s

Return JSON:
{"is_market_outcome": true|false, "category":"price|flow|decision|none"}`

const stage5TranslateSystemPrompt = `You are a financial-Chinese translator. Translate each item into concise professional Chinese. Keep already-Chinese items unchanged. Return JSON only.`

const stage5TranslateUserPrompt = `Translate the following id/text pairs into Chinese. Return {"translations":[{"id":"...","text":"..."}]} only.

%s`

const stage5SummarySystemPrompt = `Produce a one-sentence Chinese summary of the thesis package. Return JSON only.`

const stage5SummaryUserPrompt = `Summarize this thesis package in one Chinese sentence. Return {"summary":"..."} only.

%s`

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
