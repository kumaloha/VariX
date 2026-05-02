package compile

import "strings"

func dedupeAuxEdges(edges []auxEdge) []auxEdge {
	seen := map[string]struct{}{}
	out := make([]auxEdge, 0, len(edges))
	for _, edge := range edges {
		key := edge.Kind + "|" + edge.From + "|" + edge.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, edge)
	}
	return out
}

func buildAuxEdgesFromSupport(nodes []graphNode, raw []struct {
	From        string `json:"from"`
	To          string `json:"to"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}) []auxEdge {
	valid := map[string]struct{}{}
	for _, n := range nodes {
		valid[n.ID] = struct{}{}
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		out = append(out, auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        "evidence",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	return out
}

func buildAuxEdgesFromExplanation(nodes []graphNode, raw []struct {
	From        string `json:"from"`
	To          string `json:"to"`
	SourceQuote string `json:"source_quote"`
	Reason      string `json:"reason"`
}) []auxEdge {
	valid := map[string]struct{}{}
	for _, n := range nodes {
		valid[n.ID] = struct{}{}
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		if _, ok := valid[e.From]; !ok {
			continue
		}
		if _, ok := valid[e.To]; !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		out = append(out, auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        "explanation",
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		})
	}
	return out
}

func buildAuxEdgesFromSupportEdges(nodes []graphNode, raw []supportEdgePatch) []auxEdge {
	valid := map[string]graphNode{}
	for _, n := range nodes {
		valid[n.ID] = n
	}
	out := make([]auxEdge, 0, len(raw))
	for _, e := range raw {
		fromNode, ok := valid[e.From]
		if !ok {
			continue
		}
		toNode, ok := valid[e.To]
		if !ok {
			continue
		}
		if e.From == e.To {
			continue
		}
		kind, ok := normalizeSupportKind(e.Kind)
		if !ok {
			continue
		}
		edge := auxEdge{
			From:        e.From,
			To:          e.To,
			Kind:        kind,
			SourceQuote: strings.TrimSpace(e.SourceQuote),
			Reason:      strings.TrimSpace(e.Reason),
		}
		if isLikelyMainlineAuxEdge(edge, fromNode, toNode) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func isLikelyMainlineAuxEdge(edge auxEdge, fromNode, toNode graphNode) bool {
	switch strings.TrimSpace(edge.Kind) {
	case "explanation", "supplementary":
	default:
		return false
	}
	if looksLikeAuxiliaryDetailNode(fromNode.Text) {
		return false
	}
	if !looksLikeOutcomeOrProcessEndpoint(fromNode.Text) || !looksLikeOutcomeOrProcessEndpoint(toNode.Text) {
		return false
	}
	context := strings.ToLower(strings.Join([]string{
		fromNode.Text,
		toNode.Text,
		fromNode.SourceQuote,
		toNode.SourceQuote,
		edge.SourceQuote,
		edge.Reason,
	}, " "))
	return containsAnyText(context, supportDriveMarkers())
}

func looksLikeAuxiliaryDetailNode(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if looksLikeOutcomeOrProcessEndpoint(text) && !containsAnyText(lower, []string{"赎回申请", "赎回请求", "机构资金", "占比", "比例", "不良贷款"}) {
		return false
	}
	if looksLikePureQuantOrThreshold(lower) || looksLikePureRuleOrLimit(lower) {
		return true
	}
	for _, marker := range []string{
		"底层资产", "企业贷款", "日常流动性", "机构资金", "机构资金占比", "贷款标准", "估值透明度", "pik", "不良贷款", "赎回申请", "赎回请求", "国防预算", "defense budget",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeOutcomeOrProcessEndpoint(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if looksLikeSubjectChangeNode(text) || looksLikeConcreteBranchResult(lower) {
		return true
	}
	for _, marker := range []string{
		"转冷", "转向", "抛售", "被抛售", "收缩", "飙升", "回落", "被推高", "高企", "维持高位", "支出上升", "被挤压", "形成", "受影响", "被压低", "被拖累", "成本上升", "居高不下", "flight to cash", "现金为王", "现象出现",
	} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func normalizeSupportKind(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "evidence":
		return "evidence", true
	case "inference", "inferential", "proof":
		return "inference", true
	case "explanation":
		return "explanation", true
	case "supplement", "supplementary":
		return "supplementary", true
	default:
		return "", false
	}
}

func auxNodeRole(edge auxEdge, nodeID string) (string, bool) {
	switch edge.Kind {
	case "evidence", "inference":
		if edge.From == nodeID {
			return edge.Kind, true
		}
	case "explanation":
		if edge.From == nodeID {
			return "explanation", true
		}
	case "supplementary":
		if edge.From == nodeID {
			return "supplementary", true
		}
	}
	return "", false
}
