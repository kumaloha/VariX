package compile

import (
	"context"
	"fmt"
	"strings"
)

func stage2Supplement(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2SupplementSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2SupplementUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		SupplementLinks []struct {
			A           string `json:"a"`
			B           string `json:"b"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"supplement_links"`
		SameThingLinks []struct {
			A           string `json:"a"`
			B           string `json:"b"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"same_thing_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "supplement", &result); err != nil {
		return graphState{}, err
	}
	valid := map[string]graphNode{}
	for _, n := range state.Nodes {
		valid[n.ID] = n
	}
	links := append(result.SupplementLinks, result.SameThingLinks...)
	auxEdges := make([]auxEdge, 0, len(links)*2)
	for _, e := range links {
		if _, ok := valid[e.A]; !ok {
			continue
		}
		if _, ok := valid[e.B]; !ok {
			continue
		}
		if e.A == e.B {
			continue
		}
		auxEdges = append(auxEdges, auxEdge{
			From:        e.A,
			To:          e.B,
			Kind:        "supplementary",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
		auxEdges = append(auxEdges, auxEdge{
			From:        e.B,
			To:          e.A,
			Kind:        "supplementary",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	state.AuxEdges = append(state.AuxEdges, auxEdges...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	state.BranchHeads = nil
	return state, nil
}

type supportEdgeResult struct {
	SupportEdges []supportEdgePatch `json:"support_edges"`
}

type supportEdgePatch struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}

func stage2Support(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2SupportSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2SupportUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result supportEdgeResult
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "support", &result); err != nil {
		return graphState{}, err
	}
	state.AuxEdges = append(state.AuxEdges, buildAuxEdgesFromSupportEdges(state.Nodes, result.SupportEdges)...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	state.BranchHeads = nil
	return state, nil
}

func stage2Evidence(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2EvidenceSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2EvidenceUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		SupportLinks []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"support_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "evidence", &result); err != nil {
		return graphState{}, err
	}
	state.AuxEdges = append(state.AuxEdges, buildAuxEdgesFromSupport(state.Nodes, result.SupportLinks)...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	return state, nil
}

func stage2Explanation(ctx context.Context, rt runtimeChat, model string, bundle Bundle, state graphState) (graphState, error) {
	if len(state.Nodes) == 0 {
		return state, nil
	}
	systemPrompt, err := renderStage2ExplanationSystemPrompt()
	if err != nil {
		return graphState{}, err
	}
	userPrompt, err := renderStage2ExplanationUserPrompt(serializeRelationNodes(state.Nodes), bundle.TextContext())
	if err != nil {
		return graphState{}, err
	}
	var result struct {
		ExplanationLinks []struct {
			From        string `json:"from"`
			To          string `json:"to"`
			SourceQuote string `json:"source_quote"`
			Reason      string `json:"reason"`
		} `json:"explanation_links"`
	}
	if err := stageJSONCall(ctx, rt, model, bundle, systemPrompt, userPrompt, "explanation", &result); err != nil {
		return graphState{}, err
	}
	state.AuxEdges = append(state.AuxEdges, buildAuxEdgesFromExplanation(state.Nodes, result.ExplanationLinks)...)
	state.AuxEdges = dedupeAuxEdges(state.AuxEdges)
	return state, nil
}

func collapseClusters(state graphState) graphState {
	if len(state.Nodes) == 0 {
		return state
	}
	if len(state.AuxEdges) == 0 {
		state = demoteSupportingRoleNodes(state)
		if len(state.BranchHeads) == 0 {
			state.BranchHeads = nil
			for _, node := range state.Nodes {
				state.BranchHeads = append(state.BranchHeads, node.ID)
			}
			state.BranchHeads = dedupeStrings(state.BranchHeads)
		}
		return pruneDanglingEdges(state)
	}
	nodeIndex := map[string]graphNode{}
	for _, node := range state.Nodes {
		nodeIndex[node.ID] = node
	}
	undirected := map[string][]string{}
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	nodeRole := map[string]string{}
	for _, edge := range state.AuxEdges {
		if _, ok := nodeIndex[edge.From]; !ok {
			continue
		}
		if _, ok := nodeIndex[edge.To]; !ok {
			continue
		}
		undirected[edge.From] = append(undirected[edge.From], edge.To)
		undirected[edge.To] = append(undirected[edge.To], edge.From)
		outDegree[edge.From]++
		inDegree[edge.To]++
		if role, ok := auxNodeRole(edge, edge.From); ok {
			nodeRole[edge.From] = role
		}
		if role, ok := auxNodeRole(edge, edge.To); ok {
			if _, exists := nodeRole[edge.To]; !exists {
				nodeRole[edge.To] = role
			}
		}
	}
	visited := map[string]struct{}{}
	heads := make([]string, 0)
	keep := map[string]struct{}{}
	offGraph := append([]offGraphItem(nil), state.OffGraph...)
	for _, node := range state.Nodes {
		if _, seen := visited[node.ID]; seen {
			continue
		}
		component := collectAuxComponent(undirected, node.ID, visited)
		headID := chooseClusterHead(component, state.AuxEdges, nodeIndex)
		heads = append(heads, headID)
		keep[headID] = struct{}{}
		for _, memberID := range component {
			if memberID == headID {
				continue
			}
			member := nodeIndex[memberID]
			role := nodeRole[memberID]
			if strings.TrimSpace(role) == "" {
				role = "supplementary"
			}
			offGraph = append(offGraph, offGraphItem{
				ID:          fmt.Sprintf("cluster_%s_%s", role, memberID),
				Text:        member.Text,
				Role:        role,
				AttachesTo:  headID,
				SourceQuote: member.SourceQuote,
			})
		}
	}
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, node := range state.Nodes {
		if _, ok := keep[node.ID]; ok {
			nodes = append(nodes, node)
		}
	}
	state.Nodes = nodes
	state.BranchHeads = dedupeStrings(heads)
	state.OffGraph = offGraph
	state = demoteSupportingRoleNodes(state)
	state = pruneDanglingEdges(state)
	return state
}

func demoteSupportingRoleNodes(state graphState) graphState {
	if len(state.Nodes) == 0 || !hasMainlineDiscourseNode(state.Nodes) {
		return state
	}
	attachTo := firstMainlineDiscourseNodeID(state.Nodes)
	nodes := make([]graphNode, 0, len(state.Nodes))
	branchHeads := make([]string, 0, len(state.BranchHeads))
	demoted := map[string]struct{}{}
	for _, node := range state.Nodes {
		if !isSupportingDiscourseRole(node.DiscourseRole) || preserveSupportingDiscourseRole(state.ArticleForm, node.DiscourseRole) {
			nodes = append(nodes, node)
			continue
		}
		role := normalizeDiscourseRole(node.DiscourseRole)
		if role == "" {
			role = "supplementary"
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:          fmt.Sprintf("discourse_%s_%s", role, node.ID),
			Text:        node.Text,
			Role:        role,
			AttachesTo:  attachTo,
			SourceQuote: node.SourceQuote,
		})
		demoted[node.ID] = struct{}{}
	}
	if len(demoted) == 0 {
		return state
	}
	for _, id := range state.BranchHeads {
		if _, ok := demoted[id]; ok {
			continue
		}
		branchHeads = append(branchHeads, id)
	}
	state.Nodes = nodes
	state.BranchHeads = dedupeStrings(branchHeads)
	return state
}

func preserveSupportingDiscourseRole(articleForm, role string) bool {
	normalizedRole := normalizeDiscourseRole(role)
	switch normalizeArticleForm(articleForm) {
	case "evidence_backed_forecast":
		return normalizedRole == "evidence"
	case "risk_list":
		return normalizedRole == "caveat"
	default:
		return false
	}
}

func hasMainlineDiscourseNode(nodes []graphNode) bool {
	return firstMainlineDiscourseNodeID(nodes) != ""
}

func firstMainlineDiscourseNodeID(nodes []graphNode) string {
	for _, node := range nodes {
		if isMainlineDiscourseRole(node.DiscourseRole) {
			return node.ID
		}
	}
	return ""
}

func isMainlineDiscourseRole(role string) bool {
	switch normalizeDiscourseRole(role) {
	case "thesis", "mechanism", "implication", "market_move", "satire_target", "implied_thesis":
		return true
	default:
		return false
	}
}

func isSupportingDiscourseRole(role string) bool {
	switch normalizeDiscourseRole(role) {
	case "evidence", "example", "caveat":
		return true
	default:
		return false
	}
}
