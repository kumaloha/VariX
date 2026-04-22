package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kumaloha/VariX/varix/compile"
)

type graphRole string

const (
	roleDriver          graphRole = "driver"
	roleTransmission    graphRole = "transmission"
	roleTargetCandidate graphRole = "target_candidate"
	roleTarget          graphRole = "target"
	roleOrphan          graphRole = "orphan"
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

type relationKind string

const (
	relationCausal      relationKind = "causal"
	relationSupports    relationKind = "supports"
	relationSupplements relationKind = "supplements"
	relationExplains    relationKind = "explains"
	relationNone        relationKind = "none"
)

func countRole(state graphState, role graphRole) int {
	count := 0
	for _, n := range state.Nodes {
		if n.Role == role {
			count++
		}
	}
	return count
}

func stage1Extract(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle) (graphState, error) {
	var payload map[string]any
	if err := stageJSONCall(ctx, rt, model, bundle, stage1SystemPrompt, fmt.Sprintf(stage1UserPrompt, bundle.TextContext()), "stage1 extract", &payload); err != nil {
		return graphState{}, err
	}
	state := graphState{}
	state.Nodes = decodeStage1Nodes(payload["nodes"])
	state.Edges = decodeStage1Edges(payload["edges"])
	state.OffGraph = decodeStage1OffGraph(payload["off_graph"])
	state = fillMissingStage1IDs(state)
	return state, nil
}

func stage2Dedup(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	state = normalizeStage1State(state)
	uf := newUF(state.Nodes)
	for _, pair := range dedupCandidatePairs(state.Nodes) {
		var result struct {
			Equivalent bool `json:"equivalent"`
		}
		if err := stageJSONCall(ctx, rt, model, bundle, stage2SystemPrompt, fmt.Sprintf(stage2UserPrompt, pair[0].Text, pair[0].SourceQuote, pair[1].Text, pair[1].SourceQuote), "stage2 dedup", &result); err != nil {
			return graphState{}, err
		}
		if result.Equivalent {
			uf.union(pair[0].ID, pair[1].ID)
		}
	}
	redirect := map[string]string{}
	canonicals := map[string]graphNode{}
	for _, group := range uf.groups(state.Nodes) {
		canonical := chooseCanonical(group)
		canonicals[canonical.ID] = canonical
		for _, n := range group {
			redirect[n.ID] = canonical.ID
			if n.ID == canonical.ID {
				continue
			}
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID: fmt.Sprintf("sup_%s", n.ID), Text: n.Text, Role: "supplementary", AttachesTo: canonical.ID, SourceQuote: n.SourceQuote,
			})
		}
	}
	nodes := make([]graphNode, 0, len(canonicals))
	for _, n := range canonicals {
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
	return state, nil
}

