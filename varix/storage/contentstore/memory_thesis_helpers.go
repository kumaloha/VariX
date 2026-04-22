package contentstore

import (
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
