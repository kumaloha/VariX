package contentstore

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func buildGlobalClusters(nodes []memory.AcceptedNode, dedupeGroups []memory.DedupeGroup, contradictionGroups []memory.ContradictionGroup, now time.Time) []memory.GlobalCluster {
	byID := map[string]memory.AcceptedNode{}
	for _, node := range nodes {
		byID[node.NodeID] = node
	}

	adj := map[string]map[string]struct{}{}
	addEdge := func(a, b string) {
		if !hasDistinctNonEmptyPair(a, b) {
			return
		}
		if adj[a] == nil {
			adj[a] = map[string]struct{}{}
		}
		if adj[b] == nil {
			adj[b] = map[string]struct{}{}
		}
		adj[a][b] = struct{}{}
		adj[b][a] = struct{}{}
	}
	nodeIDs := make([]string, 0, len(byID))
	for id := range byID {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	for _, group := range dedupeGroups {
		for i := 0; i < len(group.NodeIDs); i++ {
			for j := i + 1; j < len(group.NodeIDs); j++ {
				addEdge(group.NodeIDs[i], group.NodeIDs[j])
			}
		}
	}
	for _, group := range contradictionGroups {
		for i := 0; i < len(group.NodeIDs); i++ {
			for j := i + 1; j < len(group.NodeIDs); j++ {
				addEdge(group.NodeIDs[i], group.NodeIDs[j])
			}
		}
	}
	for i := 0; i < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			left := byID[nodeIDs[i]]
			right := byID[nodeIDs[j]]
			if theme := sharedMacroTheme(left.NodeText, right.NodeText); theme != "" {
				addEdge(left.NodeID, right.NodeID)
				continue
			}
			if !sameGlobalClusterFamily(left, right) {
				continue
			}
			if nonEmptySemanticPhrase(left.NodeText, right.NodeText) != "" {
				addEdge(left.NodeID, right.NodeID)
			}
		}
	}

	seen := map[string]struct{}{}
	var clusters []memory.GlobalCluster
	for _, start := range nodeIDs {
		if _, ok := seen[start]; ok {
			continue
		}
		component := collectComponent(start, adj, seen)
		sort.Strings(component)
		cluster := buildGlobalCluster(component, byID, contradictionGroups, now)
		clusters = append(clusters, cluster)
	}

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].CanonicalProposition < clusters[j].CanonicalProposition
	})
	return clusters
}
func collectComponent(start string, adj map[string]map[string]struct{}, seen map[string]struct{}) []string {
	queue := []string{start}
	component := make([]string, 0, 4)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		component = append(component, current)
		for next := range adj[current] {
			if _, ok := seen[next]; !ok {
				queue = append(queue, next)
			}
		}
	}
	return component
}
func buildGlobalCluster(component []string, byID map[string]memory.AcceptedNode, contradictionGroups []memory.ContradictionGroup, now time.Time) memory.GlobalCluster {
	supporting := make([]string, 0)
	conflictingSet := map[string]struct{}{}
	conditional := make([]string, 0)
	predictive := make([]string, 0)

	for _, group := range contradictionGroups {
		if overlap(component, group.NodeIDs) {
			for _, id := range group.NodeIDs {
				conflictingSet[id] = struct{}{}
			}
		}
	}

	for _, id := range component {
		node := byID[id]
		switch node.NodeKind {
		case string(compile.NodeExplicitCondition), string(compile.NodeImplicitCondition):
			conditional = append(conditional, id)
		case string(compile.NodePrediction):
			predictive = append(predictive, id)
		default:
			if _, conflicting := conflictingSet[id]; !conflicting {
				supporting = append(supporting, id)
			}
		}
	}

	conflicting := make([]string, 0, len(conflictingSet))
	for id := range conflictingSet {
		conflicting = append(conflicting, id)
	}
	sort.Strings(supporting)
	sort.Strings(conflicting)
	sort.Strings(conditional)
	sort.Strings(predictive)

	rep := chooseRepresentativeNode(component, byID)
	canonical := buildCanonicalProposition(component, byID)
	if len(conflicting) > 0 && !strings.HasPrefix(canonical, "关于「") {
		if conflictCanonical, ok := deriveContradictionProposition(conflicting, byID); ok {
			canonical = conflictCanonical
		}
	}
	coreSupporting := selectCoreNodes(supporting, byID, 2)
	coreConditional := selectCoreNodes(conditional, byID, 2)
	coreConclusions := selectCoreNodes(filterNodesByKind(component, byID, string(compile.NodeConclusion)), byID, 1)
	corePredictive := selectCoreNodes(predictive, byID, 2)
	expanded := cloneStringSlice(component)
	sort.Strings(expanded)
	return memory.GlobalCluster{
		ClusterID:              "cluster:" + strings.Join(component, "|"),
		CanonicalProposition:   canonical,
		Summary:                buildClusterSummary(canonical, supporting, conflicting, conditional, predictive, byID),
		RepresentativeNodeID:   rep,
		SupportingNodeIDs:      supporting,
		ConflictingNodeIDs:     conflicting,
		ConditionalNodeIDs:     conditional,
		PredictiveNodeIDs:      predictive,
		CoreSupportingNodeIDs:  coreSupporting,
		CoreConditionalNodeIDs: coreConditional,
		CoreConclusionNodeIDs:  coreConclusions,
		CorePredictiveNodeIDs:  corePredictive,
		ExpandedNodeIDs:        expanded,
		SynthesizedEdges:       buildSynthesizedEdges(coreSupporting, coreConditional, coreConclusions, corePredictive),
		Active:                 true,
		UpdatedAt:              now,
	}
}
func buildGlobalOpenQuestions(clusters []memory.GlobalCluster) []string {
	out := make([]string, 0)
	for _, cluster := range clusters {
		if len(cluster.ConflictingNodeIDs) > 0 {
			out = append(out, fmt.Sprintf("cluster %s contains unresolved contradictions", cluster.ClusterID))
		}
	}
	sort.Strings(out)
	return out
}
func overlap(component, group []string) bool {
	set := map[string]struct{}{}
	for _, id := range component {
		set[id] = struct{}{}
	}
	for _, id := range group {
		if _, ok := set[id]; ok {
			return true
		}
	}
	return false
}
