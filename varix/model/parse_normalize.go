package model

import (
	"strings"
)

func normalizeNodeTiming(graph *ReasoningGraph) {
	if graph == nil {
		return
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		switch node.Kind {
		case NodeFact, NodeImplicitCondition, NodeMechanism:
			if node.OccurredAt.IsZero() && !node.ValidFrom.IsZero() {
				node.OccurredAt = node.ValidFrom
			}
		case NodePrediction:
			if node.PredictionStartAt.IsZero() && !node.ValidFrom.IsZero() {
				node.PredictionStartAt = node.ValidFrom
			}
			if node.PredictionDueAt.IsZero() && !node.ValidTo.IsZero() {
				node.PredictionDueAt = node.ValidTo
			}
			if node.PredictionDueAt.IsZero() && !node.PredictionStartAt.IsZero() {
				if due, ok := inferPredictionDueAtFromText(node.Text, node.PredictionStartAt); ok {
					node.PredictionDueAt = due
				}
			}
		}
	}
}
func normalizeTransmissionPaths(paths []TransmissionPath) {
	for i := range paths {
		paths[i].Driver = strings.TrimSpace(paths[i].Driver)
		paths[i].Target = strings.TrimSpace(paths[i].Target)
		paths[i].Steps = normalizeStringList(paths[i].Steps)
	}
}
func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		normalized = append(normalized, strings.TrimSpace(value))
	}
	return normalized
}
func normalizeNodeTaxonomy(graph *ReasoningGraph) {
	if graph == nil {
		return
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		text := strings.TrimSpace(node.Text)
		if text != "" && shouldNormalizeToExplicitCondition(node.Kind, text) {
			node.Kind = NodeExplicitCondition
		}
		if normalized, err := node.normalizedSchema(); err == nil {
			*node = normalized
		}
	}
}
func shouldNormalizeToExplicitCondition(kind NodeKind, text string) bool {
	if !isExplicitConditionText(text) {
		return false
	}
	switch kind {
	case "", NodeFact:
		return true
	default:
		return false
	}
}
func isExplicitConditionText(text string) bool {
	text = strings.TrimSpace(text)
	prefixes := []string{"如果", "若", "一旦", "假如", "倘若", "如若"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}
