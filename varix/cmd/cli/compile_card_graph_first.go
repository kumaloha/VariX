package main

import (
	"fmt"
	c "github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"sort"
	"strings"
)

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
