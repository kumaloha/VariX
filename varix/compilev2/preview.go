package compilev2

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
)

type FlowPreviewOptions struct {
	StopAfter string
}

type FlowPreviewResult struct {
	Platform    string           `json:"platform"`
	ExternalID  string           `json:"external_id"`
	URL         string           `json:"url"`
	ArticleForm string           `json:"article_form,omitempty"`
	Extract     PreviewGraph     `json:"extract"`
	Refine      PreviewGraph     `json:"refine"`
	Aggregate   PreviewGraph     `json:"aggregate"`
	Support     PreviewGraph     `json:"support"`
	Cluster     PreviewGraph     `json:"cluster"`
	Supplement  PreviewGraph     `json:"supplement"`
	Collapse    PreviewGraph     `json:"collapse"`
	Evidence    PreviewGraph     `json:"evidence"`
	Explanation PreviewGraph     `json:"explanation"`
	Relations   PreviewGraph     `json:"relations"`
	Spines      []PreviewSpine   `json:"spines,omitempty"`
	Classify    PreviewGraph     `json:"classify"`
	Render      compile.Output   `json:"render"`
	Metrics     map[string]int64 `json:"metrics"`
}

type PreviewGraph struct {
	Nodes       []PreviewNode     `json:"nodes"`
	Edges       []PreviewEdge     `json:"edges"`
	AuxEdges    []PreviewEdge     `json:"aux_edges,omitempty"`
	OffGraph    []PreviewOffGraph `json:"off_graph"`
	BranchHeads []string          `json:"branch_heads,omitempty"`
	Rounds      int               `json:"rounds"`
}

type PreviewNode struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	SourceQuote   string `json:"source_quote,omitempty"`
	Role          string `json:"role,omitempty"`
	DiscourseRole string `json:"discourse_role,omitempty"`
	Ontology      string `json:"ontology,omitempty"`
	IsTarget      bool   `json:"is_target,omitempty"`
}

type PreviewEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type PreviewOffGraph struct {
	ID          string `json:"id,omitempty"`
	Text        string `json:"text"`
	Role        string `json:"role"`
	AttachesTo  string `json:"attaches_to,omitempty"`
	SourceQuote string `json:"source_quote,omitempty"`
}

type PreviewSpine struct {
	ID          string        `json:"id"`
	Level       string        `json:"level"`
	Priority    int           `json:"priority"`
	Policy      string        `json:"policy,omitempty"`
	Thesis      string        `json:"thesis"`
	NodeIDs     []string      `json:"node_ids"`
	Edges       []PreviewEdge `json:"edges"`
	Scope       string        `json:"scope,omitempty"`
	FamilyKey   string        `json:"family_key,omitempty"`
	FamilyLabel string        `json:"family_label,omitempty"`
	FamilyScope string        `json:"family_scope,omitempty"`
}

