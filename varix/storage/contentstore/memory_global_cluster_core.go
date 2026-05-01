package contentstore

import (
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/memory"
)

func selectCoreNodes(ids []string, byID map[string]memory.AcceptedNode, max int) []string {
	if len(ids) <= max {
		out := cloneStringSlice(ids)
		sort.Strings(out)
		return out
	}
	sorted := cloneStringSlice(ids)
	sort.Slice(sorted, func(i, j int) bool {
		left := strings.TrimSpace(byID[sorted[i]].NodeText)
		right := strings.TrimSpace(byID[sorted[j]].NodeText)
		if len([]rune(left)) != len([]rune(right)) {
			return len([]rune(left)) > len([]rune(right))
		}
		return sorted[i] < sorted[j]
	})
	out := cloneStringSlice(sorted[:max])
	sort.Strings(out)
	return out
}
func filterNodesByKind(component []string, byID map[string]memory.AcceptedNode, kind string) []string {
	out := make([]string, 0)
	for _, id := range component {
		if byID[id].NodeKind == kind {
			out = append(out, id)
		}
	}
	return out
}
func buildSynthesizedEdges(coreSupporting, coreConditional, coreConclusions, corePredictive []string) []memory.GlobalClusterEdge {
	edges := make([]memory.GlobalClusterEdge, 0)
	add := func(froms, tos []string, kind string) {
		for _, from := range froms {
			for _, to := range tos {
				if !hasDistinctNonEmptyPair(from, to) {
					continue
				}
				edges = append(edges, memory.GlobalClusterEdge{From: from, To: to, Kind: kind})
			}
		}
	}
	add(coreSupporting, coreConclusions, "supporting->conclusion")
	add(coreConditional, coreConclusions, "conditional->conclusion")
	add(coreConclusions, corePredictive, "conclusion->prediction")
	add(coreConditional, corePredictive, "conditional->prediction")
	return edges
}
