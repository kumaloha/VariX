package compilev2

import (
	"fmt"
	"github.com/kumaloha/VariX/varix/compile"
	"strings"
)

func renderOffGraph(items []offGraphItem, cn func(id, fallback string) string) (evidence, explanation, supplementary []string) {
	for _, item := range items {
		switch item.Role {
		case "evidence", "inference":
			evidence = append(evidence, cn(item.ID, item.Text))
		case "explanation":
			explanation = append(explanation, cn(item.ID, item.Text))
		default:
			supplementary = append(supplementary, cn(item.ID, item.Text))
		}
	}
	return
}

func renderOffGraphDetails(items []offGraphItem, cn func(id, fallback string) string) []map[string]any {
	details := make([]map[string]any, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(cn(item.ID, item.Text))
		if text == "" {
			continue
		}
		entry := map[string]any{
			"kind":        authorClaimKindForOffGraphRole(item.Role),
			"text":        text,
			"source":      "off_graph",
			"source_text": strings.TrimSpace(item.Text),
		}
		if id := strings.TrimSpace(item.ID); id != "" {
			entry["source_id"] = id
		}
		if role := strings.TrimSpace(item.Role); role != "" {
			entry["role"] = role
		}
		if attach := strings.TrimSpace(item.AttachesTo); attach != "" {
			entry["attaches_to"] = attach
		}
		if quote := strings.TrimSpace(item.SourceQuote); quote != "" {
			entry["source_quote"] = quote
		}
		details = append(details, entry)
	}
	return details
}

func renderTransmissionPathDetails(paths []renderedPath, cn func(id, fallback string) string) []map[string]any {
	details := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		transmission := renderPathToTransmission(path, cn)
		if strings.TrimSpace(transmission.Driver) == "" || strings.TrimSpace(transmission.Target) == "" {
			continue
		}
		edgeEvidence := renderPathEdgeEvidence(path, cn)
		entry := map[string]any{
			"kind":          "inference_path",
			"from":          transmission.Driver,
			"to":            transmission.Target,
			"steps":         compile.CloneStrings(transmission.Steps),
			"edge_evidence": edgeEvidence,
		}
		if branch := strings.TrimSpace(path.branchID); branch != "" {
			entry["branch"] = branch
		}
		if context := renderPathEvidenceContext(edgeEvidence); context != "" {
			entry["source_quote"] = context
			entry["context"] = context
		}
		details = append(details, entry)
	}
	return details
}

func renderPathEdgeEvidence(path renderedPath, cn func(id, fallback string) string) []map[string]any {
	nodes := renderedPathNodeIndex(path)
	edges := path.edges
	if len(edges) == 0 {
		edges = fallbackEdgesForRenderedPath(path)
	}
	out := make([]map[string]any, 0, len(edges))
	for _, edge := range edges {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if from == "" || to == "" || from == to {
			continue
		}
		fromNode := nodes[from]
		toNode := nodes[to]
		sourceQuote := strings.TrimSpace(edge.SourceQuote)
		if sourceQuote == "" {
			sourceQuote = strings.Join(nonEmptyStrings(fromNode.SourceQuote, toNode.SourceQuote), " / ")
		}
		item := map[string]any{
			"from":      from,
			"to":        to,
			"from_text": cn(fromNode.ID, fromNode.Text),
			"to_text":   cn(toNode.ID, toNode.Text),
		}
		if sourceQuote != "" {
			item["source_quote"] = sourceQuote
		}
		if reason := strings.TrimSpace(edge.Reason); reason != "" {
			item["reason"] = reason
		}
		out = append(out, item)
	}
	return out
}

func renderedPathNodeIndex(path renderedPath) map[string]graphNode {
	out := map[string]graphNode{}
	if id := strings.TrimSpace(path.driver.ID); id != "" {
		out[id] = path.driver
	}
	for _, step := range path.steps {
		if id := strings.TrimSpace(step.ID); id != "" {
			out[id] = step
		}
	}
	if id := strings.TrimSpace(path.target.ID); id != "" {
		out[id] = path.target
	}
	return out
}

func fallbackEdgesForRenderedPath(path renderedPath) []PreviewEdge {
	nodeIDs := renderedPathNodeIDs(path)
	if len(nodeIDs) < 2 {
		return nil
	}
	nodes := renderedPathNodeIndex(path)
	out := make([]PreviewEdge, 0, len(nodeIDs)-1)
	for i := 0; i+1 < len(nodeIDs); i++ {
		out = append(out, fallbackPreviewEdgeForPathSegment(nodeIDs[i], nodeIDs[i+1], nodes))
	}
	return out
}

func renderPathEvidenceContext(edgeEvidence []map[string]any) string {
	parts := make([]string, 0, len(edgeEvidence)*2)
	for _, item := range edgeEvidence {
		parts = append(parts, detailMapString(item, "source_quote"), detailMapString(item, "reason"))
	}
	return strings.Join(nonEmptyStrings(parts...), " / ")
}

func detailMapString(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	if value, ok := item[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func authorClaimKindForOffGraphRole(role string) string {
	switch strings.TrimSpace(role) {
	case "evidence", "inference":
		return "proof_point"
	case "explanation":
		return "explanation"
	default:
		return "supplementary_proof"
	}
}

func pruneDanglingEdges(state graphState) graphState {
	valid := map[string]struct{}{}
	for _, n := range state.Nodes {
		valid[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = dedupeEdges(edges)
	return state
}

func fallbackSummary(drivers, targets []string) string {
	switch {
	case len(drivers) > 0 && len(targets) > 0:
		return fmt.Sprintf("%s影响%s。", drivers[0], targets[0])
	case len(targets) > 0:
		return fmt.Sprintf("核心结果：%s。", targets[0])
	case len(drivers) > 0:
		return fmt.Sprintf("核心驱动：%s。", drivers[0])
	default:
		return "未能稳定提取主线。"
	}
}

func confidenceFromState(drivers, targets []string, paths []compile.TransmissionPath) string {
	if len(paths) > 0 && len(drivers) > 0 && len(targets) > 0 {
		return "medium"
	}
	if len(drivers) > 0 || len(targets) > 0 {
		return "low"
	}
	return "low"
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func serializeNodeList(nodes []graphNode) string {
	return joinSerializedLines(len(nodes), func(out *[]string) {
		for _, n := range nodes {
			*out = append(*out, fmt.Sprintf("%s: %s", n.ID, n.Text))
		}
	})
}

func serializeEdgeList(edges []graphEdge) string {
	return joinSerializedLines(len(edges), func(out *[]string) {
		for _, e := range edges {
			*out = append(*out, fmt.Sprintf("%s -> %s", e.From, e.To))
		}
	})
}

func serializeRelationNodes(nodes []graphNode) string {
	return joinSerializedLines(len(nodes), func(out *[]string) {
		for _, n := range nodes {
			*out = append(*out, fmt.Sprintf("%s | %s | role=%s | discourse_role=%s | ontology=%s | quote=%s", n.ID, n.Text, n.Role, n.DiscourseRole, n.Ontology, n.SourceQuote))
		}
	})
}

func serializeBranchHeads(state graphState) string {
	return joinSerializedLines(len(state.BranchHeads), func(out *[]string) {
		for _, id := range state.BranchHeads {
			node, ok := nodeByID(state.Nodes, id)
			if !ok {
				continue
			}
			*out = append(*out, fmt.Sprintf("%s | %s", node.ID, node.Text))
		}
	})
}
