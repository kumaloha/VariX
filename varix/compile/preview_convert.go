package compile

import "strings"

func toPreviewGraph(state graphState) PreviewGraph {
	out := PreviewGraph{
		Nodes:         make([]PreviewNode, 0, len(state.Nodes)),
		Edges:         make([]PreviewEdge, 0, len(state.Edges)),
		AuxEdges:      make([]PreviewEdge, 0, len(state.AuxEdges)),
		OffGraph:      make([]PreviewOffGraph, 0, len(state.OffGraph)),
		BranchHeads:   append([]string(nil), state.BranchHeads...),
		SemanticUnits: append([]SemanticUnit(nil), state.SemanticUnits...),
		Rounds:        state.Rounds,
	}
	for _, node := range state.Nodes {
		out.Nodes = append(out.Nodes, PreviewNode{
			ID:            node.ID,
			Text:          node.Text,
			SourceQuote:   node.SourceQuote,
			Role:          string(node.Role),
			DiscourseRole: node.DiscourseRole,
			Ontology:      node.Ontology,
			IsTarget:      node.IsTarget,
		})
	}
	for _, edge := range state.Edges {
		out.Edges = append(out.Edges, PreviewEdge{
			From:        edge.From,
			To:          edge.To,
			Kind:        edge.Kind,
			SourceQuote: edge.SourceQuote,
			Reason:      edge.Reason,
		})
	}
	for _, edge := range state.AuxEdges {
		out.AuxEdges = append(out.AuxEdges, PreviewEdge{
			From:        edge.From,
			To:          edge.To,
			Kind:        edge.Kind,
			SourceQuote: edge.SourceQuote,
			Reason:      edge.Reason,
		})
	}
	for _, item := range state.OffGraph {
		out.OffGraph = append(out.OffGraph, PreviewOffGraph{
			ID:          item.ID,
			Text:        item.Text,
			Role:        item.Role,
			AttachesTo:  item.AttachesTo,
			SourceQuote: item.SourceQuote,
		})
	}
	return out
}

func fromPreviewGraph(graph PreviewGraph, spines []PreviewSpine, articleForm string) graphState {
	state := graphState{
		Nodes:         make([]graphNode, 0, len(graph.Nodes)),
		Edges:         make([]graphEdge, 0, len(graph.Edges)),
		AuxEdges:      make([]auxEdge, 0, len(graph.AuxEdges)),
		OffGraph:      make([]offGraphItem, 0, len(graph.OffGraph)),
		BranchHeads:   append([]string(nil), graph.BranchHeads...),
		Spines:        append([]PreviewSpine(nil), spines...),
		SemanticUnits: append([]SemanticUnit(nil), graph.SemanticUnits...),
		ArticleForm:   strings.TrimSpace(articleForm),
		Rounds:        graph.Rounds,
	}
	for _, node := range graph.Nodes {
		state.Nodes = append(state.Nodes, graphNode{
			ID:            strings.TrimSpace(node.ID),
			Text:          strings.TrimSpace(node.Text),
			SourceQuote:   strings.TrimSpace(node.SourceQuote),
			Role:          graphRole(strings.TrimSpace(node.Role)),
			DiscourseRole: strings.TrimSpace(node.DiscourseRole),
			Ontology:      strings.TrimSpace(node.Ontology),
			IsTarget:      node.IsTarget,
		})
	}
	for _, edge := range graph.Edges {
		state.Edges = append(state.Edges, graphEdge{
			From:        strings.TrimSpace(edge.From),
			To:          strings.TrimSpace(edge.To),
			Kind:        strings.TrimSpace(edge.Kind),
			SourceQuote: strings.TrimSpace(edge.SourceQuote),
			Reason:      strings.TrimSpace(edge.Reason),
		})
	}
	for _, edge := range graph.AuxEdges {
		state.AuxEdges = append(state.AuxEdges, auxEdge{
			From:        strings.TrimSpace(edge.From),
			To:          strings.TrimSpace(edge.To),
			Kind:        strings.TrimSpace(edge.Kind),
			SourceQuote: strings.TrimSpace(edge.SourceQuote),
			Reason:      strings.TrimSpace(edge.Reason),
		})
	}
	for _, item := range graph.OffGraph {
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:          strings.TrimSpace(item.ID),
			Text:        strings.TrimSpace(item.Text),
			Role:        strings.TrimSpace(item.Role),
			AttachesTo:  strings.TrimSpace(item.AttachesTo),
			SourceQuote: strings.TrimSpace(item.SourceQuote),
		})
	}
	return state
}

func cloneGraphState(state graphState) graphState {
	return graphState{
		Nodes:         append([]graphNode(nil), state.Nodes...),
		Edges:         append([]graphEdge(nil), state.Edges...),
		AuxEdges:      append([]auxEdge(nil), state.AuxEdges...),
		OffGraph:      append([]offGraphItem(nil), state.OffGraph...),
		BranchHeads:   append([]string(nil), state.BranchHeads...),
		CoverageHints: append([]coverageHint(nil), state.CoverageHints...),
		Spines:        append([]PreviewSpine(nil), state.Spines...),
		SemanticUnits: append([]SemanticUnit(nil), state.SemanticUnits...),
		ArticleForm:   state.ArticleForm,
		Rounds:        state.Rounds,
	}
}

func escapeMermaidLabel(value string) string {
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
