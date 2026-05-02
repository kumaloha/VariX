package compile

import (
	"fmt"
	"sort"
	"strings"
)

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