func (c *Client) PreviewFlow(ctx context.Context, bundle compile.Bundle, opts FlowPreviewOptions) (FlowPreviewResult, error) {
	if c == nil || c.runtime == nil {
		return FlowPreviewResult{}, fmt.Errorf("compile v2 client is nil")
	}
	result := FlowPreviewResult{
		Platform:   bundle.Source,
		ExternalID: bundle.ExternalID,
		URL:        bundle.URL,
		Metrics:    map[string]int64{},
	}

	start := time.Now()
	extractState, err := stage1Extract(ctx, c.runtime, c.model, bundle)
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("extract: %w", err)
	}
	result.Extract = toPreviewGraph(extractState)
	result.ArticleForm = extractState.ArticleForm
	result.Metrics["extract_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "extract" {
		return result, nil
	}

	start = time.Now()
	refineState, err := stage1Refine(ctx, c.runtime, c.model, bundle, cloneGraphState(extractState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("refine: %w", err)
	}
	result.Refine = toPreviewGraph(refineState)
	result.Metrics["refine_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "refine" {
		return result, nil
	}

	start = time.Now()
	aggregateState, err := stage1Aggregate(ctx, c.runtime, c.model, bundle, cloneGraphState(refineState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("aggregate: %w", err)
	}
	result.Aggregate = toPreviewGraph(aggregateState)
	result.Metrics["aggregate_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "aggregate" {
		return result, nil
	}

	start = time.Now()
	supportState, err := stage2Support(ctx, c.runtime, c.model, bundle, cloneGraphState(aggregateState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("support: %w", err)
	}
	result.Support = toPreviewGraph(supportState)
	result.Cluster = result.Support
	result.Supplement = result.Support
	result.Metrics["support_ms"] = time.Since(start).Milliseconds()
	result.Metrics["cluster_ms"] = result.Metrics["support_ms"]
	if stop := strings.TrimSpace(opts.StopAfter); stop == "support" || stop == "supplement" || stop == "cluster" {
		return result, nil
	}

	start = time.Now()
	selectedState := collapseClusters(cloneGraphState(supportState))
	result.Collapse = toPreviewGraph(selectedState)
	result.Metrics["collapse_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "collapse" {
		return result, nil
	}

	start = time.Now()
	relationsState, err := stage3Mainline(ctx, c.runtime, c.model, bundle, cloneGraphState(selectedState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("relations: %w", err)
	}
	result.Relations = toPreviewGraph(relationsState)
	result.Spines = relationsState.Spines
	if len(result.Spines) == 0 {
		result.Spines = derivePreviewSpines(result.Relations)
	}
	result.Metrics["relations_ms"] = time.Since(start).Milliseconds()
	if stop := strings.TrimSpace(opts.StopAfter); stop == "mainline" || stop == "relations" || stop == "spines" || stop == "drives" {
		return result, nil
	}

	start = time.Now()
	classifyState, err := stage3Classify(ctx, c.runtime, c.model, bundle, cloneGraphState(relationsState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("classify: %w", err)
	}
	result.Classify = toPreviewGraph(classifyState)
	result.Metrics["classify_ms"] = time.Since(start).Milliseconds()
	if strings.TrimSpace(opts.StopAfter) == "classify" {
		return result, nil
	}

	start = time.Now()
	rendered, err := stage5Render(ctx, c.runtime, c.model, bundle, cloneGraphState(classifyState))
	if err != nil {
		return FlowPreviewResult{}, fmt.Errorf("render: %w", err)
	}
	result.Render = rendered
	result.Metrics["render_ms"] = time.Since(start).Milliseconds()
	return result, nil
}

func BuildMainlineMarkdown(result FlowPreviewResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s:%s\n\n", result.Platform, result.ExternalID)
	fmt.Fprintf(&b, "URL: %s\n\n", result.URL)
	fmt.Fprintf(&b, "Drivers: %d\n", len(result.Render.Drivers))
	fmt.Fprintf(&b, "Targets: %d\n", len(result.Render.Targets))
	fmt.Fprintf(&b, "Paths: %d\n\n", len(result.Render.TransmissionPaths))
	if len(result.Spines) > 0 {
		b.WriteString("Spines:\n")
		for _, spine := range result.Spines {
			family := ""
			if strings.TrimSpace(spine.FamilyKey) != "" {
				family = fmt.Sprintf(" [%s/%s/%s]", spine.FamilyKey, spine.FamilyLabel, spine.FamilyScope)
			}
			fmt.Fprintf(&b, "- Spine %s (%s, priority %d)%s: %s\n", spine.ID, spine.Level, spine.Priority, family, spine.Thesis)
		}
		b.WriteString("\n")
	}
	b.WriteString("```mermaid\nflowchart LR\n")
	if len(result.Spines) > 0 {
		writePreviewSpinesMermaid(&b, relationLabelMap(result.Relations), result.Spines)
		b.WriteString("```\n")
		return b.String()
	}
	if len(result.Relations.Edges) > 0 {
		writePreviewEdgesMermaid(&b, relationLabelMap(result.Relations), result.Relations.Edges)
		b.WriteString("```\n")
		return b.String()
	}
	if len(result.Render.TransmissionPaths) == 0 {
		b.WriteString("  n0[\"No mainline path\"]\n")
		b.WriteString("```\n")
		return b.String()
	}
	labelToID := map[string]string{}
	nextID := 1
	nodeID := func(label string) string {
		label = strings.TrimSpace(label)
		if label == "" {
			label = "Unnamed"
		}
		if id, ok := labelToID[label]; ok {
			return id
		}
		id := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = id
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, escapeMermaidLabel(label))
		return id
	}
	for _, path := range result.Render.TransmissionPaths {
		prev := nodeID(path.Driver)
		prevLabel := strings.TrimSpace(path.Driver)
		for _, step := range path.Steps {
			step = strings.TrimSpace(step)
			if step == "" || strings.EqualFold(step, prevLabel) {
				continue
			}
			cur := nodeID(step)
			fmt.Fprintf(&b, "  %s --> %s\n", prev, cur)
			prev = cur
			prevLabel = step
		}
		target := nodeID(path.Target)
		if !strings.EqualFold(strings.TrimSpace(path.Target), prevLabel) {
			fmt.Fprintf(&b, "  %s --> %s\n", prev, target)
		}
	}
	b.WriteString("```\n")
	return b.String()
}

func relationLabelMap(graph PreviewGraph) map[string]string {
	labels := map[string]string{}
	for _, node := range graph.Nodes {
		labels[node.ID] = node.Text
	}
	return labels
}

func collectSpineEdges(spines []PreviewSpine) []PreviewEdge {
	var out []PreviewEdge
	for _, spine := range spines {
		out = append(out, spine.Edges...)
	}
	return out
}

func writePreviewSpinesMermaid(b *strings.Builder, labels map[string]string, spines []PreviewSpine) {
	labelToID := map[string]string{}
	nextID := 1
	nodeIDForLabel := func(label string) string {
		if label == "" {
			label = "Unnamed"
		}
		if existing, ok := labelToID[label]; ok {
			return existing
		}
		renderID := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = renderID
		fmt.Fprintf(b, "  %s[\"%s\"]\n", renderID, escapeMermaidLabel(label))
		return renderID
	}
	nodeID := func(id string) string {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			label = strings.TrimSpace(id)
		}
		return nodeIDForLabel(label)
	}
	renderedNodes := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	for _, spine := range spines {
		if isSatiricalPreviewSpine(spine) && writeSatiricalPreviewSpineMermaid(b, labels, spine, nodeIDForLabel, renderedNodes, seenEdges) {
			continue
		}
		for _, edge := range spine.Edges {
			if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
				continue
			}
			from := nodeID(edge.From)
			to := nodeID(edge.To)
			renderedNodes[edge.From] = struct{}{}
			renderedNodes[edge.To] = struct{}{}
			if from == to {
				continue
			}
			key := from + "->" + to
			if _, ok := seenEdges[key]; ok {
				continue
			}
			seenEdges[key] = struct{}{}
			fmt.Fprintf(b, "  %s --> %s\n", from, to)
		}
	}
	for _, spine := range spines {
		for _, id := range spine.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := renderedNodes[id]; ok {
				continue
			}
			nodeID(id)
			renderedNodes[id] = struct{}{}
		}
	}
}

