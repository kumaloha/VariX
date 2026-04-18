package contentstore

import (
	"sort"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/memory"
)

func buildCausalThesis(thesis memory.CandidateThesis, nodesByID map[string]memory.AcceptedNode) memory.CausalThesis {
	nodes := make([]memory.AcceptedNode, 0, len(thesis.NodeIDs))
	sourceRefs := make([]string, 0, len(thesis.NodeIDs))
	for _, id := range thesis.NodeIDs {
		node, ok := nodesByID[id]
		if !ok {
			continue
		}
		nodes = append(nodes, node)
		sourceRefs = append(sourceRefs, node.SourcePlatform+":"+node.SourceExternalID)
	}
	roles := assignNodeRoles(nodes)
	edges := buildCausalEdges(nodes, roles)
	corePath := extractCorePath(nodes, roles)
	supporting := supportingNodeIDs(nodes, corePath)

	return memory.CausalThesis{
		CausalThesisID:    thesis.ThesisID + "-causal",
		ThesisID:          thesis.ThesisID,
		Status:            "draft",
		CoreQuestion:      thesis.TopicLabel,
		NodeRoles:         roles,
		Edges:             edges,
		CorePathNodeIDs:   corePath,
		SupportingNodeIDs: supporting,
		SourceRefs:        uniqueStrings(sourceRefs),
		TraceabilityMap:   buildTraceabilityMap(nodes),
		CompletenessScore: completenessScore(corePath),
		AbstractionReady:  len(corePath) >= 2,
	}
}

func assignNodeRoles(nodes []memory.AcceptedNode) map[string]string {
	out := make(map[string]string, len(nodes))
	for _, node := range nodes {
		switch node.NodeKind {
		case string(compile.NodeFact):
			out[node.NodeID] = "fact"
		case string(compile.NodeExplicitCondition):
			out[node.NodeID] = "condition"
		case string(compile.NodeMechanism):
			out[node.NodeID] = "mechanism"
		case string(compile.NodeImplicitCondition):
			out[node.NodeID] = "mechanism"
		case string(compile.NodeConclusion):
			out[node.NodeID] = "conclusion"
		case string(compile.NodePrediction):
			out[node.NodeID] = "prediction"
		default:
			out[node.NodeID] = "supporting"
		}
	}
	return out
}

func buildCausalEdges(nodes []memory.AcceptedNode, roles map[string]string) []memory.CausalEdge {
	out := make([]memory.CausalEdge, 0)
	for i := 0; i < len(nodes); i++ {
		for j := 0; j < len(nodes); j++ {
			if i == j {
				continue
			}
			if !hierarchyTransitionAllowed(nodes[i].NodeKind, nodes[j].NodeKind) {
				continue
			}
			out = append(out, memory.CausalEdge{
				From:       nodes[i].NodeID,
				To:         nodes[j].NodeID,
				Kind:       causalEdgeKind(roles[nodes[i].NodeID], roles[nodes[j].NodeID]),
				Confidence: 0.6,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func extractCorePath(nodes []memory.AcceptedNode, roles map[string]string) []string {
	byRole := map[string][]string{
		"fact":       {},
		"condition":  {},
		"mechanism":  {},
		"conclusion": {},
		"prediction": {},
	}
	for _, node := range nodes {
		role := roles[node.NodeID]
		if _, ok := byRole[role]; ok {
			byRole[role] = append(byRole[role], node.NodeID)
		}
	}
	for _, ids := range byRole {
		sort.Strings(ids)
	}
	path := make([]string, 0, 5)
	if len(byRole["fact"]) > 0 {
		path = append(path, byRole["fact"][0])
	}
	if len(byRole["condition"]) > 0 {
		path = append(path, byRole["condition"][0])
	}
	if len(byRole["mechanism"]) > 0 {
		path = append(path, byRole["mechanism"][0])
	}
	if len(byRole["conclusion"]) > 0 {
		path = append(path, byRole["conclusion"][0])
	}
	if len(byRole["prediction"]) > 0 {
		path = append(path, byRole["prediction"][0])
	}
	return path
}

func buildTraceabilityMap(nodes []memory.AcceptedNode) map[string][]string {
	out := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		out[node.NodeID] = []string{node.NodeID}
	}
	return out
}

func supportingNodeIDs(nodes []memory.AcceptedNode, corePath []string) []string {
	core := map[string]struct{}{}
	for _, id := range corePath {
		core[id] = struct{}{}
	}
	out := make([]string, 0)
	for _, node := range nodes {
		if _, ok := core[node.NodeID]; ok {
			continue
		}
		out = append(out, node.NodeID)
	}
	sort.Strings(out)
	return out
}

func completenessScore(corePath []string) float64 {
	switch {
	case len(corePath) >= 4:
		return 1.0
	case len(corePath) == 3:
		return 0.8
	case len(corePath) == 2:
		return 0.6
	case len(corePath) == 1:
		return 0.3
	default:
		return 0
	}
}

func causalEdgeKind(fromRole, toRole string) string {
	switch {
	case fromRole == "fact" && (toRole == "condition" || toRole == "mechanism"):
		return "supports"
	case (fromRole == "fact" || fromRole == "condition" || fromRole == "mechanism") && toRole == "conclusion":
		return "causes"
	case fromRole == "conclusion" && toRole == "prediction":
		return "extends_to"
	default:
		return "supports"
	}
}
