package contentstore

import (
	"sort"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/graphmodel"
	"github.com/kumaloha/VariX/varix/memory"
)

func buildHierarchy(nodes []memory.AcceptedNode, record compile.Record, verification compile.Verification, graphFirstSubgraph graphmodel.ContentSubgraph, hasGraphFirstSubgraph bool) []memory.HierarchyLink {
	active := map[string]struct{}{}
	nodeKindByID := map[string]string{}
	for _, node := range nodes {
		active[node.NodeID] = struct{}{}
		nodeKindByID[node.NodeID] = node.NodeKind
	}
	factStatusByNode := factStatusMap(verification)
	out := make([]memory.HierarchyLink, 0)
	seen := map[string]struct{}{}
	for _, edge := range preferredHierarchyEdges(record, graphFirstSubgraph, hasGraphFirstSubgraph) {
		if _, ok := active[edge.From]; !ok {
			continue
		}
		if _, ok := active[edge.To]; !ok {
			continue
		}
		if !hierarchyTransitionAllowed(nodeKindByID[edge.From], nodeKindByID[edge.To]) {
			continue
		}
		if status, ok := factStatusByNode[edge.From]; ok && status != compile.FactStatusClearlyTrue {
			continue
		}
		link := memory.HierarchyLink{
			ParentNodeID: edge.From,
			ParentKind:   nodeKindByID[edge.From],
			ChildNodeID:  edge.To,
			ChildKind:    nodeKindByID[edge.To],
			Kind:         string(edge.Kind),
			Source:       edge.Source,
			Hint:         graphHierarchyHint(edge.Kind),
		}
		key := link.ParentNodeID + "->" + link.ChildNodeID
		seen[key] = struct{}{}
		out = append(out, link)
	}

	nodesByKind := groupNodesByKind(nodes)
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeFact)], nodesByKind[string(compile.NodeExplicitCondition)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeFact)], nodesByKind[string(compile.NodeImplicitCondition)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeImplicitCondition)], nodesByKind[string(compile.NodeConclusion)])
	if len(nodesByKind[string(compile.NodeImplicitCondition)]) == 0 {
		addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeFact)], nodesByKind[string(compile.NodeConclusion)])
	}
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeExplicitCondition)], nodesByKind[string(compile.NodePrediction)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(compile.NodeConclusion)], nodesByKind[string(compile.NodePrediction)])
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentNodeID != out[j].ParentNodeID {
			return out[i].ParentNodeID < out[j].ParentNodeID
		}
		if out[i].ChildNodeID != out[j].ChildNodeID {
			return out[i].ChildNodeID < out[j].ChildNodeID
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Hint < out[j].Hint
	})
	return out
}

func addInferredHierarchyLinks(out *[]memory.HierarchyLink, seen map[string]struct{}, factStatusByNode map[string]compile.FactStatus, parents, children []memory.AcceptedNode) {
	for _, parent := range parents {
		if status, ok := factStatusByNode[parent.NodeID]; ok && status != compile.FactStatusClearlyTrue {
			continue
		}
		for _, child := range children {
			if !hierarchyTransitionAllowed(parent.NodeKind, child.NodeKind) {
				continue
			}
			key := parent.NodeID + "->" + child.NodeID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			*out = append(*out, memory.HierarchyLink{
				ParentNodeID: parent.NodeID,
				ParentKind:   parent.NodeKind,
				ChildNodeID:  child.NodeID,
				ChildKind:    child.NodeKind,
				Kind:         "inferred",
				Source:       "inferred",
				Hint:         inferredHierarchyHint(parent.NodeKind, child.NodeKind),
			})
		}
	}
}

func hierarchyTransitionAllowed(parentKind, childKind string) bool {
	switch {
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeExplicitCondition):
		return true
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeImplicitCondition):
		return true
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeMechanism):
		return true
	case parentKind == string(compile.NodeFact) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodeImplicitCondition):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodeMechanism):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeImplicitCondition) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeMechanism) && childKind == string(compile.NodeConclusion):
		return true
	case parentKind == string(compile.NodeExplicitCondition) && childKind == string(compile.NodePrediction):
		return true
	case parentKind == string(compile.NodeConclusion) && childKind == string(compile.NodePrediction):
		return true
	default:
		return false
	}
}

func graphHierarchyHint(kind compile.EdgeKind) string {
	switch kind {
	case compile.EdgeDerives:
		return "compiled-derives"
	case compile.EdgePositive:
		return "compiled-supports"
	case compile.EdgePresets:
		return "compiled-presets"
	default:
		return "compiled-link"
	}
}

func inferredHierarchyHint(parentKind, childKind string) string {
	return nodeKindSlug(parentKind) + "-to-" + nodeKindSlug(childKind)
}

func nodeKindSlug(kind string) string {
	switch kind {
	case string(compile.NodeFact):
		return "fact"
	case string(compile.NodeExplicitCondition):
		return "explicit-condition"
	case string(compile.NodeAssumption):
		return "implicit-condition"
	case string(compile.NodeConclusion):
		return "conclusion"
	case string(compile.NodePrediction):
		return "prediction"
	default:
		return "node"
	}
}

func groupNodesByKind(nodes []memory.AcceptedNode) map[string][]memory.AcceptedNode {
	out := map[string][]memory.AcceptedNode{}
	for _, node := range nodes {
		out[node.NodeKind] = append(out[node.NodeKind], node)
	}
	return out
}

type hierarchyEdge struct {
	From   string
	To     string
	Kind   compile.EdgeKind
	Source string
}

func preferredHierarchyEdges(record compile.Record, graphFirstSubgraph graphmodel.ContentSubgraph, hasGraphFirstSubgraph bool) []hierarchyEdge {
	compileKeys := map[string]struct{}{}
	for _, edge := range record.Output.Graph.Edges {
		compileKeys[edge.From+"->"+edge.To] = struct{}{}
	}
	graphFirstOnly := false
	if hasGraphFirstSubgraph && len(graphFirstSubgraph.Edges) > 0 {
		if len(graphFirstSubgraph.Edges) != len(record.Output.Graph.Edges) {
			graphFirstOnly = true
		} else {
			for _, edge := range graphFirstSubgraph.Edges {
				if _, ok := compileKeys[edge.From+"->"+edge.To]; !ok {
					graphFirstOnly = true
					break
				}
			}
		}
		out := make([]hierarchyEdge, 0, len(graphFirstSubgraph.Edges))
		for _, edge := range graphFirstSubgraph.Edges {
			kind := compile.EdgePositive
			switch edge.Type {
			case graphmodel.EdgeTypeExplains:
				kind = compile.EdgeExplains
			case graphmodel.EdgeTypeContext:
				kind = compile.EdgePresets
			case graphmodel.EdgeTypeSupports:
				kind = compile.EdgeDerives
			case graphmodel.EdgeTypeDrives:
				kind = compile.EdgePositive
			}
			source := "graph"
			if graphFirstOnly {
				source = "graph_first"
			}
			out = append(out, hierarchyEdge{From: edge.From, To: edge.To, Kind: kind, Source: source})
		}
		return out
	}
	out := make([]hierarchyEdge, 0, len(record.Output.Graph.Edges))
	for _, edge := range record.Output.Graph.Edges {
		out = append(out, hierarchyEdge{From: edge.From, To: edge.To, Kind: edge.Kind, Source: "graph"})
	}
	return out
}