func isSatiricalPreviewSpine(spine PreviewSpine) bool {
	return strings.EqualFold(strings.TrimSpace(spine.Policy), "satirical_analogy")
}

func writeSatiricalPreviewSpineMermaid(
	b *strings.Builder,
	labels map[string]string,
	spine PreviewSpine,
	nodeIDForLabel func(string) string,
	renderedNodes map[string]struct{},
	seenEdges map[string]struct{},
) bool {
	vehicle, ok := satiricalVehicleLabel(labels, spine)
	if !ok {
		return false
	}
	mechanism := satiricalMechanismLabel(labels, spine)
	conclusion := satiricalConclusionLabel(labels, spine)
	if mechanism == "" && conclusion == "" {
		mechanism = strings.TrimSpace(spine.Thesis)
	}
	if mechanism == "" && conclusion == "" {
		return false
	}
	vehicleID := nodeIDForLabel(vehicle)
	prevID := vehicleID
	for _, label := range []string{mechanism, conclusion} {
		label = strings.TrimSpace(label)
		if label == "" || strings.EqualFold(label, vehicle) {
			continue
		}
		nextID := nodeIDForLabel(label)
		if nextID != prevID {
			key := prevID + "->" + nextID
			if _, ok := seenEdges[key]; !ok {
				seenEdges[key] = struct{}{}
				fmt.Fprintf(b, "  %s --> %s\n", prevID, nextID)
			}
		}
		prevID = nextID
	}
	for _, id := range spine.NodeIDs {
		if strings.TrimSpace(id) != "" {
			renderedNodes[id] = struct{}{}
		}
	}
	return true
}

