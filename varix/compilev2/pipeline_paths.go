package compilev2

import (
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
)

type renderedPath struct {
	branchID string
	driver   graphNode
	target   graphNode
	steps    []graphNode
	edges    []PreviewEdge
}

func extractSpinePaths(state graphState) []renderedPath {
	if len(state.Spines) == 0 {
		return nil
	}
	valid := map[string]struct{}{}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		valid[node.ID] = struct{}{}
		nodeIndex[node.ID] = node
	}
	out := make([]renderedPath, 0)
	seen := map[string]struct{}{}
	for _, spine := range state.Spines {
		nodeIDs := validSpineNodeIDs(spine, valid)
		if len(nodeIDs) < 2 {
			continue
		}
		sources, terminals := spineSourceAndTerminalIDs(spine, nodeIDs, valid, nodeIndex)
		adj := spineAdjacency(spine, valid, nodeIndex)
		if len(adj) == 0 && len(nodeIDs) >= 2 {
			for i := 0; i+1 < len(nodeIDs); i++ {
				adj[nodeIDs[i]] = append(adj[nodeIDs[i]], nodeIDs[i+1])
			}
		}
		for _, source := range sources {
			for _, terminal := range terminals {
				pathIDs := shortestPath(adj, source, terminal)
				if len(pathIDs) < 2 {
					continue
				}
				key := strings.Join(pathIDs, "->")
				if _, ok := seen[key]; ok {
					continue
				}
				driver, ok := nodeByID(state.Nodes, source)
				if !ok {
					continue
				}
				target, ok := nodeByID(state.Nodes, terminal)
				if !ok {
					continue
				}
				steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
				for _, id := range pathIDs[1 : len(pathIDs)-1] {
					if node, ok := nodeByID(state.Nodes, id); ok {
						steps = append(steps, node)
					}
				}
				seen[key] = struct{}{}
				out = append(out, renderedPath{
					branchID: spine.ID,
					driver:   driver,
					target:   target,
					steps:    steps,
					edges:    previewEdgesForPath(pathIDs, spineProjectionEdges(spine, nodeIndex), nodeIndex),
				})
			}
		}
	}
	return out
}

func spineAdjacency(spine PreviewSpine, valid map[string]struct{}, nodes map[string]graphNode) map[string][]string {
	adj := map[string][]string{}
	for _, edge := range spineProjectionEdges(spine, nodes) {
		if _, ok := valid[edge.From]; !ok {
			continue
		}
		if _, ok := valid[edge.To]; !ok {
			continue
		}
		if edge.From == edge.To {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	return adj
}

func extractPaths(state graphState, drivers, targets []graphNode) []renderedPath {
	adj := map[string][]string{}
	for _, e := range state.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		nodeIndex[node.ID] = node
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
			out = append(out, renderedPath{driver: d, target: t, steps: steps, edges: graphEdgesForPath(pathIDs, state.Edges, nodeIndex)})
		}
	}
	return out
}

func graphEdgesForPath(pathIDs []string, edges []graphEdge, nodes map[string]graphNode) []PreviewEdge {
	previewEdges := make([]PreviewEdge, 0, len(edges))
	for _, edge := range edges {
		previewEdges = append(previewEdges, previewEdgeFromGraphEdge(edge))
	}
	return previewEdgesForPath(pathIDs, previewEdges, nodes)
}

func previewEdgesForPath(pathIDs []string, edges []PreviewEdge, nodes map[string]graphNode) []PreviewEdge {
	if len(pathIDs) < 2 {
		return nil
	}
	edgeIndex := map[string]PreviewEdge{}
	for _, edge := range edges {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if from == "" || to == "" || from == to {
			continue
		}
		key := from + "->" + to
		if existing, ok := edgeIndex[key]; ok && strings.TrimSpace(existing.SourceQuote) != "" {
			continue
		}
		edgeIndex[key] = edge
	}
	out := make([]PreviewEdge, 0, len(pathIDs)-1)
	for i := 0; i+1 < len(pathIDs); i++ {
		from := strings.TrimSpace(pathIDs[i])
		to := strings.TrimSpace(pathIDs[i+1])
		if from == "" || to == "" || from == to {
			continue
		}
		edge, ok := edgeIndex[from+"->"+to]
		if !ok {
			edge = fallbackPreviewEdgeForPathSegment(from, to, nodes)
		}
		if strings.TrimSpace(edge.From) == "" {
			edge.From = from
		}
		if strings.TrimSpace(edge.To) == "" {
			edge.To = to
		}
		out = append(out, edge)
	}
	return out
}

func fallbackPreviewEdgeForPathSegment(from, to string, nodes map[string]graphNode) PreviewEdge {
	edge := PreviewEdge{From: from, To: to}
	quotes := []string{strings.TrimSpace(nodes[from].SourceQuote), strings.TrimSpace(nodes[to].SourceQuote)}
	edge.SourceQuote = strings.Join(nonEmptyStrings(quotes...), " / ")
	return edge
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
			queue = append(queue, item{id: next, path: appendPathNode(cur.path, next)})
		}
	}
	return nil
}

func appendPathNode(path []string, next string) []string {
	cloned := compile.CloneStrings(path)
	return append(cloned, next)
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

func pruneTransitiveRelations(edges []graphEdge) []graphEdge {
	edges = dedupeEdges(edges)
	out := make([]graphEdge, 0, len(edges))
	for i, edge := range edges {
		if hasAlternateMainlinePath(edges, i, edge.From, edge.To) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func hasAlternateMainlinePath(edges []graphEdge, skipIndex int, from, to string) bool {
	adj := map[string][]string{}
	for i, edge := range edges {
		if i == skipIndex {
			continue
		}
		if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" || edge.From == edge.To {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
	}
	path := shortestPath(adj, from, to)
	return len(path) >= 3
}
