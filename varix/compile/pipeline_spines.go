package compile

import (
	"fmt"
	"sort"
	"strings"
)

func buildSpinesFromLLM(raw []mainlineSpinePatch, rawEdges []graphEdge, finalEdges []graphEdge, valid map[string]graphNode, articleForm string) []PreviewSpine {
	if len(raw) == 0 {
		return nil
	}
	out := make([]PreviewSpine, 0, len(raw))
	for i, item := range raw {
		nodeIDs := make([]string, 0, len(item.NodeIDs))
		seenNodes := map[string]struct{}{}
		for _, id := range item.NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := valid[id]; !ok {
				continue
			}
			if _, ok := seenNodes[id]; ok {
				continue
			}
			seenNodes[id] = struct{}{}
			nodeIDs = append(nodeIDs, id)
		}
		spineEdges := make([]PreviewEdge, 0, len(item.EdgeIndexes))
		seenEdges := map[string]struct{}{}
		for _, edgeIndex := range item.EdgeIndexes {
			if edgeIndex < 0 || edgeIndex >= len(rawEdges) {
				continue
			}
			edge := rawEdges[edgeIndex]
			if _, ok := valid[edge.From]; !ok {
				continue
			}
			if _, ok := valid[edge.To]; !ok {
				continue
			}
			if !hasEdge(finalEdges, edge.From, edge.To) {
				continue
			}
			key := edge.From + "->" + edge.To
			if _, ok := seenEdges[key]; ok {
				continue
			}
			seenEdges[key] = struct{}{}
			spineEdges = append(spineEdges, previewEdgeFromGraphEdge(edge))
		}
		if len(spineEdges) == 0 {
			for _, edge := range finalEdges {
				if _, ok := seenNodes[edge.From]; !ok {
					continue
				}
				if _, ok := seenNodes[edge.To]; !ok {
					continue
				}
				key := edge.From + "->" + edge.To
				if _, ok := seenEdges[key]; ok {
					continue
				}
				seenEdges[key] = struct{}{}
				spineEdges = append(spineEdges, previewEdgeFromGraphEdge(edge))
			}
		}
		if len(nodeIDs) == 0 {
			continue
		}
		unitIDs := make([]string, 0, len(item.UnitIDs))
		seenUnits := map[string]struct{}{}
		for _, id := range item.UnitIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seenUnits[id]; ok {
				continue
			}
			seenUnits[id] = struct{}{}
			unitIDs = append(unitIDs, id)
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("s%d", len(out)+1)
		}
		level := normalizePreviewSpineLevel(item.Level)
		priority := item.Priority
		if priority <= 0 {
			priority = i + 1
		}
		out = append(out, PreviewSpine{
			ID:       id,
			Level:    level,
			Priority: priority,
			Policy:   normalizePreviewSpinePolicy(item.Policy),
			Thesis:   strings.TrimSpace(item.Thesis),
			NodeIDs:  nodeIDs,
			UnitIDs:  unitIDs,
			Edges:    spineEdges,
			Scope:    normalizePreviewSpineScope(item.Scope, level),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	policy := policyForArticleForm(articleForm)
	out = inferMissingSpinePolicies(out, valid, policy)
	out = applySpinePolicy(out, valid, policy)
	out = compactSpines(out, valid)
	out = enforceSpineBudget(out, valid, policy)
	return assignSpineFamilies(out, valid)
}
func renumberSpinePriorities(spines []PreviewSpine) []PreviewSpine {
	for i := range spines {
		spines[i].Priority = i + 1
	}
	return spines
}
func previewEdgeFromGraphEdge(edge graphEdge) PreviewEdge {
	return PreviewEdge{
		From:        edge.From,
		To:          edge.To,
		Kind:        edge.Kind,
		SourceQuote: edge.SourceQuote,
		Reason:      edge.Reason,
	}
}
func normalizePreviewSpineLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "primary", "branch", "local":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "branch"
	}
}
func normalizePreviewSpineScope(value, level string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "article", "section", "paragraph", "branch", "local":
		return strings.ToLower(strings.TrimSpace(value))
	}
	switch level {
	case "primary":
		return "article"
	case "local":
		return "local"
	default:
		return "branch"
	}
}
func normalizePreviewSpinePolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "causal_mechanism", "forecast_inference", "investment_implication", "satirical_analogy", "concept_explanation", "risk_family", "market_update", "management_declaration", "capital_allocation_rule", "policy_guidance", "policy_stance", "commitment", "operating_plan", "risk_boundary", "non_action_boundary":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
