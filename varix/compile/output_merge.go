package compile

import (
	"fmt"
	"strings"
)

func mergeCompileOutputs(nodes NodeExtractionOutput, fullGraph FullGraphOutput, thesis ThesisOutput) Output {
	topics := thesis.Topics
	if len(topics) == 0 {
		topics = fullGraph.Topics
	}
	if len(topics) == 0 {
		topics = nodes.Topics
	}
	confidence := strings.TrimSpace(thesis.Confidence)
	if confidence == "" {
		confidence = strings.TrimSpace(fullGraph.Confidence)
	}
	if confidence == "" {
		confidence = strings.TrimSpace(nodes.Confidence)
	}
	details := thesis.Details
	if details.IsEmpty() {
		details = fullGraph.Details
	}
	if details.IsEmpty() {
		details = nodes.Details
	}
	return Output{
		Summary:    thesis.Summary,
		Drivers:    thesis.Drivers,
		Targets:    thesis.Targets,
		Graph:      ReasoningGraph{Nodes: nodes.Graph.Nodes, Edges: fullGraph.Graph.Edges},
		Details:    details,
		Topics:     topics,
		Confidence: confidence,
	}
}

func mergeNodeOutputs(base NodeExtractionOutput, challenge NodeExtractionOutput) NodeExtractionOutput {
	if len(challenge.Graph.Nodes) == 0 {
		return base
	}
	out := base
	seen := map[string]struct{}{}
	usedIDs := map[string]struct{}{}
	for _, node := range out.Graph.Nodes {
		seen[nodeDedupKey(node)] = struct{}{}
		usedIDs[node.ID] = struct{}{}
	}
	next := 1
	for _, node := range challenge.Graph.Nodes {
		key := nodeDedupKey(node)
		if _, ok := seen[key]; ok {
			continue
		}
		if strings.TrimSpace(node.ID) == "" {
			node.ID = ""
		}
		for strings.TrimSpace(node.ID) == "" || hasStringKey(usedIDs, node.ID) {
			node.ID = fmt.Sprintf("n_challenge_%d", next)
			next++
		}
		usedIDs[node.ID] = struct{}{}
		seen[key] = struct{}{}
		out.Graph.Nodes = append(out.Graph.Nodes, node)
	}
	if out.Details.IsEmpty() {
		out.Details = challenge.Details
	}
	if len(out.Topics) == 0 {
		out.Topics = challenge.Topics
	}
	if strings.TrimSpace(out.Confidence) == "" {
		out.Confidence = challenge.Confidence
	}
	return out
}

func mergeFullGraphOutputs(base FullGraphOutput, challenge FullGraphOutput) FullGraphOutput {
	if len(challenge.Graph.Edges) == 0 {
		return base
	}
	out := base
	byPair := map[string]int{}
	for i, edge := range out.Graph.Edges {
		byPair[edgePairKey(edge)] = i
	}
	for _, edge := range challenge.Graph.Edges {
		if idx, ok := byPair[edgePairKey(edge)]; ok {
			out.Graph.Edges[idx] = edge
			continue
		}
		byPair[edgePairKey(edge)] = len(out.Graph.Edges)
		out.Graph.Edges = append(out.Graph.Edges, edge)
	}
	if out.Details.IsEmpty() {
		out.Details = challenge.Details
	}
	if len(out.Topics) == 0 {
		out.Topics = challenge.Topics
	}
	if strings.TrimSpace(out.Confidence) == "" {
		out.Confidence = challenge.Confidence
	}
	return out
}

func nodeDedupKey(node GraphNode) string {
	return string(node.Kind) + "|" + strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(node.Text)), " "))
}

func edgePairKey(edge GraphEdge) string {
	return edge.From + "->" + edge.To
}

func hasStringKey(m map[string]struct{}, key string) bool {
	_, ok := m[key]
	return ok
}

func buildCausalProjection(nodes []GraphNode, edges []GraphEdge) ReasoningGraph {
	if len(nodes) == 0 || len(edges) == 0 {
		return ReasoningGraph{}
	}
	selectedEdges := make([]GraphEdge, 0, len(edges))
	selectedIDs := map[string]struct{}{}
	for _, edge := range edges {
		if edge.Kind != EdgePositive {
			continue
		}
		selectedEdges = append(selectedEdges, edge)
		selectedIDs[edge.From] = struct{}{}
		selectedIDs[edge.To] = struct{}{}
	}
	if len(selectedEdges) == 0 {
		return ReasoningGraph{}
	}
	selectedNodes := make([]GraphNode, 0, len(selectedIDs))
	for _, node := range nodes {
		if _, ok := selectedIDs[node.ID]; ok {
			selectedNodes = append(selectedNodes, node)
		}
	}
	return ReasoningGraph{Nodes: selectedNodes, Edges: selectedEdges}
}

func applyBundleTimingFallbacks(bundle Bundle, graph *ReasoningGraph) {
	if graph == nil || bundle.PostedAt.IsZero() {
		return
	}
	fallback := bundle.PostedAt.UTC()
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		switch node.Kind {
		case NodeFact, NodeImplicitCondition, NodeMechanism:
			if node.OccurredAt.IsZero() && node.ValidFrom.IsZero() {
				node.OccurredAt = fallback
			}
		case NodePrediction:
			if node.PredictionStartAt.IsZero() && node.ValidFrom.IsZero() {
				node.PredictionStartAt = fallback
			}
		}
	}
}
