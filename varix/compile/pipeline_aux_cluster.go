package compile

import "strings"

func collectAuxComponent(adj map[string][]string, start string, visited map[string]struct{}) []string {
	stack := []string{start}
	component := make([]string, 0)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		component = append(component, id)
		stack = append(stack, adj[id]...)
	}
	return component
}

func chooseClusterHead(component []string, edges []auxEdge, nodeIndex map[string]graphNode) string {
	if len(component) == 0 {
		return ""
	}
	member := map[string]struct{}{}
	for _, id := range component {
		member[id] = struct{}{}
	}
	inScore := map[string]float64{}
	outScore := map[string]float64{}
	inCount := map[string]int{}
	outCount := map[string]int{}
	for _, edge := range edges {
		if _, ok := member[edge.From]; !ok {
			continue
		}
		if _, ok := member[edge.To]; !ok {
			continue
		}
		w := auxEdgeWeight(edge.Kind)
		outScore[edge.From] += w
		inScore[edge.To] += w
		outCount[edge.From]++
		inCount[edge.To]++
	}
	candidates := make([]string, 0, len(component))
	for _, candidate := range component {
		// A support edge means `from` is serving another node, so it cannot be
		// the component core. If the model creates a cycle, fall back below.
		if outCount[candidate] == 0 {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		candidates = component
	}
	best := candidates[0]
	bestScore := clusterHeadScore(best, inScore, outScore, nodeIndex)
	bestTie := clusterHeadTieBreak(nodeIndex[best].Text)
	for _, candidate := range candidates[1:] {
		score := clusterHeadScore(candidate, inScore, outScore, nodeIndex)
		tie := clusterHeadTieBreak(nodeIndex[candidate].Text)
		switch {
		case score > bestScore:
			best = candidate
			bestScore = score
			bestTie = tie
		case score == bestScore && inCount[candidate] > inCount[best]:
			best = candidate
			bestTie = tie
		case score == bestScore && inCount[candidate] == inCount[best] && outCount[candidate] < outCount[best]:
			best = candidate
			bestTie = tie
		case score == bestScore && inCount[candidate] == inCount[best] && outCount[candidate] == outCount[best] && tie > bestTie:
			best = candidate
			bestTie = tie
		}
	}
	return best
}

func auxEdgeWeight(kind string) float64 {
	switch strings.TrimSpace(kind) {
	case "evidence":
		return 3.0
	case "inference":
		return 3.25
	case "explanation":
		return 2.0
	case "supplementary":
		return 2.5
	default:
		return 1.0
	}
}

func canonicalityScore(nodeID string, inScore, outScore map[string]float64) float64 {
	return inScore[nodeID] - outScore[nodeID]
}

func clusterHeadScore(nodeID string, inScore, outScore map[string]float64, nodeIndex map[string]graphNode) float64 {
	node := nodeIndex[nodeID]
	return canonicalityScore(nodeID, inScore, outScore) + discourseRoleHeadBoost(node.DiscourseRole) + 0.35*clusterHeadTieBreak(node.Text)
}

func discourseRoleHeadBoost(role string) float64 {
	switch normalizeDiscourseRole(role) {
	case "thesis":
		return 8.0
	case "mechanism":
		return 5.0
	case "implication":
		return 3.0
	case "market_move":
		return 2.0
	case "caveat":
		return -1.0
	case "evidence":
		return -3.0
	case "example":
		return -4.0
	default:
		return 0
	}
}

func clusterHeadTieBreak(text string) float64 {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	score := 0.0
	if looksLikeSubjectChangeNode(text) {
		score += 4.0
	}
	if looksLikeConcreteBranchResult(lower) {
		score += 2.5
	}
	if looksLikePureQuantOrThreshold(lower) {
		score -= 3.0
	}
	if looksLikePureRuleOrLimit(lower) {
		score -= 3.5
	}
	if looksLikeBroadCommentary(lower) {
		score -= 2.5
	}
	if looksLikeForecastOrDominoFraming(lower) {
		score -= 2.5
	}
	if looksLikeProcessSummary(lower) {
		score -= 2.0
	}
	return score
}