func satiricalVehicleLabel(labels map[string]string, spine PreviewSpine) (string, bool) {
	for _, id := range spine.NodeIDs {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			continue
		}
		lower := strings.ToLower(label)
		if strings.Contains(label, "幸运游戏") {
			return "讽刺寓言：幸运游戏", true
		}
		if strings.Contains(label, "寓言") || strings.Contains(label, "类比") || strings.Contains(lower, "allegory") || strings.Contains(lower, "analogy") {
			return "讽刺寓言：" + compactPreviewLabel(label, 32), true
		}
	}
	for _, id := range spine.NodeIDs {
		if label := strings.TrimSpace(labels[id]); label != "" {
			return "讽刺寓言：" + compactPreviewLabel(label, 32), true
		}
	}
	return "", false
}

func satiricalMechanismLabel(labels map[string]string, spine PreviewSpine) string {
	for _, markerSet := range [][]string{
		{"包装", "公平"},
		{"内部", "赢家"},
		{"吸引", "75"},
		{"忽悠"},
		{"不公平"},
		{"机制"},
	} {
		if label := firstSatiricalLabelMatching(labels, spine, markerSet, false); label != "" {
			return "映射机制：" + compactPreviewLabel(label, 42)
		}
	}
	return ""
}

func satiricalConclusionLabel(labels map[string]string, spine PreviewSpine) string {
	for _, markerSet := range [][]string{
		{"现实", "骗局"},
		{"风险识别"},
		{"不是智商"},
		{"净亏"},
		{"承担", "成本"},
		{"买单"},
	} {
		if label := firstSatiricalLabelMatching(labels, spine, markerSet, true); label != "" {
			return "批判结论：" + compactPreviewLabel(label, 42)
		}
	}
	return ""
}

func firstSatiricalLabelMatching(labels map[string]string, spine PreviewSpine, markers []string, preferLast bool) string {
	matches := make([]string, 0, 1)
	for _, id := range spine.NodeIDs {
		label := strings.TrimSpace(labels[id])
		if label == "" || isSatiricalStoryDetail(label) {
			continue
		}
		ok := true
		for _, marker := range markers {
			if !strings.Contains(label, marker) {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, label)
		}
	}
	if len(matches) == 0 {
		return ""
	}
	if preferLast {
		return matches[len(matches)-1]
	}
	return matches[0]
}

func isSatiricalStoryDetail(label string) bool {
	for _, marker := range []string{"每月", "每人", "利率", "委托贷款", "本金贷回", "抽一人", "抽完", "中奖者"} {
		if strings.Contains(label, marker) {
			return true
		}
	}
	return false
}

func compactPreviewLabel(label string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(label))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func writePreviewEdgesMermaid(b *strings.Builder, labels map[string]string, edges []PreviewEdge) {
	labelToID := map[string]string{}
	nextID := 1
	nodeID := func(id string) string {
		label := strings.TrimSpace(labels[id])
		if label == "" {
			label = strings.TrimSpace(id)
		}
		if label == "" {
			label = "Unnamed"
		}
		if existing, ok := labelToID[label]; ok {
			return existing
		}
		renderID := "n" + strconv.Itoa(nextID)
		nextID++
		labelToID[label] = renderID
		fmt.Fprintf(b, "  %s[\"%s\"]\n", renderID, escapeMermaidLabel(label))
		return renderID
	}
	seenEdges := map[string]struct{}{}
	for _, edge := range edges {
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
			continue
		}
		from := nodeID(edge.From)
		to := nodeID(edge.To)
		if from == to {
			continue
		}
		key := from + "->" + to
		if _, ok := seenEdges[key]; ok {
			continue
		}
		seenEdges[key] = struct{}{}
		fmt.Fprintf(b, "  %s --> %s\n", from, to)
	}
}