func normalizeStage1State(state graphState) graphState {
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		n.ID = strings.TrimSpace(n.ID)
		n.Text = strings.TrimSpace(n.Text)
		n.SourceQuote = strings.TrimSpace(n.SourceQuote)
		if n.Text == "" {
			continue
		}
		nodes = append(nodes, n)
	}
	state.Nodes = nodes

	validIDs := map[string]struct{}{}
	for _, n := range state.Nodes {
		validIDs[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		e.From = strings.TrimSpace(e.From)
		e.To = strings.TrimSpace(e.To)
		if e.From == "" || e.To == "" || e.From == e.To {
			continue
		}
		if _, ok := validIDs[e.From]; !ok {
			continue
		}
		if _, ok := validIDs[e.To]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = edges

	off := make([]offGraphItem, 0, len(state.OffGraph))
	for _, o := range state.OffGraph {
		o.Text = strings.TrimSpace(o.Text)
		if o.Text == "" {
			continue
		}
		o.Role = strings.TrimSpace(o.Role)
		if o.Role == "" {
			o.Role = "supplementary"
		}
		off = append(off, o)
	}
	state.OffGraph = off
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
		var result struct {
			IsMarketOutcome bool   `json:"is_market_outcome"`
			Category        string `json:"category"`
		}
		if err := stageJSONCall(ctx, rt, model, bundle, stage3SystemPrompt, fmt.Sprintf(stage3UserPrompt, n.Text, n.SourceQuote), "stage3 classify", &result); err != nil {
			return graphState{}, err
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
	state = pruneDanglingEdges(state)
	return state, nil
}

func stage3Relations(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	var result struct {
		CausalEdges []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"causal_edges"`
		SupportLinks []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"support_links"`
		SupplementLinks []struct {
			Primary   string `json:"primary"`
			Secondary string `json:"secondary"`
		} `json:"supplement_links"`
		ExplanationLinks []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"explanation_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, stage3RelationSystemPrompt, fmt.Sprintf(stage3RelationUserPrompt, serializeRelationNodes(state.Nodes)), "stage3 relation", &result); err != nil {
		return graphState{}, err
	}
	valid := map[string]graphNode{}
	for _, n := range state.Nodes {
		valid[n.ID] = n
	}
	demoted := map[string]struct{}{}
	newEdges := make([]graphEdge, 0, len(result.CausalEdges))
	for _, e := range result.CausalEdges {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		newEdges = append(newEdges, graphEdge{From: e.From, To: e.To})
	}
	for _, e := range result.SupportLinks {
		fromNode, okFrom := valid[e.From]
		if !okFrom {
			continue
		}
		toNode, okTo := valid[e.To]
		if !okTo {
			continue
		}
		if shouldDemoteSupportToSupplement(fromNode, toNode) {
			primaryID, secondaryNode := chooseSupplementPrimary(fromNode, toNode)
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID:          fmt.Sprintf("sup_%s_%s", secondaryNode.ID, primaryID),
				Text:        secondaryNode.Text,
				Role:        "supplementary",
				AttachesTo:  primaryID,
				SourceQuote: secondaryNode.SourceQuote,
			})
			demoted[secondaryNode.ID] = struct{}{}
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:          fmt.Sprintf("evi_%s_%s", e.From, e.To),
			Text:        fromNode.Text,
			Role:        "evidence",
			AttachesTo:  e.To,
			SourceQuote: fromNode.SourceQuote,
		})
		demoted[e.From] = struct{}{}
	}
	for _, e := range result.ExplanationLinks {
		fromNode, okFrom := valid[e.From]
		if !okFrom {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:          fmt.Sprintf("exp_%s_%s", e.From, e.To),
			Text:        fromNode.Text,
			Role:        "explanation",
			AttachesTo:  e.To,
			SourceQuote: fromNode.SourceQuote,
		})
		demoted[e.From] = struct{}{}
	}
	for _, e := range result.SupplementLinks {
		secondaryNode, okSecondary := valid[e.Secondary]
		if !okSecondary {
			continue
		}
		if _, ok := valid[e.Primary]; !ok {
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:          fmt.Sprintf("sup_%s_%s", e.Secondary, e.Primary),
			Text:        secondaryNode.Text,
			Role:        "supplementary",
			AttachesTo:  e.Primary,
			SourceQuote: secondaryNode.SourceQuote,
		})
		demoted[e.Secondary] = struct{}{}
	}
	state.Edges = dedupeEdges(newEdges)
	if len(demoted) > 0 {
		nodes := make([]graphNode, 0, len(state.Nodes))
		for _, n := range state.Nodes {
			if _, ok := demoted[n.ID]; ok {
				continue
			}
			nodes = append(nodes, n)
		}
		state.Nodes = nodes
	}
	return state, nil
}

