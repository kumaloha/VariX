package main

import (
	"fmt"
	"github.com/kumaloha/VariX/varix/model"
	"sort"
	"strings"
)

func graphFirstNodeSection(subgraph model.ContentSubgraph, keep func(model.ContentNode) bool) []string {
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

func graphFirstEvidenceSection(subgraph model.ContentSubgraph) []string {
	out := graphFirstNodeSection(subgraph, func(node model.ContentNode) bool {
		return node.GraphRole == model.GraphRoleEvidence
	})
	if len(out) > 0 {
		return out
	}
	byID := graphNodeIndex(subgraph)
	for _, edge := range subgraph.Edges {
		if edge.Type != model.EdgeTypeSupports {
			continue
		}
		if node, ok := byID[edge.From]; ok {
			out = appendUniqueString(out, graphFirstNodeLabel(node))
		}
	}
	return out
}

func graphFirstExplanationSection(subgraph model.ContentSubgraph) []string {
	out := graphFirstNodeSection(subgraph, func(node model.ContentNode) bool {
		return node.GraphRole == model.GraphRoleContext
	})
	byID := graphNodeIndex(subgraph)
	for _, edge := range subgraph.Edges {
		if edge.Type != model.EdgeTypeExplains {
			continue
		}
		if node, ok := byID[edge.From]; ok {
			out = appendUniqueString(out, graphFirstNodeLabel(node))
		}
	}
	return out
}

func graphFirstLogicChains(subgraph model.ContentSubgraph) []string {
	nodeByID := graphNodeIndex(subgraph)
	primaryDriveAdj := map[string][]string{}
	primaryDriveNodes := map[string]struct{}{}
	for _, edge := range subgraph.Edges {
		if edge.Type != model.EdgeTypeDrives {
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
		if node.GraphRole == model.GraphRoleDriver {
			starts = append(starts, node.ID)
		}
		if node.GraphRole == model.GraphRoleTarget {
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

func graphFirstCollectPaths(current string, adj map[string][]string, targets map[string]struct{}, nodeByID map[string]model.ContentNode, path []string, visiting map[string]bool, out *[]string, seen map[string]struct{}) {
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

func graphNodeIndex(subgraph model.ContentSubgraph) map[string]model.ContentNode {
	out := make(map[string]model.ContentNode, len(subgraph.Nodes))
	for _, node := range subgraph.Nodes {
		out[node.ID] = node
	}
	return out
}

func graphFirstNodeLabel(node model.ContentNode) string {
	rawText := strings.TrimSpace(node.RawText)
	sourceQuote := strings.TrimSpace(node.SourceQuote)
	subjectText := strings.TrimSpace(node.SubjectText)
	changeText := strings.TrimSpace(node.ChangeText)
	switch {
	case rawText != "":
		return rawText
	case sourceQuote != "":
		return sourceQuote
	case model.HasDistinctNonEmptyPair(subjectText, changeText):
		return subjectText + " " + changeText
	default:
		return strings.TrimSpace(model.FirstNonEmpty(subjectText, changeText, node.ID))
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

func preferGraphFirstSection(recordBased, graph []string) []string {
	if graphFirstSectionRichness(graph) >= graphFirstSectionRichness(recordBased) {
		return graph
	}
	return recordBased
}

func preferGraphFirstLogicChains(recordBased, graph []string) []string {
	if graphFirstChainRichness(graph) >= graphFirstChainRichness(recordBased) {
		return graph
	}
	return recordBased
}

func graphFirstVerificationSummary(subgraph model.ContentSubgraph) []string {
	nodeCounts := map[model.VerificationStatus]int{}
	edgeCounts := map[model.VerificationStatus]int{}
	for _, node := range subgraph.Nodes {
		nodeCounts[node.VerificationStatus]++
	}
	for _, edge := range subgraph.Edges {
		status := edge.VerificationStatus
		if status == "" {
			status = model.VerificationPending
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

func formatVerificationCounts(counts map[model.VerificationStatus]int) string {
	parts := make([]string, 0, 4)
	for _, status := range []model.VerificationStatus{
		model.VerificationPending,
		model.VerificationProved,
		model.VerificationDisproved,
		model.VerificationUnverifiable,
	} {
		if counts[status] == 0 {
			continue
		}
		parts = append(parts, string(status)+"="+fmt.Sprintf("%d", counts[status]))
	}
	return strings.Join(parts, ", ")
}