func derivePreviewSpines(graph PreviewGraph) []PreviewSpine {
	if len(graph.Edges) == 0 {
		return nil
	}
	nodeIndex := map[string]PreviewNode{}
	for _, node := range graph.Nodes {
		nodeIndex[node.ID] = node
	}
	undirected := map[string][]string{}
	edgeByNode := map[string][]PreviewEdge{}
	for _, edge := range graph.Edges {
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
			continue
		}
		undirected[edge.From] = append(undirected[edge.From], edge.To)
		undirected[edge.To] = append(undirected[edge.To], edge.From)
		edgeByNode[edge.From] = append(edgeByNode[edge.From], edge)
		edgeByNode[edge.To] = append(edgeByNode[edge.To], edge)
	}
	visited := map[string]struct{}{}
	type componentSpine struct {
		nodeIDs []string
		edges   []PreviewEdge
		score   int
	}
	components := make([]componentSpine, 0)
	for id := range undirected {
		if _, ok := visited[id]; ok {
			continue
		}
		stack := []string{id}
		componentIDs := map[string]struct{}{}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, ok := visited[cur]; ok {
				continue
			}
			visited[cur] = struct{}{}
			componentIDs[cur] = struct{}{}
			stack = append(stack, undirected[cur]...)
		}
		edges := make([]PreviewEdge, 0)
		for _, edge := range graph.Edges {
			if _, ok := componentIDs[edge.From]; !ok {
				continue
			}
			if _, ok := componentIDs[edge.To]; !ok {
				continue
			}
			if edge.From == edge.To {
				continue
			}
			edges = append(edges, edge)
		}
		if len(edges) == 0 {
			continue
		}
		nodeIDs := topologicalPreviewNodeOrder(componentIDs, edges)
		components = append(components, componentSpine{
			nodeIDs: nodeIDs,
			edges:   edges,
			score:   previewSpineScore(nodeIDs, edges, nodeIndex),
		})
	}
	sort.SliceStable(components, func(i, j int) bool {
		if components[i].score != components[j].score {
			return components[i].score > components[j].score
		}
		return strings.Join(components[i].nodeIDs, "\x00") < strings.Join(components[j].nodeIDs, "\x00")
	})
	spines := make([]PreviewSpine, 0, len(components))
	for i, component := range components {
		level := "branch"
		scope := "branch"
		if i == 0 && len(component.edges) >= 3 {
			level = "primary"
			scope = "article"
		} else if len(component.edges) == 1 && !componentHasTarget(component.nodeIDs, nodeIndex) {
			level = "local"
			scope = "local"
		}
		spines = append(spines, PreviewSpine{
			ID:       fmt.Sprintf("s%d", i+1),
			Level:    level,
			Priority: i + 1,
			Thesis:   previewSpineThesis(component.nodeIDs, nodeIndex),
			NodeIDs:  component.nodeIDs,
			Edges:    component.edges,
			Scope:    scope,
		})
	}
	return assignSpineFamilies(spines, graphNodeMapFromPreview(graph.Nodes))
}

func graphNodeMapFromPreview(nodes []PreviewNode) map[string]graphNode {
	out := map[string]graphNode{}
	for _, node := range nodes {
		out[node.ID] = graphNode{
			ID:       node.ID,
			Text:     node.Text,
			Ontology: node.Ontology,
			IsTarget: node.IsTarget,
		}
	}
	return out
}

func topologicalPreviewNodeOrder(componentIDs map[string]struct{}, edges []PreviewEdge) []string {
	inDegree := map[string]int{}
	out := make([]string, 0, len(componentIDs))
	for id := range componentIDs {
		inDegree[id] = 0
	}
	for _, edge := range edges {
		if _, ok := componentIDs[edge.From]; !ok {
			continue
		}
		if _, ok := componentIDs[edge.To]; !ok {
			continue
		}
		inDegree[edge.To]++
	}
	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)
	adj := map[string][]string{}
	for _, edge := range edges {
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	for id := range adj {
		sort.Strings(adj[id])
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		out = append(out, cur)
		for _, next := range adj[cur] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
				sort.Strings(queue)
			}
		}
	}
	if len(out) != len(componentIDs) {
		seen := map[string]struct{}{}
		for _, id := range out {
			seen[id] = struct{}{}
		}
		rest := make([]string, 0)
		for id := range componentIDs {
			if _, ok := seen[id]; !ok {
				rest = append(rest, id)
			}
		}
		sort.Strings(rest)
		out = append(out, rest...)
	}
	return out
}