func stage4Validate(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState, maxRounds int) (graphState, error) {
	if maxRounds <= 0 {
		return state, nil
	}
	paragraphs := splitParagraphs(bundle.TextContext())
	if len(paragraphs) == 0 {
		return state, nil
	}
	for round := 0; round < maxRounds; round++ {
		totalPatches := 0
		for _, para := range paragraphs {
			var patch struct {
				MissingNodes []struct {
					Text              string `json:"text"`
					SourceQuote       string `json:"source_quote"`
					SuggestedRoleHint string `json:"suggested_role_hint"`
				} `json:"missing_nodes"`
				MissingEdges []struct {
					FromText string `json:"from_text"`
					ToText   string `json:"to_text"`
				} `json:"missing_edges"`
				Misclassified []struct {
					NodeID string `json:"node_id"`
					Issue  string `json:"issue"`
				} `json:"misclassified"`
			}
			if err := stageJSONCall(ctx, rt, model, bundle, stage4SystemPrompt, fmt.Sprintf(stage4UserPrompt, para, serializeNodeList(state.Nodes), serializeEdgeList(state.Edges)), "stage4 validate", &patch); err != nil {
				return graphState{}, err
			}
			totalPatches += len(patch.MissingNodes) + len(patch.MissingEdges) + len(patch.Misclassified)
			state = applyValidatePatch(state, patch)
		}
		var err error
		state, err = stage2Dedup(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage3Classify(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage3Relations(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state, err = stage3Classify(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state.Rounds++
		if totalPatches < 2 {
			break
		}
	}
	return state, nil
}

func stage5Render(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (compile.Output, error) {
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterNodesByRole(state.Nodes, roleTarget)
	paths := extractPaths(state, drivers, targets)
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
		summary = fallbackSummary(driversOut, targetsOut)
	}
	if strings.TrimSpace(summary) == "" {
		summary = fallbackSummary(driversOut, targetsOut)
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
		Confidence:         confidenceFromState(driversOut, targetsOut, transmission),
	}, nil
}

func applyValidatePatch(state graphState, patch struct {
	MissingNodes []struct {
		Text              string `json:"text"`
		SourceQuote       string `json:"source_quote"`
		SuggestedRoleHint string `json:"suggested_role_hint"`
	} `json:"missing_nodes"`
	MissingEdges []struct {
		FromText string `json:"from_text"`
		ToText   string `json:"to_text"`
	} `json:"missing_edges"`
	Misclassified []struct {
		NodeID string `json:"node_id"`
		Issue  string `json:"issue"`
	} `json:"misclassified"`
}) graphState {
	nextNode := len(state.Nodes) + 1
	textToID := map[string]string{}
	for _, n := range state.Nodes {
		textToID[normalizeText(n.Text)] = n.ID
	}
	for _, item := range patch.MissingNodes {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		key := normalizeText(text)
		if _, ok := textToID[key]; ok {
			continue
		}
		id := fmt.Sprintf("n%d", nextNode)
		nextNode++
		state.Nodes = append(state.Nodes, graphNode{ID: id, Text: text, SourceQuote: strings.TrimSpace(item.SourceQuote)})
		textToID[key] = id
	}
	for _, item := range patch.MissingEdges {
		fromID := textToID[normalizeText(item.FromText)]
		toID := textToID[normalizeText(item.ToText)]
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		if !hasEdge(state.Edges, fromID, toID) {
			state.Edges = append(state.Edges, graphEdge{From: fromID, To: toID})
		}
	}
	for _, item := range patch.Misclassified {
		if strings.TrimSpace(item.NodeID) == "" {
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:         fmt.Sprintf("mis_%s", item.NodeID),
			Text:       strings.TrimSpace(item.Issue),
			Role:       "supplementary",
			AttachesTo: strings.TrimSpace(item.NodeID),
		})
	}
	return state
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

func dedupeEdges(edges []graphEdge) []graphEdge {
	seen := map[string]struct{}{}
	out := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		key := e.From + "->" + e.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out
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
	var result struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, stage5TranslateSystemPrompt, fmt.Sprintf(stage5TranslateUserPrompt, string(payload)), "stage5 translate", &result); err != nil {
		return nil, err
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
	var result struct {
		Summary string `json:"summary"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, stage5SummarySystemPrompt, fmt.Sprintf(stage5SummaryUserPrompt, string(payload)), "stage5 summary", &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Summary), nil
}

func stageJSONCall(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, systemPrompt string, userPrompt string, stageName string, target any) error {
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, systemPrompt, userPrompt)
	if err != nil {
		return err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return err
	}
	if err := parseJSONObject(resp.Text, target); err != nil {
		return fmt.Errorf("%s parse: %w", stageName, err)
	}
	return nil
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

func pruneDanglingEdges(state graphState) graphState {
	valid := map[string]struct{}{}
	for _, n := range state.Nodes {
		valid[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = dedupeEdges(edges)
	return state
}

func fallbackSummary(drivers, targets []string) string {
	switch {
	case len(drivers) > 0 && len(targets) > 0:
		return fmt.Sprintf("%s影响%s。", drivers[0], targets[0])
	case len(targets) > 0:
		return fmt.Sprintf("核心结果：%s。", targets[0])
	case len(drivers) > 0:
		return fmt.Sprintf("核心驱动：%s。", drivers[0])
	default:
		return "未能稳定提取主线。"
	}
}

func confidenceFromState(drivers, targets []string, paths []compile.TransmissionPath) string {
	if len(paths) > 0 && len(drivers) > 0 && len(targets) > 0 {
		return "medium"
	}
	if len(drivers) > 0 || len(targets) > 0 {
		return "low"
	}
	return "low"
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func serializeNodeList(nodes []graphNode) string {
	return joinSerializedLines(len(nodes), func(out *[]string) {
		for _, n := range nodes {
			*out = append(*out, fmt.Sprintf("%s: %s", n.ID, n.Text))
		}
	})
}

func serializeEdgeList(edges []graphEdge) string {
	return joinSerializedLines(len(edges), func(out *[]string) {
		for _, e := range edges {
			*out = append(*out, fmt.Sprintf("%s -> %s", e.From, e.To))
		}
	})
}

func serializeRelationNodes(nodes []graphNode) string {
	return joinSerializedLines(len(nodes), func(out *[]string) {
		for _, n := range nodes {
			*out = append(*out, fmt.Sprintf("%s | %s | role=%s | ontology=%s | quote=%s", n.ID, n.Text, n.Role, n.Ontology, n.SourceQuote))
		}
	})
}

func joinSerializedLines(capacity int, appendLines func(*[]string)) string {
	lines := make([]string, 0, capacity)
	appendLines(&lines)
	return strings.Join(lines, "\n")
}

func isOutcomeLikeNode(n graphNode) bool {
	if n.Role == roleTarget || n.Role == roleTargetCandidate {
		return true
	}
	if strings.TrimSpace(n.Ontology) != "" && strings.TrimSpace(n.Ontology) != "none" {
		return true
	}
	return false
}

func shouldDemoteSupportToSupplement(fromNode, toNode graphNode) bool {
	return isOutcomeLikeNode(fromNode) && isOutcomeLikeNode(toNode)
}

func chooseSupplementPrimary(left, right graphNode) (string, graphNode) {
	if isLabelLikeNode(left.Text) && !isLabelLikeNode(right.Text) {
		return right.ID, left
	}
	if isLabelLikeNode(right.Text) && !isLabelLikeNode(left.Text) {
		return left.ID, right
	}
	if directnessScore(left.Text) >= directnessScore(right.Text) {
		return left.ID, right
	}
	return right.ID, left
}

func isLabelLikeNode(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{" trade", " narrative", "交易", "叙事", "story", "regime"} {
		if strings.Contains(t, marker) {
			return true
		}
	}
	return false
}

func directnessScore(text string) int {
	score := 0
	t := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{"流入", "流出", "上涨", "下跌", "rise", "fall", "inflow", "outflow", "yield", "spread", "price", "position", "hedge", "allocation"} {
		if strings.Contains(t, strings.ToLower(marker)) {
			score += 2
		}
	}
	if !isLabelLikeNode(text) {
		score++
	}
	if len([]rune(text)) < 40 {
		score++
	}
	return score
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func dedupCandidatePairs(nodes []graphNode) [][2]graphNode {
	out := make([][2]graphNode, 0)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if semanticSimilarity(nodes[i].Text, nodes[j].Text) < 0.38 {
				continue
			}
			out = append(out, [2]graphNode{nodes[i], nodes[j]})
		}
	}
	return out
}

func semanticSimilarity(a, b string) float64 {
	na, nb := normalizeText(a), normalizeText(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return 0.8
	}
	j := jaccard(tokenSet(na), tokenSet(nb))
	bg := bigramDice(na, nb)
	if bg > j {
		return bg
	}
	return j
}

func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, tok := range strings.Fields(s) {
		out[tok] = struct{}{}
	}
	return out
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func bigramDice(a, b string) float64 {
	ba := bigrams(a)
	bb := bigrams(b)
	if len(ba) == 0 || len(bb) == 0 {
		return 0
	}
	inter := 0
	for k, ca := range ba {
		if cb, ok := bb[k]; ok {
			if ca < cb {
				inter += ca
			} else {
				inter += cb
			}
		}
	}
	total := 0
	for _, c := range ba {
		total += c
	}
	for _, c := range bb {
		total += c
	}
	if total == 0 {
		return 0
	}
	return float64(2*inter) / float64(total)
}

func bigrams(s string) map[string]int {
	runes := []rune(s)
	out := map[string]int{}
	if len(runes) < 2 {
		return out
	}
	for i := 0; i < len(runes)-1; i++ {
		out[string(runes[i:i+2])]++
	}
	return out
}

type uf struct{ parent map[string]string }

func newUF(nodes []graphNode) *uf {
	parent := map[string]string{}
	for _, n := range nodes {
		parent[n.ID] = n.ID
	}
	return &uf{parent: parent}
}

func (u *uf) find(x string) string {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *uf) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

func (u *uf) groups(nodes []graphNode) [][]graphNode {
	grouped := map[string][]graphNode{}
	for _, n := range nodes {
		grouped[u.find(n.ID)] = append(grouped[u.find(n.ID)], n)
	}
	out := make([][]graphNode, 0, len(grouped))
	for _, g := range grouped {
		out = append(out, g)
	}
	return out
}

func chooseCanonical(group []graphNode) graphNode {
	best := group[0]
	for _, n := range group[1:] {
		if len(n.SourceQuote) > len(best.SourceQuote) {
			best = n
			continue
		}
		if len(n.SourceQuote) == len(best.SourceQuote) && utf8.RuneCountInString(n.Text) > utf8.RuneCountInString(best.Text) {
			best = n
		}
	}
	return best
}
