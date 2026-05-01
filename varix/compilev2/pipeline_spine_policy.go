package compilev2

import (
	"strings"
)

func inferMissingSpinePolicies(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	for i := range spines {
		current := normalizePreviewSpinePolicy(spines[i].Policy)
		if isSatiricalArticleForm(policy.ArticleForm) && spineHasDiscourseRole(spines[i], valid, "analogy", "satire_target", "implied_thesis") {
			if current == "" || current == "causal_mechanism" {
				spines[i].Policy = "satirical_analogy"
				continue
			}
		}
		if current == "" && spineHasRelationKind(spines[i], "inference") {
			spines[i].Policy = "forecast_inference"
			continue
		}
		spines[i].Policy = current
	}
	return spines
}
func isSatiricalArticleForm(articleForm string) bool {
	switch normalizeArticleForm(articleForm) {
	case "institutional_satire", "satirical_financial_commentary":
		return true
	default:
		return false
	}
}
func spineHasRelationKind(spine PreviewSpine, kind string) bool {
	for _, edge := range spine.Edges {
		if strings.EqualFold(strings.TrimSpace(edge.Kind), strings.TrimSpace(kind)) {
			return true
		}
	}
	return false
}
func applySpinePolicy(spines []PreviewSpine, valid map[string]graphNode, policy spinePolicy) []PreviewSpine {
	for i := range spines {
		if policy.PreserveInvestmentImplications && spineHasDiscourseRole(spines[i], valid, "implication", "market_move") && spines[i].Level == "local" {
			spines[i].Level = "branch"
			if spines[i].Scope == "local" {
				spines[i].Scope = "branch"
			}
		}
	}
	switch policy.PrimaryMode {
	case "none":
		for i := range spines {
			if spines[i].Level != "primary" {
				continue
			}
			spines[i].Level = "branch"
			if spines[i].Scope == "article" {
				spines[i].Scope = "branch"
			}
		}
		return renumberSpinePriorities(spines)
	default:
		return enforceSinglePrimarySpine(spines)
	}
}
func spineHasDiscourseRole(spine PreviewSpine, valid map[string]graphNode, roles ...string) bool {
	wanted := map[string]struct{}{}
	for _, role := range roles {
		wanted[normalizeDiscourseRole(role)] = struct{}{}
	}
	for _, id := range spine.NodeIDs {
		node, ok := valid[id]
		if !ok {
			continue
		}
		if _, ok := wanted[normalizeDiscourseRole(node.DiscourseRole)]; ok {
			return true
		}
	}
	return false
}
func enforceSinglePrimarySpine(spines []PreviewSpine) []PreviewSpine {
	if len(spines) == 0 {
		return spines
	}
	primaryIndex := -1
	for i := range spines {
		if spines[i].Level != "primary" {
			continue
		}
		if primaryIndex == -1 {
			primaryIndex = i
			continue
		}
		spines[i].Level = "branch"
		if spines[i].Scope == "article" {
			spines[i].Scope = "branch"
		}
	}
	if primaryIndex != -1 {
		return renumberSpinePriorities(spines)
	}
	promoteIndex := 0
	for i := range spines {
		if len(spines[i].Edges) > 0 {
			promoteIndex = i
			break
		}
	}
	spines[promoteIndex].Level = "primary"
	spines[promoteIndex].Scope = "article"
	return renumberSpinePriorities(spines)
}
func compactSpines(spines []PreviewSpine, valid map[string]graphNode) []PreviewSpine {
	if len(spines) < 3 {
		return spines
	}
	sellPressureIndexes := make([]int, 0)
	for i, spine := range spines {
		if spine.Level == "primary" {
			continue
		}
		if isCryptoSellPressureSpine(spine, valid) {
			sellPressureIndexes = append(sellPressureIndexes, i)
		}
	}
	if len(sellPressureIndexes) < 2 {
		return spines
	}
	return mergeSpineIndexes(spines, sellPressureIndexes, "Crypto liquidity / sell-pressure mechanics drive Bitcoin weakness")
}
func isCryptoSellPressureSpine(spine PreviewSpine, valid map[string]graphNode) bool {
	text := strings.ToLower(spine.Thesis)
	for _, id := range spine.NodeIDs {
		if node, ok := valid[id]; ok {
			text += " " + strings.ToLower(node.Text)
		}
	}
	for _, edge := range spine.Edges {
		text += " " + strings.ToLower(edge.SourceQuote) + " " + strings.ToLower(edge.Reason)
	}
	if !containsAnyText(text, []string{"bitcoin", "btc", "比特币", "crypto", "加密"}) {
		return false
	}
	return containsAnyText(text, []string{
		"etf outflow", "etf outflows", "outflow", "outflows",
		"market maker", "market makers", "sell into", "selling pressure", "sell-pressure",
		"stablecoin", "stable coin", "supply contraction", "liquidation", "long liquidation",
		"卖压", "出流", "稳定币", "做市", "清算",
	})
}
func mergeSpineIndexes(spines []PreviewSpine, indexes []int, thesis string) []PreviewSpine {
	indexSet := map[int]struct{}{}
	for _, index := range indexes {
		indexSet[index] = struct{}{}
	}
	first := indexes[0]
	merged := PreviewSpine{
		ID:       spines[first].ID,
		Level:    "branch",
		Priority: spines[first].Priority,
		Thesis:   thesis,
		Scope:    "branch",
	}
	seenNodes := map[string]struct{}{}
	seenEdges := map[string]struct{}{}
	for _, index := range indexes {
		for _, id := range spines[index].NodeIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seenNodes[id]; ok {
				continue
			}
			seenNodes[id] = struct{}{}
			merged.NodeIDs = append(merged.NodeIDs, id)
		}
		for _, edge := range spines[index].Edges {
			if strings.TrimSpace(edge.From) == "" || strings.TrimSpace(edge.To) == "" {
				continue
			}
			key := edge.From + "->" + edge.To
			if _, ok := seenEdges[key]; ok {
				continue
			}
			seenEdges[key] = struct{}{}
			merged.Edges = append(merged.Edges, edge)
		}
	}
	out := make([]PreviewSpine, 0, len(spines)-len(indexes)+1)
	for i, spine := range spines {
		if i == first {
			out = append(out, merged)
			continue
		}
		if _, ok := indexSet[i]; ok {
			continue
		}
		out = append(out, spine)
	}
	for i := range out {
		out[i].Priority = i + 1
	}
	return out
}