func previewSpineScore(nodeIDs []string, edges []PreviewEdge, nodes map[string]PreviewNode) int {
	score := len(edges)*10 + len(nodeIDs)
	if componentHasTarget(nodeIDs, nodes) {
		score += 5
	}
	for _, id := range nodeIDs {
		text := strings.ToLower(nodes[id].Text)
		if containsAnyText(text, []string{"真实财富", "real wealth", "购买力", "流动性", "危机", "风险", "uncertainty", "pressure", "承压"}) {
			score += 3
		}
	}
	return score
}

func componentHasTarget(nodeIDs []string, nodes map[string]PreviewNode) bool {
	for _, id := range nodeIDs {
		if nodes[id].IsTarget {
			return true
		}
	}
	return false
}

func previewSpineThesis(nodeIDs []string, nodes map[string]PreviewNode) string {
	if len(nodeIDs) == 0 {
		return ""
	}
	start := strings.TrimSpace(nodes[nodeIDs[0]].Text)
	end := strings.TrimSpace(nodes[nodeIDs[len(nodeIDs)-1]].Text)
	switch {
	case start == "":
		return end
	case end == "" || strings.EqualFold(start, end):
		return start
	default:
		return start + " -> " + end
	}
}

func toPreviewGraph(state graphState) PreviewGraph {
	out := PreviewGraph{
		Nodes:       make([]PreviewNode, 0, len(state.Nodes)),
		Edges:       make([]PreviewEdge, 0, len(state.Edges)),
		AuxEdges:    make([]PreviewEdge, 0, len(state.AuxEdges)),
		OffGraph:    make([]PreviewOffGraph, 0, len(state.OffGraph)),
		BranchHeads: append([]string(nil), state.BranchHeads...),
		Rounds:      state.Rounds,
	}
	for _, node := range state.Nodes {
		out.Nodes = append(out.Nodes, PreviewNode{
			ID:            node.ID,
			Text:          node.Text,
			SourceQuote:   node.SourceQuote,
			Role:          string(node.Role),
			DiscourseRole: node.DiscourseRole,
			Ontology:      node.Ontology,
			IsTarget:      node.IsTarget,
		})
	}
	for _, edge := range state.Edges {
		out.Edges = append(out.Edges, PreviewEdge{
			From:        edge.From,
			To:          edge.To,
			Kind:        edge.Kind,
			SourceQuote: edge.SourceQuote,
			Reason:      edge.Reason,
		})
	}
	for _, edge := range state.AuxEdges {
		out.AuxEdges = append(out.AuxEdges, PreviewEdge{
			From:        edge.From,
			To:          edge.To,
			Kind:        edge.Kind,
			SourceQuote: edge.SourceQuote,
			Reason:      edge.Reason,
		})
	}
	for _, item := range state.OffGraph {
		out.OffGraph = append(out.OffGraph, PreviewOffGraph{
			ID:          item.ID,
			Text:        item.Text,
			Role:        item.Role,
			AttachesTo:  item.AttachesTo,
			SourceQuote: item.SourceQuote,
		})
	}
	return out
}

func cloneGraphState(state graphState) graphState {
	return graphState{
		Nodes:       append([]graphNode(nil), state.Nodes...),
		Edges:       append([]graphEdge(nil), state.Edges...),
		AuxEdges:    append([]auxEdge(nil), state.AuxEdges...),
		OffGraph:    append([]offGraphItem(nil), state.OffGraph...),
		BranchHeads: append([]string(nil), state.BranchHeads...),
		Spines:      append([]PreviewSpine(nil), state.Spines...),
		ArticleForm: state.ArticleForm,
		Rounds:      state.Rounds,
	}
}

func escapeMermaidLabel(value string) string {
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
