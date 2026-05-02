package contentstore

import (
	"sort"

	"github.com/kumaloha/VariX/varix/memory"
	"github.com/kumaloha/VariX/varix/model"
)

func buildHierarchy(nodes []memory.AcceptedNode, record model.Record, verification model.Verification, graphFirstSubgraph model.ContentSubgraph, hasGraphFirstSubgraph bool) []memory.HierarchyLink {
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
		if status, ok := factStatusByNode[edge.From]; ok && status != model.FactStatusClearlyTrue {
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
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(model.NodeFact)], nodesByKind[string(model.NodeExplicitCondition)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(model.NodeFact)], nodesByKind[string(model.NodeImplicitCondition)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(model.NodeImplicitCondition)], nodesByKind[string(model.NodeConclusion)])
	if len(nodesByKind[string(model.NodeImplicitCondition)]) == 0 {
		addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(model.NodeFact)], nodesByKind[string(model.NodeConclusion)])
	}
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(model.NodeExplicitCondition)], nodesByKind[string(model.NodePrediction)])
	addInferredHierarchyLinks(&out, seen, factStatusByNode, nodesByKind[string(model.NodeConclusion)], nodesByKind[string(model.NodePrediction)])
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

func addInferredHierarchyLinks(out *[]memory.HierarchyLink, seen map[string]struct{}, factStatusByNode map[string]model.FactStatus, parents, children []memory.AcceptedNode) {
	for _, parent := range parents {
		if status, ok := factStatusByNode[parent.NodeID]; ok && status != model.FactStatusClearlyTrue {
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
	case parentKind == string(model.NodeFact) && childKind == string(model.NodeExplicitCondition):
		return true
	case parentKind == string(model.NodeFact) && childKind == string(model.NodeImplicitCondition):
		return true
	case parentKind == string(model.NodeFact) && childKind == string(model.NodeMechanism):
		return true
	case parentKind == string(model.NodeFact) && childKind == string(model.NodeConclusion):
		return true
	case parentKind == string(model.NodeExplicitCondition) && childKind == string(model.NodeImplicitCondition):
		return true
	case parentKind == string(model.NodeExplicitCondition) && childKind == string(model.NodeMechanism):
		return true
	case parentKind == string(model.NodeExplicitCondition) && childKind == string(model.NodeConclusion):
		return true
	case parentKind == string(model.NodeImplicitCondition) && childKind == string(model.NodeConclusion):
		return true
	case parentKind == string(model.NodeMechanism) && childKind == string(model.NodeConclusion):
		return true
	case parentKind == string(model.NodeExplicitCondition) && childKind == string(model.NodePrediction):
		return true
	case parentKind == string(model.NodeConclusion) && childKind == string(model.NodePrediction):
		return true
	default:
		return false
	}
}

func graphHierarchyHint(kind model.EdgeKind) string {
	switch kind {
	case model.EdgeDerives:
		return "compiled-derives"
	case model.EdgePositive:
		return "compiled-supports"
	case model.EdgePresets:
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
	case string(model.NodeFact):
		return "fact"
	case string(model.NodeExplicitCondition):
		return "explicit-condition"
	case string(model.NodeAssumption):
		return "implicit-condition"
	case string(model.NodeConclusion):
		return "conclusion"
	case string(model.NodePrediction):
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
	Kind   model.EdgeKind
	Source string
}

func preferredHierarchyEdges(record model.Record, graphFirstSubgraph model.ContentSubgraph, hasGraphFirstSubgraph bool) []hierarchyEdge {
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
			kind := model.EdgePositive
			switch edge.Type {
			case model.EdgeTypeExplains:
				kind = model.EdgeExplains
			case model.EdgeTypeContext:
				kind = model.EdgePresets
			case model.EdgeTypeSupports:
				kind = model.EdgeDerives
			case model.EdgeTypeDrives:
				kind = model.EdgePositive
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
