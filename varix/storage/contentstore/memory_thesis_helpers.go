package contentstore

import (
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func thesisSourceRef(node memory.AcceptedNode) string {
	return node.SourcePlatform + ":" + node.SourceExternalID
}

func collectAcceptedNodes(ids []string, nodesByID map[string]memory.AcceptedNode) ([]memory.AcceptedNode, []string) {
	nodes := make([]memory.AcceptedNode, 0, len(ids))
	sourceRefs := make([]string, 0, len(ids))
	for _, id := range ids {
		node, ok := nodesByID[id]
		if !ok {
			continue
		}
		nodes = append(nodes, node)
		sourceRefs = append(sourceRefs, thesisSourceRef(node))
	}
	return nodes, sourceRefs
}

func nodeRoleForKind(kind string) string {
	switch kind {
	case string(compile.NodeFact):
		return "fact"
	case string(compile.NodeExplicitCondition):
		return "condition"
	case string(compile.NodeMechanism), string(compile.NodeImplicitCondition):
		return "mechanism"
	case string(compile.NodeConclusion):
		return "conclusion"
	case string(compile.NodePrediction):
		return "prediction"
	default:
		return "supporting"
	}
}

func nonEmptySemanticPhrase(left, right string) string {
	phrase, ok := sharedSemanticPhrase(left, right)
	if !ok {
		return ""
	}
	return strings.TrimSpace(phrase)
}

func selfTraceabilityMap(nodes []memory.AcceptedNode) map[string][]string {
	out := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		out[node.NodeID] = []string{node.NodeID}
	}
	return out
}

func sortedNonCoreNodeIDs(nodes []memory.AcceptedNode, corePath []string) []string {
	core := map[string]struct{}{}
	for _, id := range corePath {
		core[id] = struct{}{}
	}
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := core[node.NodeID]; ok {
			continue
		}
		out = append(out, node.NodeID)
	}
	sort.Strings(out)
	return out
}

func firstTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	return firstTrimmed(values...)
}
